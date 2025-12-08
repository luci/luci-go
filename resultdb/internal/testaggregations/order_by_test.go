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

package testaggregations

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestParseOrderBy(t *testing.T) {
	ftt.Run("ParseOrderBy", t, func(t *ftt.Test) {
		t.Run("Valid", func(t *ftt.Test) {
			t.Run("Default", func(t *ftt.Test) {
				o, err := ParseOrderBy("")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, o, should.Match(Ordering{ByLevelFirst: true}))
			})

			t.Run("Explicit Default", func(t *ftt.Test) {
				o, err := ParseOrderBy("id.level,id.id")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, o, should.Match(Ordering{ByLevelFirst: true}))
			})

			t.Run("UI Sort Order", func(t *ftt.Test) {
				o, err := ParseOrderBy("id.level,ui_priority desc,id.id")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, o, should.Match(Ordering{ByLevelFirst: true, ByUIPriority: true}))
			})

			t.Run("ID Only", func(t *ftt.Test) {
				o, err := ParseOrderBy("id.id")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, o, should.Match(Ordering{}))
			})

			t.Run("UI Priority Desc, ID", func(t *ftt.Test) {
				o, err := ParseOrderBy("ui_priority desc,id.id")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, o, should.Match(Ordering{ByUIPriority: true}))
			})

			t.Run("Level Only", func(t *ftt.Test) {
				o, err := ParseOrderBy("id.level")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, o, should.Match(Ordering{ByLevelFirst: true}))
			})

			t.Run("UI Priority Desc", func(t *ftt.Test) {
				o, err := ParseOrderBy("ui_priority desc")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, o, should.Match(Ordering{ByUIPriority: true}))
			})
		})

		t.Run("Invalid", func(t *ftt.Test) {
			t.Run("Level Descending", func(t *ftt.Test) {
				_, err := ParseOrderBy("id.level desc")
				assert.Loosely(t, err, should.ErrLike("can only sort by `id.level` in ascending order"))
			})

			t.Run("UI Priority Ascending", func(t *ftt.Test) {
				_, err := ParseOrderBy("ui_priority")
				assert.Loosely(t, err, should.ErrLike("can only sort by `ui_priority` in descending order"))
			})

			t.Run("ID Descending", func(t *ftt.Test) {
				_, err := ParseOrderBy("id.id desc")
				assert.Loosely(t, err, should.ErrLike("can only sort by `id.id` in ascending order"))
			})

			t.Run("Unknown Field", func(t *ftt.Test) {
				_, err := ParseOrderBy("unknown_field")
				assert.Loosely(t, err, should.ErrLike("unsupported order by clause"))
			})

			t.Run("Wrong Order (ID before Level)", func(t *ftt.Test) {
				_, err := ParseOrderBy("id.id,id.level")
				assert.Loosely(t, err, should.ErrLike("unsupported order by clause"))
			})

			t.Run("Wrong Order (UI Priority before Level)", func(t *ftt.Test) {
				_, err := ParseOrderBy("ui_priority desc,id.level")
				assert.Loosely(t, err, should.ErrLike("unsupported order by clause"))
			})

			t.Run("Syntax Error", func(t *ftt.Test) {
				_, err := ParseOrderBy("id.level,")
				assert.Loosely(t, err, should.ErrLike("syntax error"))
			})
		})
	})
}
