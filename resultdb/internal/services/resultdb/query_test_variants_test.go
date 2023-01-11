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

package resultdb

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc/codes"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/testvariants"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
)

func TestQueryTestVariants(t *testing.T) {
	Convey(`QueryTestVariants`, t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestResults},
				{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestExonerations},
				{Realm: "testproject:testlimitedrealm", Permission: rdbperms.PermListLimitedTestResults},
				{Realm: "testproject:testlimitedrealm", Permission: rdbperms.PermListLimitedTestExonerations},
			},
		})
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		testutil.MustApply(
			ctx,
			insert.InvocationWithInclusions("inv0", pb.Invocation_ACTIVE, map[string]interface{}{"Realm": "testproject:testrealm"}, "inv1")...,
		)
		testutil.MustApply(
			ctx,
			insert.Invocation("inv1", pb.Invocation_ACTIVE, map[string]interface{}{"Realm": "testproject:testrealm"}),
		)
		testutil.MustApply(ctx, testutil.CombineMutations(
			insert.TestResults("inv0", "T1", nil, pb.TestStatus_FAIL),
			insert.TestResults("inv0", "T2", nil, pb.TestStatus_FAIL),
			insert.TestResults("inv1", "T3", nil, pb.TestStatus_PASS),
			insert.TestResults("inv1", "T1", pbutil.Variant("a", "b"), pb.TestStatus_FAIL, pb.TestStatus_PASS),
			insert.TestExonerations("inv0", "T1", nil, pb.ExonerationReason_OCCURS_ON_OTHER_CLS),
		)...)
		testutil.MustApply(
			ctx,
			insert.Invocation("inv2", pb.Invocation_ACTIVE, map[string]interface{}{"Realm": "testproject:testlimitedrealm"}),
		)
		testutil.MustApply(ctx, testutil.CombineMutations(
			insert.TestResults("inv2", "T4", nil, pb.TestStatus_PASS),
		)...)

		srv := &resultDBServer{}

		Convey(`Permission denied`, func() {
			req := &pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/inv0"},
			}
			// Test PermListLimitedTestResults is required if the user does not have
			// both PermListTestResults and PermListTestExonerations.
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestResults},
					{Realm: "testproject:testrealm", Permission: rdbperms.PermListLimitedTestExonerations},
				},
			})
			_, err := srv.QueryTestVariants(ctx, req)
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
			So(err, ShouldErrLike, "resultdb.testResults.listLimited")

			// Test PermListLimitedTestExonerations is required if the user does not
			// have both PermListTestResults and PermListTestExonerations.
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "testproject:testrealm", Permission: rdbperms.PermListTestExonerations},
					{Realm: "testproject:testrealm", Permission: rdbperms.PermListLimitedTestResults},
				},
			})
			_, err = srv.QueryTestVariants(ctx, req)
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
			So(err, ShouldErrLike, "resultdb.testExonerations.listLimited")
		})

		Convey(`Query succeeds with limited permissions`, func() {
			res, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/inv2"},
			})
			So(err, ShouldBeNil)
			So(len(res.TestVariants), ShouldEqual, 1)
		})

		Convey(`Valid with included invocation`, func() {
			res, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/inv0"},
			})
			So(err, ShouldBeNil)
			So(res.NextPageToken, ShouldEqual, pagination.Token("EXPECTED", "", ""))

			So(len(res.TestVariants), ShouldEqual, 3)
			getTVStrings := func(tvs []*pb.TestVariant) []string {
				tvStrings := make([]string, len(tvs))
				for i, tv := range tvs {
					tvStrings[i] = fmt.Sprintf("%d/%s/%s", int32(tv.Status), tv.TestId, tv.VariantHash)
				}
				return tvStrings
			}
			So(getTVStrings(res.TestVariants), ShouldResemble, []string{
				"10/T2/e3b0c44298fc1c14",
				"30/T1/c467ccce5a16dc72",
				"40/T1/e3b0c44298fc1c14",
			})
		})

		Convey(`Valid without included invocation`, func() {
			res, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/inv1"},
			})
			So(err, ShouldBeNil)
			So(res.NextPageToken, ShouldEqual, pagination.Token("EXPECTED", "", ""))

			So(len(res.TestVariants), ShouldEqual, 1)
			getTVStrings := func(tvs []*pb.TestVariant) []string {
				tvStrings := make([]string, len(tvs))
				for i, tv := range tvs {
					tvStrings[i] = fmt.Sprintf("%d/%s/%s", int32(tv.Status), tv.TestId, tv.VariantHash)
				}
				return tvStrings
			}
			So(getTVStrings(res.TestVariants), ShouldResemble, []string{
				"30/T1/c467ccce5a16dc72",
			})
		})

		Convey(`Valid with missing included invocation`, func() {
			testutil.MustApply(
				ctx,
				// The invocation missinginv is missing in Invocations table.
				insert.Inclusion("inv0", "missinginv"),
			)
			res, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/inv0", "invocations/inv1"},
			})
			So(err, ShouldBeNil)
			So(res.NextPageToken, ShouldEqual, pagination.Token("EXPECTED", "", ""))

			So(len(res.TestVariants), ShouldEqual, 3)
			getTVStrings := func(tvs []*pb.TestVariant) []string {
				tvStrings := make([]string, len(tvs))
				for i, tv := range tvs {
					tvStrings[i] = fmt.Sprintf("%d/%s/%s", int32(tv.Status), tv.TestId, tv.VariantHash)
				}
				return tvStrings
			}
			So(getTVStrings(res.TestVariants), ShouldResemble, []string{
				"10/T2/e3b0c44298fc1c14",
				"30/T1/c467ccce5a16dc72",
				"40/T1/e3b0c44298fc1c14",
			})
		})

		Convey(`Try next page`, func() {
			res, err := srv.QueryTestVariants(ctx, &pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/inv0", "invocations/inv1"},
				PageSize:    3,
				PageToken:   pagination.Token("EXPECTED", "", ""),
			})

			So(err, ShouldBeNil)
			So(len(res.TestVariants), ShouldEqual, 1)
		})
	})
}

