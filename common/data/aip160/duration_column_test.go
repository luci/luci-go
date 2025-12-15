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

package aip160

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestDurationColumn(t *testing.T) {
	ftt.Run("DurationColumn", t, func(t *ftt.Test) {
		table := NewDatabaseTable().WithFields(
			NewField().WithFieldPath("duration").WithBackend(
				NewDurationColumn("duration_col").Build(),
			).Filterable().Sortable().Build(),
		).Build()

		t.Run("RestrictionQuery", func(t *ftt.Test) {
			t.Run("valid equality", func(t *ftt.Test) {
				filter, err := ParseFilter("duration = 20s")
				assert.Loosely(t, err, should.BeNil)

				sql, args, err := table.WhereClause(filter, "t", "p_")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, sql, should.Equal("(t.duration_col = 20000000000)"))
				assert.Loosely(t, args, should.BeEmpty)
			})

			t.Run("valid inequality", func(t *ftt.Test) {
				filter, err := ParseFilter(`duration != 1.5s`)
				assert.Loosely(t, err, should.BeNil)

				sql, args, err := table.WhereClause(filter, "t", "p_")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, sql, should.Equal("(t.duration_col != 1500000000)"))
				assert.Loosely(t, args, should.BeEmpty)
			})

			t.Run("valid less than", func(t *ftt.Test) {
				filter, err := ParseFilter(`duration<0.000000001s`)
				assert.Loosely(t, err, should.BeNil)

				sql, args, err := table.WhereClause(filter, "t", "p_")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, sql, should.Equal("(t.duration_col < 1)"))
				assert.Loosely(t, args, should.BeEmpty)
			})

			t.Run("invalid duration format (no s)", func(t *ftt.Test) {
				filter, err := ParseFilter(`duration = 20`)
				assert.Loosely(t, err, should.BeNil)

				_, _, err = table.WhereClause(filter, "t", "p_")
				assert.Loosely(t, err, should.ErrLike(`argument for field "duration": "20" is not a valid duration, expected a number followed by 's' (e.g. "20.1s")`))
			})

			t.Run("invalid duration format (not a number)", func(t *ftt.Test) {
				filter, err := ParseFilter(`duration = "foos"`)
				assert.Loosely(t, err, should.BeNil)

				_, _, err = table.WhereClause(filter, "t", "p_")
				assert.Loosely(t, err, should.ErrLike(`argument for field "duration": durations must be an unquoted number with 's' suffix like 1.2s but got a quoted string "foos"`))
			})

			t.Run("unsupported operator", func(t *ftt.Test) {
				filter, err := ParseFilter(`duration: 20s`)
				assert.Loosely(t, err, should.BeNil)

				_, _, err = table.WhereClause(filter, "t", "p_")
				assert.Loosely(t, err, should.ErrLike("operator \":\" not implemented for field \"duration\" of type DURATION"))
			})
		})
	})
}
