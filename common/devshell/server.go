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

package devshell

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/iotools"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/runtime/paniccatcher"
)

// EnvKey is the name of the environment variable which contains the Devshell
// server port number which is picked up by Devshell clients.
const EnvKey = "DEVSHELL_CLIENT_PORT"

// Server runs a Devshell server.
type Server struct {
	// Source is used to obtain OAuth2 tokens.
	Source oauth2.TokenSource
	// Email is the email associated with the token.
	Email string

	// Port is a local TCP port to bind to or 0 to allow the OS to pick one.
	Port int

	l        sync.Mutex
	listener net.Listener       // to know what to stop in killServe, nil after that
	wg       sync.WaitGroup     // +1 for each request being processed now
	ctx      context.Context    // derived from ctx in Start, never resets to nil after that
	cancel   context.CancelFunc // cancels 'ctx'
	stopped  chan struct{}      // closed when serve() goroutine stops
}

// Start launches background goroutine with the serving loop.
//
// The provided context is used as base context for request handlers and for
// logging.
//
// The server must be eventually stopped with Stop().
func (s *Server) Start(ctx context.Context) (*net.TCPAddr, error) {
	s.l.Lock()
	defer s.l.Unlock()

	if s.ctx != nil {
		return nil, fmt.Errorf("already initialized")
	}

	// Bind to the port.
	addr, err := s.initializeLocked(ctx)
	if err != nil {
		return nil, err
	}

	// Start serving in background.
	s.stopped = make(chan struct{})
	go func() {
		defer close(s.stopped)
		if err := s.serve(); err != nil {
			logging.WithError(err).Errorf(s.ctx, "Unexpected error in the DevShell server loop")
		}
	}()

	return addr, nil
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

	if s.stopped == nil {
		return nil // the serving loop is not running
	}

	// Wait for the serving loop to actually stop.
	select {
	case <-s.stopped:
		logging.Debugf(ctx, "The DevShell server stopped")
	case <-clock.After(ctx, 10*time.Second):
		logging.Errorf(ctx, "Giving up waiting for the DevShell server to stop")
	}

	return nil
}

// initializeLocked binds the server to a local port and prepares it for
// serving.
//
// Called under the lock.
func (s *Server) initializeLocked(ctx context.Context) (*net.TCPAddr, error) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.Port))
	if err != nil {
		return nil, err
	}

	s.ctx, s.cancel = context.WithCancel(ctx)
	s.listener = ln

	return ln.Addr().(*net.TCPAddr), nil
}

// serve runs the serving loop.
//
// It unblocks once killServe is called and all pending requests are served.
//
// Returns nil if serving was stopped by killServe or non-nil if it failed for
// some other reason.
func (s *Server) serve() (err error) {
	s.l.Lock()
	if s.listener == nil {
		s.l.Unlock()
		return fmt.Errorf("already closed")
	}
	listener := s.listener // accessed outside the lock
	s.l.Unlock()

	for {
		var conn net.Conn
		conn, err = listener.Accept()
		if err != nil {
			break
		}

		client := &client{
			conn:   &iotools.DeadlineReader{conn, 0},
			source: s.Source,
			email:  s.Email,
			ctx:    s.ctx,
		}

		s.wg.Add(1)
		go func() {
			defer s.wg.Done()

			paniccatcher.Do(func() {
				if err := client.handle(); err != nil {
					logging.Fields{
						logging.ErrorKey: err,
					}.Errorf(client.ctx, "failed to handle client request")
				}
			}, func(p *paniccatcher.Panic) {
				logging.Fields{
					"panicReason": p.Reason,
				}.Errorf(client.ctx, "panic during client handshake:\n%s", p.Stack)
			})
		}()
	}

	s.wg.Wait() // Wait for all pending requests to finish

	// If it was a planned shutdown with killServe(), ignore the error. It says
	// that the listening socket was closed.
	s.l.Lock()
	if s.listener == nil {
		err = nil
	}
	s.l.Unlock()

	return
}

// killServe notifies the serving goroutine to stop (if it is running).
func (s *Server) killServe() error {
	s.l.Lock()
	defer s.l.Unlock()
	if s.ctx == nil {
		return fmt.Errorf("not initialized")
	}
	// Stop accepting requests, unblocks serve(). Do it only once.
	if s.listener != nil {
		logging.Debugf(s.ctx, "Stopping the DevShell server...")
		if err := s.listener.Close(); err != nil {
			logging.WithError(err).Errorf(s.ctx, "failed to close the listening socket")
		}
		s.listener = nil
	}
	s.cancel() // notify all running handlers to stop
	return nil
}

type client struct {
	conn net.Conn

	source oauth2.TokenSource
	email  string

	ctx context.Context
}

func (c *client) handle() error {
	defer c.conn.Close()

	if _, err := c.readRequest(); err != nil {
		if err := c.sendResponse([]interface{}{err.Error()}); err != nil {
			return fmt.Errorf("failed to send error: %v", err)
		}
		return nil
	}

	// Get the token.
	t, err := c.source.Token()
	if err != nil {
		if err := c.sendResponse([]interface{}{"cannot get access token"}); err != nil {
			return fmt.Errorf("failed to send error: %v", err)
		}
		return err
	}

	// Expiration is in seconds from now so compute the correct format.
	expiry := int(t.Expiry.Sub(clock.Now(c.ctx)).Seconds())

	return c.sendResponse([]interface{}{c.email, nil, t.AccessToken, expiry})
}

func (c *client) readRequest() ([]interface{}, error) {
	header := make([]byte, 6)
	if _, err := c.conn.Read(header); err != nil {
		return nil, fmt.Errorf("failed to read the header: %v", err)
	}

	// The first six bytes contain the length separated by a newline.
	str := strings.SplitN(string(header), "\n", 2)
	if len(str) != 2 {
		return nil, fmt.Errorf("no newline in the first 6 bytes")
	}

	l, err := strconv.Atoi(str[0])
	if err != nil {
		return nil, fmt.Errorf("length is not a number: %v", err)
	}

	data := make([]byte, l)
	copy(data, str[1][:])

	// Read the rest of the message.
	if l > len(str[1]) {
		if _, err := c.conn.Read(data[len(str[1]):]); err != nil {
			return nil, fmt.Errorf("failed to receive request: %v", err)
		}
	}

	// Parse the message to ensure it's a correct JSON.
	request := []interface{}{}
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, fmt.Errorf("failed to deserialize from JSON: %v", err)
	}

	return request, nil
}

func (c *client) sendResponse(response []interface{}) error {
	// Encode the response as JSON array (aka JsPbLite format).
	payload, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to serialize to JSON: %v", err)
	}

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("%d\n", len(payload)))
	buf.Write(payload)
	if _, err := c.conn.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("failed to send response: %v", err)
	}
	return nil
}
