// Copyright 2023 The LUCI Authors.
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
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	. "go.chromium.org/luci/common/testing/assertions"
	btv "go.chromium.org/luci/resultdb/internal/baselines/testvariants"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/span"
)

func TestValidateNewTestVariantsRequest(t *testing.T) {

	Convey(`Validate Request`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		req := &pb.QueryNewTestVariantsRequest{}

		Convey(`Valid`, func() {
			tctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:@project", Permission: rdbperms.PermGetBaseline},
					{Realm: "chromium:try", Permission: rdbperms.PermListTestResults},
				},
			})

			testutil.MustApply(tctx,
				insert.Invocation(
					"build:12345",
					pb.Invocation_FINALIZED,
					map[string]any{
						"Realm": "chromium:try",
					},
				),
			)

			req.Invocation = "invocations/build:12345"
			req.Baseline = "projects/chromium/baselines/try:Linux Asan"
			err := validateQueryNewTestVariantsRequest(tctx, req)
			So(err, ShouldBeNil)
		})

		Convey(`Invocation Bad Format`, func() {
			req.Invocation = "build:12345"
			err := validateQueryNewTestVariantsRequest(ctx, req)
			So(err, ShouldHaveAppStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "invocation")
		})

		Convey(`Invocation Unsupported Value`, func() {
			req.Invocation = "invocations/build!12345"
			err := validateQueryNewTestVariantsRequest(ctx, req)
			So(err, ShouldHaveAppStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "invocation")

		})

		Convey(`Baseline Bad Format`, func() {
			req.Invocation = "invocations/build:12345"
			req.Baseline = "try:linux-rel"
			err := validateQueryNewTestVariantsRequest(ctx, req)
			So(err, ShouldHaveAppStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "baseline")
		})

		Convey(`Baseline Unsupported Value`, func() {
			req.Invocation = "invocations/build:12345"
			req.Baseline = "projects/-/baselines/try!linux-rel"
			err := validateQueryNewTestVariantsRequest(ctx, req)
			So(err, ShouldHaveAppStatus, codes.InvalidArgument)
			So(err, ShouldErrLike, "baseline")
		})

		Convey(`Invocation Not Found`, func() {
			req.Invocation = "invocations/build:12345"
			req.Baseline = "projects/chromium/baselines/try:Linux Asan"
			err := validateQueryNewTestVariantsRequest(ctx, req)
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
			So(err, ShouldErrLike, noPermissionsError)
		})

		Convey(`Missing resultdb.baselines.get`, func() {
			tctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:try", Permission: rdbperms.PermListTestResults},
				},
			})

			testutil.MustApply(tctx,
				insert.Invocation(
					"build:12345",
					pb.Invocation_FINALIZED,
					map[string]any{
						"Realm": "chromium:try",
					},
				),
			)

			req.Invocation = "invocations/build:12345"
			req.Baseline = "projects/chromium/baselines/try:linux-rel"
			err := validateQueryNewTestVariantsRequest(tctx, req)
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
			So(err, ShouldErrLike, noPermissionsError)
		})

		Convey(`Missing resultdb.testResults.list`, func() {
			tctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:try", Permission: rdbperms.PermGetBaseline},
				},
			})

			testutil.MustApply(tctx,
				insert.Invocation(
					"build:12345",
					pb.Invocation_FINALIZED,
					map[string]any{
						"Realm": "chromium:try",
					},
				),
			)

			req.Invocation = "invocations/build:12345"
			req.Baseline = "projects/chromium/baselines/try:linux-rel"
			err := validateQueryNewTestVariantsRequest(tctx, req)
			So(err, ShouldHaveAppStatus, codes.PermissionDenied)
			So(err, ShouldErrLike, noPermissionsError)
		})
	})
}

