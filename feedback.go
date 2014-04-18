package apns

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"net"
	"time"
)

// Wait at most this many seconds for feedback data from Apple.
const FeedbackTimeoutSeconds = 5

// FeedbackChannel will receive individual responses from Apple.
var FeedbackChannel = make(chan (*FeedbackResponse))

// If there's nothing to read, ShutdownChannel gets a true.
var ShutdownChannel = make(chan bool)

// FeedbackResponse represents a device token that Apple has
// indicated should not be sent to in the future.
type FeedbackResponse struct {
	Timestamp   uint32
	DeviceToken string
}

// NewFeedbackResponse creates and returns a FeedbackResponse structure.
func NewFeedbackResponse() (resp *FeedbackResponse) {
	resp = new(FeedbackResponse)
	return
}

// ListenForFeedback connects to the Apple Feedback Service
// and checks for device tokens.
//
// Feedback consists of device tokens that should
// not be sent to in the future; Apple *does* monitor that
// you respect this so you should be checking it ;)
func (client *Client) ListenForFeedback() (err error) {
	cert, err := tls.LoadX509KeyPair(client.CertificateFile, client.KeyFile)
	if err != nil {
		return err
	}

	conf := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	conn, err := net.Dial("tcp", client.Gateway)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(FeedbackTimeoutSeconds * time.Second))

	tlsConn := tls.Client(conn, conf)
	err = tlsConn.Handshake()
	if err != nil {
		return err
	}

	var tokenLength uint16
	buffer := make([]byte, 38, 38)
	deviceToken := make([]byte, 32, 32)

	for {
		_, err := tlsConn.Read(buffer)
		if err != nil {
			ShutdownChannel <- true
			break
		}

		resp := NewFeedbackResponse()

		r := bytes.NewReader(buffer)
		binary.Read(r, binary.BigEndian, &resp.Timestamp)
		binary.Read(r, binary.BigEndian, &tokenLength)
		binary.Read(r, binary.BigEndian, &deviceToken)
		if tokenLength != 32 {
			return errors.New("token length should be equal to 32, but isn't")
		}
		resp.DeviceToken = hex.EncodeToString(deviceToken)

		FeedbackChannel <- resp
	}

	return nil
}
