package apns

import (
	"crypto/tls"
	"encoding/binary"
	"sync"
	"time"
)

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

	sentNotifications []notification
	// beware, if the notificaiton queue stores pointers instead of the full object a threading bug might occur.
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

// Queue will enqueue the notification for processing.
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
	payload, err := n.pushNotification.ToBytes()
	if err != nil {
		return err
	}

	if this.connection == nil {
		err := this.connect()
		if err != nil {
			return err
		}
	}
	_, err = this.connection.Write(payload)
	if err != nil {
		//if connection failed, try to reconnect and rewrite payload
		err = this.connection.Close()
		if err != nil {
		}
		err = this.connect()
		if err != nil {
			return err
		}
		_, err = this.connection.Write(payload)
		if err != nil {
			return err
		}
	}
	n.sentTime = time.Now()
	this.lock.Lock()
	this.sentNotifications = append(this.sentNotifications, n)
	this.lock.Unlock()
	return nil
}

func (this *Client) cleanSent() {
	this.lock.Lock()

	newSentNotifications := []notification{}

	for _, n := range this.sentNotifications {
		// After TIMEOUT_SECONDS we assume that we wont get error back from apple about a message
		if time.Now().Before(n.sentTime.Add(TIMEOUT_SECONDS * time.Second)) {
			newSentNotifications = append(newSentNotifications, n)
		}
	}
	this.sentNotifications = newSentNotifications

	this.lock.Unlock()
}

func (this *Client) receiveOne() {

	buffer := make([]byte, 6)
	_, err := this.connection.Read(buffer)
	if err != nil {
		this.connection.Close()
		return
	}
	//time.Sleep(3 * time.Second)
	this.lock.Lock()
	{
		id := binary.BigEndian.Uint32(buffer[2:6])

		if buffer[1] != 0 {
			this.handleBadNotification(id, buffer[1])
		}

		this.connection.Close()
	}
	this.lock.Unlock()
}

func (this *Client) handleBadNotification(id uint32, responseCode uint8) {
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
				this.queuedNotifications <- n2
			}
			this.sentNotifications = []notification{}
			return
		}
	}
}
