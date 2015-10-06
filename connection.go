package apns

import (
	"bufio"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"sync"
	"time"
)

const (
	handShakeTimeout = 5   // seconds
	peekTimeout      = 500 // milliseconds
	peekFrequency    = 100 // every N writes
	keepAlive        = 10  // minutes
)

var ErrNoConnection = errors.New("no connection")

type Connection struct {
	connection    *tls.Conn
	writeCount    int
	connectTime   time.Time
	peekOnce      sync.Once
	peekWaitGroup sync.WaitGroup
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
		peekResult := true
		peekOnceReset := false

		// we only want one routine running this at a time
		c.peekWaitGroup.Add(1)
		c.peekOnce.Do(func() {
			peekOnceReset = true
			if err := c.Peek(); err != nil {
				peekResult = false
			} else {
				peekResult = true
			}
		})
		c.peekWaitGroup.Done()

		// on reset, wait for all of the once.Do() to finish
		// otherwise they'll block indefinitely if we reset it
		if peekOnceReset {
			c.peekWaitGroup.Wait()
			c.peekOnce = sync.Once{}
		}

		// another routine may have closed the connect, so double check
		if !peekResult || c.connection == nil {
			return false
		}
	}

	return true
}

func (c *Connection) Peek() error {
	if c.connection == nil {
		return ErrNoConnection
	}

	// wait a set time for a response
	timeoutChannel := make(chan bool, 1)
	go func() {
		time.Sleep(peekTimeout * time.Millisecond)
		timeoutChannel <- true
	}()

	// try to peek at one byte on the connection
	// only consider an io.EOF a failure
	responseChannel := make(chan bool, 1)
	go func() {
		reader := bufio.NewReader(c.connection)
		_, err := reader.Peek(1)
		responseChannel <- err != io.EOF
	}()

	// wait for our channels, and if the response channel is false, close the connection
	select {
	case r := <-responseChannel:
		if !r {
			c.Close()
			return io.EOF
		}
	case <-timeoutChannel:
	}

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
		return 0, ErrNoConnection
	}
	return c.connection.Read(b)
}

func (c *Connection) Write(b []byte) (n int, err error) {
	if !c.IsOpen() {
		return 0, ErrNoConnection
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
		return ErrNoConnection
	}
	return c.connection.SetDeadline(t)
}

func (c *Connection) SetReadDeadline(t time.Time) error {
	if !c.IsOpen() {
		return ErrNoConnection
	}
	return c.connection.SetReadDeadline(t)
}

func (c *Connection) SetWriteDeadline(t time.Time) error {
	if !c.IsOpen() {
		return ErrNoConnection
	}
	return c.connection.SetWriteDeadline(t)
}

func (c *Connection) ConnectionState() tls.ConnectionState {
	if !c.IsOpen() {
		return tls.ConnectionState{}
	}
	return c.connection.ConnectionState()
}
