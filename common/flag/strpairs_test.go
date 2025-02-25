// Copyright 2018 The LUCI Authors.
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

package flag

import (
	"flag"
	"testing"

	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestStrPairs(t *testing.T) {
	t.Parallel()

	ftt.Run("StringPairs", t, func(t *ftt.Test) {
		m := strpair.Map{}
		v := StringPairs(m)

		t.Run("Set", func(t *ftt.Test) {
			t.Run("Once", func(t *ftt.Test) {
				assert.Loosely(t, v.Set("a:1"), should.BeNil)
				assert.Loosely(t, m.Format(), should.Match([]string{"a:1"}))

				t.Run("Second time", func(t *ftt.Test) {
					assert.Loosely(t, v.Set("b:1"), should.BeNil)
					assert.Loosely(t, m.Format(), should.Match([]string{"a:1", "b:1"}))
				})

				t.Run("Same key", func(t *ftt.Test) {
					assert.Loosely(t, v.Set("a:2"), should.BeNil)
					assert.Loosely(t, m.Format(), should.Match([]string{"a:1", "a:2"}))
				})
			})
			t.Run("No colon", func(t *ftt.Test) {
				assert.Loosely(t, v.Set("a"), should.ErrLike("no colon"))
			})
			t.Run("Value has a colon", func(t *ftt.Test) {
				assert.Loosely(t, v.Set("a:1:1"), should.BeNil)
				assert.Loosely(t, m.Format(), should.Match([]string{"a:1:1"}))
			})
		})

		t.Run("String", func(t *ftt.Test) {
			m.Add("a", "1")
			m.Add("a", "2")
			// Value has a colon.
			m.Add("b", "1:1")
			assert.Loosely(t, v.String(), should.Equal("a:1, a:2, b:1:1"))
		})

		t.Run("Get", func(t *ftt.Test) {
			assert.Loosely(t, v.(flag.Getter).Get(), should.Match(m))
		})
	})
}
