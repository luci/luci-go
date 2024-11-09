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
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/runtime/paniccatcher"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butlerlib/streamproto"
)

type listener interface {
	io.Closer
	Accept() (io.ReadCloser, error)
	Addr() net.Addr
}

func mkListener(l net.Listener) listener {
	return &accepterImpl{l}
}

type accepterImpl struct {
	net.Listener
}

func (a *accepterImpl) Accept() (io.ReadCloser, error) {
	conn, err := a.Listener.Accept()
	if err != nil {
		return nil, err
	}
	return conn.(io.ReadCloser), nil
}

// streamParams are parameters representing a negotiated stream ready to
// deliver.
type streamParams struct {
	// The stream's ReadCloser connection.
	rc io.ReadCloser
	// Negotiated stream descriptor.
	descriptor *logpb.LogStreamDescriptor
}

// StreamServer implements a Logdog Butler listener.
//
// This holds a named pipe listener (either a unix domain socket or windows
// Named Pipe, depending on the platform). Local processes on the machine may
// connect to this named pipe to stream data to Logdog.
type StreamServer struct {
	log logging.Logger

	// address is the string returned by the Address method.
	address string

	// gen is a generator function that is called to produce the stream server's
	// Listener. On success, it returns the instantiated Listener, which will be
	// closed when the stream server is closed.
	gen   func() (listener, error)
	l     listener
	laddr string

	streamParamsC   chan *streamParams
	closedC         chan struct{}
	acceptFinishedC chan struct{}

	// closed is an atomically-protected integer. If its value is 1, the stream
	// server has been closed.
	closed int32

	// discardC is a testing channel. If not nil, failed client connections will
	// be written here.
	discardC chan *streamClient
}

func (s *StreamServer) String() string {
	return fmt.Sprintf("%T(%s)", s, s.laddr)
}

// Address returns a string that can be used by the "streamclient" package to
// return a client for this StreamServer.
//
// Full package is:
// go.chromium.org/luci/logdog/butlerlib/streamclient
func (s *StreamServer) Address() string {
	return s.address
}

// Listen performs initial connection and setup and runs a goroutine to listen
// for new connections.
func (s *StreamServer) Listen() error {
	// Create a listener (OS-specific).
	var err error
	s.l, err = s.gen()
	if err != nil {
		return err
	}

	s.laddr = s.l.Addr().String()
	s.streamParamsC = make(chan *streamParams)
	s.closedC = make(chan struct{})
	s.acceptFinishedC = make(chan struct{})

	// Poll the Listener for new connections in a separate goroutine. This will
	// terminate when the server is Close()d.
	go s.serve()
	return nil
}

// Next blocks, returning a new Stream when one is available. If the stream
// server has closed, this will return nil.
func (s *StreamServer) Next() (io.ReadCloser, *logpb.LogStreamDescriptor) {
	if streamParams, ok := <-s.streamParamsC; ok {
		return streamParams.rc, streamParams.descriptor
	}
	return nil, nil
}

// Close terminates the stream server, cleaning up resources.
func (s *StreamServer) Close() {
	if s.l == nil {
		panic("server is not currently serving")
	}

	// Mark that we've been closed.
	atomic.StoreInt32(&s.closed, 1)

	// Close our Listener. This will cause our 'Accept' goroutine to terminate.
	s.l.Close()
	close(s.closedC)
	<-s.acceptFinishedC

	// Close our streamParamsC to signal that we're closed. Any blocking Next will
	// drain the channel, then return with nil.
	close(s.streamParamsC)
	s.l = nil
}

