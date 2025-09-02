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

package reauth

import (
	"context"
	"net"
	"os"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/ssh"
)

// DefaultAgentDialer dials the agent based on SSH_AUTH_SOCK environment
// variable.
type DefaultAgentDialer struct{}

func (d DefaultAgentDialer) Dial(ctx context.Context) (ssh.AgentConn, error) {
	sshAuthSock, _ := os.LookupEnv(EnvSSHAuthSock)
	if sshAuthSock == "" {
		return nil, errors.Fmt("%s isn't set", EnvSSHAuthSock)
	}

	c, err := d.DialAddr(sshAuthSock)
	if err != nil {
		return nil, err
	}

	return ssh.NewAgentConn(c), nil
}

// Try to dial `addr` by trying supported protocols (i.e. TCP or Unix) in order.
func (d DefaultAgentDialer) DialAddr(addr string) (net.Conn, error) {
	if a, err := net.ResolveTCPAddr("tcp", addr); err == nil {
		return net.DialTCP("tcp", nil, a)
	}

	if a, err := net.ResolveUnixAddr("unix", addr); err == nil {
		return net.DialUnix("unix", nil, a)
	}

	return nil, errors.Fmt("no matching protocol for address: %v", addr)
}
