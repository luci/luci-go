// Copyright 2025 The LUCI Authors.
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

package ssh

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
)

// AgentConn represents an SSH agent connection.
type AgentConn interface {
	// Read reads and returns the next message.
	Read() (*AgentMessage, error)

	// Write writes `msg`.
	Write(msg AgentMessage) error

	// Close closes the connection.
	Close() error

	// RemoteAddr returns the net.Addr of the remote peer.
	RemoteAddr() net.Addr
}

type AgentClient struct {
	AgentConn
}

// NewAgentClient creates a new AgentClient from an AgentConn.
func NewAgentClient(c AgentConn) AgentClient {
	return AgentClient{AgentConn: c}
}

// SendExtensionRequest sends `req` over AgentConn `c` for processing, and
// returns the response.
func (c AgentClient) SendExtensionRequest(req AgentExtensionRequest) ([]byte, error) {
	payload, err := MarshalBody(req)
	if err != nil {
		return nil, errors.WrapIf(err, "failed to marshal message payload")
	}

	msg := AgentMessage{
		Code:    AgentcExtension,
		Payload: payload,
	}
	if err := c.Write(msg); err != nil {
		return nil, errors.WrapIf(err, "failed to write request")
	}

	resp, err := c.Read()
	if err != nil {
		return nil, errors.WrapIf(err, "failed to read response")
	}

	switch resp.Code {
	case AgentSuccess:
		return resp.Payload, nil
	case AgentFailure:
		return nil, fmt.Errorf("extension request failed (SSH_AGENT_FAILURE), response: %v", resp.Payload)
	case AgentExtensionFailure:
		// Agent understood the extension, but failed to handle it.
		return nil, fmt.Errorf("extension request failed (SSH_AGENT_EXTENSION_FAILURE): %s", string(resp.Payload))
	default:
		return nil, fmt.Errorf("unknown extension request response code: %v, response body: %v", resp.Code, resp.Payload)
	}
}

type AgentDialer interface {
	// Dial returns a AgentConn to an SSH agent.
	Dial(context.Context) (AgentConn, error)
}

// AgentExtensionHandler defines the function signature of a SSH agent
// extension message handler.
//
// The handler receives a []byte, representing the request payload.
//
// The handler should return a response `AgentMessage`.
type AgentExtensionHandler func([]byte) AgentMessage

// AgentExtensionDispatcher defines a mapping of SSH agent extension
// names to their handler functions.
type AgentExtensionDispatcher map[string]AgentExtensionHandler

// ExtensionAgent intercepts and handles SSH agent extension requests
// registered in `dispatcher`, and sends all other messages to an upstream SSH
// agent (dialed by `upstreamDialer`).
type ExtensionAgent struct {
	// UpstreamDialer specifies the `AgentDialer` that ExtensionAgent proxies
	// non-extension SSH Agent messages or extension messages that aren't
	// registered in `Dispatcher`
	UpstreamDialer AgentDialer

	// The net.Listener that ExtensionAgent should accept connections from.
	Listener net.Listener

	// A mapping from SSH `ExtensionType` to its handler.
	Dispatcher AgentExtensionDispatcher

	wg sync.WaitGroup
}

// Close closes the listener and all client connections.
func (a *ExtensionAgent) Close() error {
	if err := a.Listener.Close(); err != nil {
		return err
	}
	a.wg.Wait()
	return nil
}

// Addr returns a net.Addr that can be use to connect to this agent.
func (a *ExtensionAgent) Addr() net.Addr {
	return a.Listener.Addr()
}

