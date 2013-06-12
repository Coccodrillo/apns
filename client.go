package apns

import (
	"crypto/tls"
	"net"
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
}

// Constructor.
func NewClient(gateway, certificateFile, keyFile string) (c *Client) {
	c = new(Client)
	c.Gateway = gateway
	c.CertificateFile = certificateFile
	c.KeyFile = keyFile
	return
}

// Connects to the APN service and sends your push notification.
// Remember that if the submission is successful, Apple won't reply.
func (this *Client) Send(pn *PushNotification) (resp *PushNotificationResponse) {
	resp = new(PushNotificationResponse)

	payload, err := pn.ToBytes()
	if err != nil {
		resp.Success = false
		resp.Error = err
	}

	err = this.ConnectAndWrite(resp, payload)
	if err != nil {
		resp.Success = false
		resp.Error = err
	}

	resp.Success = true
	resp.Error = nil

	return
}

// In lieu of a timeout (which would be available in Go 1.1)
// we use a timeout channel pattern instead. We start two goroutines,
// one of which just sleeps for TIMEOUT_SECONDS seconds, while the other
// waits for a response from the Apple servers.
//
// Whichever channel puts data on first is the "winner". As such, it's
// possible to get a false positive if Apple takes a long time to respond.
// It's probably not a deal-breaker, but something to be aware of.
func (this *Client) ConnectAndWrite(resp *PushNotificationResponse, payload []byte) (err error) {
	var cert tls.Certificate

	if len(this.CertificateBase64) == 0 && len(this.KeyBase64) == 0 {
		cert, err = tls.X509KeyPair([]byte(this.CertificateBase64), []byte(this.KeyBase64))
	} else {
		cert, err = tls.LoadX509KeyPair(this.CertificateFile, this.KeyFile)
	}

	if err != nil {
		return err
	}

	conf := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	conn, err := net.Dial("tcp", this.Gateway)
	if err != nil {
		return err
	}

	tlsConn := tls.Client(conn, conf)
	err = tlsConn.Handshake()
	if err != nil {
		return err
	}

	_, err = tlsConn.Write(payload)
	if err != nil {
		return err
	}

	// Create one channel that will serve to handle
	// timeouts when the notification succeeds.
	timeoutChannel := make(chan bool, 1)
	go func() {
		time.Sleep(time.Second * TIMEOUT_SECONDS)
		timeoutChannel <- true
	}()

	// This channel will contain the binary response
	// from Apple in the event of a failure.
	responseChannel := make(chan []byte, 1)
	go func() {
		buffer := make([]byte, 6, 6)
		conn.Read(buffer)
		responseChannel <- buffer
	}()

	// First one back wins!
	// The data structure for an APN response is as follows:
	//
	// command    -> 1 byte
	// status     -> 1 byte
	// identifier -> 4 bytes
	//
	// The first byte will always be set to 8.
	resp = NewPushNotificationResponse()
	select {
	case r := <-responseChannel:
		resp.Success = false
		resp.AppleResponse = APPLE_PUSH_RESPONSES[r[1]]
	case <-timeoutChannel:
		resp.Success = true
	}

	return nil
}
