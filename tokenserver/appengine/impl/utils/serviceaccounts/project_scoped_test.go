// Copyright 2018 The LUCI Authors.
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

package projectscope

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestPem(t *testing.T) {
	Convey("Shorten string", t, func() {
		So(shorten("abc", 4), ShouldResemble, "abc")
		So(shorten("abc", 3), ShouldResemble, "abc")
		So(shorten("abc", -1), ShouldResemble, "")

		So(shorten("abc", 2), ShouldResemble, "ab")
		So(shorten("abcdefgh", 4), ShouldResemble, "ab9c")
		So(shorten("abcdefgh", 8), ShouldResemble, "abcdefgh")
		So(shorten("abcdefgh", 6), ShouldResemble, "abc9c5")
	})

	Convey("IsValid matches correct pattern", t, func() {
		So(IsValid("foo"), ShouldResemble, false)
		So(IsValid("barfoo"), ShouldResemble, true)
		So(IsValid("barfoo-"), ShouldResemble, false)
		So(IsValid("foo-bar"), ShouldResemble, true)
		So(IsValid("f-----r"), ShouldResemble, true)
		So(IsValid("f0909a8-"), ShouldResemble, false)
		So(IsValid("foobarbazfoobarbazfoobarbazfoobarbaz"), ShouldResemble, false)
	})

	Convey("GenerateAccountEmail generates compliant account ids", t, func() {
		var accountId string
		var err error

		accountId, err = GenerateAccountEmail("foo", "bar")
		So(err, ShouldBeNil)
		So(accountId, ShouldResemble, "foo-bar")

		accountId, err = GenerateAccountEmail("foobarbazfoobarbazfoobarbazfoobarbaz", "foobarbazfoobarbazfoobarbazfoobarbaz")
		So(err, ShouldBeNil)
		So(accountId, ShouldResemble, "foobarba8d7e7ed-foobarba8d7e7ed")
	})
}
