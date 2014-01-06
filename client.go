package apns

import (
	"crypto/tls"
	"encoding/binary"
	//"log"
	"sync"
	"time"
)

// beware, there might be a threading bug in here! (seen if the queue contains pointers to notifications instead of objects) /Victor 20131213

// You'll need to provide your own CertificateFile
// and KeyFile to send notifications. Ideally, you'll
// just set the CertificateFile and KeyFile fields to
// a location on drive where the certs can be loaded,
// but if you prefer you can use the CertificateBase64
// and KeyBase64 fields to store the actual contents.
type Client struct {
	Gateway           string
	CertificateFile   string
	CertificateBase64 string
	KeyFile           string
	KeyBase64         string

	connection *tls.Conn

	sentNotifications   []notification
	queuedNotifications chan notification
	errorChannel        chan *PushNotificationResponse

	lock *sync.Mutex
}

type notification struct {
	pushNotification PushNotification
	success          bool
	sentTime         time.Time
}

// Constructor. Use this if you want to set cert and key blocks manually.
func BareClient(gateway, certificateBase64, keyBase64 string) (c *Client) {
	c = newClient()
	c.Gateway = gateway
	c.CertificateBase64 = certificateBase64
	c.KeyBase64 = keyBase64
	return
}

// Constructor. Use this if you want to load cert and key blocks from a file.
func NewClient(gateway, certificateFile, keyFile string) (c *Client) {
	c = newClient()
	c.Gateway = gateway
	c.CertificateFile = certificateFile
	c.KeyFile = keyFile
	return
}

func newClient() *Client {
	c := &Client{}
	c.sentNotifications = []notification{}
	c.queuedNotifications = make(chan notification, 10000)
	c.lock = &sync.Mutex{}
	c.errorChannel = make(chan *PushNotificationResponse, 1000)
	return c
}

// FailedNotifications returns a channel on which failed notifications will be returned.
// At most 1000 failed notifications can be kept here, if more notifications fails those
// notifications will be dropped silently.
func (this *Client) FailedNotifications() chan *PushNotificationResponse {
	return this.errorChannel
}

// Run sends queued notifications.
func (this *Client) Run() error {
	for {
		n := <-this.queuedNotifications
		//log.Printf("run %v %v %p", n.pushNotification.Identifier, len(this.queuedNotifications), n)
		//log.Printf("%p", n)
		err := this.sendOne(n)
		if err != nil {
			resp := NewPushNotificationResponse(&n.pushNotification)
			resp.Error = &err
			this.reportFailed(resp)
			return err
		}
		this.cleanSent()
	}
}

// Queue will enqueue the notification for later processing.
func (this *Client) Queue(pn *PushNotification) {
	this.queuedNotifications <- notification{pushNotification: *pn}
}

func (this *Client) reportFailed(pn *PushNotificationResponse) {
	// Never block to make sure we continue to send notifications even if no one is reading the failed ones.
	select {
	case this.errorChannel <- pn:
	default:
	}
}

func (this *Client) connect() error {
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

func (this *Client) sendOne(n notification) error {
	//log.Println("sendOne", n.pushNotification.Identifier)
	payload, err := n.pushNotification.ToBytes()
	if err != nil {
		return err
	}

	if this.connection == nil {
		err := this.connect()
		if err != nil {
			//log.Println(err)
			return err
		}
	}
	_, err = this.connection.Write(payload)
	if err != nil {
		//if not connected, try to reconnect and rewrite payload
		//log.Println(err)

		err = this.connection.Close()
		if err != nil {
			//log.Println(err)
		}
		err = this.connect()
		if err != nil {
			//log.Println(err)
			return err
		}
		_, err = this.connection.Write(payload)
		if err != nil {
			//log.Println(err)
			return err
		}
	}
	n.sentTime = time.Now()
	//log.Println("sent", n.pushNotification.Identifier)
	this.lock.Lock()
	this.sentNotifications = append(this.sentNotifications, n)
	this.lock.Unlock()
	return nil
}

func (this *Client) cleanSent() {
	//log.Println("clean")
	this.lock.Lock()

	newSentNotifications := []notification{}

	for _, n := range this.sentNotifications {
		// After TIMEOUT_SECONDS we assume that we wont get error back from apple about a message
		if time.Now().Before(n.sentTime.Add(TIMEOUT_SECONDS * time.Second)) {
			newSentNotifications = append(newSentNotifications, n)
			//log.Println("uncleaned", n.pushNotification.Identifier)
		} else {
			//log.Println("cleaned", n.pushNotification.Identifier)
		}
	}
	this.sentNotifications = newSentNotifications

	this.lock.Unlock()
}

func (this *Client) receiveOne() {

	buffer := make([]byte, 6)
	_, err := this.connection.Read(buffer)
	if err != nil {
		//log.Println(err)
		err = this.connection.Close()
		//log.Println(err)
		return
	}
	//time.Sleep(3 * time.Second)
	this.lock.Lock()
	{
		id := int32(binary.BigEndian.Uint32(buffer[2:6]))

		if buffer[1] != 0 {
			this.handleBadNotification(id, buffer[1])
		}

		err = this.connection.Close()
		if err != nil {
			//log.Println(err)
		}
	}
	this.lock.Unlock()
}

func (this *Client) handleBadNotification(id int32, responseCode uint8) {
	//log.Println("bad Notification", id)
	for i, n := range this.sentNotifications {
		if n.pushNotification.Identifier == id {
			// requeue all after this item
			// report failed on error chan
			// throw all before id away (they are ok)
			resp := NewPushNotificationResponse(&n.pushNotification)
			appleResponse := APPLE_PUSH_RESPONSES[responseCode]
			resp.AppleResponse = &appleResponse
			this.reportFailed(resp)

			for _, n2 := range this.sentNotifications[i+1:] {
				//log.Println("requeued", n2.pushNotification.Identifier)
				this.queuedNotifications <- n2
			}
			this.sentNotifications = []notification{}
			return
		}
	}
}
