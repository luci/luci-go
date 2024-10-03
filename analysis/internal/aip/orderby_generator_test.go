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

func TestOrderByClause(t *testing.T) {
	ftt.Run("OrderByClause", t, func(t *ftt.Test) {
		table := NewTable().WithColumns(
			NewColumn().WithFieldPath("foo").WithDatabaseName("db_foo").Sortable().Build(),
			NewColumn().WithFieldPath("bar").WithDatabaseName("db_bar").Sortable().Build(),
			NewColumn().WithFieldPath("baz").WithDatabaseName("db_baz").Sortable().Build(),
			NewColumn().WithFieldPath("unsortable").WithDatabaseName("unsortable").Build(),
		).Build()

		t.Run("Empty order by", func(t *ftt.Test) {
			result, err := table.OrderByClause([]OrderBy{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.BeEmpty)
		})
		t.Run("Single order by", func(t *ftt.Test) {
			result, err := table.OrderByClause([]OrderBy{
				{
					FieldPath: NewFieldPath("foo"),
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal("ORDER BY db_foo\n"))
		})
		t.Run("Multiple order by", func(t *ftt.Test) {
			result, err := table.OrderByClause([]OrderBy{
				{
					FieldPath:  NewFieldPath("foo"),
					Descending: true,
				},
				{
					FieldPath: NewFieldPath("bar"),
				},
				{
					FieldPath:  NewFieldPath("baz"),
					Descending: true,
				},
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, result, should.Equal("ORDER BY db_foo DESC, db_bar, db_baz DESC\n"))
		})
		t.Run("Unsortable field in order by", func(t *ftt.Test) {
			_, err := table.OrderByClause([]OrderBy{
				{
					FieldPath:  NewFieldPath("unsortable"),
					Descending: true,
				},
			})
			assert.Loosely(t, err, should.ErrLike(`no sortable field named "unsortable", valid fields are foo, bar, baz`))
		})
		t.Run("Repeated field in order by", func(t *ftt.Test) {
			_, err := table.OrderByClause([]OrderBy{
				{
					FieldPath: NewFieldPath("foo"),
				},
				{
					FieldPath: NewFieldPath("foo"),
				},
			})
			assert.Loosely(t, err, should.ErrLike(`field appears in order_by multiple times: "foo"`))
		})
	})
}

func TestMergeWithDefaultOrder(t *testing.T) {
	ftt.Run("MergeWithDefaultOrder", t, func(t *ftt.Test) {
		defaultOrder := []OrderBy{
			{
				FieldPath:  NewFieldPath("foo"),
				Descending: true,
			}, {
				FieldPath: NewFieldPath("bar"),
			}, {
				FieldPath:  NewFieldPath("baz"),
				Descending: true,
			},
		}
		t.Run("Empty order", func(t *ftt.Test) {
			result := MergeWithDefaultOrder(defaultOrder, nil)
			assert.Loosely(t, result, should.Resemble(defaultOrder))
		})
		t.Run("Non-empty order", func(t *ftt.Test) {
			order := []OrderBy{
				{
					FieldPath:  NewFieldPath("other"),
					Descending: true,
				},
				{
					FieldPath: NewFieldPath("baz"),
				},
			}
			result := MergeWithDefaultOrder(defaultOrder, order)
			assert.Loosely(t, result, should.Resemble([]OrderBy{
				{
					FieldPath:  NewFieldPath("other"),
					Descending: true,
				},
				{
					FieldPath: NewFieldPath("baz"),
				},
				{
					FieldPath:  NewFieldPath("foo"),
					Descending: true,
				}, {
					FieldPath: NewFieldPath("bar"),
				},
			}))
		})
	})
}
