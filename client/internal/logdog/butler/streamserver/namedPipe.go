// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package streamserver

import (
	"io"
	"net"
	"sync"

	"github.com/luci/luci-go/client/logdog/butlerlib/streamproto"
	"github.com/luci/luci-go/common/iotools"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// An client ID, used for logging only
type clientID int

// namedPipeStreamParams are parameters representing a negotiated stream ready
// to deliver.
type namedPipeStreamParams struct {
	// The stream's ReadCloser connection.
	rc io.ReadCloser
	// Negotiated stream properties.
	properties *streamproto.Properties
}

type namedPipeGenFunc func() (net.Listener, error)

// Base class for OS-specific stream server implementations.
type namedPipeServer struct {
	context.Context

	gen             func() (net.Listener, error)
	streamParamsC   chan *namedPipeStreamParams
	closedC         chan struct{}
	acceptFinishedC chan struct{}
	l               net.Listener // The Listener to use for client connections.
}

// Initializes the base structure members.
func createNamedPipeServer(ctx context.Context, gen namedPipeGenFunc) *namedPipeServer {
	return &namedPipeServer{
		Context:         log.SetFilter(ctx, "streamServer"),
		gen:             gen,
		streamParamsC:   make(chan *namedPipeStreamParams),
		closedC:         make(chan struct{}),
		acceptFinishedC: make(chan struct{}),
	}
}

// Implements StreamServer.Connect
func (s *namedPipeServer) Listen() error {
	// Create a listener (OS-specific).
	var err error
	s.l, err = s.gen()
	if err != nil {
		return err
	}

	// Poll the Listener for new connections in a separate goroutine. This will
	// terminate when the server is Close()d.
	go s.serve()
	return nil
}

func (s *namedPipeServer) Next() (io.ReadCloser, *streamproto.Properties) {
	if streamParams, ok := <-s.streamParamsC; ok {
		return streamParams.rc, streamParams.properties
	}
	return nil, nil
}

// Implements StreamServer.Close
func (s *namedPipeServer) Close() {
	if s.l == nil {
		panic("server is not currently serving")
	}

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
func (s *namedPipeServer) serve() {
	defer close(s.acceptFinishedC)

	nextID := clientID(0)
	clientWG := sync.WaitGroup{}
	for {
		log.Debugf(s, "Beginning Accept() loop cycle.")
		conn, err := s.l.Accept()
		if err != nil {
			log.Fields{
				log.ErrorKey: err,
			}.Errorf(s, "Error during Accept().")
			break
		}

		// Spawn a goroutine to handle this connection. This goroutine will take
		// ownership of the connection, closing it as appropriate.
		npConn := &namedPipeConn{
			closedC: s.closedC,
			id:      nextID,
			conn:    &iotools.DeadlineReader{conn, 0},
		}
		npConn.Context = log.SetFields(s, log.Fields{
			"id":     npConn.id,
			"local":  npConn.conn.LocalAddr(),
			"remote": npConn.conn.RemoteAddr(),
		})

		clientWG.Add(1)
		go func() {
			defer clientWG.Done()

			defer func() {
				if r := recover(); r != nil {
					log.Fields{
						log.ErrorKey: r,
					}.Errorf(npConn, "Failed to negotitate stream client.")
				}
			}()

			npConn.handle(s.streamParamsC)
		}()
		nextID++
	}

	// Wait for client connections to finish.
	clientWG.Wait()
	log.Fields{
		"streamServer":     s,
		"totalConnections": nextID,
	}.Infof(s, "Exiting serve loop.")
}

//
// namedPipeConn
//

// Manages a single named pipe connection.
type namedPipeConn struct {
	context.Context

	closedC chan struct{} // Signal channel to indicate that the server has closed.
	id      clientID      // Client ID, used for debugging correlation.
	conn    net.Conn      // The underlying client connection.

	// decoupleMu is used to ensure that decoupleConn is called at most one time.
	decoupleMu sync.Mutex
}

// handle initializes the supplied connection. On success, it will prepare
// the connection as a Butler stream and pass its parameters through the
// supplied channel.
//
// On failure, the connection will be closed.
//
// This method returns true if the handshake completed and the stream was
// passed onto the server, and false otherwise.
func (c *namedPipeConn) handle(paramsC chan *namedPipeStreamParams) bool {
	log.Infof(c, "Received new connection.")

	// Close the connection as a failsafe. If we have already decoupled it, this
	// will end up being a no-op.
	defer c.closeConn()

	// Perform our handshake. We pass the connection explicitly into this method
	// because it can get decoupled during operation.
	p, err := handshake(c, c.conn)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(c, "Failed to perform handshake.")
		return false
	}

	// Successful handshake. Forward our properties.
	paramsC <- &namedPipeStreamParams{c.decoupleConn(), p}
	return true
}

// handshake handles the handshaking and registration of a single connection. If
// the connection successfully handshakes, it will be registered as a stream and
// supplied to the local streamParamsC; otherwise, it will be closed.
//
// The client connection opens with a handshake protocol. Once complete, the
// connection itself becomes the stream.
func handshake(ctx context.Context, conn net.Conn) (*streamproto.Properties, error) {
	log.Infof(ctx, "Beginning handshake.")
	hs := handshakeProtocol{}
	return hs.Handshake(ctx, conn)
}

// Closes the underlying connection.
func (c *namedPipeConn) closeConn() {
	conn := c.decoupleConn()
	if conn != nil {
		if err := conn.Close(); err != nil {
			log.Fields{
				log.ErrorKey: err,
			}.Warningf(c, "Error on connection close.")
		}
	}
}

// Decouples the active connection, returning it and setting the connection to
// nil.
func (c *namedPipeConn) decoupleConn() (conn net.Conn) {
	c.decoupleMu.Lock()
	defer c.decoupleMu.Unlock()

	conn, c.conn = c.conn, nil
	return
}
