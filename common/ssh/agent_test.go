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
	"net"
	"testing"

	"golang.org/x/net/nettest"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

type fallbackDialer struct{}

func (_ fallbackDialer) Dial(ctx context.Context) AgentConn {
	return newFallbackAgent()
}

type proxyDialer struct {
	connections chan AgentConn
}

func (p proxyDialer) Dial(ctx context.Context) AgentConn {
	s, c := net.Pipe()
	p.connections <- NewAgentConn(s)
	return NewAgentConn(c)
}

func (p proxyDialer) getConnection() AgentConn {
	return <-p.connections
}

func newProxyDialer() proxyDialer {
	return proxyDialer{
		connections: make(chan AgentConn, 1),
	}
}

type testUpstreamDialer interface {
	Dial(context.Context) AgentConn
}

// Caller is responsible for calling Close() on the returned agent and client.
func newTestExtensionAgentAndClient(
	ctx context.Context,
	t *testing.T,
	dispatcher AgentExtensionDispatcher,
	upstream testUpstreamDialer,
) (*ExtensionAgent, AgentConn) {
	listener, err := nettest.NewLocalListener("tcp")
	if err != nil {
		t.Fatal(err)
	}
	agent := ExtensionAgent{
		Dispatcher:     dispatcher,
		Listener:       listener,
		UpstreamDialer: upstream,
	}
	go agent.ListenLoop(ctx)

	clientConn, err := net.Dial(listener.Addr().Network(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return &agent, NewAgentConn(clientConn)
}

func TestHandleClient(t *testing.T) {
	ctx := context.Background()

	const (
		succeedExtension = "echo@go.chromium.org"
		failExtension    = "fail@go.chromium.org"
		otherExtension   = "other@example.com"
	)

	dispatcher := AgentExtensionDispatcher{
		succeedExtension: func(p []byte) AgentMessage {
			return AgentMessage{
				Code:    AgentSuccess,
				Payload: []byte("echo"),
			}
		},
		failExtension: func(p []byte) AgentMessage {
			return AgentMessage{
				Code:    AgentExtensionFailure,
				Payload: []byte(failExtension),
			}
		},
	}

	t.Run("Extension dispatch", func(t *testing.T) {
		t.Parallel()
		t.Run("succeed response", func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			agent, client := newTestExtensionAgentAndClient(ctx, t, dispatcher, fallbackDialer{})
			defer agent.Close()
			defer client.Close()

			payload, err := MarshalBody(AgentExtensionRequest{
				ExtensionType: succeedExtension,
				ExtensionData: []byte("echo"),
			})
			assert.NoErr(t, err)
			err = client.Write(AgentMessage{Code: AgentcExtension, Payload: payload})
			assert.NoErr(t, err)

			resp, err := client.Read()
			assert.NoErr(t, err)
			assert.Loosely(t, resp.Code, should.Equal(AgentSuccess))
			assert.Loosely(t, resp.Payload, should.Match("echo"))
		})

		t.Run("extension failure response", func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			agent, client := newTestExtensionAgentAndClient(ctx, t, dispatcher, fallbackDialer{})
			defer agent.Close()
			defer client.Close()

			payload, err := MarshalBody(AgentExtensionRequest{
				ExtensionType: failExtension,
				ExtensionData: []byte("echo"),
			})
			assert.NoErr(t, err)
			err = client.Write(AgentMessage{Code: AgentcExtension, Payload: payload})
			assert.NoErr(t, err)

			resp, err := client.Read()
			assert.NoErr(t, err)
			assert.Loosely(t, resp.Code, should.Equal(AgentExtensionFailure))
			assert.Loosely(t, resp.Payload, should.Match(failExtension))
		})

		t.Run("upstream failure", func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			agent, client := newTestExtensionAgentAndClient(ctx, t, dispatcher, fallbackDialer{})
			defer agent.Close()
			defer client.Close()

			payload, err := MarshalBody(AgentExtensionRequest{
				ExtensionType: otherExtension,
				ExtensionData: []byte("other extension's message"),
			})
			assert.NoErr(t, err)
			err = client.Write(AgentMessage{Code: AgentcExtension, Payload: payload})
			assert.NoErr(t, err)

			resp, err := client.Read()
			assert.NoErr(t, err)
			assert.Loosely(t, resp.Code, should.Equal(AgentFailure))
			assert.Loosely(t, resp.Payload, should.Match([]byte{}))
		})
	})

	t.Run("Proxy other messages", func(t *testing.T) {
		t.Parallel()
		const testSSHAgentcRequest = 11
		const testSSHAgentResponse = 12

		t.Run("proxy non-extension messages", func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			upstream := newProxyDialer()
			agent, client := newTestExtensionAgentAndClient(ctx, t, dispatcher, upstream)
			defer agent.Close()
			defer client.Close()

			go func() {
				conn := upstream.getConnection()
				msg, err := conn.Read()
				assert.NoErr(t, err)
				assert.Loosely(t, msg.Code, should.Equal(testSSHAgentcRequest))
				assert.Loosely(t, msg.Payload, should.Match("upstream"))

				err = conn.Write(AgentMessage{Code: testSSHAgentResponse, Payload: []byte("resp")})
				assert.NoErr(t, err)
			}()

			err := client.Write(AgentMessage{Code: testSSHAgentcRequest, Payload: []byte("upstream")})
			assert.NoErr(t, err)

			// Read reply from upstream
			resp, err := client.Read()
			assert.NoErr(t, err)
			assert.Loosely(t, resp.Code, should.Equal(testSSHAgentResponse))
			assert.Loosely(t, resp.Payload, should.Match("resp"))
		})

		t.Run("proxy extension message not registered to dispatcher", func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			upstream := newProxyDialer()
			agent, client := newTestExtensionAgentAndClient(ctx, t, dispatcher, upstream)
			defer agent.Close()
			defer client.Close()

			testReq := AgentExtensionRequest{
				ExtensionType: "test@example.com",
				ExtensionData: []byte("abcd"),
			}

			go func() {
				conn := upstream.getConnection()
				msg, err := conn.Read()
				assert.NoErr(t, err)
				assert.Loosely(t, msg.Code, should.Equal(AgentcExtension))

				var gotReq AgentExtensionRequest
				err = UnmarshalBody(msg.Payload, &gotReq)
				assert.NoErr(t, err)
				assert.Loosely(t, gotReq, should.Match(testReq))

				err = conn.Write(AgentMessage{Code: AgentSuccess, Payload: []byte("resp")})
				assert.NoErr(t, err)
			}()

			payload, err := MarshalBody(testReq)
			assert.NoErr(t, err)
			err = client.Write(AgentMessage{Code: AgentcExtension, Payload: payload})
			assert.NoErr(t, err)

			// Read reply from upstream
			resp, err := client.Read()
			assert.NoErr(t, err)
			assert.Loosely(t, resp.Code, should.Equal(AgentSuccess))
			assert.Loosely(t, resp.Payload, should.Match("resp"))
		})
	})
}
