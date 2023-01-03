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

func TestWhereClause(t *testing.T) {
	Convey("WhereClause", t, func() {
		table := NewTable().WithColumns(
			NewColumn().WithFieldPath("foo").WithDatabaseName("db_foo").FilterableImplicitly().Build(),
			NewColumn().WithFieldPath("bar").WithDatabaseName("db_bar").FilterableImplicitly().Build(),
			NewColumn().WithFieldPath("baz").WithDatabaseName("db_baz").Filterable().Build(),
			NewColumn().WithFieldPath("kv").WithDatabaseName("db_kv").KeyValue().Filterable().Build(),
			NewColumn().WithFieldPath("bool").WithDatabaseName("db_bool").Bool().Filterable().Build(),
			NewColumn().WithFieldPath("unfilterable").WithDatabaseName("unfilterable").Build(),
		).Build()

		Convey("Empty filter", func() {
			result, pars, err := table.WhereClause(&Filter{}, "p_")
			So(err, ShouldBeNil)
			So(pars, ShouldHaveLength, 0)
			So(result, ShouldEqual, "(TRUE)")
		})
		Convey("Simple filter", func() {
			Convey("has operator", func() {
				filter, err := ParseFilter("foo:somevalue")
				So(err, ShouldEqual, nil)

				result, pars, err := table.WhereClause(filter, "p_")
				So(err, ShouldBeNil)
				So(pars, ShouldResemble, []QueryParameter{
					{
						Name:  "p_0",
						Value: "%somevalue%",
					},
				})
				So(result, ShouldEqual, "(db_foo LIKE @p_0)")
			})
			Convey("equals operator", func() {
				filter, err := ParseFilter("foo = somevalue")
				So(err, ShouldEqual, nil)

				result, pars, err := table.WhereClause(filter, "p_")
				So(err, ShouldBeNil)
				So(pars, ShouldResemble, []QueryParameter{
					{
						Name:  "p_0",
						Value: "somevalue",
					},
				})
				So(result, ShouldEqual, "(COALESCE(db_foo, '') = @p_0)")
			})
			Convey("equals operator on bool column", func() {
				filter, err := ParseFilter("bool = true AND bool = false")
				So(err, ShouldEqual, nil)

				result, pars, err := table.WhereClause(filter, "p_")
				So(err, ShouldBeNil)
				So(pars, ShouldBeNil)
				So(result, ShouldEqual, "((COALESCE(db_bool, FALSE) = TRUE) AND (COALESCE(db_bool, FALSE) = FALSE))")
			})
			Convey("not equals operator", func() {
				filter, err := ParseFilter("foo != somevalue")
				So(err, ShouldEqual, nil)

				result, pars, err := table.WhereClause(filter, "p_")
				So(err, ShouldBeNil)
				So(pars, ShouldResemble, []QueryParameter{
					{
						Name:  "p_0",
						Value: "somevalue",
					},
				})
				So(result, ShouldEqual, "(COALESCE(db_foo, '') <> @p_0)")
			})
			Convey("not equals operator on bool column", func() {
				filter, err := ParseFilter("bool != true AND bool != false")
				So(err, ShouldEqual, nil)

				result, pars, err := table.WhereClause(filter, "p_")
				So(err, ShouldBeNil)
				So(pars, ShouldBeNil)
				So(result, ShouldEqual, "((COALESCE(db_bool, FALSE) <> TRUE) AND (COALESCE(db_bool, FALSE) <> FALSE))")
			})
			Convey("implicit match operator", func() {
				filter, err := ParseFilter("somevalue")
				So(err, ShouldEqual, nil)

				result, pars, err := table.WhereClause(filter, "p_")
				So(err, ShouldBeNil)
				So(pars, ShouldResemble, []QueryParameter{
					{
						Name:  "p_0",
						Value: "%somevalue%",
					},
				})
				So(result, ShouldEqual, "(db_foo LIKE @p_0 OR db_bar LIKE @p_0)")
			})
			Convey("key value contains operator", func() {
				filter, err := ParseFilter("kv.key:somevalue")
				So(err, ShouldEqual, nil)

				result, pars, err := table.WhereClause(filter, "p_")
				So(err, ShouldBeNil)
				So(pars, ShouldResemble, []QueryParameter{
					{
						Name:  "p_0",
						Value: "key",
					},
					{
						Name:  "p_1",
						Value: "%somevalue%",
					},
				})
				So(result, ShouldEqual, "(EXISTS (SELECT key, value FROM UNNEST(db_kv) WHERE key = @p_0 AND value LIKE @p_1))")
			})
			Convey("key value equal operator", func() {
				filter, err := ParseFilter("kv.key=somevalue")
				So(err, ShouldEqual, nil)

				result, pars, err := table.WhereClause(filter, "p_")
				So(err, ShouldBeNil)
				So(pars, ShouldResemble, []QueryParameter{
					{
						Name:  "p_0",
						Value: "key",
					},
					{
						Name:  "p_1",
						Value: "somevalue",
					},
				})
				So(result, ShouldEqual, "(EXISTS (SELECT key, value FROM UNNEST(db_kv) WHERE key = @p_0 AND value = @p_1))")
			})
			Convey("key value not equal operator", func() {
				filter, err := ParseFilter("kv.key!=somevalue")
				So(err, ShouldEqual, nil)

				result, pars, err := table.WhereClause(filter, "p_")
				So(err, ShouldBeNil)
				So(pars, ShouldResemble, []QueryParameter{
					{
						Name:  "p_0",
						Value: "key",
					},
					{
						Name:  "p_1",
						Value: "somevalue",
					},
				})
				So(result, ShouldEqual, "(EXISTS (SELECT key, value FROM UNNEST(db_kv) WHERE key = @p_0 AND value <> @p_1))")
			})
			Convey("unsupported composite to LIKE", func() {
				filter, err := ParseFilter("foo:(somevalue)")
				So(err, ShouldEqual, nil)

				_, _, err = table.WhereClause(filter, "p_")
				So(err, ShouldErrLike, "composite expressions are not allowed as RHS to has (:) operator")
			})
			Convey("unsupported composite to equals", func() {
				filter, err := ParseFilter("foo=(somevalue)")
				So(err, ShouldEqual, nil)

				_, _, err = table.WhereClause(filter, "p_")
				So(err, ShouldErrLike, "composite expressions in arguments not implemented yet")
			})
			Convey("unsupported field LHS", func() {
				filter, err := ParseFilter("foo.baz=blah")
				So(err, ShouldEqual, nil)

				_, _, err = table.WhereClause(filter, "p_")
				So(err, ShouldErrLike, "fields are only supported for key value columns")
			})
			Convey("unsupported field RHS", func() {
				filter, err := ParseFilter("foo=blah.baz")
				So(err, ShouldEqual, nil)

				_, _, err = table.WhereClause(filter, "p_")
				So(err, ShouldErrLike, "fields not implemented yet")
			})
			Convey("field on RHS of has", func() {
				filter, err := ParseFilter("foo:blah.baz")
				So(err, ShouldEqual, nil)

				_, _, err = table.WhereClause(filter, "p_")
				So(err, ShouldErrLike, "fields are not allowed on the RHS of has (:) operator")
			})
		})
		Convey("Complex filter", func() {
			filter, err := ParseFilter("implicit (foo=explicitone) OR -bar=explicittwo AND foo!=explicitthree OR baz:explicitfour")
			So(err, ShouldEqual, nil)

			result, pars, err := table.WhereClause(filter, "p_")
			So(err, ShouldBeNil)
			So(pars, ShouldResemble, []QueryParameter{
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
			})
			So(result, ShouldEqual, "((db_foo LIKE @p_0 OR db_bar LIKE @p_0) AND ((COALESCE(db_foo, '') = @p_1) OR (NOT (COALESCE(db_bar, '') = @p_2))) AND ((COALESCE(db_foo, '') <> @p_3) OR (db_baz LIKE @p_4)))")
		})
	})
}
