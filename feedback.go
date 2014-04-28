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
const FEEDBACK_TIMEOUT_SECONDS = 5

// FeedbackChannel will receive individual responses from Apple.
var FeedbackChannel = make(chan (*FeedbackResponse))

// If there's nothing to read, ShutdownChannel gets a true.
var ShutdownChannel = make(chan bool)

type FeedbackResponse struct {
	Timestamp   uint32
	DeviceToken string
}

// Constructor.
func NewFeedbackResponse() (resp *FeedbackResponse) {
	resp = new(FeedbackResponse)
	return
}

// Connect to the Apple Feedback Service and check for feedback.
// Feedback consists of device identifiers that should
// not be sent to in the future; Apple does monitor that
// you respect this so you should be checking it ;)
func (this *Client) ListenForFeedback() (err error) {
	var cert tls.Certificate

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

	conn, err := net.Dial("tcp", this.Gateway)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(FEEDBACK_TIMEOUT_SECONDS * time.Second))

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
			return errors.New("Token length should be equal to 32, but isn't.")
		}
		resp.DeviceToken = hex.EncodeToString(deviceToken)

		FeedbackChannel <- resp
	}

	return nil
}
