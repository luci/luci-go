// Copyright 2026 The LUCI Authors.
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

package lowlatency

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestParseOrderBy(t *testing.T) {
	ftt.Run("ParseOrderBy", t, func(t *ftt.Test) {
		t.Run("Default", func(t *ftt.Test) {
			o, err := ParseOrderBy("")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, o, should.Match(Ordering{PositionAscending: false}))
		})

		t.Run("Position Ascending", func(t *ftt.Test) {
			o, err := ParseOrderBy("position")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, o, should.Match(Ordering{PositionAscending: true}))
		})

		t.Run("Position Descending", func(t *ftt.Test) {
			o, err := ParseOrderBy("position desc")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, o, should.Match(Ordering{PositionAscending: false}))
		})

		t.Run("Invalid Field", func(t *ftt.Test) {
			_, err := ParseOrderBy("other_field")
			assert.Loosely(t, err, should.ErrLike("unsupported order by field"))
		})

		t.Run("Multiple Fields", func(t *ftt.Test) {
			_, err := ParseOrderBy("position, other_field")
			assert.Loosely(t, err, should.ErrLike("unsupported order by clause"))
		})
	})
}
