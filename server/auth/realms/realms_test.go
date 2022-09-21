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

package realms

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateProjectName(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		So(ValidateProjectName("something-blah"), ShouldBeNil)
		So(ValidateProjectName("@internal"), ShouldBeNil)

		So(ValidateProjectName("something-BLAH"), ShouldNotBeNil)
		So(ValidateProjectName(""), ShouldNotBeNil)
		So(ValidateProjectName(":"), ShouldNotBeNil)
		So(ValidateProjectName("@blah"), ShouldNotBeNil)
	})
}

func TestValidateRealmName(t *testing.T) {
	t.Parallel()

	Convey("GlobalScope", t, func() {
		call := func(r string) error {
			return ValidateRealmName(r, GlobalScope)
		}

		So(call("something-blah:realm"), ShouldBeNil)
		So(call("something-blah:a/b/c"), ShouldBeNil)
		So(call("something-blah:@root"), ShouldBeNil)
		So(call("something-blah:@legacy"), ShouldBeNil)
		So(call("something-blah:@project"), ShouldBeNil)

		So(call("@internal:realm"), ShouldBeNil)
		So(call("@internal:a/b/c"), ShouldBeNil)
		So(call("@internal:@root"), ShouldBeNil)
		So(call("@internal:@legacy"), ShouldBeNil)
		So(call("@internal:@project"), ShouldBeNil)

		So(call("realm"), ShouldErrLike, "should be <project>:<realm>")
		So(call("BLAH:realm"), ShouldErrLike, "bad project name")
		So(call("@blah:zzz"), ShouldErrLike, "bad project name")
		So(call("blah:"), ShouldErrLike, "the realm name should match")
		So(call("blah:@zzz"), ShouldErrLike, "the realm name should match")
	})

	Convey("ProjectScope", t, func() {
		call := func(r string) error {
			return ValidateRealmName(r, ProjectScope)
		}

		So(call("realm"), ShouldBeNil)
		So(call("a/b/c"), ShouldBeNil)
		So(call("@root"), ShouldBeNil)
		So(call("@legacy"), ShouldBeNil)
		So(call("@project"), ShouldBeNil)

		So(call("blah:realm"), ShouldNotBeNil)
		So(call(":realm"), ShouldNotBeNil)
		So(call("realm:"), ShouldNotBeNil)
		So(call("@zzz"), ShouldNotBeNil)
	})
}

func TestRealmSplitJoin(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		a, b := Split("a:b")
		So(a, ShouldEqual, "a")
		So(b, ShouldEqual, "b")

		a, b = Split(":")
		So(a, ShouldEqual, "")
		So(b, ShouldEqual, "")

		So(func() { Split("ab") }, ShouldPanic)

		So(Join("a", "b"), ShouldEqual, "a:b")
	})
}
