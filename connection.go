package apns

import (
	"bufio"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"time"
)

const (
	handShakeTimeout = 5   // seconds
	peekTimeout      = 250 // miliseconds
	peekFrequency    = 25  // every N writes
	keepAlive        = 10  // minutes
)

type Connection struct {
	connection  *tls.Conn
	writeCount  int
	connectTime time.Time
}

type Connectioner interface {
	Write(b []byte) (n int, err error)
}

func (c *Connection) Open(gateway string, config *tls.Config) error {
	// connect to the gateway
	nc, err := net.Dial("tcp", gateway)
	if err != nil {
		return err
	}

	// attempt a handshake
	tc := tls.Client(nc, config)
	tc.SetDeadline(time.Now().Add(time.Duration(handShakeTimeout) * time.Second))
	if err = tc.Handshake(); err != nil {
		return err
	}
	tc.SetDeadline(time.Time{})

	// keep the connection
	c.connection = tc
	c.connectTime = time.Now()
	c.writeCount = 0

	return nil
}

func (c *Connection) IsOpen() bool {
	if c.connection == nil {
		return false
	}

	// has the connection expired?
	if time.Now().After(c.connectTime.Add(time.Duration(keepAlive) * time.Minute)) {
		return false
	}

	// should we check if the connection is still alive?
	if c.writeCount > 0 && c.writeCount%peekFrequency == 0 {
		if err := c.Peek(); err != nil {
			return false
		}
	}

	return true
}

func (c *Connection) Peek() error {
	if c.connection == nil {
		return errors.New("no connection")
	}

	// attempt to peek at one byte
	// if we get an EOF, close the connection
	reader := bufio.NewReader(c.connection)
	c.connection.SetReadDeadline(time.Now().Add(time.Duration(peekTimeout) * time.Millisecond))
	if _, err := reader.Peek(1); err == io.EOF {
		c.Close()
		return err
	}

	c.connection.SetReadDeadline(time.Time{})

	return nil
}

func (c *Connection) Close() error {
	if c.connection == nil {
		return nil
	}

	err := c.connection.Close()
	if err == nil {
		c.connection = nil
		c.writeCount = 0
		c.connectTime = time.Time{}
	}

	return err
}

func (c *Connection) Read(b []byte) (n int, err error) {
	if !c.IsOpen() {
		return 0, errors.New("no connection")
	}
	return c.connection.Read(b)
}

func (c *Connection) Write(b []byte) (n int, err error) {
	if !c.IsOpen() {
		return 0, errors.New("no connection")
	}

	c.writeCount++
	return c.connection.Write(b)
}

func (c *Connection) LocalAddr() net.Addr {
	if !c.IsOpen() {
		return nil
	}
	return c.connection.LocalAddr()
}

func (c *Connection) RemoteAddr() net.Addr {
	if !c.IsOpen() {
		return nil
	}
	return c.connection.RemoteAddr()
}

func (c *Connection) SetDeadline(t time.Time) error {
	if !c.IsOpen() {
		return errors.New("no connection")
	}
	return c.connection.SetDeadline(t)
}

func (c *Connection) SetReadDeadline(t time.Time) error {
	if !c.IsOpen() {
		return errors.New("no connection")
	}
	return c.connection.SetReadDeadline(t)
}

func (c *Connection) SetWriteDeadline(t time.Time) error {
	if !c.IsOpen() {
		return errors.New("no connection")
	}
	return c.connection.SetWriteDeadline(t)
}

func (c *Connection) ConnectionState() tls.ConnectionState {
	if !c.IsOpen() {
		return tls.ConnectionState{}
	}
	return c.connection.ConnectionState()
}
