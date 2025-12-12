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

func TestStringColumn(t *testing.T) {
	ftt.Run("StringColumn", t, func(t *ftt.Test) {
		table := NewDatabaseTable().WithFields(
			NewField().WithFieldPath("foo").WithBackend(NewStringColumn("db_foo")).FilterableImplicitly().Build(),
			NewField().WithFieldPath("bar").WithBackend(NewStringColumn("db_bar")).FilterableImplicitly().Build(),
		).Build()

		t.Run("has operator", func(t *ftt.Test) {
			filter, err := ParseFilter("foo:somevalue")
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pars, should.Match([]SqlQueryParameter{
				{
					Name:  "p_0",
					Value: "%somevalue%",
				},
			}))
			assert.Loosely(t, result, should.Equal("(T.db_foo LIKE @p_0)"))
		})

		t.Run("equals operator", func(t *ftt.Test) {
			filter, err := ParseFilter("foo = somevalue")
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pars, should.Match([]SqlQueryParameter{
				{
					Name:  "p_0",
					Value: "somevalue",
				},
			}))
			assert.Loosely(t, result, should.Equal("(T.db_foo = @p_0)"))
		})

		t.Run("not equals operator", func(t *ftt.Test) {
			filter, err := ParseFilter("foo != somevalue")
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pars, should.Match([]SqlQueryParameter{
				{
					Name:  "p_0",
					Value: "somevalue",
				},
			}))
			assert.Loosely(t, result, should.Equal("(T.db_foo <> @p_0)"))
		})

		t.Run("implicit match operator", func(t *ftt.Test) {
			filter, err := ParseFilter("somevalue")
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pars, should.Match([]SqlQueryParameter{
				{
					Name:  "p_0",
					Value: "%somevalue%",
				},
				{
					Name:  "p_1",
					Value: "%somevalue%",
				},
			}))
			assert.Loosely(t, result, should.Equal("((T.db_foo LIKE @p_0) OR (T.db_bar LIKE @p_1))"))
		})

		t.Run("unsupported composite to LIKE", func(t *ftt.Test) {
			filter, err := ParseFilter("foo:(somevalue)")
			assert.Loosely(t, err, should.BeNil)

			_, _, err = table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.ErrLike(`argument for field "foo": composite expressions in arguments not supported yet`))
		})

		t.Run("unsupported composite to equals", func(t *ftt.Test) {
			filter, err := ParseFilter("foo=(somevalue)")
			assert.Loosely(t, err, should.BeNil)

			_, _, err = table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.ErrLike(`argument for field "foo": composite expressions in arguments not supported yet`))
		})
	})
}
