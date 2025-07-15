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
	"net"
	"sync/atomic"
)

// fallbackAgent acts as an SSH agent that always fails the request with
// AgentFailure.
//
// This implements AgentConn interface, and can be used when SSH_AUTH_SOCK
// isn't set.
type fallbackAgent struct {
	closed atomic.Bool
}

var _ AgentConn = (*fallbackAgent)(nil)

func (c *fallbackAgent) Read() (*AgentMessage, error) {
	if c.closed.Load() {
		return nil, net.ErrClosed
	}
	return &AgentMessage{Code: AgentFailure}, nil
}

func (c *fallbackAgent) Write(msg AgentMessage) error {
	if c.closed.Load() {
		return net.ErrClosed
	}
	return nil
}

func (c *fallbackAgent) Close() error {
	c.closed.Store(true)
	return nil
}

func (c *fallbackAgent) RemoteAddr() net.Addr {
	return nil
}

func newFallbackAgent() *fallbackAgent {
	return &fallbackAgent{}
}
