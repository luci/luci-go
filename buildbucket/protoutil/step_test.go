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

package protoutil

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestParentStepName(t *testing.T) {
	t.Parallel()

	Convey("ParentStepName", t, func() {
		Convey("with an empty name", func() {
			So(ParentStepName(""), ShouldEqual, "")
		})

		Convey("with a parent", func() {
			So(ParentStepName("a|b"), ShouldEqual, "a")
			So(ParentStepName("a|b|c"), ShouldEqual, "a|b")
			So(ParentStepName("|b|"), ShouldEqual, "|b")
		})

		Convey("without a paraent", func() {
			So(ParentStepName("a.b"), ShouldEqual, "")
		})
	})
}

func TestValidateStepName(t *testing.T) {
	t.Parallel()

	Convey("Validate", t, func() {
		Convey("with an empty name", func() {
			So(ValidateStepName(""), ShouldErrLike, "required")
		})

		Convey("with valid names", func() {
			So(ValidateStepName("a"), ShouldBeNil)
			So(ValidateStepName("a|b"), ShouldBeNil)
			So(ValidateStepName("a|b|c"), ShouldBeNil)
		})

		Convey("with invalid names", func() {
			errMsg := `there must be at least one character before and after "|"`
			So(ValidateStepName("|"), ShouldErrLike, errMsg)
			So(ValidateStepName("a|"), ShouldErrLike, errMsg)
			So(ValidateStepName("|a"), ShouldErrLike, errMsg)
			So(ValidateStepName("a||b"), ShouldErrLike, errMsg)
		})
	})
}