func TestValidateQueryTestVariantsRequest(t *testing.T) {
	Convey(`validateQueryTestVariantsRequest`, t, func() {
		Convey(`negative result_limit`, func() {
			err := validateQueryTestVariantsRequest(&pb.QueryTestVariantsRequest{
				Invocations: []string{"invocations/invx"},
				ResultLimit: -1,
			})
			So(err, ShouldErrLike, `result_limit: negative`)
		})
	})
}

func TestDetermineListAccessLevel(t *testing.T) {
	Convey("determineListAccessLevel", t, func() {
		ctx := auth.WithState(testutil.SpannerTestContext(t), &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "testproject:r1", Permission: rdbperms.PermListArtifacts},
				{Realm: "testproject:r1", Permission: rdbperms.PermListTestExonerations},
				{Realm: "testproject:r1", Permission: rdbperms.PermListTestResults},
				{Realm: "testproject:r2", Permission: rdbperms.PermListLimitedTestExonerations},
				{Realm: "testproject:r2", Permission: rdbperms.PermListLimitedTestResults},
				{Realm: "testproject:r2", Permission: rdbperms.PermListTestExonerations},
				{Realm: "testproject:r2", Permission: rdbperms.PermListTestResults},
				{Realm: "testproject:r3", Permission: rdbperms.PermListLimitedTestExonerations},
				{Realm: "testproject:r3", Permission: rdbperms.PermListLimitedTestResults},
			},
		})
		testutil.MustApply(
			ctx,
			insert.Invocation("i0", pb.Invocation_ACTIVE, map[string]interface{}{"Realm": "testproject:r0"}),
			insert.Invocation("i1", pb.Invocation_ACTIVE, map[string]interface{}{"Realm": "testproject:r1"}),
			insert.Invocation("i2", pb.Invocation_ACTIVE, map[string]interface{}{"Realm": "testproject:r2"}),
			insert.Invocation("i2b", pb.Invocation_ACTIVE, map[string]interface{}{"Realm": "testproject:r2"}),
			insert.Invocation("i3", pb.Invocation_ACTIVE, map[string]interface{}{"Realm": "testproject:r3"}),
			insert.Invocation("i3b", pb.Invocation_ACTIVE, map[string]interface{}{"Realm": "testproject:r3"}),
		)

		Convey("Access denied", func() {
			ids := invocations.NewIDSet(invocations.ID("i0"), invocations.ID("i2"))
			accessLevel, err := determineListAccessLevel(ctx, ids)
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
			So(accessLevel, ShouldEqual, testvariants.AccessLevelInvalid)
		})
		Convey("No common access level", func() {
			ids := invocations.NewIDSet(invocations.ID("i1"), invocations.ID("i3"))
			accessLevel, err := determineListAccessLevel(ctx, ids)
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
			So(accessLevel, ShouldEqual, testvariants.AccessLevelInvalid)
		})
		Convey("Limited access", func() {
			ids := invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i2b"),
				invocations.ID("i3"), invocations.ID("i3b"))
			accessLevel, err := determineListAccessLevel(ctx, ids)
			So(err, ShouldBeNil)
			So(accessLevel, ShouldEqual, testvariants.AccessLevelSAL1)
		})
		Convey("Full access", func() {
			ids := invocations.NewIDSet(invocations.ID("i1"), invocations.ID("i2"),
				invocations.ID("i2b"))
			accessLevel, err := determineListAccessLevel(ctx, ids)
			So(err, ShouldBeNil)
			So(accessLevel, ShouldEqual, testvariants.AccessLevelUnrestricted)
		})
		Convey("No invocations", func() {
			accessLevel, err := determineListAccessLevel(ctx, invocations.NewIDSet())
			So(err, ShouldBeNil)
			So(accessLevel, ShouldEqual, testvariants.AccessLevelInvalid)
		})
	})
}
