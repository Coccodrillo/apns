package apns

import (
	"crypto/tls"
	"fmt"
	"strings"
	"sync"
)

type ConnectionPool struct {
	size          int
	position      int
	positionMutex sync.Mutex
	gateway       string
	config        *tls.Config
	connections   []*Connection
}

func NewConnectionPool(numConnections int, gateway string, certificate tls.Certificate) *ConnectionPool {
	gatewayParts := strings.Split(gateway, ":")
	config := &tls.Config{
		Certificates: []tls.Certificate{certificate},
		ServerName:   gatewayParts[0],
	}

	c := ConnectionPool{
		size:    numConnections,
		gateway: gateway,
		config:  config,
	}

	// init the connections
	c.connections = make([]*Connection, c.size, c.size)
	for i := 0; i < c.size; i++ {
		c.connections[i] = &Connection{}
	}

	return &c
}

func (p *ConnectionPool) GetConnection() (*Connection, error) {
	// increment the position, but ensure only one routine is doing this at a time
	// otherwise, we may go out of range when getting our connection
	p.positionMutex.Lock()

	// our position is 1 to size
	p.position++
	if p.position > p.size {
		p.position = 1
	}

	c := p.connections[p.position-1]

	p.positionMutex.Unlock()

	var err error
	if !c.IsOpen() {
		err = c.Open(p.gateway, p.config)
	}

	return c, err
}

func (p *ConnectionPool) Write(b []byte) (*Connection, int, error) {
	var err error
	var bytesWritten int
	var c *Connection

	for i := 0; i < p.size; i++ {
		c, err = p.GetConnection()
		if err != nil {
			continue
		}

		bytesWritten, err = c.Write(b)
		if err != nil || bytesWritten == 0 {
			c.Close()
			continue
		}

		break
	}

	if err == nil && bytesWritten == 0 && len(b) > 0 {
		err = fmt.Errorf("no bytes written on connection - expected %d", len(b))
	}

	return c, bytesWritten, err
}

func (p *ConnectionPool) Close() error {
	for i := 0; i < p.size; i++ {
		if err := p.connections[i].Close(); err != nil {
			return err
		}
	}
	return nil
}
