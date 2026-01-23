// Copyright 2026 The LUCI Authors.
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

package spanutil

import (
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestWhereAfterClause(t *testing.T) {
	ftt.Run("WhereAfterClause", t, func(t *ftt.Test) {
		t.Run("Empty", func(t *ftt.Test) {
			params := make(map[string]any)
			clause := WhereAfterClause(nil, "p_", params)
			assert.Loosely(t, clause, should.Equal("(FALSE)"))
			assert.Loosely(t, params, should.BeEmpty)
		})

		t.Run("Single Column", func(t *ftt.Test) {
			params := make(map[string]any)
			token := []PageTokenElement{
				{ColumnName: "A", AfterValue: 1},
			}
			clause := WhereAfterClause(token, "p_", params)
			assert.Loosely(t, clause, should.Equal("((A > @p_A))"))
			assert.Loosely(t, params, should.Match(map[string]any{"p_A": 1}))
		})

		t.Run("Multiple Columns", func(t *ftt.Test) {
			params := make(map[string]any)
			token := []PageTokenElement{
				{ColumnName: "A", AfterValue: 1},
				{ColumnName: "B", AfterValue: "b"},
				{ColumnName: "C", AfterValue: 3.14},
			}
			clause := WhereAfterClause(token, "p_", params)
			expected := `((A > @p_A)
 OR (A = @p_A AND B > @p_B)
 OR (A = @p_A AND B = @p_B AND C > @p_C))`
			assert.Loosely(t, clause, should.Equal(expected))
			assert.Loosely(t, params, should.Match(map[string]any{
				"p_A": 1,
				"p_B": "b",
				"p_C": 3.14,
			}))
		})

		t.Run("With AtLimit", func(t *ftt.Test) {
			params := make(map[string]any)
			token := []PageTokenElement{
				{ColumnName: "A", AfterValue: 1, AtLimit: true},
				{ColumnName: "B", AfterValue: "b"},
				{ColumnName: "C", AfterValue: 3.14},
			}
			clause := WhereAfterClause(token, "p_", params)
			// A is at limit, so we skip (A > @p_A).
			// We start with B.
			expected := `(A = @p_A AND ((B > @p_B)
 OR (B = @p_B AND C > @p_C)))`
			assert.Loosely(t, clause, should.Equal(expected))
			assert.Loosely(t, params, should.Match(map[string]any{
				"p_A": 1,
				"p_B": "b",
				"p_C": 3.14,
			}))
		})

		t.Run("All AtLimit", func(t *ftt.Test) {
			params := make(map[string]any)
			token := []PageTokenElement{
				{ColumnName: "A", AfterValue: 1, AtLimit: true},
				{ColumnName: "B", AfterValue: "b", AtLimit: true},
			}
			clause := WhereAfterClause(token, "p_", params)
			assert.Loosely(t, clause, should.Equal("(FALSE)"))
			assert.Loosely(t, params, should.BeEmpty)
		})
	})
}
