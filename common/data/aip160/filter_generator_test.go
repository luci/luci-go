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

func TestWhereClause(t *testing.T) {
	ftt.Run("WhereClause", t, func(t *ftt.Test) {
		table := NewDatabaseTable().WithFields(
			NewField().WithFieldPath("foo").WithBackend(NewStringColumn("db_foo")).FilterableImplicitly().Build(),
			NewField().WithFieldPath("bar").WithBackend(NewStringColumn("db_bar")).FilterableImplicitly().Build(),
			NewField().WithFieldPath("baz").WithBackend(NewStringColumn("db_baz")).Filterable().Build(),
		).Build()

		t.Run("Empty filter", func(t *ftt.Test) {
			result, pars, err := table.WhereClause(&Filter{}, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pars, should.HaveLength(0))
			assert.Loosely(t, result, should.Equal("(TRUE)"))
		})

		t.Run("Complex filter", func(t *ftt.Test) {
			filter, err := ParseFilter("implicit (foo=\"explicitone\") OR -bar=\"explicittwo\" AND foo!=\"explicitthree\" OR baz:\"explicitfour\"")
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pars, should.Match([]SqlQueryParameter{
				{
					Name:  "p_0",
					Value: "%implicit%",
				},
				{
					Name:  "p_1",
					Value: "%implicit%",
				},
				{
					Name:  "p_2",
					Value: "explicitone",
				},
				{
					Name:  "p_3",
					Value: "explicittwo",
				},
				{
					Name:  "p_4",
					Value: "explicitthree",
				},
				{
					Name:  "p_5",
					Value: "%explicitfour%",
				},
			}))
			assert.Loosely(t, result, should.Equal("(((T.db_foo LIKE @p_0) OR (T.db_bar LIKE @p_1)) AND ((T.db_foo = @p_2) OR (NOT (T.db_bar = @p_3))) AND ((T.db_foo <> @p_4) OR (T.db_baz LIKE @p_5)))"))
		})
	})
}
