// Copyright 2017 The LUCI Authors.
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

package upload

import (
	"testing"
	"time"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock/testclock"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestIDs(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func() {
		ctx := gaetesting.TestingContext()
		ctx, clk := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

		Convey("NewOpID works", func() {
			op1, err := NewOpID(ctx)
			So(err, ShouldBeNil)
			So(op1, ShouldEqual, 1)

			op2, err := NewOpID(ctx)
			So(err, ShouldBeNil)
			So(op2, ShouldEqual, 2)
		})

		Convey("Wrap/unwrap works", func() {
			caller := identity.Identity("user:caller@example.com")
			wrapped, err := WrapOpID(ctx, 1234, caller)
			So(err, ShouldBeNil)

			// Ok.
			unwrapped, err := UnwrapOpID(ctx, wrapped, caller)
			So(err, ShouldBeNil)
			So(unwrapped, ShouldEqual, 1234)

			// Wrong caller ID.
			_, err = UnwrapOpID(ctx, wrapped, "user:another@example.com")
			So(err, ShouldErrLike, "failed to validate")

			// Expired.
			clk.Add(5*time.Hour + time.Minute)
			_, err = UnwrapOpID(ctx, wrapped, caller)
			So(err, ShouldErrLike, "failed to validate")
		})
	})
}