func ValidateCheckBaselineStatus(t *testing.T) {

	Convey(`ValidateCheckBaselineStatus`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		req := &pb.QueryNewTestVariantsRequest{}

		Convey(`Baseline Not Found`, func() {
			tctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:try", Permission: rdbperms.PermGetBaseline},
				},
			})

			testutil.MustApply(tctx,
				insert.Invocation(
					"build:12345",
					pb.Invocation_FINALIZED,
					map[string]any{
						"Realm": "chromium:try",
					},
				),
			)

			req.Invocation = "invocations/build:12345"
			req.Baseline = "try:Linux Asan"
			_, err := checkBaselineStatus(tctx, "chromium", req.Baseline)
			So(err, ShouldBeNil)
		})

		Convey(`Baseline Less Than 72 Hours`, func() {
			tctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:try", Permission: rdbperms.PermGetBaseline},
				},
			})

			testutil.MustApply(tctx,
				insert.Invocation(
					"build:12345",
					pb.Invocation_FINALIZED,
					map[string]any{
						"Realm": "chromium:try",
					},
				),
				spanutil.InsertMap("Baselines", map[string]any{
					"Project":         "chromium",
					"BaselineId":      "try:Linux Asan",
					"LastUpdatedTime": spanner.CommitTimestamp,
					"CreationTime":    time.Now().UTC().Add(-time.Hour * 24),
				}),
			)

			req.Invocation = "invocations/build:12345"
			req.Baseline = "try:Linux Asan"
			isReady, err := checkBaselineStatus(tctx, "chromium", req.Baseline)
			So(err, ShouldBeNil)
			So(isReady, ShouldBeFalse)
		})

		Convey(`Baseline Greater Than 72 Hours`, func() {
			tctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:try", Permission: rdbperms.PermGetBaseline},
				},
			})

			testutil.MustApply(tctx,
				insert.Invocation(
					"build:12345",
					pb.Invocation_FINALIZED,
					map[string]any{
						"Realm": "chromium:try",
					},
				),
				spanutil.InsertMap("Baselines", map[string]any{
					"Project":         "chromium",
					"BaselineId":      "try:Linux Asan",
					"LastUpdatedTime": spanner.CommitTimestamp,
					"CreationTime":    time.Now().UTC().Add(-time.Hour * 96),
				}),
			)

			req.Invocation = "invocations/build:12345"
			req.Baseline = "try:Linux Asan"
			isReady, err := checkBaselineStatus(tctx, "chromium", req.Baseline)
			So(err, ShouldBeNil)
			So(isReady, ShouldBeTrue)
		})
	})
}

func TestFindNewTests(t *testing.T) {

	Convey(`FindNewTests`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		idSet := invocations.NewIDSet()

		Convey(`No Invs`, func() {
			ntvs, err := findNewTests(ctx, "chromium", "baseline_id", idSet)
			So(ntvs, ShouldBeNil)
			So(err, ShouldBeNil)
		})

		v := &pb.Variant{
			Def: map[string]string{
				"bucket":  "try",
				"builder": "Linux Asan",
			},
		}
		vh := pbutil.VariantHash(v)
		testutil.MustApply(ctx,
			btv.Create("chromium", "try:Linux Asan", "X", vh),
		)
		invID := invocations.ID("inv0")
		idSet.Add(invID)
		testutil.MustApply(ctx,
			insert.Invocation(
				invID,
				pb.Invocation_FINALIZED,
				map[string]any{
					"Realm": "chromium:try",
				},
			),
		)
		testutil.MustApply(ctx,
			testutil.CombineMutations(
				insert.TestResults(
					"inv0",
					"X",
					v,
					pb.TestStatus_PASS,
				),
			)...)

		Convey(`No New`, func() {
			nt, err := findNewTests(ctx, "chromium", "try:Linux Asan", idSet)
			So(err, ShouldBeNil)
			So(len(nt), ShouldEqual, 0)
		})

		// Add one test result and re-run. There should be one new.
		Convey(`One New`, func() {
			testutil.MustApply(ctx,
				testutil.CombineMutations(
					insert.TestResults(
						"inv0",
						"Y",
						v,
						pb.TestStatus_PASS,
					),
				)...)

			nt, err := findNewTests(ctx, "chromium", "try:Linux Asan", idSet)
			So(err, ShouldBeNil)
			So(len(nt), ShouldEqual, 1)
			So(nt[0].TestId, ShouldEqual, "Y")
		})

		Convey(`# Baselines Test Variants > # Test Results`, func() {
			testutil.MustApply(ctx,
				btv.Create("chromium", "try:Linux Asan", "A", vh),
				btv.Create("chromium", "try:Linux Asan", "B", vh),
				btv.Create("chromium", "try:Linux Asan", "C", vh),
				// Add Y so that there should be no more new tests.
				btv.Create("chromium", "try:Linux Asan", "Y", vh),
			)

			nt, err := findNewTests(ctx, "chromium", "try:Linux Asan", idSet)
			So(err, ShouldBeNil)
			So(len(nt), ShouldEqual, 0)
		})
	})
}

