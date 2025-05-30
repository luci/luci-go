// Copyright 2017 The LUCI Authors.
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

// Package localsrv provides helpers for running local TCP servers.
//
// It is used by various machine-local authentication protocols to launch
// local listening servers.
package localsrv

import (
	"context"
	"fmt"
	"net"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// Server runs a local TCP server.
type Server struct {
	l        sync.Mutex
	name     string             // name passed to Start
	listener net.Listener       // to know what to stop in killServe, nil after that
	wg       sync.WaitGroup     // +1 for each request being processed now
	ctx      context.Context    // derived from ctx in Start, never resets to nil after that
	cancel   context.CancelFunc // cancels 'ctx'
	stopped  chan struct{}      // closed when serve() goroutine stops
}

// ServeFunc is called from internal goroutine to run the server loop.
//
// When server stops, the given listener will be closed and the given context
// will be canceled. The wait group is used to wait for pending requests:
// increment it when starting processing a request, and decrement when done.
//
// If ServeFunc returns after the listener is closed, the returned error is
// ignored (it is most likely caused by the closed listener).
type ServeFunc func(c context.Context, l net.Listener, wg *sync.WaitGroup) error

// Start launches background goroutine with the serving loop 'serve'.
//
// Returns the address the listening socket is bound to.
//
// The provided context is used as base context for request handlers and for
// logging. 'name' identifies this server in logs, and 'port' specifies a TCP
// port number to bind to (or 0 to auto-pick one).
//
// The server must be eventually stopped with Stop().
func (s *Server) Start(ctx context.Context, name string, port int, serve ServeFunc) (*net.TCPAddr, error) {
	s.l.Lock()
	defer s.l.Unlock()

	if s.ctx != nil {
		return nil, errors.New("already initialized")
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		return nil, errors.Fmt("failed to create listening socket: %w", err)
	}

	s.name = name
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.listener = ln

	// Start serving in background.
	s.stopped = make(chan struct{})
	go func() {
		defer close(s.stopped)
		if err := s.serve(serve); err != nil {
			logging.WithError(err).Errorf(s.ctx, "Unexpected error in the server loop of %q", s.name)
		}
	}()

	return ln.Addr().(*net.TCPAddr), nil
}

// Stop closes the listening socket, notifies pending requests to abort and
// stops the internal serving goroutine.
//
// Safe to call multiple times. Once stopped, the server cannot be started again
// (make a new instance of Server instead).
//
// Uses the given context for the deadline when waiting for the serving loop
// to stop.
func (s *Server) Stop(ctx context.Context) error {
	// Close the socket. It notifies the serving loop to stop.
	if err := s.killServe(); err != nil {
		return err
	}

	// Wait for the serving loop to actually stop.
	select {
	case <-s.stopped:
		logging.Debugf(ctx, "The local server %q has stopped", s.name)
	case <-clock.After(ctx, 10*time.Second):
		logging.Errorf(ctx, "Giving up waiting for the local server %q to stop", s.name)
	}

	return nil
}

// serve runs the serving loop.
//
// It unblocks once killServe is called and all pending requests are served.
//
// Returns nil if serving was stopped by killServe or non-nil if it failed for
// some other reason.
func (s *Server) serve(cb ServeFunc) error {
	s.l.Lock()
	if s.listener == nil {
		s.l.Unlock()
		return errors.New("already closed")
	}
	listener := s.listener // accessed outside the lock
	ctx := s.ctx
	s.l.Unlock()

	err := cb(ctx, listener, &s.wg) // blocks until killServe() is called
	s.wg.Wait()                     // waits for all pending requests

	// If it was a planned shutdown with killServe(), ignore the error. It says
	// that the listening socket was closed.
	s.l.Lock()
	if s.listener == nil {
		err = nil
	}
	s.l.Unlock()

	if err != nil {
		return errors.Fmt("error in the serving loop: %w", err)
	}
	return nil
}

// killServe notifies the serving goroutine to stop (if it is running).
func (s *Server) killServe() error {
	s.l.Lock()
	defer s.l.Unlock()

	if s.ctx == nil {
		return errors.New("not initialized")
	}

	// Stop accepting requests, unblocks serve(). Do it only once.
	if s.listener != nil {
		logging.Debugf(s.ctx, "Stopping the local server %q...", s.name)
		if err := s.listener.Close(); err != nil {
			logging.WithError(err).Errorf(s.ctx, "Failed to close the listening socket of %q", s.name)
		}
		s.listener = nil
	}
	s.cancel() // notify all running handlers to stop

	return nil
}
