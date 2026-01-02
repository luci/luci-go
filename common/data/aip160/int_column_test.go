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

func TestIntegerColumn(t *testing.T) {
	ftt.Run("IntegerColumn", t, func(t *ftt.Test) {
		table := NewDatabaseTable().WithFields(
			NewField().WithFieldPath("int").WithBackend(
				NewIntegerColumn("int_col"),
			).Filterable().Sortable().Build(),
		).Build()

		t.Run("RestrictionQuery", func(t *ftt.Test) {
			t.Run("valid equality", func(t *ftt.Test) {
				filter, err := ParseFilter("int = 123")
				assert.Loosely(t, err, should.BeNil)

				sql, args, err := table.WhereClause(filter, "t", "p_")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, sql, should.Equal("(t.int_col = 123)"))
				assert.Loosely(t, args, should.BeEmpty)
			})

			t.Run("valid inequality", func(t *ftt.Test) {
				filter, err := ParseFilter(`int != 456`)
				assert.Loosely(t, err, should.BeNil)

				sql, args, err := table.WhereClause(filter, "t", "p_")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, sql, should.Equal("(t.int_col <> 456)"))
				assert.Loosely(t, args, should.BeEmpty)
			})

			t.Run("valid comparison operators", func(t *ftt.Test) {
				operators := []string{">", "<", ">=", "<="}
				for _, op := range operators {
					t.Run(op, func(t *ftt.Test) {
						filter, err := ParseFilter("int " + op + " 789")
						assert.Loosely(t, err, should.BeNil)

						sql, args, err := table.WhereClause(filter, "t", "p_")
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, sql, should.Equal("(t.int_col "+op+" 789)"))
						assert.Loosely(t, args, should.BeEmpty)
					})
				}
			})

			t.Run("invalid argument (quoted string)", func(t *ftt.Test) {
				filter, err := ParseFilter(`int = "123"`)
				assert.Loosely(t, err, should.BeNil)

				_, _, err = table.WhereClause(filter, "t", "p_")
				assert.Loosely(t, err, should.ErrLike(`argument for field "int": expected an unquoted integer literal but found double-quoted string "123"`))
			})

			t.Run("invalid argument (field navigation)", func(t *ftt.Test) {
				filter, err := ParseFilter(`int = a.b`)
				assert.Loosely(t, err, should.BeNil)

				_, _, err = table.WhereClause(filter, "t", "p_")
				assert.Loosely(t, err, should.ErrLike(`argument for field "int": field navigation (using '.') is not supported`))
			})

			t.Run("invalid argument (not an integer)", func(t *ftt.Test) {
				filter, err := ParseFilter(`int = abc`)
				assert.Loosely(t, err, should.BeNil)

				_, _, err = table.WhereClause(filter, "t", "p_")
				assert.Loosely(t, err, should.ErrLike(`argument for field "int": expected an integer literal but found "abc"`))
			})

			t.Run("unsupported operator", func(t *ftt.Test) {
				filter, err := ParseFilter(`int: 123`)
				assert.Loosely(t, err, should.BeNil)

				_, _, err = table.WhereClause(filter, "t", "p_")
				assert.Loosely(t, err, should.ErrLike(`operator ":" not implemented for field "int" of type INTEGER`))
			})
		})
	})
}
