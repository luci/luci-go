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

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestOrderByClause(t *testing.T) {
	Convey("OrderByClause", t, func() {
		table := NewTable().WithColumns(
			NewColumn().WithFieldPath("foo").WithDatabaseName("db_foo").Sortable().Build(),
			NewColumn().WithFieldPath("bar").WithDatabaseName("db_bar").Sortable().Build(),
			NewColumn().WithFieldPath("baz").WithDatabaseName("db_baz").Sortable().Build(),
			NewColumn().WithFieldPath("unsortable").WithDatabaseName("unsortable").Build(),
		).Build()

		Convey("Empty order by", func() {
			result, err := table.OrderByClause([]OrderBy{})
			So(err, ShouldBeNil)
			So(result, ShouldEqual, "")
		})
		Convey("Single order by", func() {
			result, err := table.OrderByClause([]OrderBy{
				{
					FieldPath: NewFieldPath("foo"),
				},
			})
			So(err, ShouldBeNil)
			So(result, ShouldEqual, "ORDER BY db_foo\n")
		})
		Convey("Multiple order by", func() {
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
			So(err, ShouldBeNil)
			So(result, ShouldEqual, "ORDER BY db_foo DESC, db_bar, db_baz DESC\n")
		})
		Convey("Unsortable field in order by", func() {
			_, err := table.OrderByClause([]OrderBy{
				{
					FieldPath:  NewFieldPath("unsortable"),
					Descending: true,
				},
			})
			So(err, ShouldErrLike, `no sortable field named "unsortable", valid fields are foo, bar, baz`)
		})
		Convey("Repeated field in order by", func() {
			_, err := table.OrderByClause([]OrderBy{
				{
					FieldPath: NewFieldPath("foo"),
				},
				{
					FieldPath: NewFieldPath("foo"),
				},
			})
			So(err, ShouldErrLike, `field appears in order_by multiple times: "foo"`)
		})
	})
}

func TestMergeWithDefaultOrder(t *testing.T) {
	Convey("MergeWithDefaultOrder", t, func() {
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
		Convey("Empty order", func() {
			result := MergeWithDefaultOrder(defaultOrder, nil)
			So(result, ShouldResemble, defaultOrder)
		})
		Convey("Non-empty order", func() {
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
			So(result, ShouldResemble, []OrderBy{
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
			})
		})
	})
}
