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

// Package devshell implements Devshell protocol for locally getting auth token.
//
// Some Google Cloud tools know how to use it for authentication.
package devshell

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/oauth2"

	"go.chromium.org/luci/auth/integration/internal/localsrv"
	"go.chromium.org/luci/common/clock"
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

	srv localsrv.Server
}

// Start launches background goroutine with the serving loop.
//
// The provided context is used as base context for request handlers and for
// logging.
//
// The server must be eventually stopped with Stop().
func (s *Server) Start(ctx context.Context) (*net.TCPAddr, error) {
	return s.srv.Start(ctx, "devshell", s.Port, s.serve)
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
	return s.srv.Stop(ctx)
}

// serve runs the serving loop.
func (s *Server) serve(ctx context.Context, l net.Listener, wg *sync.WaitGroup) error {
	for {
		conn, err := l.Accept()
		if err != nil {
			return err
		}

		client := &client{
			conn:   conn,
			source: s.Source,
			email:  s.Email,
			ctx:    ctx,
		}

		wg.Add(1)
		go func() {
			defer wg.Done()

			paniccatcher.Do(func() {
				if err := client.handle(); err != nil {
					logging.Fields{
						logging.ErrorKey: err,
					}.Errorf(client.ctx, "failed to handle client request")
				}
			}, func(p *paniccatcher.Panic) {
				p.Log(client.ctx, "panic during client handshake: %s", p.Reason)
			})
		}()
	}
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
		if err := c.sendResponse([]any{err.Error()}); err != nil {
			return fmt.Errorf("failed to send error: %v", err)
		}
		return nil
	}

	// Get the token.
	t, err := c.source.Token()
	if err != nil {
		if err := c.sendResponse([]any{"cannot get access token"}); err != nil {
			return fmt.Errorf("failed to send error: %v", err)
		}
		return err
	}

	// Expiration is in seconds from now so compute the correct format.
	expiry := int(t.Expiry.Sub(clock.Now(c.ctx)).Seconds())

	return c.sendResponse([]any{c.email, nil, t.AccessToken, expiry})
}

func (c *client) readRequest() ([]any, error) {
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
	request := []any{}
	if err := json.Unmarshal(data, &request); err != nil {
		return nil, fmt.Errorf("failed to deserialize from JSON: %v", err)
	}

	return request, nil
}

func (c *client) sendResponse(response []any) error {
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
