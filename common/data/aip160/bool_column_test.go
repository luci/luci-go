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

func TestBoolColumn(t *testing.T) {
	ftt.Run("BoolColumn", t, func(t *ftt.Test) {
		table := NewDatabaseTable().WithFields(
			NewField().WithFieldPath("bool").WithBackend(NewBoolColumn("db_bool")).Filterable().Build(),
		).Build()

		t.Run("equals operator", func(t *ftt.Test) {
			filter, err := ParseFilter("bool = true AND bool = false")
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pars, should.BeNil)
			assert.Loosely(t, result, should.Equal("((T.db_bool = TRUE) AND (T.db_bool = FALSE))"))
		})

		t.Run("not equals operator", func(t *ftt.Test) {
			filter, err := ParseFilter("bool != true AND bool != false")
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pars, should.BeNil)
			assert.Loosely(t, result, should.Equal("((T.db_bool <> TRUE) AND (T.db_bool <> FALSE))"))
		})

		t.Run("argument is invalid", func(t *ftt.Test) {
			filter, err := ParseFilter("bool = a.b")
			assert.Loosely(t, err, should.BeNil)

			_, _, err = table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.ErrLike(`argument for field "bool": field navigation (using '.') is not supported`))
		})

		t.Run("operator not implemented", func(t *ftt.Test) {
			filter, err := ParseFilter("bool > true")
			assert.Loosely(t, err, should.BeNil)

			_, _, err = table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.ErrLike(`operator ">" not implemented for field "bool" of type BOOL`))
		})
	})
}
