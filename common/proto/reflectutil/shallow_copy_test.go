// Copyright 2022 The LUCI Authors.
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

package reflectutil

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestShallowCopy(t *testing.T) {
	t.Parallel()

	ftt.Run(`ShallowCopy`, t, func(t *ftt.Test) {
		t.Run(`nil`, func(t *ftt.Test) {
			msg := (*TestShallowCopyMessage)(nil)
			newMsg := ShallowCopy(msg).(*TestShallowCopyMessage)
			assert.Loosely(t, newMsg, should.BeNil)
		})

		t.Run(`basic fields`, func(t *ftt.Test) {
			msg := &TestShallowCopyMessage{
				Field: "hello",
				RepeatedField: []string{
					"this", "is", "a", "test",
				},
				MappedField: map[string]string{
					"this": "is",
					"a":    "test",
				},
			}
			newMsg := ShallowCopy(msg).(*TestShallowCopyMessage)
			assert.Loosely(t, newMsg, should.Match(msg))

			msg.RepeatedField[0] = "meepmorp"
			assert.Loosely(t, newMsg.RepeatedField[0], should.Equal("meepmorp"))

			msg.MappedField["this"] = "meepmorp"
			assert.Loosely(t, newMsg.MappedField["this"], should.Equal("meepmorp"))
		})

		t.Run(`msg fields`, func(t *ftt.Test) {
			msg := &TestShallowCopyMessage{
				InnerMsg: &TestShallowCopyMessage_Inner{Field: "hello"},
				RepeatedMsg: []*TestShallowCopyMessage_Inner{
					{Field: "this"}, {Field: "is"}, {Field: "a"}, {Field: "test"},
				},
				MappedMsg: map[string]*TestShallowCopyMessage_Inner{
					"this": {Field: "is"},
					"a":    {Field: "test"},
				},
			}
			newMsg := ShallowCopy(msg).(*TestShallowCopyMessage)
			assert.Loosely(t, newMsg, should.Match(msg))

			msg.InnerMsg.Field = "meepmorp"
			assert.Loosely(t, newMsg.InnerMsg.Field, should.Equal("meepmorp"))

			msg.RepeatedMsg[0].Field = "meepmorp"
			assert.Loosely(t, newMsg.RepeatedMsg[0].Field, should.Equal("meepmorp"))

			msg.MappedMsg["this"].Field = "meepmorp"
			assert.Loosely(t, newMsg.MappedMsg["this"].Field, should.Equal("meepmorp"))
		})
	})
}