// Message loop for handling a new client connection.
func (a *ExtensionAgent) handleClient(ctx context.Context, conn net.Conn) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	client := NewAgentConn(conn)
	defer client.Close()

	upstream, err := a.UpstreamDialer.Dial(ctx)
	if err != nil {
		logging.Infof(ctx, "Failed to dial upstream agent: %v", err)
		return
	}
	defer upstream.Close()

	// This closes the client connection when the ListenLoop's context has been
	// cancelled (e.g. when the ExtensionAgent is closing).
	go func() {
		<-ctx.Done()
		client.Close()
		upstream.Close()
	}()

	for {
		msg, err := client.Read()
		if err != nil {
			if errors.Is(err, io.EOF) {
				logging.Infof(ctx, "Client socket closed: %v:%v", client.RemoteAddr().Network(), client.RemoteAddr().String())
			} else if !errors.Is(err, net.ErrClosed) {
				logging.Warningf(ctx, "Failed to read SSH Agent client message: %v, socket: %v", err, conn)
			}
			return
		}
		logging.Infof(ctx, "Read client message code %d, payload length: %d", msg.Code, len(msg.Payload))

		// Handle SSH agent extension requests registered in `extensionDispatcher`.
		if msg.Code == AgentcExtension {
			var req AgentExtensionRequest
			if err := UnmarshalBody(msg.Payload, &req); err != nil {
				logging.Warningf(ctx, "Failed to unmarshal SSH Agent extension request: %v", err)
				return
			}

			logging.Infof(ctx, "Received SSH Agent extension: %v, payload len: %v", req.ExtensionType, len(req.ExtensionData))
			if handler, ok := a.Dispatcher[req.ExtensionType]; ok {
				logging.Infof(ctx, "Dispatching LUCI SSH extension: %s", req.ExtensionType)
				if err := client.Write(handler(req.ExtensionData)); err != nil {
					if !errors.Is(err, net.ErrClosed) {
						logging.Warningf(ctx, "Failed to write message to client: %v", err)
					}
					return
				}
				continue
			}
		}

		// If our agent didn't handle the request, proxy it to upstream.
		logging.Infof(ctx, "Forwarding SSH agent message %d to upstream", msg.Code)
		if err := upstream.Write(*msg); err != nil {
			if !errors.Is(err, net.ErrClosed) {
				logging.Warningf(ctx, "Failed to write message to upstream socket: %v", err)
			}
			return
		}
		resp, err := upstream.Read()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				logging.Warningf(ctx, "Failed to read message from upstream socket: %v", err)
			}
			return
		}
		if err := client.Write(*resp); err != nil {
			if !errors.Is(err, net.ErrClosed) {
				logging.Warningf(ctx, "Failed to write message to client: %v", err)
			}
			return
		}
	}
}

// ListenLoop causes ExtensionAgent to start accepting client connections. This
// method blocks forever until Close() is called.
func (a *ExtensionAgent) ListenLoop(ctx context.Context) error {
	clientCtx, cancelFunc := context.WithCancel(ctx)
	defer cancelFunc()

	for {
		conn, err := a.Listener.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				logging.Infof(ctx, "Socket listener closed.")
				return nil
			} else {
				return err
			}
		}

		// Spawn a goroutine to handle each client.
		a.wg.Add(1)
		go func() {
			defer a.wg.Done()
			a.handleClient(clientCtx, conn)
		}()
	}
}

// upstreamWithFallback is an agentDialer that connects to the `upstream`
// SSH_AUTH_SOCK if available. Otherwise it connects to a fallbackAgent.
type UpstreamWithFallback struct {
	// Upstream SSH_AUTH_SOCK.
	Upstream string
}

var _ AgentDialer = (*UpstreamWithFallback)(nil)

// Dial connects to an upstream SSH_AUTH_SOCK. On failure, it returns a
// fallback channel and an error explaining why it failed.
func (u UpstreamWithFallback) Dial(ctx context.Context) (AgentConn, error) {
	if u.Upstream == "" {
		return NewFallbackAgent(), errors.New("no upstream SSH_AUTH_SOCK")
	}

	var d net.Dialer
	upConn, err := d.DialContext(ctx, "unix", u.Upstream)
	if err != nil {
		return NewFallbackAgent(), errors.WrapIf(err, "failed to dial SSH_AUTH_SOCK")
	}

	return NewAgentConn(upConn), nil
}
