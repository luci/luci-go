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

func TestParseOrderBy(t *testing.T) {
	Convey("ParseOrderBy", t, func() {
		// Test examples from the AIP-132 spec.
		Convey("Values should be a comma separated list of fields", func() {
			result, err := ParseOrderBy("foo,bar")
			So(err, ShouldBeNil)
			So(result, ShouldResemble, []OrderBy{
				{
					FieldPath: NewFieldPath("foo"),
				},
				{
					FieldPath: NewFieldPath("bar"),
				},
			})

			result, err = ParseOrderBy("foo")
			So(err, ShouldBeNil)
			So(result, ShouldResemble, []OrderBy{
				{
					FieldPath: NewFieldPath("foo"),
				},
			})
		})
		Convey("The default sort order is ascending", func() {
			result, err := ParseOrderBy("foo desc, bar")
			So(err, ShouldBeNil)
			So(result, ShouldResemble, []OrderBy{
				{
					FieldPath:  NewFieldPath("foo"),
					Descending: true,
				},
				{
					FieldPath: NewFieldPath("bar"),
				},
			})
		})
		Convey("Redundant space characters in the syntax are insignificant", func() {
			expectedResult := []OrderBy{
				{
					FieldPath: NewFieldPath("foo"),
				},
				{
					FieldPath:  NewFieldPath("bar"),
					Descending: true,
				},
			}
			result, err := ParseOrderBy("foo, bar desc")
			So(err, ShouldBeNil)
			So(result, ShouldResemble, expectedResult)

			result, err = ParseOrderBy("  foo  ,  bar desc  ")
			So(err, ShouldBeNil)
			So(result, ShouldResemble, expectedResult)

			result, err = ParseOrderBy("foo,bar desc")
			So(err, ShouldBeNil)
			So(result, ShouldResemble, expectedResult)
		})
		Convey("Subfields are specified with a . character", func() {
			result, err := ParseOrderBy("foo.bar, foo.foo.bar desc")
			So(err, ShouldBeNil)
			So(result, ShouldResemble, []OrderBy{
				{
					FieldPath: NewFieldPath("foo", "bar"),
				},
				{
					FieldPath:  NewFieldPath("foo", "foo", "bar"),
					Descending: true,
				},
			})
		})
		Convey("Quoted strings can be used instead of string literals", func() {
			result, err := ParseOrderBy("foo.`bar`, foo.foo.`a-backtick-```.bar desc")
			So(err, ShouldBeNil)
			So(result, ShouldResemble, []OrderBy{
				{
					FieldPath: NewFieldPath("foo", "bar"),
				},
				{
					FieldPath:  NewFieldPath("foo", "foo", "a-backtick-`", "bar"),
					Descending: true,
				},
			})
		})
		Convey("Invalid input is rejected", func() {
			_, err := ParseOrderBy("`something")
			So(err, ShouldErrLike, "syntax error: 1:1: invalid input text \"`something\"")
		})
		Convey("Empty order by", func() {
			Convey("Spaces only", func() {
				result, err := ParseOrderBy("   ")
				So(err, ShouldBeNil)
				So(result, ShouldHaveLength, 0)
			})
			Convey("Totally empty", func() {
				result, err := ParseOrderBy("")
				So(err, ShouldBeNil)
				So(result, ShouldHaveLength, 0)
			})
		})
	})
}
