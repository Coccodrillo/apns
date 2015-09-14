package apns

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestApnsNewConnectionPool(t *testing.T) {
	Convey("NewConnectionPool()", t, func() {
		credentials := [][]byte{[]byte("certificate")}
		gateway := "mygateway"
		certificate := tls.Certificate{Certificate: credentials}

		Convey("Should store our gateway and certificate", func() {
			p := NewConnectionPool(1, gateway, certificate)
			So(p, ShouldNotBeNil)
			So(p.size, ShouldEqual, 1)
			So(p.gateway, ShouldEqual, gateway)
			So(p.config.Certificates[0], ShouldResemble, certificate)
			So(p.config.Certificates[0].Certificate, ShouldResemble, credentials)
		})

		Convey("Should support multiple connections", func() {
			p := NewConnectionPool(5, gateway, certificate)
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
		Convey("Fake server should return an error", func() {
			p := NewConnectionPool(1, "fake", certificate)
			c, err := p.GetConnection()
			So(c, ShouldNotBeNil)
			So(c.IsOpen(), ShouldBeFalse)
			So(err, ShouldNotBeNil)
			So(p.position, ShouldEqual, 1)
		})

		Convey("Pool of one, should return the same connection", func() {
			p := NewConnectionPool(1, ts.URL[8:], ts.TLS.Certificates[0])
			// need to skip verification for this to accept our cert
			p.config.InsecureSkipVerify = true
			c, err := p.GetConnection()
			So(c, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(p.position, ShouldEqual, 1)

			unique := c.ConnectionState().TLSUnique

			Convey("Position should stay the same and connection should be re-used", func() {
				c, err := p.GetConnection()
				So(c, ShouldNotBeNil)
				So(err, ShouldBeNil)
				So(p.position, ShouldEqual, 1)
				So(c.ConnectionState().TLSUnique, ShouldResemble, unique)
			})
		})

		Convey("Pool of two, should loop over connections", func() {
			p := NewConnectionPool(2, ts.URL[8:], ts.TLS.Certificates[0])
			So(p.position, ShouldEqual, 0)
			// need to skip verification for this to accept our cert
			p.config.InsecureSkipVerify = true
			c, err := p.GetConnection()
			So(c, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(p.position, ShouldEqual, 1)

			unique := c.ConnectionState().TLSUnique

			Convey("Position should increase and get a new connection", func() {
				c, err := p.GetConnection()
				So(c, ShouldNotBeNil)
				So(err, ShouldBeNil)
				So(p.position, ShouldEqual, 2)
				So(c.ConnectionState().TLSUnique, ShouldNotResemble, unique)

				unique2 := c.ConnectionState().TLSUnique

				Convey("Position should go back to 1 and should re-use connection", func() {
					c, err := p.GetConnection()
					So(c, ShouldNotBeNil)
					So(err, ShouldBeNil)
					So(p.position, ShouldEqual, 1)
					So(c.ConnectionState().TLSUnique, ShouldResemble, unique)

					Convey("Position should go back to 2 and should re-use connection", func() {
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
}

func TestApnsConnectionPoolWrite(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	poolSize := 10
	Convey("Write()", t, func() {
		p := NewConnectionPool(poolSize, ts.URL[8:], ts.TLS.Certificates[0])
		p.config.InsecureSkipVerify = true // need to skip verification for this to accept our cert

		Convey("Bytes written should match", func() {
			c, bytesWritten, err := p.Write([]byte("12345"))
			So(err, ShouldBeNil)
			So(bytesWritten, ShouldEqual, 5)
			unique := c.ConnectionState().TLSUnique
			Convey("Next write should be on different connection", func() {
				c, bytesWritten, err := p.Write([]byte("test"))
				So(err, ShouldBeNil)
				So(bytesWritten, ShouldEqual, 4)
				So(c.ConnectionState().TLSUnique, ShouldNotResemble, unique)
			})

			Convey("If we close all of the connections, should reconnect", func() {
				p.Close()

				_, bytesWritten, err := p.Write([]byte("6789"))
				So(err, ShouldBeNil)
				So(bytesWritten, ShouldEqual, 4)
			})

		})

		/*
			Convey("Should return an error if we write 0 bytes", func() {
				//p.config.InsecureSkipVerify = false // set to false so the handshake fails
				_, bytesWritten, err := p.Write([]byte("hello"))
				So(bytesWritten, ShouldEqual, 0)
				So(err, ShouldNotBeNil)
				So(err.Error(), ShouldEqual, "foobar")
			})
		*/

		Convey("Should give an error if the server is closed", func() {
			ts.Close()

			// force a peek
			for i := 0; i < poolSize; i++ {
				p.connections[i].writeCount = peekFrequency

			}

			_, _, err := p.Write([]byte("test"))
			So(err, ShouldNotBeNil)

		})
	})
}

func TestApnsConnectionPoolClose(t *testing.T) {
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	poolSize := 10
	Convey("Close()", t, func() {
		p := NewConnectionPool(poolSize, ts.URL[8:], ts.TLS.Certificates[0])
		p.config.InsecureSkipVerify = true // need to skip verification for this to accept our cert

		Convey("Should not return an error if no open connections", func() {
			err := p.Close()
			So(err, ShouldBeNil)
		})

		Convey("Should not return an error if there are open connections", func() {
			c, err := p.GetConnection()
			So(err, ShouldBeNil)
			So(c.IsOpen(), ShouldBeTrue)

			err = p.Close()
			So(err, ShouldBeNil)
			So(c.IsOpen(), ShouldBeFalse)
		})
	})
}