func TestE2E(t *testing.T) {

	Convey(`E2E`, t, func() {
		ctx := testutil.SpannerTestContext(t)

		// Permissions. "resultdb.baselines.get" and "resultdb.testResults.list"
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: "chromium:try", Permission: rdbperms.PermListTestResults},
				{Realm: "chromium:@project", Permission: rdbperms.PermGetBaseline},
			},
		})

		// Invocation with nested invocations. Used as request.
		invID := invocations.ID("inv0")
		testutil.MustApply(ctx, testutil.CombineMutations(
			insert.InvocationWithInclusions(invID, pb.Invocation_FINALIZED, map[string]any{"Realm": "chromium:try"}, "inv1", "inv3"),
			insert.InvocationWithInclusions(invocations.ID("inv1"), pb.Invocation_FINALIZED, nil, "inv2"),
			insert.InvocationWithInclusions(invocations.ID("inv2"), pb.Invocation_FINALIZED, nil),
			insert.InvocationWithInclusions(invocations.ID("inv3"), pb.Invocation_FINALIZED, nil),
		)...)

		// Baseline should be ready, creation time set > 72 hours.
		testutil.MustApply(ctx,
			spanutil.InsertMap("Baselines", map[string]any{
				"Project":         "chromium",
				"BaselineId":      "try:Linux Asan",
				"LastUpdatedTime": clock.Now(ctx).UTC().Add(-time.Hour * 10),
				"CreationTime":    clock.Now(ctx).UTC().Add(-time.Hour * 96),
			}),
		)

		v := &pb.Variant{
			Def: map[string]string{
				"bucket":  "try",
				"builder": "Linux Asan",
			},
		}
		vh := pbutil.VariantHash(v)

		// TestResults for invocation from request
		testutil.MustApply(ctx, testutil.CombineMutations(
			insert.TestResults("inv0", "A", v, pb.TestStatus_FAIL, pb.TestStatus_PASS),
			insert.TestResults("inv1", "B", v, pb.TestStatus_PASS, pb.TestStatus_PASS),
			insert.TestResults("inv1", "C", v, pb.TestStatus_PASS, pb.TestStatus_PASS),
			insert.TestResults("inv2", "D", v, pb.TestStatus_PASS, pb.TestStatus_PASS),
			insert.TestResults("inv3", "E", v, pb.TestStatus_FAIL, pb.TestStatus_PASS),
		)...)

		// BaselineTestVariants (meaning that they were run previously with a successful run)
		testutil.MustApply(ctx,
			btv.Create("chromium", "try:Linux Asan", "C", vh),
			btv.Create("chromium", "try:Linux Asan", "D", vh),
		)

		req := &pb.QueryNewTestVariantsRequest{
			Baseline:   "projects/chromium/baselines/try:Linux Asan",
			Invocation: "invocations/inv0",
		}

		svc := newTestResultDBService()
		resp, err := svc.QueryNewTestVariants(ctx, req)
		So(err, ShouldBeNil)
		So(resp.GetIsBaselineReady(), ShouldBeTrue)

		// A, B, E
		nt := resp.GetNewTestVariants()
		So(len(nt), ShouldEqual, 3)
		expected := []*pb.QueryNewTestVariantsResponse_NewTestVariant{
			&pb.QueryNewTestVariantsResponse_NewTestVariant{TestId: "A", VariantHash: vh},
			&pb.QueryNewTestVariantsResponse_NewTestVariant{TestId: "B", VariantHash: vh},
			&pb.QueryNewTestVariantsResponse_NewTestVariant{TestId: "E", VariantHash: vh},
		}
		So(nt, ShouldResemble, expected)
	})
}
