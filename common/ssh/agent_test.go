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

package ssh_test

import (
	"context"
	"net"
	"testing"

	"golang.org/x/net/nettest"

	"go.chromium.org/luci/common/ssh"
	"go.chromium.org/luci/common/ssh/testutil"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

// Caller is responsible for calling Close() on the returned agent and client.
func newTestExtensionAgentAndClient(
	ctx context.Context,
	t *testing.T,
	dispatcher ssh.AgentExtensionDispatcher,
	upstream ssh.AgentDialer,
) (*ssh.ExtensionAgent, ssh.AgentConn) {
	listener, err := nettest.NewLocalListener("tcp")
	if err != nil {
		t.Fatal(err)
	}
	agent := ssh.ExtensionAgent{
		Dispatcher:     dispatcher,
		Listener:       listener,
		UpstreamDialer: upstream,
	}
	go agent.ListenLoop(ctx)

	clientConn, err := net.Dial(listener.Addr().Network(), listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return &agent, ssh.NewAgentConn(clientConn)
}

func TestHandleClient(t *testing.T) {
	ctx := context.Background()

	const (
		succeedExtension = "echo@go.chromium.org"
		failExtension    = "fail@go.chromium.org"
		otherExtension   = "other@example.com"
	)

	dispatcher := ssh.AgentExtensionDispatcher{
		succeedExtension: func(ctx context.Context, p []byte) ssh.AgentMessage {
			return ssh.AgentMessage{
				Code:    ssh.AgentSuccess,
				Payload: []byte("echo"),
			}
		},
		failExtension: func(ctx context.Context, p []byte) ssh.AgentMessage {
			return ssh.AgentMessage{
				Code:    ssh.AgentExtensionFailure,
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

			agent, client := newTestExtensionAgentAndClient(ctx, t, dispatcher, testutil.FallbackDialer{})
			defer agent.Close()
			defer client.Close()

			payload, err := ssh.MarshalBody(ssh.AgentExtensionRequest{
				ExtensionType: succeedExtension,
				ExtensionData: []byte("echo"),
			})
			assert.NoErr(t, err)
			err = client.Write(ssh.AgentMessage{Code: ssh.AgentcExtension, Payload: payload})
			assert.NoErr(t, err)

			resp, err := client.Read()
			assert.NoErr(t, err)
			assert.Loosely(t, resp.Code, should.Equal(ssh.AgentSuccess))
			assert.Loosely(t, resp.Payload, should.Match("echo"))
		})

		t.Run("extension failure response", func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			agent, client := newTestExtensionAgentAndClient(ctx, t, dispatcher, testutil.FallbackDialer{})
			defer agent.Close()
			defer client.Close()

			payload, err := ssh.MarshalBody(ssh.AgentExtensionRequest{
				ExtensionType: failExtension,
				ExtensionData: []byte("echo"),
			})
			assert.NoErr(t, err)
			err = client.Write(ssh.AgentMessage{Code: ssh.AgentcExtension, Payload: payload})
			assert.NoErr(t, err)

			resp, err := client.Read()
			assert.NoErr(t, err)
			assert.Loosely(t, resp.Code, should.Equal(ssh.AgentExtensionFailure))
			assert.Loosely(t, resp.Payload, should.Match(failExtension))
		})

		t.Run("upstream failure", func(t *testing.T) {
			t.Parallel()
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			agent, client := newTestExtensionAgentAndClient(ctx, t, dispatcher, testutil.FallbackDialer{})
			defer agent.Close()
			defer client.Close()

			payload, err := ssh.MarshalBody(ssh.AgentExtensionRequest{
				ExtensionType: otherExtension,
				ExtensionData: []byte("other extension's message"),
			})
			assert.NoErr(t, err)
			err = client.Write(ssh.AgentMessage{Code: ssh.AgentcExtension, Payload: payload})
			assert.NoErr(t, err)

			resp, err := client.Read()
			assert.NoErr(t, err)
			assert.Loosely(t, resp.Code, should.Equal(ssh.AgentFailure))
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

			upstream := testutil.NewTestAgentDialer(func(c ssh.AgentConn) {
				msg, err := c.Read()
				assert.NoErr(t, err)
				assert.Loosely(t, msg.Code, should.Equal(testSSHAgentcRequest))
				assert.Loosely(t, msg.Payload, should.Match("upstream"))

				err = c.Write(ssh.AgentMessage{Code: testSSHAgentResponse, Payload: []byte("resp")})
				assert.NoErr(t, err)
			})
			defer upstream.Shutdown()

			agent, client := newTestExtensionAgentAndClient(ctx, t, dispatcher, upstream)
			defer agent.Close()
			defer client.Close()

			err := client.Write(ssh.AgentMessage{Code: testSSHAgentcRequest, Payload: []byte("upstream")})
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

			testReq := ssh.AgentExtensionRequest{
				ExtensionType: "test@example.com",
				ExtensionData: []byte("abcd"),
			}

			upstream := testutil.NewTestAgentDialer(func(c ssh.AgentConn) {
				msg, err := c.Read()
				assert.NoErr(t, err)
				assert.Loosely(t, msg.Code, should.Equal(ssh.AgentcExtension))

				var gotReq ssh.AgentExtensionRequest
				err = ssh.UnmarshalBody(msg.Payload, &gotReq)
				assert.NoErr(t, err)
				assert.Loosely(t, gotReq, should.Match(testReq))

				err = c.Write(ssh.AgentMessage{Code: ssh.AgentSuccess, Payload: []byte("resp")})
				assert.NoErr(t, err)
			})
			defer upstream.Shutdown()

			agent, client := newTestExtensionAgentAndClient(ctx, t, dispatcher, upstream)
			defer agent.Close()
			defer client.Close()

			payload, err := ssh.MarshalBody(testReq)
			assert.NoErr(t, err)
			err = client.Write(ssh.AgentMessage{Code: ssh.AgentcExtension, Payload: payload})
			assert.NoErr(t, err)

			// Read reply from upstream
			resp, err := client.Read()
			assert.NoErr(t, err)
			assert.Loosely(t, resp.Code, should.Equal(ssh.AgentSuccess))
			assert.Loosely(t, resp.Payload, should.Match("resp"))
		})
	})
}

func TestUpstreamWithFallbackDialer(t *testing.T) {
	ctx := context.Background()
	t.Parallel()

	t.Run("Fallback without failing", func(t *testing.T) {
		dialer := ssh.UpstreamWithFallback{}
		c, err := dialer.Dial(ctx)
		assert.NoErr(t, err)
		assert.Loosely(t, c, should.NotBeNil)
		assert.Loosely(t, c, should.HaveType[*ssh.FallbackAgent])
	})
}
