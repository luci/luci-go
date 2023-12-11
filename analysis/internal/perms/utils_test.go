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

package perms

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func init() {
	rdbperms.PermListTestResults.AddFlags(realms.UsedInQueryRealms)
	rdbperms.PermListTestExonerations.AddFlags(realms.UsedInQueryRealms)
	rdbperms.PermGetArtifact.AddFlags(realms.UsedInQueryRealms)
	rdbperms.PermListArtifacts.AddFlags(realms.UsedInQueryRealms)
}

func TestQueryRealms(t *testing.T) {
	Convey("QueryRealms", t, func() {
		ctx := context.Background()

		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{
					Realm:      "project1:realm1",
					Permission: rdbperms.PermListTestResults,
				},
				{
					Realm:      "project1:realm1",
					Permission: rdbperms.PermListTestExonerations,
				},
				{
					Realm:      "project1:realm1",
					Permission: rdbperms.PermGetArtifact,
				},
				{
					Realm:      "project1:realm2",
					Permission: rdbperms.PermListTestResults,
				},
				{
					Realm:      "project1:realm2",
					Permission: rdbperms.PermListTestExonerations,
				},
				{
					Realm:      "project2:realm1",
					Permission: rdbperms.PermListTestResults,
				},
				{
					Realm:      "project2:realm1",
					Permission: rdbperms.PermGetArtifact,
				},
				{
					Realm:      "project2:realm1",
					Permission: rdbperms.PermListArtifacts,
				},
			},
		})

		Convey("QueryRealms", func() {
			Convey("no permission specified", func() {
				realms, err := QueryRealms(ctx, "project1", nil)
				So(err, ShouldErrLike, "at least one permission must be specified")
				So(realms, ShouldBeEmpty)
			})

			Convey("no project specified", func() {
				realms, err := QueryRealms(ctx, "", nil, rdbperms.PermListTestResults)
				So(err, ShouldErrLike, "project must be specified")
				So(realms, ShouldBeEmpty)
			})

			Convey("check single permission", func() {
				realms, err := QueryRealms(ctx, "project1", nil, rdbperms.PermListTestResults)
				So(err, ShouldBeNil)
				So(realms, ShouldResemble, []string{"project1:realm1", "project1:realm2"})
			})

			Convey("check multiple permissions", func() {
				realms, err := QueryRealms(ctx, "project1", nil, rdbperms.PermListTestResults, rdbperms.PermGetArtifact)
				So(err, ShouldBeNil)
				So(realms, ShouldResemble, []string{"project1:realm1"})
			})

			Convey("no matched realms", func() {
				realms, err := QueryRealms(ctx, "project1", nil, rdbperms.PermListTestExonerations, rdbperms.PermListArtifacts)
				So(err, ShouldBeNil)
				So(realms, ShouldBeEmpty)
			})

			Convey("no matched realms with non-empty method variant", func() {
				realms, err := QueryRealmsNonEmpty(ctx, "project1", nil, rdbperms.PermListTestExonerations, rdbperms.PermListArtifacts)
				So(err, ShouldErrLike, "caller does not have permissions", "in any realm in project \"project1\"")
				So(err, ShouldHaveAppStatus, codes.PermissionDenied)
				So(realms, ShouldBeEmpty)
			})
		})
		Convey("QuerySubRealms", func() {
			Convey("no permission specified", func() {
				realms, err := QuerySubRealmsNonEmpty(ctx, "project1", "realm1", nil)
				So(err, ShouldErrLike, "at least one permission must be specified")
				So(realms, ShouldBeEmpty)
			})

			Convey("no project specified", func() {
				realms, err := QuerySubRealmsNonEmpty(ctx, "", "", nil, rdbperms.PermListTestResults)
				So(err, ShouldErrLike, "project must be specified")
				So(realms, ShouldBeEmpty)
			})

			Convey("project scope", func() {
				Convey("check single permission", func() {
					realms, err := QuerySubRealmsNonEmpty(ctx, "project1", "", nil, rdbperms.PermListTestResults)
					So(err, ShouldBeNil)
					So(realms, ShouldResemble, []string{"realm1", "realm2"})
				})

				Convey("check multiple permissions", func() {
					realms, err := QuerySubRealmsNonEmpty(ctx, "project1", "", nil, rdbperms.PermListTestResults, rdbperms.PermGetArtifact)
					So(err, ShouldBeNil)
					So(realms, ShouldResemble, []string{"realm1"})
				})

				Convey("no matched realms", func() {
					realms, err := QuerySubRealmsNonEmpty(ctx, "project1", "", nil, rdbperms.PermListTestExonerations, rdbperms.PermListArtifacts)
					So(err, ShouldErrLike, "caller does not have permissions", "in any realm in project \"project1\"")
					So(err, ShouldHaveAppStatus, codes.PermissionDenied)
					So(realms, ShouldBeEmpty)
				})
			})

			Convey("realm scope", func() {
				Convey("check single permission", func() {
					realms, err := QuerySubRealmsNonEmpty(ctx, "project1", "realm1", nil, rdbperms.PermListTestResults)
					So(err, ShouldBeNil)
					So(realms, ShouldResemble, []string{"realm1"})
				})

				Convey("check multiple permissions", func() {
					realms, err := QuerySubRealmsNonEmpty(ctx, "project1", "realm1", nil, rdbperms.PermListTestResults, rdbperms.PermGetArtifact)
					So(err, ShouldBeNil)
					So(realms, ShouldResemble, []string{"realm1"})
				})

				Convey("no matched realms", func() {
					realms, err := QuerySubRealmsNonEmpty(ctx, "project1", "realm1", nil, rdbperms.PermListTestExonerations, rdbperms.PermListArtifacts)
					So(err, ShouldErrLike, "caller does not have permission", "in realm \"project1:realm1\"")
					So(err, ShouldHaveAppStatus, codes.PermissionDenied)
					So(realms, ShouldBeEmpty)
				})
			})
		})
	})
}
