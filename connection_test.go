package apns

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

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
		Convey("Fake server should return an error", func() {
			err := c.Open("fake", config)
			So(c.IsOpen(), ShouldBeFalse)
			So(err, ShouldNotBeNil)
		})

		Convey("Server with bad certificate should return an error", func() {
			err := c.Open(ts.URL[8:], config)
			So(c.IsOpen(), ShouldBeFalse)
			So(err, ShouldNotBeNil)

		})

		Convey("Server with good certificate should not return an error", func() {
			// need to skip verification for this to accept our cert
			config.InsecureSkipVerify = true
			err := c.Open(ts.URL[8:], config)
			So(c.IsOpen(), ShouldBeTrue)
			So(err, ShouldBeNil)
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

	Convey("Open()", t, func() {
		Convey("Should be false if we didn't open a connection", func() {
			So(c.IsOpen(), ShouldBeFalse)
		})

		Convey("Should be true if we open a connection", func() {
			// need to skip verification for this to accept our cert
			config.InsecureSkipVerify = true
			c.Open(ts.URL[8:], config)
			So(c.IsOpen(), ShouldBeTrue)
		})

		Convey("Should be false if the connection expires", func() {
			c.connectTime = c.connectTime.Add(-(keepAlive + 1) * time.Minute)
			So(c.IsOpen(), ShouldBeFalse)
		})

		Convey("Should be false if we close the connection", func() {
			c.Close()
			So(c.IsOpen(), ShouldBeFalse)
		})

		Convey("Open another connection and stop the server", func() {
			// need to skip verification for this to accept our cert
			config.InsecureSkipVerify = true
			c.Open(ts.URL[8:], config)
			So(c.IsOpen(), ShouldBeTrue)

			ts.Close()

			Convey("Should still be considered open", func() {
				So(c.IsOpen(), ShouldBeTrue)
			})

			Convey("If we force a peek, should not be open", func() {
				c.writeCount = peekFrequency
				So(c.IsOpen(), ShouldBeFalse)
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
		Convey("Should return an error if we don't have a connection", func() {
			err := c.Peek()
			So(err, ShouldNotBeNil)
		})

		Convey("Should not return an error on an open connection", func() {
			// need to skip verification for this to accept our cert
			config.InsecureSkipVerify = true
			c.Open(ts.URL[8:], config)
			err := c.Peek()
			So(err, ShouldBeNil)

			Convey("Should return an error if we close the connection", func() {
				c.Close()
				err := c.Peek()
				So(err, ShouldNotBeNil)
			})
		})

		Convey("Should return an error if we peek on an open connection with a stopped server", func() {
			// need to skip verification for this to accept our cert
			config.InsecureSkipVerify = true
			c.Open(ts.URL[8:], config)

			ts.Close()

			err := c.Peek()
			So(err, ShouldNotBeNil)

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
		Convey("Should not return an error if we don't have a connection", func() {
			err := c.Close()
			So(err, ShouldBeNil)
		})

		Convey("Should not return an error with an open connection, and reset our stats", func() {
			// need to skip verification for this to accept our cert
			config.InsecureSkipVerify = true
			c.Open(ts.URL[8:], config)
			c.writeCount = 10
			err := c.Close()
			So(err, ShouldBeNil)
			So(c.writeCount, ShouldEqual, 0)
			So(c.connectTime, ShouldResemble, time.Time{})
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
		Convey("Should return an error if we don't have a connection", func() {
			_, err := c.Read(buffer)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "no connection")
		})

		Convey("Should timeout on an open connection", func() {
			// need to skip verification for this to accept our cert
			config.InsecureSkipVerify = true
			c.Open(ts.URL[8:], config)
			c.connection.SetReadDeadline(time.Now().Add(time.Duration(10) * time.Millisecond))
			_, err := c.Read(buffer)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "i/o timeout")
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
		Convey("Should return an error if we don't have a connection", func() {
			_, err := c.Write([]byte("test"))
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "no connection")
		})

		Convey("Should write the expected bytes on an open connection", func() {
			// need to skip verification for this to accept our cert
			config.InsecureSkipVerify = true
			c.Open(ts.URL[8:], config)
			n, err := c.Write([]byte("test"))
			So(err, ShouldBeNil)
			So(n, ShouldEqual, 4)
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
		Convey("Should return nil if we don't have a connection", func() {
			a := c.LocalAddr()
			So(a, ShouldBeNil)
		})

		Convey("Should not return nil on an open connection", func() {
			// need to skip verification for this to accept our cert
			config.InsecureSkipVerify = true
			c.Open(ts.URL[8:], config)
			a := c.LocalAddr()
			So(a, ShouldNotBeNil)
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
		Convey("Should return nil if we don't have a connection", func() {
			a := c.RemoteAddr()
			So(a, ShouldBeNil)
		})

		Convey("Should not return nil on an open connection", func() {
			// need to skip verification for this to accept our cert
			config.InsecureSkipVerify = true
			c.Open(ts.URL[8:], config)
			a := c.RemoteAddr()
			So(a, ShouldNotBeNil)
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
		Convey("Should return an error if we don't have a connection", func() {
			err := c.SetDeadline(time.Now())
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "no connection")
		})

		Convey("Should not return an error on an open connection", func() {
			// need to skip verification for this to accept our cert
			config.InsecureSkipVerify = true
			c.Open(ts.URL[8:], config)
			err := c.SetDeadline(time.Now())
			So(err, ShouldBeNil)
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
		Convey("Should return an error if we don't have a connection", func() {
			err := c.SetReadDeadline(time.Now())
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "no connection")
		})

		Convey("Should not return an error on an open connection", func() {
			// need to skip verification for this to accept our cert
			config.InsecureSkipVerify = true
			c.Open(ts.URL[8:], config)
			err := c.SetReadDeadline(time.Now())
			So(err, ShouldBeNil)
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
		Convey("Should return an error if we don't have a connection", func() {
			err := c.SetWriteDeadline(time.Now())
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldEqual, "no connection")
		})

		Convey("Should not return an error on an open connection", func() {
			// need to skip verification for this to accept our cert
			config.InsecureSkipVerify = true
			c.Open(ts.URL[8:], config)
			err := c.SetWriteDeadline(time.Now())
			So(err, ShouldBeNil)
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
		Convey("Should return an empty tls.ConnectionState if we don't have a connection", func() {
			s := c.ConnectionState()
			So(s, ShouldResemble, tls.ConnectionState{})
		})

		Convey("Should not return an empty tls.ConnectionState on an open connection", func() {
			// need to skip verification for this to accept our cert
			config.InsecureSkipVerify = true
			c.Open(ts.URL[8:], config)
			s := c.ConnectionState()
			So(s, ShouldNotResemble, tls.ConnectionState{})
		})
	})
}
