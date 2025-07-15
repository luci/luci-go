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
	"testing"

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestFallbackAgent(t *testing.T) {
	t.Run("Read", func(t *testing.T) {
		t.Parallel()
		agent := newFallbackAgent()
		msg, err := agent.Read()
		assert.NoErr(t, err)
		assert.That(t, msg.Code, should.Equal(AgentFailure))
		assert.That(t, any(msg.Payload), should.BeNil)
	})

	t.Run("Write", func(t *testing.T) {
		t.Parallel()
		agent := newFallbackAgent()
		err := agent.Write(AgentMessage{Code: AgentMessageCode(0)})
		assert.NoErr(t, err)
	})

	t.Run("Close", func(t *testing.T) {
		t.Parallel()
		agent := newFallbackAgent()
		agent.Close()

		t.Run("Read", func(t *testing.T) {
			t.Parallel()
			_, err := agent.Read()
			assert.That(t, err, should.Equal(net.ErrClosed))
		})

		t.Run("Write", func(t *testing.T) {
			t.Parallel()
			err := agent.Write(AgentMessage{Code: AgentMessageCode(0)})
			assert.That(t, err, should.Equal(net.ErrClosed))
		})
	})
}
