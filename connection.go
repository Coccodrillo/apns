package apns

import (
	"crypto/tls"
	"encoding/binary"
	"log"
	"time"
)

//ResponseQueueSize indicates how many APNS responses may be buffered.
var ResponseQueueSize = 10000

//SentBufferSize is the maximum number of sent notifications which may be buffered.
var SentBufferSize = 10000

var maxBackoff = 20 * time.Second

//Connection represents a single connection to APNS.
type Connection struct {
	Client
	conn   *tls.Conn
	queue  chan PushNotification
	errors chan BadPushNotification
}

//NewConnection initializes an APNS connection. Use Connection.Start() to actually start sending notifications.
func NewConnection(client *Client) *Connection {
	c := new(Connection)
	c.Client = *client
	queue := make(chan PushNotification)
	errors := make(chan BadPushNotification)
	c.queue = queue
	c.errors = errors
	return c
}

//Response is a reply from APNS - see apns.ApplePushResponses.
type Response struct {
	Status     uint8
	Identifier uint32
}

func newResponse() Response {
	r := Response{}
	return r
}

//BadPushNotification represents a notification which APNS didn't like.
type BadPushNotification struct {
	PushNotification
	Status uint8
}

//Enqueue adds a push notification to the end of the "sending" queue.
func (conn *Connection) Enqueue(pn *PushNotification) {
	go func(pn *PushNotification) {
		conn.queue <- *pn
	}(pn)
}

//Errors gives you a channel of the push notifications Apple rejected.
func (conn *Connection) Errors() (errors <-chan BadPushNotification) {
	return conn.errors
}

//Start initiates a connection to APNS and asnchronously sends notifications which have been queued.
func (conn *Connection) Start() error {
	//Connect to APNS. The reason this is here as well as in sender is that this probably catches any unavoidable errors in a synchronous fashion, while in sender it can reconnect after temporary errors (which should work most of the time.)
	err := conn.connect()
	if err != nil {
		return err
	}
	//Start sender goroutine
	sent := make(chan PushNotification)
	go conn.sender(conn.queue, sent)
	//Start reader goroutine
	responses := make(chan Response, ResponseQueueSize)
	go conn.reader(responses)
	//Start limbo goroutine
	go conn.limbo(sent, responses, conn.errors, conn.queue)
	return nil
}

//Stop gracefully closes the connection - it waits for the sending queue to clear, and then shuts down.
func (conn *Connection) Stop() {
	//We can't just close the main queue channel, because retries might still need to be sent there.
	//
}

func (conn *Connection) sender(queue <-chan PushNotification, sent chan PushNotification) {
	defer conn.conn.Close()
	var backoff = time.Duration(100)
	for {
		pn, ok := <-conn.queue
		if !ok {
			//That means the Connection is stopped
			//close sent?
			return
		} else {
			//If not connected, connect
			if conn.conn == nil {
				for {
					err := conn.connect()
					if err != nil {
						//Exponential backoff up to a limit
						log.Println("APNS: Error connecting to server: ", err)
						backoff = backoff * 2
						if backoff > maxBackoff {
							backoff = maxBackoff
						}
						time.Sleep(backoff)
					} else {
						backoff = 100
						break
					}
				}
			}
			//Then send the push notification
			//TODO(draaglom): Do buffering as per the APNS docs
			payload, err := pn.ToBytes()
			if err != nil {
				log.Println(err)
				//Should report this on the bad notifications channel probably
			} else {
				_, err = conn.conn.Write(payload)
				if err != nil {
					log.Println(err)
					//Disconnect?
				} else {
					sent <- pn
				}
			}
		}
	}
}

func (conn *Connection) reader(responses chan<- Response) {
	buffer := make([]byte, 6)
	for {
		n, err := conn.conn.Read(buffer)
		if err != nil && n < 6 {
			log.Println("APNS: Error before reading complete response", n, err)
			conn.conn.Close()
			conn.conn = nil
			return
		}
		command := uint8(buffer[0])
		if command != 8 {
			log.Println("Something went wrong: command should have been 8; it was actually", command)
		}
		resp := newResponse()
		resp.Identifier = binary.BigEndian.Uint32(buffer[2:6])
		resp.Status = uint8(buffer[1])
		responses <- resp
		conn.conn.Close()
		conn.conn = nil
		return
	}
}

func (conn *Connection) limbo(sent <-chan PushNotification, responses chan Response, errors chan BadPushNotification, queue chan PushNotification) {
	limbo := make(chan PushNotification, SentBufferSize)
	ticker := time.NewTicker(1 * time.Second)
	timeNextNotification := true
	for {
		select {
		case pn := <-sent:
			//Drop it into the array
			limbo <- pn
			if timeNextNotification {
				//Is there a cleaner way of doing this?
				go func(pn PushNotification) {
					<-time.After(TimeoutSeconds * time.Second)
					successResp := newResponse()
					successResp.Identifier = pn.Identifier
					responses <- successResp
				}(pn)
				timeNextNotification = false
			}
		case resp, ok := <-responses:
			if !ok {
				//If the responses channel is closed,
				//that means we're shutting down the connection.
			}
			switch {
			case resp.Status == 0:
				//Status 0 is a "success" response generated by a timeout in the library.
				for pn := range limbo {
					//Drop all the notifications until we get to the timed-out one.
					//(and leave the others in limbo)
					if pn.Identifier == resp.Identifier {
						break
					}
				}
			default:
				hit := false
				for pn := range limbo {
					switch {
					case pn.Identifier != resp.Identifier && !hit:
						//We haven't seen the identified notification yet
						//so these are all successful (drop silently)
					case pn.Identifier == resp.Identifier:
						hit = true
						if resp.Status != 10 {
							//It was an error, we should report this on the error channel
							bad := BadPushNotification{PushNotification: pn, Status: resp.Status}
							go func(bad BadPushNotification) {
								errors <- bad
							}(bad)
						}
					case pn.Identifier != resp.Identifier && hit:
						//We've already seen the identified notification,
						//so these should be requeued
						conn.Enqueue(&pn)
					}
				}
			}
		case <-ticker.C:
			timeNextNotification = true
		}
	}
}

func (conn *Connection) connect() error {
	if conn.conn != nil {
		conn.conn.Close()
	}

	var cert tls.Certificate
	var err error
	if len(conn.CertificateBase64) == 0 && len(conn.KeyBase64) == 0 {
		// The user did not specify raw block contents, so check the filesystem.
		cert, err = tls.LoadX509KeyPair(conn.CertificateFile, conn.KeyFile)
	} else {
		// The user provided the raw block contents, so use that.
		cert, err = tls.X509KeyPair([]byte(conn.CertificateBase64), []byte(conn.KeyBase64))
	}

	if err != nil {
		return err
	}

	conf := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	tlsConn, err := tls.Dial("tcp", conn.Gateway, conf)
	if err != nil {
		return err
	}
	err = tlsConn.Handshake()
	if err != nil {
		_ = tlsConn.Close()
		return err
	}
	conn.conn = tlsConn
	return nil
}
