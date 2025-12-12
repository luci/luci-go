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

func TestOpaqueStringColumn(t *testing.T) {
	ftt.Run("OpaqueStringColumn", t, func(t *ftt.Test) {
		subFunc := func(sub string) string {
			if sub == "somevalue" {
				return "somevalue-v2"
			}
			return sub
		}
		table := NewDatabaseTable().WithFields(
			NewField().WithFieldPath("qux").WithBackend(NewOpaqueStringColumn("db_qux").WithEncodeFunction(subFunc).Build()).Filterable().Build(),
		).Build()

		t.Run("WithEncodeFunction filter substituted", func(t *ftt.Test) {
			filter, err := ParseFilter("qux=somevalue")
			assert.Loosely(t, err, should.BeNil)

			result, pars, err := table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, pars, should.Match([]SqlQueryParameter{
				{
					Name:  "p_0",
					Value: "somevalue-v2",
				},
			}))
			assert.Loosely(t, result, should.Equal("(T.db_qux = @p_0)"))
		})

		t.Run("WithArgumentSubstitutor filter not supported", func(t *ftt.Test) {
			filter, err := ParseFilter("qux:some")
			assert.Loosely(t, err, should.BeNil)

			_, _, err = table.WhereClause(filter, "T", "p_")
			assert.Loosely(t, err, should.ErrLike(`operator ":" not implemented for field "qux" of type OPAQUE STRING`))
		})
	})
}
