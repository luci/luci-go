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
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/span"

	btv "go.chromium.org/luci/resultdb/internal/baselines/testvariants"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestValidateNewTestVariantsRequest(t *testing.T) {
	ftt.Run(`Validate Request`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		req := &pb.QueryNewTestVariantsRequest{}

		t.Run(`Valid`, func(t *ftt.Test) {
			tctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:@project", Permission: rdbperms.PermGetBaseline},
					{Realm: "chromium:try", Permission: rdbperms.PermListTestResults},
				},
			})

			testutil.MustApply(tctx, t,
				insert.Invocation(
					"build-12345",
					pb.Invocation_FINALIZED,
					map[string]any{
						"Realm": "chromium:try",
					},
				),
			)

			req.Invocation = "invocations/build-12345"
			req.Baseline = "projects/chromium/baselines/try:Linux Asan"
			err := validateQueryNewTestVariantsRequest(tctx, req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`Invocation Bad Format`, func(t *ftt.Test) {
			req.Invocation = "build-12345"
			err := validateQueryNewTestVariantsRequest(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("invocation"))
		})

		t.Run(`Invocation Unsupported Value`, func(t *ftt.Test) {
			req.Invocation = "invocations/build!12345"
			err := validateQueryNewTestVariantsRequest(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("invocation"))
		})

		t.Run(`Baseline Bad Format`, func(t *ftt.Test) {
			req.Invocation = "invocations/build-12345"
			req.Baseline = "try:linux-rel"
			err := validateQueryNewTestVariantsRequest(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("baseline"))
		})

		t.Run(`Baseline Unsupported Value`, func(t *ftt.Test) {
			req.Invocation = "invocations/build-12345"
			req.Baseline = "projects/-/baselines/try!linux-rel"
			err := validateQueryNewTestVariantsRequest(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("baseline"))
		})

		t.Run(`Invocation Not Found`, func(t *ftt.Test) {
			req.Invocation = "invocations/build-12345"
			req.Baseline = "projects/chromium/baselines/try:Linux Asan"
			err := validateQueryNewTestVariantsRequest(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike(noPermissionsError))
		})

		t.Run(`Missing resultdb.baselines.get`, func(t *ftt.Test) {
			tctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:try", Permission: rdbperms.PermListTestResults},
				},
			})

			testutil.MustApply(tctx, t,
				insert.Invocation(
					"build-12345",
					pb.Invocation_FINALIZED,
					map[string]any{
						"Realm": "chromium:try",
					},
				),
			)

			req.Invocation = "invocations/build-12345"
			req.Baseline = "projects/chromium/baselines/try:linux-rel"
			err := validateQueryNewTestVariantsRequest(tctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike(noPermissionsError))
		})

		t.Run(`Missing resultdb.testResults.list`, func(t *ftt.Test) {
			tctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:try", Permission: rdbperms.PermGetBaseline},
				},
			})

			testutil.MustApply(tctx, t,
				insert.Invocation(
					"build-12345",
					pb.Invocation_FINALIZED,
					map[string]any{
						"Realm": "chromium:try",
					},
				),
			)

			req.Invocation = "invocations/build-12345"
			req.Baseline = "projects/chromium/baselines/try:linux-rel"
			err := validateQueryNewTestVariantsRequest(tctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike(noPermissionsError))
		})
	})
}

func ValidateCheckBaselineStatus(t *testing.T) {
	ftt.Run(`ValidateCheckBaselineStatus`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		req := &pb.QueryNewTestVariantsRequest{}

		t.Run(`Baseline Not Found`, func(t *ftt.Test) {
			tctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:try", Permission: rdbperms.PermGetBaseline},
				},
			})

			testutil.MustApply(tctx, t,
				insert.Invocation(
					"build-12345",
					pb.Invocation_FINALIZED,
					map[string]any{
						"Realm": "chromium:try",
					},
				),
			)

			req.Invocation = "invocations/build-12345"
			req.Baseline = "try:Linux Asan"
			_, err := checkBaselineStatus(tctx, "chromium", req.Baseline)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`Baseline Less Than 72 Hours`, func(t *ftt.Test) {
			tctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:try", Permission: rdbperms.PermGetBaseline},
				},
			})

			testutil.MustApply(tctx, t,
				insert.Invocation(
					"build-12345",
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

			req.Invocation = "invocations/build-12345"
			req.Baseline = "try:Linux Asan"
			isReady, err := checkBaselineStatus(tctx, "chromium", req.Baseline)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, isReady, should.BeFalse)
		})

		t.Run(`Baseline Greater Than 72 Hours`, func(t *ftt.Test) {
			tctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				IdentityPermissions: []authtest.RealmPermission{
					{Realm: "chromium:try", Permission: rdbperms.PermGetBaseline},
				},
			})

			testutil.MustApply(tctx, t,
				insert.Invocation(
					"build-12345",
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

			req.Invocation = "invocations/build-12345"
			req.Baseline = "try:Linux Asan"
			isReady, err := checkBaselineStatus(tctx, "chromium", req.Baseline)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, isReady, should.BeTrue)
		})
	})
}

