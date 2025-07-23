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

package testutil

import (
	"context"
	"net"
	"sync"

	"go.chromium.org/luci/common/ssh"
)

// FallbackDialer returns an ssh.AgentConn to an ssh.FallbackAgent.
type FallbackDialer struct{}

func (_ FallbackDialer) Dial(ctx context.Context) (ssh.AgentConn, error) {
	return ssh.NewFallbackAgent(), nil
}

// TestAgentConnHandler handles a ssh.AgentConn peer in a unit test.
type TestAgentConnHandler func(c ssh.AgentConn)

// TestDialer creates a new AgentConn when Dial() is called, and calls
// `handlerFn` on the peer of the returned AgentConn.
//
// Caller should call Shutdown() function to ensure all handlerFn exits.
type TestAgentDialer struct {
	handler TestAgentConnHandler
	wg      *sync.WaitGroup
}

func NewTestAgentDialer(h TestAgentConnHandler) TestAgentDialer {
	return TestAgentDialer{h, &sync.WaitGroup{}}
}

func (t TestAgentDialer) Dial(ctx context.Context) (ssh.AgentConn, error) {
	s, c := net.Pipe()

	t.wg.Add(1)
	go func() {
		defer t.wg.Done()
		t.handler(ssh.NewAgentConn(s))
	}()

	return ssh.NewAgentConn(c), nil
}

func (t TestAgentDialer) Shutdown() {
	t.wg.Wait()
}
