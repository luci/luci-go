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
	//"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/api/oauth2/v2"

	"go.chromium.org/luci/common/auth"
	"go.chromium.org/luci/common/iotools"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/runtime/paniccatcher"
)

// Server runs a Devshell server.
type Server struct {
	// Default token lifetime.
	Lifetime time.Duration

	// Options are the options to set in the authenticator.
	Options auth.Options

	// Port is a local TCP port to bind to or 0 to allow the OS to pick one.
	Port int

	l        sync.Mutex
	auth     *auth.Authenticator
	listener net.Listener       // to know what to stop in Close, nil after Close
	wg       sync.WaitGroup     // +1 for each request being processed now
	ctx      context.Context    // derived from ctx in Initialize
	cancel   context.CancelFunc // cancels 'ctx'
}

// DevshellContext is the Devshell context.
type DevshellContext struct {
	Port int
}

// Initialize binds the server to a local port and prepares it for serving.
//
// The provided context is used as base context for request handlers and for
// logging.
func (s *Server) Initialize(ctx context.Context) (*DevshellContext, error) {
	s.l.Lock()
	defer s.l.Unlock()

	if s.ctx != nil {
		return nil, fmt.Errorf("already initialized")
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", s.Port))
	if err != nil {
		return nil, err
	}

	s.ctx, s.cancel = context.WithCancel(ctx)
	s.listener = ln
	s.auth = auth.NewAuthenticator(ctx, auth.SilentLogin, s.Options)

	return &DevshellContext{
		Port: ln.Addr().(*net.TCPAddr).Port,
	}, nil
}

// Serve runs a serving loop.
//
// Unblocks once Close is called and all pending requests are served.
func (s *Server) Serve() (err error) {
	s.l.Lock()
	switch {
	case s.ctx == nil:
		err = fmt.Errorf("not initialized")
	case s.listener == nil:
		err = fmt.Errorf("already closed")
	}
	if err != nil {
		s.l.Unlock()
		return
	}
	listener := s.listener // accessed outside the lock
	s.l.Unlock()

	for {
		conn, err := listener.Accept()
		if err != nil {
			break
		}

		client := &client{
			conn:     &iotools.DeadlineReader{conn, 0},
			auth:     s.auth,
			lifetime: s.Lifetime,
			ctx:      s.ctx,
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

	s.l.Lock()
	if s.listener == nil {
		err = nil
	}
	s.l.Unlock()

	return
}

// Close closes the listening socket.
func (s *Server) Close() error {
	s.l.Lock()
	defer s.l.Unlock()
	if s.ctx == nil {
		return fmt.Errorf("not initialized")
	}
	// Stop accepting requests, unblock Serve(). Do it only once.
	if s.listener != nil {
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

	auth     *auth.Authenticator
	lifetime time.Duration

	ctx context.Context
}

func (c *client) handle() error {
	defer c.conn.Close()

	header := make([]byte, 6)
	if _, err := c.conn.Read(header); err != nil {
		return fmt.Errorf("failed to read the header: %v", err)
	}

	// The first six bytes contain the length separated by a newline.
	str := strings.SplitN(string(header), "\n", 2)
	if len(str) != 2 {
		return fmt.Errorf("no newline in the first 6 bytes")
	}

	l, err := strconv.Atoi(str[0])
	if err != nil {
		return fmt.Errorf("length is not a number: %v", err)
	}

	data := make([]byte, l)
	copy(data, str[1][:])

	// Read the rest of the message.
	if l > len(str[1]) {
		if _, err := c.conn.Read(data[len(str[1]):]); err != nil {
			return fmt.Errorf("failed to receive request: %v", err)
		}
	}

	// Parse the message to ensure it's a correct JSON.
	var request json.RawMessage
	if err := json.Unmarshal(data, &request); err != nil {
		return fmt.Errorf("failed to deserialize from JSON: %v", err)
	}

	// Get the user email.
	client, err := c.auth.Client()
	if err != nil {
		return fmt.Errorf("cannot get authenticated client: %v", err)
	}
	srv, err := oauth2.New(client)
	if err != nil {
		return err
	}
	u, err := srv.Userinfo.Get().Context(c.ctx).Do()
	if err != nil {
		return fmt.Errorf("cannot get user profile: %v", err)
	}

	// Get the token.
	t, err := c.auth.GetAccessToken(c.lifetime)
	if err != nil {
		return fmt.Errorf("cannot get access token: %v", err)
	}

	// Expiration is in seconds from now so compute the correct format.
	expiry := int(t.Expiry.Sub(time.Now()).Seconds())
	// Encode the response as JSON array (aka JsPbLite format).
	response := []interface{}{u.Email, nil, t.AccessToken, expiry}
	payload, err := json.Marshal(response)
	if err != nil {
		return fmt.Errorf("failed to serialize to JSON: %v", err)
	}

	var buf bytes.Buffer
	// Write the length of the payload separated by newline.
	buf.WriteString(fmt.Sprintf("%d\n", len(payload)))
	buf.Write(payload)

	// Send back the response.
	if _, err := c.conn.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("failed to send response: %v", err)
	}

	return nil
}
