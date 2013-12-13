package apns

import (
	"crypto/tls"
	"encoding/binary"
	"log"
	"net"
	"sync"
	"time"
)

// You'll need to provide your own CertificateFile
// and KeyFile to send notifications. Ideally, you'll
// just set the CertificateFile and KeyFile fields to
// a location on drive where the certs can be loaded,
// but if you prefer you can use the CertificateBase64
// and KeyBase64 fields to store the actual contents.
type MultiClient struct {
	Gateway           string
	CertificateFile   string
	CertificateBase64 string
	KeyFile           string
	KeyBase64         string

	connection *tls.Conn

	sentNotifications   []notification
	queuedNotifications chan *notification
	extra               chan *notification

	lock *sync.Mutex
}

type notification struct {
	pushNotification PushNotification
	success          bool
	sentTime         time.Time
}

// Constructor. Use this if you want to set cert and key blocks manually.
func BareMultiClient(gateway, certificateBase64, keyBase64 string) (c *MultiClient) {
	c = newMultiClient()
	c.Gateway = gateway
	c.CertificateBase64 = certificateBase64
	c.KeyBase64 = keyBase64
	return
}

// Constructor. Use this if you want to load cert and key blocks from a file.
func NewMultiClient(gateway, certificateFile, keyFile string) (c *MultiClient) {
	c = newMultiClient()
	c.Gateway = gateway
	c.CertificateFile = certificateFile
	c.KeyFile = keyFile
	return
}

func newMultiClient() *MultiClient {
	c := &MultiClient{}
	c.sentNotifications = []notification{}
	c.queuedNotifications = make(chan *notification, 10)
	c.lock = &sync.Mutex{}
	return c
}

func (this *MultiClient) connect() error {
	if this.connection != nil {
		this.connection.Close()
	}

	var cert tls.Certificate
	var err error
	if len(this.CertificateBase64) == 0 && len(this.KeyBase64) == 0 {
		// The user did not specify raw block contents, so check the filesystem.
		cert, err = tls.LoadX509KeyPair(this.CertificateFile, this.KeyFile)
	} else {
		// The user provided the raw block contents, so use that.
		cert, err = tls.X509KeyPair([]byte(this.CertificateBase64), []byte(this.KeyBase64))
	}

	if err != nil {
		return err
	}

	conf := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	tlsConn, err := tls.Dial("tcp", this.Gateway, conf)
	if err != nil {
		return err
	}
	err = tlsConn.Handshake()
	if err != nil {
		return err
	}
	this.connection = tlsConn

	go this.receiveOne()
	return nil
}

func (this *MultiClient) Run() {
	log.SetFlags(log.Ltime | log.Lshortfile)

	go func() {
		for {
			n := <-this.extra
			log.Println("extra", n.pushNotification.Identifier)
		}
	}()

	for {
		n := <-this.queuedNotifications
		this.sendOne(n)
		//this.cleanSent()
	}
}

func (this *MultiClient) Queue(pn *PushNotification) {
	this.queuedNotifications <- &notification{pushNotification: *pn}
}

func (this *MultiClient) sendOne(n *notification) {
	log.Println("sendOne", n.pushNotification.Identifier)
	payload, err := n.pushNotification.ToBytes()
	if err != nil {
		panic(err)
	}

	if this.connection == nil {
		err := this.connect()
		if err != nil {
			log.Println(err)
			return
		}
	}
	log.Println("write", n.pushNotification.Identifier)
	_, err = this.connection.Write(payload)
	if err != nil {
		//if not connected, try to reconnect and rewrite payload
		log.Println(err)

		err = this.connection.Close()
		if err != nil {
			log.Println(err)
		}
		err = this.connect()
		if err != nil {
			log.Println(err)
			panic(err)
		}
		_, err = this.connection.Write(payload)
		if err != nil {
			log.Println(err)
			panic(err)
		}
	}
	n.sentTime = time.Now()
	log.Println("wrote", n.pushNotification.Identifier)
	this.lock.Lock()
	this.sentNotifications = append(this.sentNotifications, *n)
	this.lock.Unlock()
}

func (this *MultiClient) cleanSent() {
	log.Println("clean")
	this.lock.Lock()

	newSentNotifications := []notification{}

	for _, n := range this.sentNotifications {
		// After 10 sec we assume that we wont get error back from apple about a message
		if time.Now().Before(n.sentTime.Add(1 * time.Second)) {
			newSentNotifications = append(newSentNotifications, n)
			log.Println("uncleaned", n.pushNotification.Identifier)

		} else {
			log.Println("cleaned", n.pushNotification.Identifier)
		}
	}
	this.sentNotifications = newSentNotifications

	this.lock.Unlock()
}

func (this *MultiClient) receiveOne() {

	buffer := make([]byte, 6)
	_, err := this.connection.Read(buffer)
	if err != nil {
		log.Println(err)
		err = this.connection.Close()
		log.Println(err)
		return
	}
	time.Sleep(3 * time.Second)
	this.lock.Lock()
	{
		id := int32(binary.BigEndian.Uint32(buffer[2:6]))

		if buffer[1] != 0 {
			respStr := APPLE_PUSH_RESPONSES[buffer[1]]
			log.Println("resp", respStr)
			this.handleBadNotification(id)
		}

		err = this.connection.Close()
		if err != nil {
			log.Println(err)
		}
	}
	this.lock.Unlock()
}

func (this *MultiClient) handleBadNotification(id int32) {
	log.Println("bad Notification", id)
	for i, n := range this.sentNotifications {
		if n.pushNotification.Identifier == id {
			// requeue all after this item
			// throw id away
			// throw all before id away (they are ok)
			for _, n2 := range this.sentNotifications[i+1:] {
				log.Println("requeued", n2.pushNotification.Identifier)
				this.queuedNotifications <- &n2
				this.extra <- &n2
			}
			this.sentNotifications = []notification{}
			return
		}
	}
}
