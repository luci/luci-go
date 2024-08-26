// Copyright 2015 The LUCI Authors.
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

package pattern

import (
	"regexp"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestPattern(t *testing.T) {
	t.Parallel()

	ftt.Run("Pattern", t, func(t *ftt.Test) {
		t.Run("Exact", func(t *ftt.Test) {
			p := Exact("a")
			assert.Loosely(t, p.Match("a"), should.BeTrue)
			assert.Loosely(t, p.Match("b"), should.BeFalse)
			assert.Loosely(t, p.String(), should.Equal("exact:a"))
		})

		t.Run("Regex", func(t *ftt.Test) {
			p := Regexp(regexp.MustCompile("^ab?$"))
			assert.Loosely(t, p.Match("a"), should.BeTrue)
			assert.Loosely(t, p.Match("ab"), should.BeTrue)
			assert.Loosely(t, p.Match("b"), should.BeFalse)
			assert.Loosely(t, p.String(), should.Equal("regex:^ab?$"))
		})

		t.Run("Any", func(t *ftt.Test) {
			assert.Loosely(t, Any.Match("a"), should.BeTrue)
			assert.Loosely(t, Any.Match("b"), should.BeTrue)
			assert.Loosely(t, Any.String(), should.Equal("*"))
		})

		t.Run("None", func(t *ftt.Test) {
			assert.Loosely(t, None.Match("a"), should.BeFalse)
			assert.Loosely(t, None.Match("b"), should.BeFalse)
			assert.Loosely(t, None.String(), should.BeEmpty)
		})

		t.Run("Parse", func(t *ftt.Test) {
			t.Run("Good", func(t *ftt.Test) {
				patterns := []Pattern{
					Exact("a"),
					Regexp(regexp.MustCompile("^ab$")),
					Any,
					None,
				}
				for _, p := range patterns {
					p2, err := Parse(p.String())
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, p2.String(), should.Equal(p.String()))
				}
			})

			t.Run("Without '^' and '$'", func(t *ftt.Test) {
				p, err := Parse("regex:deadbeef")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, p.String(), should.Equal("regex:^deadbeef$"))
			})

			t.Run("Bad", func(t *ftt.Test) {
				bad := []string{
					":",
					"a:",
					"a:b",
					"regex:)",
				}
				for _, s := range bad {
					_, err := Parse(s)
					assert.Loosely(t, err, should.NotBeNil)
				}
			})
		})
	})
}
