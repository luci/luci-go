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

package testresultsv2

import (
	"testing"

	"go.chromium.org/luci/common/data/aip160"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTestIDColumn(t *testing.T) {
	ftt.Run("TestIDColumn", t, func(t *ftt.Test) {
		table := aip160.NewDatabaseTable().WithFields(
			aip160.NewField().WithFieldPath("test_id").WithBackend(
				newFlatTestIDFieldBackend(),
			).Filterable().Build(),
		).Build()

		t.Run("equals operator", func(t *ftt.Test) {
			filter, err := aip160.ParseFilter(`test_id = ":module!scheme:coarse_name:fine_name#case_name"`)
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal("(ModuleName = @p_0 AND ModuleScheme = @p_1 AND T1CoarseName = @p_2 AND T2FineName = @p_3 AND T3CaseName = @p_4)"))
			assert.Loosely(t, pars, should.Match([]aip160.SqlQueryParameter{
				{
					Name:  "p_0",
					Value: "module",
				},
				{
					Name:  "p_1",
					Value: "scheme",
				},
				{
					Name:  "p_2",
					Value: "coarse_name",
				},
				{
					Name:  "p_3",
					Value: "fine_name",
				},
				{
					Name:  "p_4",
					Value: "case_name",
				},
			}))
		})

		t.Run("not equals operator", func(t *ftt.Test) {
			filter, err := aip160.ParseFilter(`test_id != "legacy_id"`)
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal("(NOT (ModuleName = @p_0 AND ModuleScheme = @p_1 AND T1CoarseName = @p_2 AND T2FineName = @p_3 AND T3CaseName = @p_4))"))
			assert.Loosely(t, pars, should.Match([]aip160.SqlQueryParameter{
				{
					Name:  "p_0",
					Value: "legacy",
				},
				{
					Name:  "p_1",
					Value: "legacy",
				},
				{
					Name:  "p_2",
					Value: "",
				},
				{
					Name:  "p_3",
					Value: "",
				},
				{
					Name:  "p_4",
					Value: "legacy_id",
				},
			}))
		})

		t.Run("argument other than constant string used", func(t *ftt.Test) {
			filter, err := aip160.ParseFilter(`test_id = 5`)
			assert.Loosely(t, err, should.BeNil)

			_, _, err = table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.ErrLike(`argument for field "test_id": expected a quoted ("") string literal but got possible field reference "5", did you mean to wrap the value in quotes?`))
		})
		t.Run("invalid test ID used", func(t *ftt.Test) {
			filter, err := aip160.ParseFilter(`test_id = ":!invalid"`)
			assert.Loosely(t, err, should.BeNil)

			_, _, err = table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.ErrLike(`argument for field "test_id": unexpected end of string at byte 9, expected delimiter ':' (test ID pattern is :module!scheme:coarse:fine#case)`))
		})

		t.Run("unsupported operator used", func(t *ftt.Test) {
			filter, err := aip160.ParseFilter(`test_id:"coarse_name:fine_name"`)
			assert.Loosely(t, err, should.BeNil)

			_, _, err = table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.ErrLike(`operator ":" not implemented for field "test_id" of type TEST_ID`))
		})
	})
}
