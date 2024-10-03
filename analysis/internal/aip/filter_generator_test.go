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

package aip

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestWhereClause(t *testing.T) {
	ftt.Run("WhereClause", t, func(t *ftt.Test) {
		subFunc := func(sub string) string {
			if sub == "somevalue" {
				return "somevalue-v2"
			}
			return sub
		}
		table := NewTable().WithColumns(
			NewColumn().WithFieldPath("foo").WithDatabaseName("db_foo").FilterableImplicitly().Build(),
			NewColumn().WithFieldPath("bar").WithDatabaseName("db_bar").FilterableImplicitly().Build(),
			NewColumn().WithFieldPath("baz").WithDatabaseName("db_baz").Filterable().Build(),
			NewColumn().WithFieldPath("kv").WithDatabaseName("db_kv").KeyValue().Filterable().Build(),
			NewColumn().WithFieldPath("array").WithDatabaseName("db_array").Array().Filterable().Build(),
			NewColumn().WithFieldPath("bool").WithDatabaseName("db_bool").Bool().Filterable().Build(),
			NewColumn().WithFieldPath("unfilterable").WithDatabaseName("unfilterable").Build(),
			NewColumn().WithFieldPath("qux").WithDatabaseName("db_qux").WithArgumentSubstitutor(subFunc).Filterable().Build(),
			NewColumn().WithFieldPath("quux").WithDatabaseName("db_quux").WithArgumentSubstitutor(subFunc).Filterable().KeyValue().Build(),
		).Build()

		t.Run("Empty filter", func(t *ftt.Test) {
			result, pars, err := table.WhereClause(&Filter{}, "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pars, should.HaveLength(0))
			assert.Loosely(t, result, should.Equal("(TRUE)"))
		})
		t.Run("Simple filter", func(t *ftt.Test) {
			t.Run("has operator", func(t *ftt.Test) {
				filter, err := ParseFilter("foo:somevalue")
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "p_")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pars, should.Resemble([]QueryParameter{
					{
						Name:  "p_0",
						Value: "%somevalue%",
					},
				}))
				assert.Loosely(t, result, should.Equal("(db_foo LIKE @p_0)"))
			})
			t.Run("equals operator", func(t *ftt.Test) {
				filter, err := ParseFilter("foo = somevalue")
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "p_")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pars, should.Resemble([]QueryParameter{
					{
						Name:  "p_0",
						Value: "somevalue",
					},
				}))
				assert.Loosely(t, result, should.Equal("(db_foo = @p_0)"))
			})
			t.Run("equals operator on bool column", func(t *ftt.Test) {
				filter, err := ParseFilter("bool = true AND bool = false")
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "p_")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pars, should.BeNil)
				assert.Loosely(t, result, should.Equal("((db_bool = TRUE) AND (db_bool = FALSE))"))
			})
			t.Run("not equals operator", func(t *ftt.Test) {
				filter, err := ParseFilter("foo != somevalue")
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "p_")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pars, should.Resemble([]QueryParameter{
					{
						Name:  "p_0",
						Value: "somevalue",
					},
				}))
				assert.Loosely(t, result, should.Equal("(db_foo <> @p_0)"))
			})
			t.Run("not equals operator on bool column", func(t *ftt.Test) {
				filter, err := ParseFilter("bool != true AND bool != false")
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "p_")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pars, should.BeNil)
				assert.Loosely(t, result, should.Equal("((db_bool <> TRUE) AND (db_bool <> FALSE))"))
			})
			t.Run("implicit match operator", func(t *ftt.Test) {
				filter, err := ParseFilter("somevalue")
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "p_")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pars, should.Resemble([]QueryParameter{
					{
						Name:  "p_0",
						Value: "%somevalue%",
					},
				}))
				assert.Loosely(t, result, should.Equal("(db_foo LIKE @p_0 OR db_bar LIKE @p_0)"))
			})
			t.Run("key value contains operator", func(t *ftt.Test) {
				filter, err := ParseFilter("kv.key:somevalue")
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "p_")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pars, should.Resemble([]QueryParameter{
					{
						Name:  "p_0",
						Value: "key",
					},
					{
						Name:  "p_1",
						Value: "%somevalue%",
					},
				}))
				assert.Loosely(t, result, should.Equal("(EXISTS (SELECT key, value FROM UNNEST(db_kv) WHERE key = @p_0 AND value LIKE @p_1))"))
			})
			t.Run("key value equal operator", func(t *ftt.Test) {
				filter, err := ParseFilter("kv.key=somevalue")
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "p_")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pars, should.Resemble([]QueryParameter{
					{
						Name:  "p_0",
						Value: "key",
					},
					{
						Name:  "p_1",
						Value: "somevalue",
					},
				}))
				assert.Loosely(t, result, should.Equal("(EXISTS (SELECT key, value FROM UNNEST(db_kv) WHERE key = @p_0 AND value = @p_1))"))
			})
			t.Run("key value not equal operator", func(t *ftt.Test) {
				filter, err := ParseFilter("kv.key!=somevalue")
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "p_")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pars, should.Resemble([]QueryParameter{
					{
						Name:  "p_0",
						Value: "key",
					},
					{
						Name:  "p_1",
						Value: "somevalue",
					},
				}))
				assert.Loosely(t, result, should.Equal("(EXISTS (SELECT key, value FROM UNNEST(db_kv) WHERE key = @p_0 AND value <> @p_1))"))
			})
			t.Run("key value missing key contains operator", func(t *ftt.Test) {
				filter, err := ParseFilter("kv:somevalue")
				assert.Loosely(t, err, should.BeNil)

				_, _, err = table.WhereClause(filter, "p_")
				assert.Loosely(t, err, should.ErrLike("key value columns must specify the key to search on"))
			})
			t.Run("array contains operator", func(t *ftt.Test) {
				filter, err := ParseFilter("array:somevalue")
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "p_")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pars, should.Resemble([]QueryParameter{
					{
						Name:  "p_0",
						Value: "somevalue",
					},
				}))
				assert.Loosely(t, result, should.Equal("(EXISTS (SELECT value FROM UNNEST(db_array) as value WHERE value LIKE @p_0))"))
			})
			t.Run("unsupported composite to LIKE", func(t *ftt.Test) {
				filter, err := ParseFilter("foo:(somevalue)")
				assert.Loosely(t, err, should.BeNil)

				_, _, err = table.WhereClause(filter, "p_")
				assert.Loosely(t, err, should.ErrLike("composite expressions are not allowed as RHS to has (:) operator"))
			})
			t.Run("unsupported composite to equals", func(t *ftt.Test) {
				filter, err := ParseFilter("foo=(somevalue)")
				assert.Loosely(t, err, should.BeNil)

				_, _, err = table.WhereClause(filter, "p_")
				assert.Loosely(t, err, should.ErrLike("composite expressions in arguments not implemented yet"))
			})
			t.Run("unsupported field LHS", func(t *ftt.Test) {
				filter, err := ParseFilter("foo.baz=blah")
				assert.Loosely(t, err, should.BeNil)

				_, _, err = table.WhereClause(filter, "p_")
				assert.Loosely(t, err, should.ErrLike("fields are only supported for key value columns"))
			})
			t.Run("unsupported field RHS", func(t *ftt.Test) {
				filter, err := ParseFilter("foo=blah.baz")
				assert.Loosely(t, err, should.BeNil)

				_, _, err = table.WhereClause(filter, "p_")
				assert.Loosely(t, err, should.ErrLike("fields not implemented yet"))
			})
			t.Run("field on RHS of has", func(t *ftt.Test) {
				filter, err := ParseFilter("foo:blah.baz")
				assert.Loosely(t, err, should.BeNil)

				_, _, err = table.WhereClause(filter, "p_")
				assert.Loosely(t, err, should.ErrLike("fields are not allowed on the RHS of has (:) operator"))
			})
			t.Run("WithArgumentSubstitutor filter substituted", func(t *ftt.Test) {
				filter, err := ParseFilter("qux=somevalue")
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "p_")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pars, should.Resemble([]QueryParameter{
					{
						Name:  "p_0",
						Value: "somevalue-v2",
					},
				}))
				assert.Loosely(t, result, should.Equal("(db_qux = @p_0)"))
			})
			t.Run("WithArgumentSubstitutor filter not supported", func(t *ftt.Test) {
				filter, err := ParseFilter("qux:some")
				assert.Loosely(t, err, should.BeNil)

				_, _, err = table.WhereClause(filter, "p_")
				assert.Loosely(t, err, should.ErrLike("cannot use has (:) operator on a field that have argSubstitute function"))
			})
			t.Run("WithArgumentSubstitutor filter key value", func(t *ftt.Test) {
				filter, err := ParseFilter("quux.somekey=somevalue")
				assert.Loosely(t, err, should.BeNil)

				result, pars, err := table.WhereClause(filter, "p_")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, pars, should.Resemble([]QueryParameter{
					{
						Name:  "p_0",
						Value: "somekey",
					},
					{
						Name:  "p_1",
						Value: "somevalue-v2",
					},
				}))
				assert.Loosely(t, result, should.Equal("(EXISTS (SELECT key, value FROM UNNEST(db_quux) WHERE key = @p_0 AND value = @p_1))"))
			})
		})
		t.Run("Complex filter", func(t *ftt.Test) {
			filter, err := ParseFilter("implicit (foo=explicitone) OR -bar=explicittwo AND foo!=explicitthree OR baz:explicitfour")
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pars, should.Resemble([]QueryParameter{
				{
					Name:  "p_0",
					Value: "%implicit%",
				},
				{
					Name:  "p_1",
					Value: "explicitone",
				},
				{
					Name:  "p_2",
					Value: "explicittwo",
				},
				{
					Name:  "p_3",
					Value: "explicitthree",
				},
				{
					Name:  "p_4",
					Value: "%explicitfour%",
				},
			}))
			assert.Loosely(t, result, should.Equal("((db_foo LIKE @p_0 OR db_bar LIKE @p_0) AND ((db_foo = @p_1) OR (NOT (db_bar = @p_2))) AND ((db_foo <> @p_3) OR (db_baz LIKE @p_4)))"))
		})
	})
}
