package apns

import (
	"crypto/tls"
	"net"
	"time"
)

// Abstracts a connection to a push notification gateway.
type Connection struct {
	gateway        string
	timeoutSeconds uint8

	tlsCertificate tls.Certificate
	tcpConnection  net.Conn
	socket         *tls.Conn
}

// Constructor. Pass the Apple production or sandbox gateways,
// or an endpoint of your choosing.
//
// Returns a Connection struct with the gateway configured and the
// push notification response timeout set to TIMEOUT_SECONDS.
func NewConnection(gateway string) (c *Connection) {
	c = new(Connection)
	c.gateway = gateway
	c.timeoutSeconds = TIMEOUT_SECONDS
	return
}

// Sets the number of seconds to wait for the gateway to respond
// after sending a push notification.
func (connection *Connection) SetTimeoutSeconds(seconds uint8) {
	connection.timeoutSeconds = seconds
}

// Create and store a tls.Certificate using the provided parameters.
// If you set rawBytes to true, the method assumes that you are
// providing base64 certificate and key data. If set to false, it
// will assume you are providing paths to files on drive.
//
// Returns an error if there is a problem creating the certificate.
func (connection *Connection) CreateCertificate(certificate, key string, rawBytes bool) (err error) {
	var cert tls.Certificate
	if rawBytes {
		cert, err = tls.X509KeyPair([]byte(certificate), []byte(key))
	} else {
		cert, err = tls.LoadX509KeyPair(certificate, key)
	}
	if err != nil {
		return err
	}
	connection.tlsCertificate = cert
	return nil
}

// Creates and sets connections to the gateway. Apple treats repeated
// connections / disconnections as a potential DoS attack; you should
// connect and reuse the connection for multiple push notifications.
//
// Returns an error if there is a problem connecting to the gateway.
func (connection *Connection) Connect() (err error) {
	conf := &tls.Config{
		Certificates: []tls.Certificate{connection.tlsCertificate},
	}
	conn, err := net.Dial("tcp", connection.gateway)
	if err != nil {
		return err
	}
	tlsConn := tls.Client(conn, conf)
	err = tlsConn.Handshake()
	if err != nil {
		return err
	}
	connection.tcpConnection = conn
	connection.socket = tlsConn
	return nil
}

// Attempts to send a push notification to the connection's gateway.
// Apple will not respond if the push notification payload is accepted,
// so a timeout channel pattern is used. Two goroutines are started: one
// waits for connection.timeoutSeconds and the other listens for a response from Apple.
// Whichever returns first is the winner, so some false-positives are possible
// if the gateway takes an excessively long amount of time to reply.
//
// Returns a PushNotificationResponse indicating success / failure and what
// error occurred, if any.
func (connection *Connection) Send(pn *PushNotification) (resp *PushNotificationResponse) {
	resp = new(PushNotificationResponse)
	resp.Success = false

	payload, err := pn.ToBytes()
	if err != nil {
		resp.Error = err
		return
	}

	_, err = connection.socket.Write(payload)
	if err != nil {
		resp.Error = err
		return
	}

	// Create one channel that will serve to handle
	// timeouts when the notification succeeds.
	timeoutChannel := make(chan bool, 1)
	go func() {
		time.Sleep(time.Second * time.Duration(connection.timeoutSeconds))
		timeoutChannel <- true
	}()

	// This channel will contain the binary response
	// from Apple in the event of a failure.
	responseChannel := make(chan []byte, 1)
	go func() {
		buffer := make([]byte, 6, 6)
		connection.socket.Read(buffer)
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
		resp.AppleResponse = APPLE_PUSH_RESPONSES[r[1]]
	case <-timeoutChannel:
		resp.Success = true
	}

	return
}

// Disconnects from the gateway.
func (connection *Connection) Disconnect() {
	connection.tcpConnection.Close()
	connection.socket.Close()
}
