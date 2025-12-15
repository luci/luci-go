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

func TestKeyValueColumn(t *testing.T) {
	ftt.Run("KeyValueColumn", t, func(t *ftt.Test) {
		table := NewDatabaseTable().WithFields(
			NewField().WithFieldPath("kv").WithBackend(NewKeyValueColumn("db_kv").WithRepeatedStruct().Build()).Filterable().Build(),
			NewField().WithFieldPath("sakv").WithBackend(NewKeyValueColumn("db_sakv").WithStringArray().Build()).Filterable().Build(),
			NewField().WithFieldPath("foo").WithBackend(NewStringColumn("db_foo")).FilterableImplicitly().Build(),
		).Build()

		t.Run("key value contains operator", func(t *ftt.Test) {
			filter, err := ParseFilter(`kv.key:"somevalue"`)
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pars, should.Match([]SqlQueryParameter{
				{
					Name:  "p_0",
					Value: "key",
				},
				{
					Name:  "p_1",
					Value: "%somevalue%",
				},
			}))
			assert.Loosely(t, result, should.Equal("(EXISTS (SELECT _v.key, _v.value FROM UNNEST(T.db_kv) as _v WHERE _v.key = @p_0 AND _v.value LIKE @p_1))"))
		})

		t.Run("key value equal operator", func(t *ftt.Test) {
			filter, err := ParseFilter(`kv.key="somevalue"`)
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pars, should.Match([]SqlQueryParameter{
				{
					Name:  "p_0",
					Value: "key",
				},
				{
					Name:  "p_1",
					Value: "somevalue",
				},
			}))
			assert.Loosely(t, result, should.Equal("(EXISTS (SELECT _v.key, _v.value FROM UNNEST(T.db_kv) as _v WHERE _v.key = @p_0 AND _v.value = @p_1))"))
		})

		t.Run("key value not equal operator", func(t *ftt.Test) {
			filter, err := ParseFilter(`kv.key!="somevalue"`)
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pars, should.Match([]SqlQueryParameter{
				{
					Name:  "p_0",
					Value: "key",
				},
				{
					Name:  "p_1",
					Value: "somevalue",
				},
			}))
			assert.Loosely(t, result, should.Equal("(EXISTS (SELECT _v.key, _v.value FROM UNNEST(T.db_kv) as _v WHERE _v.key = @p_0 AND _v.value <> @p_1))"))
		})

		t.Run("key value missing key contains operator", func(t *ftt.Test) {
			filter, err := ParseFilter(`kv:"somevalue"`)
			assert.Loosely(t, err, should.BeNil)

			_, _, err = table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.ErrLike("key value columns must specify the key to search on"))
		})

		t.Run("string array key value contains operator", func(t *ftt.Test) {
			filter, err := ParseFilter(`sakv.key:"somevalue"`)
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pars, should.Match([]SqlQueryParameter{
				{
					Name:  "p_0",
					Value: "key:%somevalue%",
				},
			}))
			assert.Loosely(t, result, should.Equal("(EXISTS (SELECT 1 FROM UNNEST(T.db_sakv) as _v WHERE _v LIKE @p_0))"))
		})

		t.Run("string array key value equal operator", func(t *ftt.Test) {
			filter, err := ParseFilter(`sakv.key="somevalue"`)
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pars, should.Match([]SqlQueryParameter{
				{
					Name:  "p_0",
					Value: "key:somevalue",
				},
			}))
			assert.Loosely(t, result, should.Equal("(@p_0 IN UNNEST(T.db_sakv))"))
		})

		t.Run("string array key value not equal operator", func(t *ftt.Test) {
			filter, err := ParseFilter(`sakv.key!="somevalue"`)
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pars, should.Match([]SqlQueryParameter{
				{
					Name:  "p_0",
					Value: "key:somevalue",
				},
				{
					Name:  "p_1",
					Value: "key:",
				},
			}))
			assert.Loosely(t, result, should.Equal("(EXISTS (SELECT 1 FROM UNNEST(T.db_sakv) as _v WHERE STARTS_WITH(_v, @p_1) AND _v <> @p_0))"))
		})

		t.Run("string array key value missing key contains operator", func(t *ftt.Test) {
			filter, err := ParseFilter("sakv:somevalue")
			assert.Loosely(t, err, should.BeNil)

			_, _, err = table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.ErrLike("key value columns must specify the key to search on"))
		})

		t.Run("unsupported field LHS", func(t *ftt.Test) {
			filter, err := ParseFilter("foo.baz=blah")
			assert.Loosely(t, err, should.BeNil)

			_, _, err = table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.ErrLike("fields are only supported for key-value columns"))
		})

		t.Run("unsupported field RHS", func(t *ftt.Test) {
			filter, err := ParseFilter("foo=blah.baz")
			assert.Loosely(t, err, should.BeNil)

			_, _, err = table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.ErrLike(`argument for field "foo": expected a quoted ("") string literal but got possible field reference "blah.baz", did you mean to wrap the value in quotes?`))
		})
	})
}
