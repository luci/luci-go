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

package lucictx

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLocalAuth(t *testing.T) {
	t.Parallel()

	Convey("SwitchLocalAccount works", t, func() {
		c := context.Background()

		Convey("No local_auth at all", func() {
			ctx, err := SwitchLocalAccount(c, "some")
			So(ctx, ShouldBeNil)
			So(err, ShouldEqual, ErrNoLocalAuthAccount)
		})

		Convey("Noop change", func() {
			c := SetLocalAuth(c, &LocalAuth{
				DefaultAccountID: "some",
				Accounts: []LocalAuthAccount{
					{ID: "some"},
				},
			})
			ctx, err := SwitchLocalAccount(c, "some")
			So(ctx, ShouldEqual, c)
			So(err, ShouldBeNil)
		})

		Convey("Switching into existing account", func() {
			c := SetLocalAuth(c, &LocalAuth{
				DefaultAccountID: "one",
				Accounts: []LocalAuthAccount{
					{ID: "one"},
					{ID: "two"},
				},
			})
			ctx, err := SwitchLocalAccount(c, "two")
			So(ctx, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(GetLocalAuth(ctx).DefaultAccountID, ShouldEqual, "two")
		})

		Convey("Switching into non-existing account", func() {
			c := SetLocalAuth(c, &LocalAuth{
				DefaultAccountID: "one",
				Accounts: []LocalAuthAccount{
					{ID: "one"},
				},
			})
			ctx, err := SwitchLocalAccount(c, "two")
			So(ctx, ShouldBeNil)
			So(err, ShouldEqual, ErrNoLocalAuthAccount)
		})
	})
}
