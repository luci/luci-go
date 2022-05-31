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
package permissions

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc/codes"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
)

func TestBatchGetTestVariants(t *testing.T) {
	Convey(`VerifyInvocations`, t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:r1", Permission: rdbperms.PermListArtifacts},
				{Realm: "testproject:r2", Permission: rdbperms.PermListArtifacts},
				{Realm: "testproject:r2", Permission: rdbperms.PermListTestExonerations},
				{Realm: "testproject:r2", Permission: rdbperms.PermListTestResults},
				{Realm: "testproject:r3", Permission: rdbperms.PermListTestExonerations},
				{Realm: "testproject:r3", Permission: rdbperms.PermListTestResults},
			},
		})
		testutil.MustApply(
			ctx,
			insert.Invocation("i0", pb.Invocation_ACTIVE, map[string]interface{}{"Realm": "testproject:r0"}),
			insert.Invocation("i1", pb.Invocation_ACTIVE, map[string]interface{}{"Realm": "testproject:r1"}),
			insert.Invocation("i2", pb.Invocation_ACTIVE, map[string]interface{}{"Realm": "testproject:r2"}),
			insert.Invocation("i3", pb.Invocation_ACTIVE, map[string]interface{}{"Realm": "testproject:r3"}),
			insert.Invocation("i3b", pb.Invocation_ACTIVE, map[string]interface{}{"Realm": "testproject:r3"}),
		)

		Convey("Access allowed", func() {
			ids := invocations.NewIDSet(invocations.ID("i1"), invocations.ID("i2"))
			err := VerifyInvocations(ctx, ids, rdbperms.PermListArtifacts)
			So(err, ShouldBeNil)

			ids = invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"))
			err = VerifyInvocations(ctx, ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults)
			So(err, ShouldBeNil)
		})
		Convey("Access denied", func() {
			ids := invocations.NewIDSet(invocations.ID("i0"))
			err := VerifyInvocations(ctx, ids, rdbperms.PermListArtifacts)
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
			So(err, ShouldErrLike, "resultdb.artifacts.list in realm of invocation i0")

			ids = invocations.NewIDSet(invocations.ID("i1"), invocations.ID("i2"))
			err = VerifyInvocations(ctx, ids, rdbperms.PermListArtifacts, rdbperms.PermListTestExonerations)
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
			So(err, ShouldErrLike, "resultdb.testExonerations.list in realm of invocation i1")

			ids = invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"))
			err = VerifyInvocations(ctx, ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults, rdbperms.PermListArtifacts)
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
			So(err, ShouldErrLike, "resultdb.artifacts.list in realm of invocation i3")
		})
		Convey("Duplicate invocations", func() {
			ids := invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"), invocations.ID("i3"))
			err := VerifyInvocations(ctx, ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults)
			So(err, ShouldBeNil)

			ids = invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"), invocations.ID("i3"))
			err = VerifyInvocations(ctx, ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults, rdbperms.PermListArtifacts)
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
			So(err, ShouldErrLike, "resultdb.artifacts.list in realm of invocation i3")
		})
		Convey("Duplicate realms", func() {
			ids := invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"), invocations.ID("i3b"))
			err := VerifyInvocations(ctx, ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults)
			So(err, ShouldBeNil)

			ids = invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"), invocations.ID("i3b"))
			err = VerifyInvocations(ctx, ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults, rdbperms.PermListArtifacts)
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
			So(err, ShouldErrLike, "resultdb.artifacts.list in realm of invocation i3")
		})
		Convey("Invocations do not exist", func() {
			ids := invocations.NewIDSet(invocations.ID("i2"), invocations.ID("iX"))
			err := VerifyInvocations(ctx, ids, rdbperms.PermListTestExonerations)
			So(err, ShouldHaveAppStatus, codes.NotFound)
			So(err, ShouldErrLike, "invocations/iX not found")

			ids = invocations.NewIDSet(invocations.ID("i2"), invocations.ID(""))
			err = VerifyInvocations(ctx, ids, rdbperms.PermListTestExonerations)
			So(err, ShouldHaveAppStatus, codes.NotFound)
			So(err, ShouldErrLike, "invocations/ not found")
		})
		Convey("No invocations", func() {
			ids := invocations.NewIDSet()
			err := VerifyInvocations(ctx, ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults)
			So(err, ShouldBeNil)
		})
	})
}
