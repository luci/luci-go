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
					Name: "foo",
				},
				{
					Name: "bar",
				},
			})

			result, err = ParseOrderBy("foo")
			So(err, ShouldBeNil)
			So(result, ShouldResemble, []OrderBy{
				{
					Name: "foo",
				},
			})
		})
		Convey("The default sort order is ascending", func() {
			result, err := ParseOrderBy("foo desc, bar")
			So(err, ShouldBeNil)
			So(result, ShouldResemble, []OrderBy{
				{
					Name:       "foo",
					Descending: true,
				},
				{
					Name: "bar",
				},
			})
		})
		Convey("Redundant space characters in the syntax are insignificant", func() {
			expectedResult := []OrderBy{
				{
					Name: "foo",
				},
				{
					Name:       "bar",
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
			result, err := ParseOrderBy("foo.bar, foo.foo desc")
			So(err, ShouldBeNil)
			So(result, ShouldResemble, []OrderBy{
				{
					Name: "foo.bar",
				},
				{
					Name:       "foo.foo",
					Descending: true,
				},
			})
		})
		Convey("Invalid input is rejected", func() {
			_, err := ParseOrderBy("`something")
			So(err, ShouldErrLike, "invalid ordering \"`something\"")
		})
		Convey("Empty order by", func() {
			result, err := ParseOrderBy("   ")
			So(err, ShouldBeNil)
			So(result, ShouldHaveLength, 0)
		})
	})
}
