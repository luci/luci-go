// Copyright 2019 The LUCI Authors.
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

package globset

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestGlobSet(t *testing.T) {
	ftt.Run("Empty", t, func(t *ftt.Test) {
		gs, err := NewBuilder().Build()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, gs, should.BeNil)
	})

	ftt.Run("Works", t, func(t *ftt.Test) {
		b := NewBuilder()
		assert.Loosely(t, b.Add("user:*@example.com"), should.BeNil)
		assert.Loosely(t, b.Add("user:*@other.example.com"), should.BeNil)
		assert.Loosely(t, b.Add("service:*"), should.BeNil)

		gs, err := b.Build()
		assert.Loosely(t, err, should.BeNil)

		assert.Loosely(t, gs, should.HaveLength(2))
		assert.Loosely(t, gs["user"].String(), should.Equal(`^((.*@example\.com)|(.*@other\.example\.com))$`))
		assert.Loosely(t, gs["service"].String(), should.Equal(`^.*$`))

		assert.Loosely(t, gs.Has("user:a@example.com"), should.BeTrue)
		assert.Loosely(t, gs.Has("user:a@other.example.com"), should.BeTrue)
		assert.Loosely(t, gs.Has("user:a@not-example.com"), should.BeFalse)
		assert.Loosely(t, gs.Has("service:zzz"), should.BeTrue)
		assert.Loosely(t, gs.Has("anonymous:anonymous"), should.BeFalse)
	})

	ftt.Run("Caches regexps", t, func(t *ftt.Test) {
		b := NewBuilder()

		assert.Loosely(t, b.Add("user:*@example.com"), should.BeNil)
		assert.Loosely(t, b.Add("user:*@other.example.com"), should.BeNil)

		gs1, err := b.Build()
		assert.Loosely(t, err, should.BeNil)

		b.Reset()
		assert.Loosely(t, b.Add("user:*@other.example.com"), should.BeNil)
		assert.Loosely(t, b.Add("user:*@example.com"), should.BeNil)

		gs2, err := b.Build()
		assert.Loosely(t, err, should.BeNil)

		// The exact same regexp object.
		assert.Loosely(t, gs1["user"], should.Equal(gs2["user"]))
	})

	ftt.Run("Edge cases in Has", t, func(t *ftt.Test) {
		b := NewBuilder()
		b.Add("user:a*@example.com")
		b.Add("user:*@other.example.com")
		gs, _ := b.Build()

		assert.Loosely(t, gs.Has("abc@example.com"), should.BeFalse)                // no "user:" prefix
		assert.Loosely(t, gs.Has("service:abc@example.com"), should.BeFalse)        // wrong prefix
		assert.Loosely(t, gs.Has("user:abc@example.com\nsneaky"), should.BeFalse)   // sneaky '/n'
		assert.Loosely(t, gs.Has("user:bbc@example.com"), should.BeFalse)           // '^' is checked
		assert.Loosely(t, gs.Has("user:abc@example.com-and-stuff"), should.BeFalse) // '$' is checked
	})
}
