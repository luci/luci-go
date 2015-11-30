// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package streamserver

import (
	"bytes"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"testing"
	"time"

	"github.com/luci/luci-go/client/logdog/butlerlib/streamproto"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

// Testing interface for OS-level test routine abstraction.
type namedPipeTester interface {
	// Decorator to create and destroy a NamedPipeServer instance.
	withServer(func(i *namedPipeTestInstance)) func()
}

// Base class for a named pipe test instance, bound to a single server.
type namedPipeTestInstance struct {
	S       *namedPipeServer
	connect func() io.WriteCloser
}

type testAddr string

func (a testAddr) Network() string { return string(a) }
func (a testAddr) String() string  { return fmt.Sprintf("test(%s)", a.Network()) }

type testConn struct {
	bytes.Buffer

	readDeadline  time.Time
	writeDeadline time.Time
}

func (c *testConn) Close() error         { return nil }
func (c *testConn) LocalAddr() net.Addr  { return testAddr("local") }
func (c *testConn) RemoteAddr() net.Addr { return testAddr("remote") }

func (c *testConn) SetReadDeadline(t time.Time) error {
	c.readDeadline = t
	return nil
}

func (c *testConn) SetWriteDeadline(t time.Time) error {
	c.writeDeadline = t
	return nil
}

func (c *testConn) SetDeadline(t time.Time) error {
	c.readDeadline = t
	c.writeDeadline = t
	return nil
}

func testNamedPipeServer(t *testing.T, npt namedPipeTester) {
	Convey(`In a testing environment`, t, func() {
		c := context.Background()
		hb := handshakeBuilder{
			magic: streamproto.ProtocolFrameHeaderMagic,
		}

		Convey(`A named pipe server will panic if closed without listening.`, func() {
			s := &namedPipeServer{}
			So(func() { s.Close() }, ShouldPanic)
		})

		Convey(`A listening named pipe server will panic if double-closed.`,
			npt.withServer(func(i *namedPipeTestInstance) {
				i.S.Close()
				So(func() { i.S.Close() }, ShouldPanic)
			}))

		Convey(`A test client connection`, func() {
			tc := testConn{}
			npc := &namedPipeConn{
				Context: c,
				id:      1337,
				conn:    &tc,
			}

			Convey(`Will reject an invalid handshake magic.`, func() {
				hb.magic = []byte(`NOT A HANDSHAKE MAGIC`)
				hb.writeTo(&tc, "", nil)
				So(npc.handle(nil), ShouldBeFalse)
			})

			Convey(`Will reject an invalid handshake.`, func() {
				hb.writeTo(&tc, "CLEARLY NOT JSON", nil)
				So(npc.handle(nil), ShouldBeFalse)
			})
		})

		Convey(`A listening named pipe server`, npt.withServer(func(i *namedPipeTestInstance) {
			// Setup a goroutine to pipe test data through a client connection.
			readerC := make(chan io.Reader)
			defer close(readerC)

			doneC := make(chan struct{})
			go func() {
				defer close(doneC)

				// Create a connection to our server instance.
				w := i.connect()
				if w == nil {
					return
				}
				defer w.Close()

				// Write supplied data to the client connection.
				r := <-readerC
				if r != nil {
					io.Copy(w, r)
				}
			}()

			Convey(`Can receive stream data.`, func() {
				// Write our handshake and data to the stream.
				handshake := `{"name": "test", "contentType": "application/octet-stream"}`
				content := bytes.Repeat([]byte("THIS IS A TEST STREAM "), 100)
				readerC <- hb.reader(handshake, content)

				// Retrieve the ensuing stream.
				stream, props := i.S.Next()
				So(stream, ShouldNotBeNil)
				defer stream.Close()
				So(props, ShouldNotBeNil)

				<-doneC

				// Consume all of the data in the stream.
				recvData, _ := ioutil.ReadAll(stream)
				So(recvData, ShouldResemble, content)
			})

			Convey(`Will exit Next if closed.`, func() {
				type streamBundle struct {
					rc    io.ReadCloser
					props *streamproto.Properties
				}
				streamC := make(chan *streamBundle)
				defer close(streamC)

				// Get the stream.
				go func() {
					rc, props := i.S.Next()
					streamC <- &streamBundle{rc, props}
				}()

				// Close the stream server.
				i.S.Close()

				// Next must exit with nil.
				bundle := <-streamC
				So(bundle.rc, ShouldBeNil)
				So(bundle.props, ShouldBeNil)
			})
		}))
	})
}
