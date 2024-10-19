// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package streamserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"testing"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"

	"github.com/golang/protobuf/ptypes"
)

type testAddr string

func (a testAddr) Network() string { return string(a) }
func (a testAddr) String() string  { return fmt.Sprintf("test(%s)", a.Network()) }

type testListener struct {
	err   error
	connC chan *testListenerConn
}

func newTestListener() *testListener {
	return &testListener{
		connC: make(chan *testListenerConn),
	}
}

func (l *testListener) Accept() (io.ReadCloser, error) {
	if l.err != nil {
		return nil, l.err
	}
	conn, ok := <-l.connC
	if !ok {
		// Listener has been closed.
		return nil, errors.New("listener closed")
	}
	return conn, nil
}

func (l *testListener) Close() error {
	close(l.connC)
	return nil
}

func (l *testListener) Addr() net.Addr {
	return testAddr("test-listener")
}

func (l *testListener) connect(c *testListenerConn) {
	l.connC <- c
}

type testListenerConn struct {
	bytes.Buffer

	panicOnRead bool
}

func (c *testListenerConn) Read(d []byte) (int, error) {
	if c.panicOnRead {
		panic("panic on read")
	}
	return c.Buffer.Read(d)
}

func (c *testListenerConn) Close() error { return nil }

func TestListenerStreamServer(t *testing.T) {
	t.Parallel()

	ftt.Run(`A stream server using a testing Listener`, t, func(t *ftt.Test) {
		var tl *testListener
		s := &StreamServer{
			log:     logging.Get(context.Background()),
			address: "test",
			gen: func() (listener, error) {
				if tl != nil {
					panic("gen called more than once")
				}
				tl = newTestListener()
				return tl, nil
			},
		}

		f := &streamproto.Flags{}

		t.Run(`Will panic if closed without listening.`, func(t *ftt.Test) {
			assert.Loosely(t, func() { s.Close() }, should.Panic)
		})

		t.Run(`Will fail to Listen if the Listener could not be created.`, func(t *ftt.Test) {
			s.gen = func() (listener, error) {
				return nil, errors.New("test error")
			}
			assert.Loosely(t, s.Listen(), should.NotBeNil)
		})

		t.Run(`Can Listen for connections.`, func(t *ftt.Test) {
			shouldClose := true
			assert.Loosely(t, s.Listen(), should.BeNil)
			defer func() {
				if shouldClose {
					s.Close()
				}
			}()

			tc := &testListenerConn{}

			t.Run(`Can close, and will panic if double-closed.`, func(t *ftt.Test) {
				s.Close()
				shouldClose = false

				assert.Loosely(t, func() { s.Close() }, should.Panic)
			})

			t.Run(`Client with an invalid handshake is rejected.`, func(t *ftt.Test) {
				s.discardC = make(chan *streamClient)
				tc.Write([]byte(`NOT A HANDSHAKE MAGIC`))

				tl.connect(tc)
				assert.Loosely(t, <-s.discardC, should.NotBeNil)
			})

			t.Run(`Client handshake panics are contained and rejected.`, func(t *ftt.Test) {
				s.discardC = make(chan *streamClient)

				tc.panicOnRead = true
				f.WriteHandshake(tc)

				tl.connect(tc)
				assert.Loosely(t, <-s.discardC, should.NotBeNil)
			})

			t.Run(`Can receive stream data.`, func(t *ftt.Test) {
				f.Name = "test"
				f.ContentType = "application/octet-stream"
				f.WriteHandshake(tc)
				content := bytes.Repeat([]byte("THIS IS A TEST STREAM "), 100)
				tc.Write(content)

				// Retrieve the ensuing stream.
				tl.connect(tc)
				stream, props := s.Next()
				assert.Loosely(t, stream, should.NotBeNil)
				defer stream.Close()
				assert.Loosely(t, props, should.NotBeNil)

				// Consume all of the data in the stream.
				recvData, _ := io.ReadAll(stream)
				assert.Loosely(t, recvData, should.Resemble(content))
			})

			t.Run(`Will exit Next if closed.`, func(t *ftt.Test) {
				streamC := make(chan *streamParams)
				defer close(streamC)

				// Get the stream.
				go func() {
					rc, props := s.Next()
					streamC <- &streamParams{rc, props}
				}()

				// Begin a client connection, but no handshake.
				tl.connect(tc)

				// Close the stream server.
				s.Close()
				shouldClose = false

				// Next must exit with nil.
				bundle := <-streamC
				assert.Loosely(t, bundle.rc, should.BeNil)
				assert.Loosely(t, bundle.descriptor, should.BeNil)
			})

			t.Run(`Will refrain from outputting clients whose handshakes finish after the server is closed.`, func(t *ftt.Test) {
				s.discardC = make(chan *streamClient, 1)

				f.Name = "test"
				f.ContentType = "application/octet-stream"
				content := bytes.Repeat([]byte("THIS IS A TEST STREAM "), 100)
				f.WriteHandshake(tc)
				tc.Write(content)

				tl.connect(tc)
				s.Close()
				shouldClose = false

				assert.Loosely(t, <-s.discardC, should.NotBeNil)
			})

		})
	})
}

// testClientServer tests to ensure that a client can create streams with a
// server.
//
// svr must be in listening state when this is called.
func testClientServer(t testing.TB, svr *StreamServer, client *streamclient.Client) {
	t.Helper()
	ctx, _ := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
	data := []byte("ohaithere")

	clientDoneC := make(chan error)
	go func() {
		var err error
		defer func() {
			clientDoneC <- err
		}()

		var stream io.WriteCloser
		if stream, err = client.NewStream(ctx, "foo/bar"); err != nil {
			return
		}
		defer func() {
			if closeErr := stream.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
		}()

		if _, err = stream.Write(data); err != nil {
			return
		}
	}()

	rc, desc := svr.Next()
	defer rc.Close()

	stamp, err := ptypes.TimestampProto(testclock.TestTimeLocal)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	assert.Loosely(t, desc, should.Resemble(&logpb.LogStreamDescriptor{
		Name:        "foo/bar",
		ContentType: "text/plain; charset=utf-8",
		Timestamp:   stamp,
	}), truth.LineContext())

	var buf bytes.Buffer
	_, err = buf.ReadFrom(rc)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	assert.Loosely(t, buf.Bytes(), should.Resemble(data), truth.LineContext())

	assert.Loosely(t, <-clientDoneC, should.BeNil, truth.LineContext())
}
