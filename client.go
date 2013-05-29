package apns

import (
	"crypto/tls"
	"net"
	"time"
)

type Client struct {
	Gateway         string
	CertificateFile string
	KeyFile         string
}

func NewClient(gateway, certificateFile, keyFile string) (c *Client) {
	c = new(Client)
	c.Gateway = gateway
	c.CertificateFile = certificateFile
	c.KeyFile = keyFile
	return
}

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

	return
}

func (this *Client) ConnectAndWrite(resp *PushNotificationResponse, payload []byte) (err error) {
	cert, err := tls.LoadX509KeyPair(this.CertificateFile, this.KeyFile)
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