func TestFindNewTests(t *testing.T) {
	ftt.Run(`FindNewTests`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		idSet := invocations.NewIDSet()

		ntvs, err := findNewTests(ctx, "chromium", "baseline_id", idSet)
		assert.Loosely(t, ntvs, should.BeNil)
		assert.Loosely(t, err, should.BeNil)

		v := &pb.Variant{
			Def: map[string]string{
				"bucket":  "try",
				"builder": "Linux Asan",
			},
		}
		vh := pbutil.VariantHash(v)
		testutil.MustApply(ctx, t,
			btv.Create("chromium", "try:Linux Asan", "X", vh),
		)
		invID := invocations.ID("inv0")
		idSet.Add(invID)
		testutil.MustApply(ctx, t,
			insert.Invocation(
				invID,
				pb.Invocation_FINALIZED,
				map[string]any{
					"Realm": "chromium:try",
				},
			),
		)
		testutil.MustApply(ctx, t,
			testutil.CombineMutations(
				insert.TestResults(t,
					"inv0",
					"X",
					v,
					pb.TestResult_PASSED,
				),
			)...)

		t.Run(`No New`, func(t *ftt.Test) {
			nt, err := findNewTests(ctx, "chromium", "try:Linux Asan", idSet)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(nt), should.BeZero)
		})

		// Add one test result and re-run. There should be one new.
		t.Run(`One New`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				testutil.CombineMutations(
					insert.TestResults(t,
						"inv0",
						"Y",
						v,
						pb.TestResult_PASSED,
					),
				)...)

			nt, err := findNewTests(ctx, "chromium", "try:Linux Asan", idSet)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(nt), should.Equal(1))
			assert.Loosely(t, nt[0].TestId, should.Equal("Y"))
		})

		t.Run(`# Baselines Test Variants > # Test Results`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				btv.Create("chromium", "try:Linux Asan", "A", vh),
				btv.Create("chromium", "try:Linux Asan", "B", vh),
				btv.Create("chromium", "try:Linux Asan", "C", vh),
				// Add Y so that there should be no more new tests.
				btv.Create("chromium", "try:Linux Asan", "Y", vh),
			)

			nt, err := findNewTests(ctx, "chromium", "try:Linux Asan", idSet)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(nt), should.BeZero)
		})
	})
}

func TestE2E(t *testing.T) {
	ftt.Run(`E2E`, t, func(t *ftt.Test) {
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
		testutil.MustApply(ctx, t, testutil.CombineMutations(
			insert.InvocationWithInclusions(invID, pb.Invocation_FINALIZED, map[string]any{"Realm": "chromium:try"}, "inv1", "inv3"),
			insert.InvocationWithInclusions(invocations.ID("inv1"), pb.Invocation_FINALIZED, nil, "inv2"),
			insert.InvocationWithInclusions(invocations.ID("inv2"), pb.Invocation_FINALIZED, nil),
			insert.InvocationWithInclusions(invocations.ID("inv3"), pb.Invocation_FINALIZED, nil),
		)...)

		// Baseline should be ready, creation time set > 72 hours.
		testutil.MustApply(ctx, t,
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
		testutil.MustApply(ctx, t, testutil.CombineMutations(
			insert.TestResults(t, "inv0", "A", v, pb.TestResult_FAILED, pb.TestResult_PASSED),
			insert.TestResults(t, "inv1", "B", v, pb.TestResult_PASSED, pb.TestResult_PASSED),
			insert.TestResults(t, "inv1", "C", v, pb.TestResult_PASSED, pb.TestResult_PASSED),
			insert.TestResults(t, "inv2", "D", v, pb.TestResult_PASSED, pb.TestResult_PASSED),
			insert.TestResults(t, "inv3", "E", v, pb.TestResult_FAILED, pb.TestResult_PASSED),
			// This is Skipped, so F should not be in the final set.
			insert.TestResults(t, "inv1", "F", v, pb.TestResult_SKIPPED),
		)...)

		// BaselineTestVariants (meaning that they were run previously with a successful run)
		testutil.MustApply(ctx, t,
			btv.Create("chromium", "try:Linux Asan", "C", vh),
			btv.Create("chromium", "try:Linux Asan", "D", vh),
		)

		req := &pb.QueryNewTestVariantsRequest{
			Baseline:   "projects/chromium/baselines/try:Linux Asan",
			Invocation: "invocations/inv0",
		}

		svc := newTestResultDBService()
		resp, err := svc.QueryNewTestVariants(ctx, req)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, resp.GetIsBaselineReady(), should.BeTrue)

		// A, B, E
		nt := resp.GetNewTestVariants()
		assert.Loosely(t, len(nt), should.Equal(3))
		expected := []*pb.QueryNewTestVariantsResponse_NewTestVariant{
			{TestId: "A", VariantHash: vh},
			{TestId: "B", VariantHash: vh},
			{TestId: "E", VariantHash: vh},
		}
		assert.Loosely(t, nt, should.Match(expected))
	})
}
