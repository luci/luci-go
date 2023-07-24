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

package limit

import (
	"testing"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

func TestUserRunLimits(t *testing.T) {
	t.Parallel()

	Convey("TestStartRun", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()
		cg := &prjcfg.ConfigGroup{Content: &cfgpb.ConfigGroup{}}

		authState := &authtest.FakeState{FakeDB: authtest.NewFakeDB()}
		ctx = auth.WithState(ctx, authState)
		addMember := func(user, grp string) {
			id, err := identity.MakeIdentity(user)
			So(err, ShouldBeNil)
			authState.FakeDB.(*authtest.FakeDB).AddMocks(authtest.MockMembership(id, grp))
		}
		newUserLimit := func(name string, principals ...string) *cfgpb.UserLimit {
			return &cfgpb.UserLimit{
				Name:       name,
				Principals: principals,
				Run: &cfgpb.UserLimit_Run{MaxActive: &cfgpb.UserLimit_Limit{
					Limit: &cfgpb.UserLimit_Limit_Unlimited{Unlimited: true},
				}},
				Tryjob: &cfgpb.UserLimit_Tryjob{MaxActive: &cfgpb.UserLimit_Limit{
					Limit: &cfgpb.UserLimit_Limit_Unlimited{Unlimited: true},
				}},
			}
		}

		addUserLimit := func(name string, principals ...string) {
			cg.Content.UserLimits = append(
				cg.Content.UserLimits, newUserLimit(name, principals...))
		}

		Convey("findUserLimit", func() {
			find := func(owner string) *cfgpb.UserLimit {
				o, err := identity.MakeIdentity(owner)
				So(err, ShouldBeNil)
				lim, err := findUserLimit(ctx, cg, &run.Run{Owner: o})
				So(err, ShouldBeNil)
				return lim
			}

			Convey("returns the first matching limit", func() {
				So(find("user:foo@example.org"), ShouldBeNil)

				addUserLimit("foo_limit", "user:foo@example.org")
				addUserLimit("bar_limit", "user:bar@example.org")
				addUserLimit("hoo_limit", "user:hoo@example.org")

				lim := find("user:foo@example.org")
				So(lim, ShouldNotBeNil)
				So(lim.Name, ShouldEqual, "foo_limit")
				lim = find("user:bar@example.org")
				So(lim, ShouldNotBeNil)
				So(lim.Name, ShouldEqual, "bar_limit")
				lim = find("user:hoo@example.org")
				So(lim, ShouldNotBeNil)
				So(lim.Name, ShouldEqual, "hoo_limit")
			})

			Convey("checks membership", func() {
				addUserLimit("limit1", "group:bots")
				addUserLimit("limit2", "user:human@example.org")

				lim := find("user:human@example.org")
				So(lim, ShouldNotBeNil)
				So(lim.Name, ShouldEqual, "limit2")

				// add the member in the bot group.
				// Then, it should return limit1, instead.
				addMember("user:human@example.org", "bots")
				lim = find("user:human@example.org")
				So(lim, ShouldNotBeNil)
				So(lim.Name, ShouldEqual, "limit1")
			})

			Convey("returns user_limit_default", func() {
				cg.Content.UserLimitDefault = newUserLimit("default")
				lim := find("user:foo@example.org")
				So(lim, ShouldNotBeNil)
				So(lim.Name, ShouldEqual, "default")
			})
		})
	})
}
