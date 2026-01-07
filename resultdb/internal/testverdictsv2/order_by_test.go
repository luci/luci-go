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

package testverdictsv2

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
			assert.Loosely(t, o, should.Equal(OrderingByID))
		})

		t.Run("test_id_structured", func(t *ftt.Test) {
			o, err := ParseOrderBy("test_id_structured")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, o, should.Equal(OrderingByID))
		})

		t.Run("ui_priority", func(t *ftt.Test) {
			o, err := ParseOrderBy("ui_priority desc, test_id_structured")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, o, should.Equal(OrderingByUIPriority))

			o, err = ParseOrderBy("ui_priority desc")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, o, should.Equal(OrderingByUIPriority))
		})

		t.Run("Invalid", func(t *ftt.Test) {
			t.Run("test_id_structured desc", func(t *ftt.Test) {
				_, err := ParseOrderBy("test_id_structured desc")
				assert.Loosely(t, err, should.ErrLike(`can only sort by "test_id_structured" in ascending order`))
			})

			t.Run("ui_priority asc", func(t *ftt.Test) {
				_, err := ParseOrderBy("ui_priority, test_id_structured")
				assert.Loosely(t, err, should.ErrLike(`can only sort by "ui_priority" in descending order`))
			})

			t.Run("unknown field", func(t *ftt.Test) {
				_, err := ParseOrderBy("unknown")
				assert.Loosely(t, err, should.ErrLike(`unsupported order by clause: "unknown"; supported orders are "test_id_structured" or "ui_priority desc,test_id_structured"`))
			})
		})
	})
}
