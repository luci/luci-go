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

package aip160

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestEnumColumn(t *testing.T) {
	ftt.Run("EnumColumn", t, func(t *ftt.Test) {
		// This enum is normally defined by a generated proto binding.
		myStatusNames := map[string]int32{
			"MYSTATUS_UNSPECIFIED": 0,
			"SUCCEEDED":            1,
			"FAILED":               2,
			"CANCELLED":            3,
			"RUNNING":              4,
			"COMPLETED_MASK":       5,
		}
		enumDef := NewEnumDefinition("luci.common.v1.MyStatus", myStatusNames, 0, 5)

		table := NewDatabaseTable().WithFields(
			NewField().WithFieldPath("status").WithBackend(
				NewEnumColumn("db_status").WithDefinition(enumDef).Build(),
			).Filterable().Build(),
		).Build()

		t.Run("equals operator", func(t *ftt.Test) {
			filter, err := ParseFilter("status = SUCCEEDED OR status = FAILED")
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pars, should.BeNil)
			assert.Loosely(t, result, should.Equal("((T.db_status = 1) OR (T.db_status = 2))"))
		})

		t.Run("not equals operator", func(t *ftt.Test) {
			filter, err := ParseFilter("status != CANCELLED AND status != RUNNING")
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pars, should.BeNil)
			assert.Loosely(t, result, should.Equal("((T.db_status <> 3) AND (T.db_status <> 4))"))
		})

		t.Run("disallowed enum value used", func(t *ftt.Test) {
			filter, err := ParseFilter("status = MYSTATUS_UNSPECIFIED")
			assert.Loosely(t, err, should.BeNil)

			_, _, err = table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.ErrLike(`argument for field "status": "MYSTATUS_UNSPECIFIED" is not allowed for this enum, expected one of [SUCCEEDED, FAILED, CANCELLED, RUNNING]`))
		})

		t.Run("invalid enum value used", func(t *ftt.Test) {
			filter, err := ParseFilter("status = BAD_VALUE")
			assert.Loosely(t, err, should.BeNil)

			_, _, err = table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.ErrLike(`argument for field "status": "BAD_VALUE" is not one of the valid enum values, expected one of [SUCCEEDED, FAILED, CANCELLED, RUNNING]`))
		})

		t.Run("unsupported operator used", func(t *ftt.Test) {
			filter, err := ParseFilter("status:CANCELLED")
			assert.Loosely(t, err, should.BeNil)

			_, _, err = table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.ErrLike(`operator ":" not implemented for field "status" of type luci.common.v1.MyStatus`))
		})
	})
}
