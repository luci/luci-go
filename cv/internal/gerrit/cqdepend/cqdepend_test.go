// Copyright 2020 The LUCI Authors.
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

package cqdepend

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestParser(t *testing.T) {
	t.Parallel()

	Convey("Parse works", t, func() {
		Convey("Basic", func() {
			So(Parse("Nothing\n\ninteresting."), ShouldBeNil)
			So(Parse("Title.\n\nCq-Depend: 456,123"), ShouldResemble, []Dep{
				{Subdomain: "", Change: 123},
				{Subdomain: "", Change: 456},
			})
		})
		Convey("Case and space incensitive", func() {
			So(Parse("Title.\n\nCQ-dePend: Any-case:456 , Zx:23 "), ShouldResemble, []Dep{
				{Subdomain: "any-case", Change: 456},
				{Subdomain: "zx", Change: 23},
			})
		})
		Convey("Dedup and multiline", func() {
			So(Parse("Title.\n\nCq-Depend: 456,123\nCq-Depend: 123,y:456"), ShouldResemble, []Dep{
				{Subdomain: "", Change: 123},
				{Subdomain: "", Change: 456},
				{Subdomain: "y", Change: 456},
			})
		})
		Convey("Ignores errors", func() {
			So(Parse("Title.\n\nCq-Depend: 2, x;3\nCq-Depend: y-review:4,z:5"), ShouldResemble, []Dep{
				{Subdomain: "", Change: 2},
				{Subdomain: "z", Change: 5},
			})
		})
		Convey("Ignores non-footers", func() {
			So(Parse("Cq-Depend: 1\n\nCq-Depend: i:2\n\nChange-Id: Ideadbeef"), ShouldBeNil)
		})
	})
}

func TestParseSingleDep(t *testing.T) {
	t.Parallel()

	Convey("parseSingleDep works", t, func() {
		Convey("OK", func() {
			d, err := parseSingleDep(" x:123 ")
			So(err, ShouldBeNil)
			So(d, ShouldResemble, Dep{Change: 123, Subdomain: "x"})

			d, err = parseSingleDep("123")
			So(err, ShouldBeNil)
			So(d, ShouldResemble, Dep{Change: 123, Subdomain: ""})
		})
		Convey("Invalid format", func() {
			_, err := parseSingleDep("weird/value:here")
			So(err, ShouldErrLike, "must match")
			_, err = parseSingleDep("https://abc.example.com:123")
			So(err, ShouldErrLike, "must match")
			_, err = parseSingleDep("abc-review.example.com:1")
			So(err, ShouldErrLike, "must match")
			_, err = parseSingleDep("no-spaces-around-colon :1")
			So(err, ShouldErrLike, "must match")
			_, err = parseSingleDep("no-spaces-around-colon: 2")
			So(err, ShouldErrLike, "must match")
		})
		Convey("Too large", func() {
			_, err := parseSingleDep("12312123123123123123")
			So(err, ShouldErrLike, "change number too large")
		})
		Convey("Disallow -Review", func() {
			_, err := parseSingleDep("x-review:1")
			So(err, ShouldErrLike, "must not include '-review'")
		})
	})
}
