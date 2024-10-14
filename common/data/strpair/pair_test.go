// Copyright 2017 The LUCI Authors.
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

package strpair

import (
	"fmt"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func ExampleParse_hierarchical() {
	pairs := []string{
		"foo:bar",
		"swarming_tag:a:1",
		"swarming_tag:b:2",
	}
	tags := ParseMap(ParseMap(pairs)["swarming_tag"])
	fmt.Println(tags.Get("a"))
	fmt.Println(tags.Get("b"))
	// Output:
	// 1
	// 2
}

func TestPairs(t *testing.T) {
	t.Parallel()

	ftt.Run("ParseMap invalid", t, func(t *ftt.Test) {
		k, v := Parse("foo")
		assert.Loosely(t, k, should.Equal("foo"))
		assert.Loosely(t, v, should.BeEmpty)
	})

	m := Map{
		"b": []string{"1"},
		"a": []string{"2"},
	}

	ftt.Run("Map", t, func(t *ftt.Test) {
		t.Run("Format", func(t *ftt.Test) {
			assert.Loosely(t, m.Format(), should.Resemble([]string{
				"a:2",
				"b:1",
			}))
		})
		t.Run("Get non-existent", func(t *ftt.Test) {
			assert.Loosely(t, m.Get("c"), should.BeEmpty)
		})
		t.Run("Get with empty", func(t *ftt.Test) {
			m2 := m.Copy()
			m2["c"] = nil
			assert.Loosely(t, m2.Get("c"), should.BeEmpty)
		})
		t.Run("Contains", func(t *ftt.Test) {
			assert.Loosely(t, m.Contains("a", "2"), should.BeTrue)
			assert.Loosely(t, m.Contains("a", "1"), should.BeFalse)
			assert.Loosely(t, m.Contains("b", "1"), should.BeTrue)
			assert.Loosely(t, m.Contains("c", "1"), should.BeFalse)
		})
		t.Run("Set", func(t *ftt.Test) {
			m2 := m.Copy()
			m2.Set("c", "3")
			assert.Loosely(t, m2, should.Resemble(Map{
				"b": []string{"1"},
				"a": []string{"2"},
				"c": []string{"3"},
			}))
		})
		t.Run("Del", func(t *ftt.Test) {
			m.Del("a")
			assert.Loosely(t, m, should.Resemble(Map{
				"b": []string{"1"},
			}))
		})
	})
}
