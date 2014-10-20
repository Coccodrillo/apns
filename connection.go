package apns

import (
	"crypto/tls"
	"encoding/binary"
	"log"
	"time"
)

//QueueSize is the maximum number of unsent notifications which can be queued.
var QueueSize = 10000

//ResponseQueueSize indicates how many APNS responses may be buffered.
var ResponseQueueSize = 10000

//SentBufferSize is the maximum number of sent notifications which may be buffered.
var SentBufferSize = 10000

var maxBackoff = 20 * time.Second

//Connection represents a single connection to APNS.
type Connection struct {
	Client
	conn            *tls.Conn
	queue           chan PushNotification    //notifications to be sent
	errors          chan BadPushNotification //error responses from APNS, delivered to the client
	responses       chan Response            //error responses from APNS, before they're matched with their original notification
	shouldReconnect chan bool                //sender waits for this signal to reconnect
	stopping        chan bool                //this signal starts the shutdown process
	stopped         chan bool                //this signals the shutdown process has finished
	senderFinished  chan bool                //sender() tells limbo() it has done
	ackFinished     chan bool                //limbo() tells sender() it's also done
}

//NewConnection initializes an APNS connection. Use Connection.Start() to actually start sending notifications.
func NewConnection(client *Client) *Connection {
	c := new(Connection)
	c.Client = *client
	c.queue = make(chan PushNotification)
	c.errors = make(chan BadPushNotification)
	c.responses = make(chan Response, ResponseQueueSize)
	c.shouldReconnect = make(chan bool)
	c.stopping = make(chan bool)
	c.stopped = make(chan bool)

	c.senderFinished = make(chan bool)
	c.ackFinished = make(chan bool)

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

type timedPushNotification struct {
	PushNotification
	time.Time
}

func (pn PushNotification) timed() timedPushNotification {
	return timedPushNotification{PushNotification: pn, Time: time.Now()}
}

//Enqueue adds a push notification to the end of the "sending" queue.
func (conn *Connection) Enqueue(pn *PushNotification) {
	go func(pn *PushNotification) {
		conn.queue <- *pn
	}(pn)
}

//Errors gives you a channel of the push notifications which Apple rejected.
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
	sent := make(chan PushNotification)
	go conn.sender(conn.queue, sent)
	go conn.limbo(sent, conn.responses, conn.errors, conn.queue)
	return nil
}

//Stop gracefully closes the connection - it waits for the sending queue to clear, and then shuts down.
func (conn *Connection) Stop() chan bool {
	log.Println("apns: shutting down.")
	conn.stopping <- true
	return conn.stopped
}

func (conn *Connection) sender(queue <-chan PushNotification, sent chan PushNotification) {
	stopping := false
	defer conn.conn.Close()
	var backoff = time.Duration(100)
	for {
		select {
		case pn, ok := <-conn.queue:
			if !ok {
				//That means the Connection is stopped
				close(sent)
				return
			}
			select {
			case <-conn.shouldReconnect: //This means we saw a response; connection is over.
				conn.conn.Close()
				conn.conn = nil
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
			default:
			}
			//Then send the push notification
			payload, err := pn.ToBytes()
			if err != nil {
				log.Println(err)
				//Should report this on the bad notifications channel probably
			} else {
				_, err := conn.conn.Write(payload)
				if err != nil {
					//Should resend this.
					go func() {
						conn.shouldReconnect <- true
					}()
				} else {
					sent <- pn
					if stopping && len(queue) == 0 {
						conn.senderFinished <- true
					}
				}
			}
		case <-conn.stopping:
			stopping = true
			if len(queue) == 0 {
				conn.senderFinished <- true
			}
		case <-conn.ackFinished:
			if len(queue) == 0 {
				close(sent)
				return
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
		conn.shouldReconnect <- true
		return
	}
}

func (conn *Connection) limbo(sent <-chan PushNotification, responses chan Response, errors chan BadPushNotification, queue chan PushNotification) {
	stopping := false
	limbo := make([]timedPushNotification, 0, SentBufferSize)
	ticker := time.NewTicker(1 * time.Second)
	for {
		select {
		case pn, ok := <-sent:
			limbo = append(limbo, pn.timed())
			stopping = false
			if !ok {
				close(errors)
				conn.stopped <- true
				return
			}
		case <-conn.senderFinished:
			stopping = true
		case resp, ok := <-responses:
			if !ok {
				//If the responses channel is closed,
				//that means we're shutting down the connection.
			}
			for i, pn := range limbo {
				if pn.Identifier == resp.Identifier {
					if resp.Status != 10 {
						//It was an error, we should report this on the error channel
						bad := BadPushNotification{PushNotification: pn.PushNotification, Status: resp.Status}
						go func(bad BadPushNotification) {
							errors <- bad
						}(bad)
					}
					if len(limbo) > i {
						toRequeue := len(limbo) - (i + 1)
						if toRequeue > 0 {
							conn.requeue(limbo[i+1:])
							stopping = false
						}
					}
				}
			}
			limbo = make([]timedPushNotification, 0, SentBufferSize)
		case <-ticker.C:
			flushed := false
			for i := range limbo {
				if limbo[i].After(time.Now().Add(-TimeoutSeconds * time.Second)) {
					newLimbo := make([]timedPushNotification, len(limbo[i:]), SentBufferSize)
					copy(newLimbo, limbo[i:])
					limbo = newLimbo
					flushed = true
					break
				}
			}
			if !flushed {
				limbo = make([]timedPushNotification, 0, SentBufferSize)
			}
			if stopping && len(limbo) == 0 {
				conn.ackFinished <- true
			}
		}
	}
}

func (conn *Connection) requeue(queue []timedPushNotification) {
	for _, pn := range queue {
		conn.Enqueue(&pn.PushNotification)
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
	go conn.reader(conn.responses)
	return nil
}
