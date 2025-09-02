// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"context"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"go.chromium.org/luci/auth/reauth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/ssh"
	"go.chromium.org/luci/common/ssh/testutil"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/webauthn"
)

func createTestPluginRequest(t *testing.T) []byte {
	payload, err := reauth.PluginEncode(
		webauthn.GetAssertionRequest{
			Type:   "get",
			Origin: "example.com",
			RequestData: webauthn.GetAssertionRequestData{
				RPID:          "rpid",
				Challenge:     "abcd",
				TimeoutMillis: 10000,
			},
		})
	if err != nil {
		t.Fatal(err)
	}
	return payload
}

func assertPluginResponseIsErrorLike(t *testing.T, out bytes.Buffer, expectedErrorSubstring string) {
	var resp webauthn.GetAssertionResponse
	if err := reauth.PluginDecode(out.Bytes(), &resp); err != nil {
		t.Fatal(errors.WrapIf(err, "output can't be decoded as a pluginResponse"))
	}
	assert.Loosely(t, resp.Type, should.Equal("getResponse"))
	assert.Loosely(t, resp.Error, should.ContainSubstring(expectedErrorSubstring))
}

func TestSSHPluginMain(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	t.Run("bad input message", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		var out bytes.Buffer
		err := sshPluginMain(
			ctx,
			testutil.FailureDialer{},
			// Incomplete plugin header.
			bytes.NewReader([]byte{0xff, 0x00}),
			&out,
		)

		assert.ErrIsLike(t, err, "failed to read plugin frame header")
		assertPluginResponseIsErrorLike(t, out, "ssh plugin local failure: pluginRead: failed to read plugin frame header: unexpected EOF")
	})

	t.Run("failed to dial", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		var out bytes.Buffer
		err := sshPluginMain(
			ctx,
			testutil.FailureDialer{},
			bytes.NewReader(createTestPluginRequest(t)),
			&out,
		)
		assert.ErrIsLike(t, err, "failed to dial")
		assertPluginResponseIsErrorLike(t, out, "ssh plugin local failure: failed to dial")
	})

	t.Run("agent connection closed", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		dialer := testutil.NewTestAgentDialer(func(c ssh.AgentConn) {
			c.Close()
		})
		defer dialer.Shutdown()

		var out bytes.Buffer
		err := sshPluginMain(
			ctx,
			dialer,
			bytes.NewReader(createTestPluginRequest(t)),
			&out,
		)
		assert.ErrIsLike(t, err, "failed to write request: ")
		assertPluginResponseIsErrorLike(t, out, "ssh plugin upstream failure: failed to write request:")
	})

	t.Run("generic agent failure", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		dialer := testutil.NewTestAgentDialer(func(c ssh.AgentConn) {
			_, err := c.Read()
			defer c.Close()
			assert.NoErr(t, err)
			c.Write(ssh.AgentMessage{Code: ssh.AgentFailure})
		})
		defer dialer.Shutdown()

		var out bytes.Buffer
		err := sshPluginMain(
			ctx,
			dialer,
			bytes.NewReader(createTestPluginRequest(t)),
			&out,
		)
		assert.ErrIsLike(t, err, "extension request failed (SSH_AGENT_FAILURE)")
		assertPluginResponseIsErrorLike(t, out, "ssh plugin upstream failure: extension request failed (SSH_AGENT_FAILURE)")
	})

	t.Run("agent succeed", func(t *testing.T) {
		t.Parallel()
		ctx, cancel := context.WithCancel(ctx)
		defer cancel()

		pluginRequest := []byte{0xca, 0xfe}
		pluginResponse := []byte{0xde, 0xca, 0xfe}

		dialer := testutil.NewTestAgentDialer(func(c ssh.AgentConn) {
			msg, err := c.Read()
			defer c.Close()

			assert.NoErr(t, err)
			assert.That(t, msg.Code, should.Equal(ssh.AgentcExtension))

			// Check payload is correct.
			var extReq ssh.AgentExtensionRequest
			err = ssh.UnmarshalBody(msg.Payload, &extReq)
			assert.NoErr(t, err)
			assert.That(t, extReq.ExtensionType, should.Equal(reauth.SSHExtensionForwardedChallenge))
			assert.That(t, extReq.ExtensionData, should.Match(pluginRequest))

			// Write response.
			c.Write(ssh.AgentMessage{Code: ssh.AgentSuccess, Payload: pluginResponse})
		})
		defer dialer.Shutdown()

		var pluginStdin bytes.Buffer
		err := reauth.PluginWriteFrame(&pluginStdin, pluginRequest)
		assert.NoErr(t, err)

		var pluginStdout bytes.Buffer
		err = sshPluginMain(
			ctx,
			dialer,
			&pluginStdin,
			&pluginStdout,
		)
		assert.NoErr(t, err)

		gotResponse, err := reauth.PluginReadFrame(&pluginStdout)
		assert.NoErr(t, err)
		assert.That(t, gotResponse, should.Match(pluginResponse))
	})
}

func TestDefaultDialerSupportedProtocols(t *testing.T) {
	t.Parallel()
	d := reauth.DefaultAgentDialer{}

	t.Run("tcp", func(t *testing.T) {
		t.Parallel()
		loAddrs, err := net.LookupIP("localhost")
		assert.NoErr(t, err)
		assert.Loosely(t, loAddrs, should.NotBeEmpty)

		l, err := net.ListenTCP("tcp", &net.TCPAddr{
			IP:   loAddrs[0],
			Port: 0,
		})
		assert.NoErr(t, err)
		t.Cleanup(func() { l.Close() })

		var wg sync.WaitGroup
		t.Cleanup(wg.Wait)
		go func() {
			defer wg.Done()
			cli, err := l.Accept()
			assert.NoErr(t, err)
			defer cli.Close()

			_, err = cli.Write([]byte("tcp"))
			assert.NoErr(t, err)
		}()
		wg.Add(1)

		s := l.Addr().String()
		c, err := d.DialAddr(s)
		assert.NoErr(t, err)
		t.Cleanup(func() { c.Close() })

		// Check socket reads correctly.
		got, err := io.ReadAll(c)
		assert.NoErr(t, err)
		assert.Loosely(t, got, should.Match([]byte("tcp")))
	})

	t.Run("unix", func(t *testing.T) {
		t.Parallel()
		ldir, err := os.MkdirTemp("", "luci_ssh_plugin_test.")
		assert.NoErr(t, err)
		t.Cleanup(func() { os.RemoveAll(ldir) })

		lpath := filepath.Join(ldir, "unix_sock")
		l, err := net.ListenUnix("unix", &net.UnixAddr{Name: lpath, Net: "unix"})
		assert.NoErr(t, err)
		t.Cleanup(func() { l.Close() })

		var wg sync.WaitGroup
		t.Cleanup(wg.Wait)
		go func() {
			defer wg.Done()
			cli, err := l.Accept()
			assert.NoErr(t, err)
			defer cli.Close()

			_, err = cli.Write([]byte("unix"))
			assert.NoErr(t, err)
		}()
		wg.Add(1)

		s := l.Addr().String()
		c, err := d.DialAddr(s)
		assert.NoErr(t, err)
		t.Cleanup(func() { c.Close() })

		// Check socket reads correctly.
		got, err := io.ReadAll(c)
		assert.NoErr(t, err)
		assert.Loosely(t, got, should.Match([]byte("unix")))
	})
}
