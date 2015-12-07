package apns

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

// apnsConnectionInsecureOpen is convenience method to skip certificate verification.
func apnsConnectionInsecureOpen(c *Connection, gateway string, config *tls.Config) error {
	config.InsecureSkipVerify = true // skip verification for the connection to accept our cert
	return c.Open(gateway, config)
}

func TestApnsConnectionOpen(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	config := &tls.Config{
		Certificates: []tls.Certificate{},
		ServerName:   ts.URL,
	}

	c := Connection{}
	defer c.Close()

	Convey("Open()", t, func() {
		Convey("When an invalid gateway is given", func() {
			Convey("Should return an error", func() {
				err := c.Open("fake", config)
				So(c.IsOpen(), ShouldBeFalse)
				So(err, ShouldNotBeNil)
			})
		})

		Convey("When an invalid certificate is given", func() {
			Convey("Should return an error", func() {
				err := c.Open(ts.URL[8:], config)
				So(c.IsOpen(), ShouldBeFalse)
				So(err, ShouldNotBeNil)

			})
		})

		Convey("When a valid certificate is given", func() {
			Convey("Should not return an error", func() {
				config.InsecureSkipVerify = true // skip verification for this to accept our cert
				err := c.Open(ts.URL[8:], config)
				So(c.IsOpen(), ShouldBeTrue)
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestApnsConnectionIsOpen(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	config := &tls.Config{
		Certificates: []tls.Certificate{},
		ServerName:   ts.URL,
	}

	c := Connection{}
	defer c.Close()

	Convey("IsOpen()", t, func() {
		Convey("When we didn't open a connection", func() {
			Convey("Should return false", func() {
				So(c.IsOpen(), ShouldBeFalse)
			})
		})

		Convey("When we open a connection", func() {
			apnsConnectionInsecureOpen(&c, ts.URL[8:], config)
			Convey("Should return true", func() {
				So(c.IsOpen(), ShouldBeTrue)
			})
		})

		Convey("When the connection expires", func() {
			c.connectTime = c.connectTime.Add(-(keepAlive + 1) * time.Minute)
			Convey("Should return false", func() {
				So(c.IsOpen(), ShouldBeFalse)
			})
		})

		Convey("When we close the connection", func() {
			c.Close()
			Convey("Should return false", func() {
				So(c.IsOpen(), ShouldBeFalse)
			})

		})

		Convey("When we open a connection and stop the server", func() {
			apnsConnectionInsecureOpen(&c, ts.URL[8:], config)
			ts.Close()

			Convey("Should return true", func() {
				So(c.IsOpen(), ShouldBeTrue)
			})

			Convey("When we force a peek", func() {
				c.writeCount = peekFrequency
				Convey("Should return false", func() {
					So(c.IsOpen(), ShouldBeFalse)
				})

			})
		})
	})
}

func TestApnsConnectionPeek(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	config := &tls.Config{
		Certificates: []tls.Certificate{},
		ServerName:   ts.URL,
	}

	c := Connection{}
	defer c.Close()

	Convey("Peek()", t, func() {
		Convey("When we don't have a connection", func() {
			Convey("Should return an ErrNoConnection error", func() {
				err := c.Peek()
				So(err, ShouldEqual, ErrNoConnection)
			})
		})

		Convey("When we have an open connection", func() {
			apnsConnectionInsecureOpen(&c, ts.URL[8:], config)
			Convey("Should not return an error", func() {
				err := c.Peek()
				So(err, ShouldBeNil)

				Convey("When we close the connection", func() {
					c.Close()
					Convey("Should return an ErrNoConnection error", func() {
						err := c.Peek()
						So(err, ShouldEqual, ErrNoConnection)
					})
				})
			})
		})

		Convey("When we have an open connection to a stopped server", func() {
			apnsConnectionInsecureOpen(&c, ts.URL[8:], config)
			ts.Close()
			Convey("Should return an error", func() {
				err := c.Peek()
				So(err, ShouldNotBeNil)
			})
		})
	})
}

func TestApnsConnectionClose(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	config := &tls.Config{
		Certificates: []tls.Certificate{},
		ServerName:   ts.URL,
	}

	c := Connection{}
	defer c.Close()

	Convey("Close()", t, func() {
		Convey("When we don't have an open connection", func() {
			Convey("Should not return an error", func() {
				err := c.Close()
				So(err, ShouldBeNil)
			})
		})

		Convey("When we have an open connection", func() {
			apnsConnectionInsecureOpen(&c, ts.URL[8:], config)
			c.writeCount = 10
			Convey("Should not return an error and reset our stats", func() {
				err := c.Close()
				So(err, ShouldBeNil)
				So(c.writeCount, ShouldEqual, 0)
				So(c.connectTime, ShouldResemble, time.Time{})
			})
		})
	})
}

func TestApnsConnectionRead(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	config := &tls.Config{
		Certificates: []tls.Certificate{},
		ServerName:   ts.URL,
	}

	c := Connection{}
	defer c.Close()

	buffer := make([]byte, 1, 1)

	Convey("Read()", t, func() {
		Convey("When we don't have an open connection", func() {
			Convey("Should return an ErrNoConnection error", func() {
				_, err := c.Read(buffer)
				So(err, ShouldNotBeNil)
				So(err, ShouldEqual, ErrNoConnection)
			})
		})

		Convey("When we have an open connection", func() {
			apnsConnectionInsecureOpen(&c, ts.URL[8:], config)
			c.connection.SetReadDeadline(time.Now().Add(time.Duration(10) * time.Millisecond))
			Convey("Should return a timeout error", func() {
				_, err := c.Read(buffer)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldContainSubstring, "i/o timeout")
			})
		})

	})
}

func TestApnsConnectionWrite(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	config := &tls.Config{
		Certificates: []tls.Certificate{},
		ServerName:   ts.URL,
	}

	c := Connection{}
	defer c.Close()

	Convey("Write()", t, func() {
		Convey("When we don't have an open connection", func() {
			Convey("Should return an ErrNoConnection error", func() {
				_, err := c.Write([]byte("test"))
				So(err, ShouldNotBeNil)
				So(err, ShouldEqual, ErrNoConnection)
			})
		})

		Convey("When we have an open connection", func() {
			apnsConnectionInsecureOpen(&c, ts.URL[8:], config)
			Convey("Should return the expected bytes written", func() {
				n, err := c.Write([]byte("test"))
				So(err, ShouldBeNil)
				So(n, ShouldEqual, 4)
			})
		})
	})
}

func TestApnsConnectionLocalAddr(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	config := &tls.Config{
		Certificates: []tls.Certificate{},
		ServerName:   ts.URL,
	}

	c := Connection{}
	defer c.Close()

	Convey("LocalAddr()", t, func() {
		Convey("When we don't have an open connection", func() {
			Convey("Should return nil", func() {
				a := c.LocalAddr()
				So(a, ShouldBeNil)
			})
		})

		Convey("When we have an open connection", func() {
			apnsConnectionInsecureOpen(&c, ts.URL[8:], config)
			Convey("Should not return nil", func() {
				a := c.LocalAddr()
				So(a, ShouldNotBeNil)
			})
		})
	})
}

func TestApnsConnectionRemoteAddr(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	config := &tls.Config{
		Certificates: []tls.Certificate{},
		ServerName:   ts.URL,
	}

	c := Connection{}
	defer c.Close()

	Convey("RemoteAddr()", t, func() {
		Convey("When we don't have an open connection", func() {
			Convey("Should return nil", func() {
				a := c.RemoteAddr()
				So(a, ShouldBeNil)
			})
		})
		Convey("When we have an open connection", func() {
			apnsConnectionInsecureOpen(&c, ts.URL[8:], config)
			Convey("Should not return nil", func() {
				a := c.RemoteAddr()
				So(a, ShouldNotBeNil)
			})
		})
	})
}

func TestApnsConnectionSetDeadline(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	config := &tls.Config{
		Certificates: []tls.Certificate{},
		ServerName:   ts.URL,
	}

	c := Connection{}
	defer c.Close()

	Convey("SetDeadline()", t, func() {
		Convey("When we don't have an open connection", func() {
			Convey("Should return an ErrNoConnection error", func() {
				err := c.SetDeadline(time.Now())
				So(err, ShouldNotBeNil)
				So(err, ShouldEqual, ErrNoConnection)
			})
		})

		Convey("When we have an open connection", func() {
			apnsConnectionInsecureOpen(&c, ts.URL[8:], config)
			Convey("Should return nil ", func() {
				err := c.SetDeadline(time.Now())
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestApnsConnectionSetReadDeadline(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	config := &tls.Config{
		Certificates: []tls.Certificate{},
		ServerName:   ts.URL,
	}

	c := Connection{}
	defer c.Close()

	Convey("SetReadDeadline()", t, func() {
		Convey("When we don't have an open connection", func() {
			Convey("Should return an ErrNoConnection error", func() {
				err := c.SetReadDeadline(time.Now())
				So(err, ShouldNotBeNil)
				So(err, ShouldEqual, ErrNoConnection)
			})
		})

		Convey("When we have an open connection", func() {
			apnsConnectionInsecureOpen(&c, ts.URL[8:], config)
			Convey("Should return nil", func() {
				err := c.SetReadDeadline(time.Now())
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestApnsConnectionSetWriteDeadline(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	config := &tls.Config{
		Certificates: []tls.Certificate{},
		ServerName:   ts.URL,
	}

	c := Connection{}
	defer c.Close()

	Convey("SetWriteDeadline()", t, func() {
		Convey("When we don't have an open connection", func() {
			Convey("Should return an ErrNoConnection error", func() {
				err := c.SetWriteDeadline(time.Now())
				So(err, ShouldNotBeNil)
				So(err, ShouldEqual, ErrNoConnection)
			})
		})

		Convey("When we have an open connection", func() {
			apnsConnectionInsecureOpen(&c, ts.URL[8:], config)
			Convey("Should return nil", func() {
				err := c.SetWriteDeadline(time.Now())
				So(err, ShouldBeNil)
			})
		})
	})
}

func TestApnsConnectionConnectionState(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	config := &tls.Config{
		Certificates: []tls.Certificate{},
		ServerName:   ts.URL,
	}

	c := Connection{}
	defer c.Close()

	Convey("ConnectionState()", t, func() {
		Convey("When we don't have an open connection", func() {
			Convey("Should return an empty tls.ConnectionState", func() {
				s := c.ConnectionState()
				So(s, ShouldResemble, tls.ConnectionState{})
			})
		})

		Convey("When we have an open connection", func() {
			Convey("Should return a non-empty tls.ConnectionState ", func() {
				apnsConnectionInsecureOpen(&c, ts.URL[8:], config)
				s := c.ConnectionState()
				So(s, ShouldNotResemble, tls.ConnectionState{})
			})
		})
	})
}
