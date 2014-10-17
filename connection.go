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
	conn            *tls.Conn
	queue           chan PushNotification
	errors          chan BadPushNotification
	responses       chan Response
	shouldReconnect chan bool
	stopping        chan bool
	stopped         chan bool
	senderFinished  chan bool
	ackFinished     chan bool
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
	//Start limbo goroutine
	go conn.limbo(sent, conn.responses, conn.errors, conn.queue)
	return nil
}

//Stop gracefully closes the connection - it waits for the sending queue to clear, and then shuts down.
func (conn *Connection) Stop() chan bool {
	log.Println("apns: shutting down.")
	conn.stopping <- true
	return conn.stopped
	//Thought: Don't necessarily need a channel here. Could signal finishing by closing errors?
}

func (conn *Connection) sender(queue <-chan PushNotification, sent chan PushNotification) {
	i := 0
	stopping := false
	defer conn.conn.Close()
	var backoff = time.Duration(100)
	for {
		select {
		case pn, ok := <-conn.queue:
			if !ok {
				log.Println("Not okay; queue closed.")
				//That means the Connection is stopped
				//close sent?
				return
			} else {
				//This means we saw a response; connection is over.
				select {
				case <-conn.shouldReconnect:
					conn.conn.Close()
					conn.conn = nil
					for {
						log.Println("Connection lost; reconnecting.")
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
							log.Println("Connected...")
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
					n, err := conn.conn.Write(payload)
					if err != nil {
						log.Println(err)
						go func() {
							conn.shouldReconnect <- true
						}()
						//Disconnect?
					} else {
						i++
						log.Println("Sent count:", i)
						log.Println("Wrote bytes:", n, "of notification:", pn.Identifier)
						sent <- pn
						if stopping && len(queue) == 0 {
							log.Println("sender: I'm stopping and I've run out of things to send. Let's see if limbo is empty.")
							conn.senderFinished <- true
						}
					}
				}
			}
		case <-conn.stopping:
			log.Println("sender: Got a stop message!")
			stopping = true
			if len(queue) == 0 {
				log.Println("sender: I'm stopping and I've run out of things to send. Let's see if limbo is empty.")
				conn.senderFinished <- true
			}
		case <-conn.ackFinished:
			log.Println("sender: limbo is empty!")
			if len(queue) == 0 {
				log.Println("sender: limbo is empty and so am I!")
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
			//Drop it into the array
			limbo = append(limbo, pn.timed())
			stopping = false
			if !ok {
				log.Println("limbo: sent is closed, so sender is done. So am I, then!")
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
			log.Println("Saw an error response; flushing up to", resp.Identifier)
			log.Println("PNs in limbo:", len(limbo))
			for i, pn := range limbo {
				if pn.Identifier == resp.Identifier {
					log.Println("Got the bad one!!! At index:", i, "identifier:", pn.Identifier)
					if resp.Status != 10 {
						//It was an error, we should report this on the error channel
						bad := BadPushNotification{PushNotification: pn.PushNotification, Status: resp.Status}
						go func(bad BadPushNotification) {
							errors <- bad
						}(bad)
					}
					if len(limbo) > i {
						log.Printf("Requeueing %d notifications\n", len(limbo)-(i+1))
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
			log.Println("Notifications in limbo:", len(limbo))
			for i := range limbo {
				if limbo[i].After(time.Now().Add(-TimeoutSeconds * time.Second)) {
					log.Printf("The first %d notifications timed out ok.\n", i)
					newLimbo := make([]timedPushNotification, len(limbo[i:]), SentBufferSize)
					copy(newLimbo, limbo[i:])
					limbo = newLimbo
					log.Println("Size of limbo is now:", len(limbo))
					flushed = true
					break
				}
			}
			if !flushed {
				log.Println("Looks like every notification has timed out.")
				limbo = make([]timedPushNotification, 0, SentBufferSize)
			}
			if stopping && len(limbo) == 0 {
				log.Println("limbo: I've flushed all my notifications. Tell sender I'm done.")
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
	//Start reader goroutine
	go conn.reader(conn.responses)
	return nil
}