// Continuously pulls connections from the supplied Listener and returns them as
// connections to streamParamsC for consumption by Next().
func (s *StreamServer) serve() {
	defer close(s.acceptFinishedC)

	nextID := 0
	clientWG := sync.WaitGroup{}
	for {
		s.log.Debugf("Beginning Accept() loop cycle.")
		conn, err := s.l.Accept()
		if err != nil {
			if atomic.LoadInt32(&s.closed) != 0 {
				s.log.Debugf("Error during Accept() encountered (closed): %s", err)
			} else {
				s.log.Errorf("Error during Accept(): %s", err)
			}
			break
		}

		// Spawn a goroutine to handle this connection. This goroutine will take
		// ownership of the connection, closing it as appropriate.
		client := &streamClient{
			log:     s.log,
			closedC: s.closedC,
			id:      nextID,
			conn:    conn,
		}

		clientWG.Add(1)
		go func() {
			defer clientWG.Done()

			var params *streamParams
			paniccatcher.Do(func() {
				var err error
				params, err = client.handle()
				if err != nil {
					s.log.Errorf("Failed to negotitate stream client: %s", err)
					params = nil
				}
			}, func(p *paniccatcher.Panic) {
				s.log.Errorf("Panic during client handshake:\n%s\n%s", p.Reason, p.Stack)
				params = nil
			})

			// Did the negotiation fail?
			if params != nil {
				// Punt the client to our Next function. If we close while waiting, close
				// the Client.
				select {
				case s.streamParamsC <- params:
					break

				case <-s.closedC:
					s.log.Warningf("Closed with client in hand. Cleaning up client.")
					params.rc.Close()
					params = nil
				}
			}

			// (Testing) write failed connections to discardC if configured.
			if params == nil && s.discardC != nil {
				s.discardC <- client
			}
		}()
		nextID++
	}

	// Wait for client connections to finish.
	clientWG.Wait()
	s.log.Infof("Exiting serve loop. Served %d connections", nextID)
}

// streamClient manages a single client connection.
type streamClient struct {
	log logging.Logger

	closedC chan struct{} // Signal channel to indicate that the server has closed.
	id      int           // Client ID, used for debugging correlation.
	conn    io.ReadCloser // The underlying client connection.

	// decoupleMu is used to ensure that decoupleConn is called at most one time.
	decoupleMu sync.Mutex
}

// handle initializes the supplied connection. On success, it will prepare
// the connection as a Butler stream and pass its parameters through the
// supplied channel.
//
// On failure, the connection will be closed.
//
// This method returns the negotiated stream parameters, or nil if the
// negotiation failed and the client was closed.
func (c *streamClient) handle() (*streamParams, error) {
	c.log.Infof("Received new connection.")

	// Close the connection as a failsafe. If we have already decoupled it, this
	// will end up being a no-op.
	defer c.closeConn()

	// Perform our handshake. We pass the connection explicitly into this method
	// because it can get decoupled during operation.
	p, err := c.handshake()
	if err != nil {
		return nil, err
	}

	// Successful handshake. Forward our properties.
	return &streamParams{c.decoupleConn(), p}, nil
}

// handshake handles the handshaking and registration of a single connection. If
// the connection successfully handshakes, it will be registered as a stream and
// supplied to the local streamParamsC; otherwise, it will be closed.
//
// The client connection opens with a handshake protocol. Once complete, the
// connection itself becomes the stream.
func (c *streamClient) handshake() (*logpb.LogStreamDescriptor, error) {
	c.log.Infof("Beginning handshake.")
	flags := &streamproto.Flags{}
	if err := flags.FromHandshake(c.conn); err != nil {
		return nil, err
	}
	return flags.Descriptor(), nil
}

// Closes the underlying connection.
func (c *streamClient) closeConn() {
	conn := c.decoupleConn()
	if conn != nil {
		if err := conn.Close(); err != nil {
			c.log.Warningf("Error on connection close: %s", err)
		}
	}
}

// Decouples the active connection, returning it and setting the connection to
// nil.
func (c *streamClient) decoupleConn() (conn io.ReadCloser) {
	c.decoupleMu.Lock()
	defer c.decoupleMu.Unlock()

	conn, c.conn = c.conn, nil
	return
}

// New creates a new StreamServer.
//
// This has a platform-specific implementation.
//
// On Mac/Linux, this makes a Unix Domain Socket based server, and `path` is
// required to be the absolute path to a filesystem location which is suitable
// for a domain socket. The location will be removed prior to, and after, use.
//
// On Windows, this makes a Named Pipe based server. `path` must be a valid UNC
// path component, and will be used to listen on the named pipe
// "\\.\$path.$PID.$UNIQUE" where $PID is the process id of the current process
// and $UNIQUE is a monotonically increasing value within this process' memory.
//
// `path` may be empty and a unique name will be chosen for you.
func New(ctx context.Context, path string) (*StreamServer, error) {
	return newStreamServer(ctx, path)
}
