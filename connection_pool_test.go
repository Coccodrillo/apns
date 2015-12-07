package apns

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func NewTestConnectionPool(numConnections int, gateway string, certificate tls.Certificate) *ConnectionPool {
	p := NewConnectionPool(numConnections, gateway, certificate)
	p.config.InsecureSkipVerify = true // we need to skip verification so our cert is accepted
	return p
}

func TestApnsNewConnectionPool(t *testing.T) {
	Convey("NewConnectionPool()", t, func() {
		credentials := [][]byte{[]byte("certificate")}
		gateway := "mygateway"
		certificate := tls.Certificate{Certificate: credentials}

		Convey("Should store our gateway and certificate", func() {
			p := NewTestConnectionPool(1, gateway, certificate)
			So(p, ShouldNotBeNil)
			So(p.size, ShouldEqual, 1)
			So(p.gateway, ShouldEqual, gateway)
			So(p.config.Certificates[0], ShouldResemble, certificate)
			So(p.config.Certificates[0].Certificate, ShouldResemble, credentials)
		})

		Convey("Should support multiple connections", func() {
			p := NewTestConnectionPool(5, gateway, certificate)
			So(p, ShouldNotBeNil)
			So(p.size, ShouldEqual, 5)
			So(len(p.connections), ShouldEqual, 5)
		})
	})
}

func TestApnsConnectionPoolGetConnection(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	certificate := tls.Certificate{}
	Convey("GetConnection()", t, func() {
		Convey("When gateway is invalid", func() {
			p := NewTestConnectionPool(1, "fake", certificate)
			Convey("Should return an error", func() {
				c, err := p.GetConnection()
				So(c, ShouldNotBeNil)
				So(c.IsOpen(), ShouldBeFalse)
				So(err, ShouldNotBeNil)
				So(p.position, ShouldEqual, 1)
			})

		})

		Convey("When the gateway is valid and the pool size is one", func() {
			p := NewTestConnectionPool(1, ts.URL[8:], ts.TLS.Certificates[0])
			Convey("Should return a connection at position 1", func() {
				c, err := p.GetConnection()
				So(c, ShouldNotBeNil)
				So(err, ShouldBeNil)
				So(p.position, ShouldEqual, 1)

				unique := c.ConnectionState().TLSUnique

				Convey("When called again", func() {
					Convey("Should return the same connection and position", func() {
						c, err := p.GetConnection()
						So(c, ShouldNotBeNil)
						So(err, ShouldBeNil)
						So(p.position, ShouldEqual, 1)
						So(c.ConnectionState().TLSUnique, ShouldResemble, unique)
					})
				})
			})
		})

		Convey("When the gateway is valid and the pool size is two", func() {
			p := NewTestConnectionPool(2, ts.URL[8:], ts.TLS.Certificates[0])
			var unique, unique2 []byte
			Convey("Should return a connection at position 1", func() {
				c, err := p.GetConnection()
				So(c, ShouldNotBeNil)
				So(err, ShouldBeNil)
				So(p.position, ShouldEqual, 1)
				unique = c.ConnectionState().TLSUnique

				Convey("When called a second time", func() {
					Convey("Should return a new connection at position 2", func() {
						c, err := p.GetConnection()
						So(c, ShouldNotBeNil)
						So(err, ShouldBeNil)
						So(p.position, ShouldEqual, 2)
						So(c.ConnectionState().TLSUnique, ShouldNotResemble, unique)
						unique2 = c.ConnectionState().TLSUnique

						Convey("When called a third time", func() {
							Convey("Should return first connection at position 1", func() {
								c, err := p.GetConnection()
								So(c, ShouldNotBeNil)
								So(err, ShouldBeNil)
								So(p.position, ShouldEqual, 1)
								So(c.ConnectionState().TLSUnique, ShouldResemble, unique)

								Convey("When called a fourth time", func() {
									Convey("Should return second connection at position 2", func() {
										c, err := p.GetConnection()
										So(c, ShouldNotBeNil)
										So(err, ShouldBeNil)
										So(p.position, ShouldEqual, 2)
										So(c.ConnectionState().TLSUnique, ShouldResemble, unique2)
									})
								})
							})
						})
					})
				})
			})
		})
	})
}

func TestApnsConnectionPoolWrite(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	poolSize := 10
	Convey("Write()", t, func() {
		Convey("When using a pool > 1", func() {
			p := NewTestConnectionPool(poolSize, ts.URL[8:], ts.TLS.Certificates[0])
			Convey("Should return the expected number of bytes written", func() {
				c, bytesWritten, err := p.Write([]byte("12345"))
				So(err, ShouldBeNil)
				So(bytesWritten, ShouldEqual, 5)
				unique := c.ConnectionState().TLSUnique

				Convey("When performing a second write", func() {
					Convey("Should be on different connection", func() {
						c, bytesWritten, err := p.Write([]byte("test"))
						So(err, ShouldBeNil)
						So(bytesWritten, ShouldEqual, 4)
						So(c.ConnectionState().TLSUnique, ShouldNotResemble, unique)
					})

				})

				Convey("When all connections are closed", func() {
					p.Close()
					Convey("Should reconnect", func() {
						_, bytesWritten, err := p.Write([]byte("6789"))
						So(err, ShouldBeNil)
						So(bytesWritten, ShouldEqual, 4)
					})
				})

			})

			Convey("When the server is closed", func() {
				ts.Close()
				Convey("Should give an error if the server is closed", func() {
					// force a peek
					for i := 0; i < poolSize; i++ {
						p.connections[i].writeCount = peekFrequency

					}
					_, _, err := p.Write([]byte("test"))
					So(err, ShouldNotBeNil)
				})
			})

		})
	})
}

func TestApnsConnectionPoolClose(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	poolSize := 10
	Convey("Close()", t, func() {
		p := NewTestConnectionPool(poolSize, ts.URL[8:], ts.TLS.Certificates[0])
		Convey("When there is no open connection", func() {
			Convey("Should not return an error", func() {
				err := p.Close()
				So(err, ShouldBeNil)
			})
		})

		Convey("When there is an open connection", func() {
			c, err := p.GetConnection()
			So(err, ShouldBeNil)
			So(c.IsOpen(), ShouldBeTrue)
			Convey("Should not return an error", func() {
				err = p.Close()
				So(err, ShouldBeNil)
				So(c.IsOpen(), ShouldBeFalse)
			})
		})

	})
}
