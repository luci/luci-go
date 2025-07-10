// Copyright 2022 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package permissions

import (
	"testing"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestVerifyInvocations(t *testing.T) {
	ftt.Run(`VerifyInvocations`, t, func(t *ftt.Test) {
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
			ctx, t,
			insert.Invocation("i0", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:r0"}),
			insert.Invocation("i1", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:r1"}),
			insert.Invocation("i2", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:r2"}),
			insert.Invocation("i3", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:r3"}),
			insert.Invocation("i3b", pb.Invocation_ACTIVE, map[string]any{"Realm": "testproject:r3"}),
		)

		t.Run("Access allowed", func(t *ftt.Test) {
			ids := invocations.NewIDSet(invocations.ID("i1"), invocations.ID("i2"))
			err := VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListArtifacts)
			assert.Loosely(t, err, should.BeNil)

			ids = invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"))
			err = VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults)
			assert.Loosely(t, err, should.BeNil)
		})
		t.Run("Access denied", func(t *ftt.Test) {
			ids := invocations.NewIDSet(invocations.ID("i0"))
			err := VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListArtifacts)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("resultdb.artifacts.list in realm of invocation i0"))

			ids = invocations.NewIDSet(invocations.ID("i1"), invocations.ID("i2"))
			err = VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListArtifacts, rdbperms.PermListTestExonerations)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("resultdb.testExonerations.list in realm of invocation i1"))

			ids = invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"))
			err = VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults, rdbperms.PermListArtifacts)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("resultdb.artifacts.list in realm of invocation i3"))
		})
		t.Run("Duplicate invocations", func(t *ftt.Test) {
			ids := invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"), invocations.ID("i3"))
			err := VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults)
			assert.Loosely(t, err, should.BeNil)

			ids = invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"), invocations.ID("i3"))
			err = VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults, rdbperms.PermListArtifacts)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("resultdb.artifacts.list in realm of invocation i3"))
		})
		t.Run("Duplicate realms", func(t *ftt.Test) {
			ids := invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"), invocations.ID("i3b"))
			err := VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults)
			assert.Loosely(t, err, should.BeNil)

			ids = invocations.NewIDSet(invocations.ID("i2"), invocations.ID("i3"), invocations.ID("i3b"))
			err = VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults, rdbperms.PermListArtifacts)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("resultdb.artifacts.list in realm of invocation i3"))
		})
		t.Run("Invocations do not exist", func(t *ftt.Test) {
			ids := invocations.NewIDSet(invocations.ID("i2"), invocations.ID("iX"))
			err := VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("invocations/iX not found"))

			ids = invocations.NewIDSet(invocations.ID("i2"), invocations.ID(""))
			err = VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike("invocations/ not found"))
		})
		t.Run("No invocations", func(t *ftt.Test) {
			ids := invocations.NewIDSet()
			err := VerifyInvocations(span.Single(ctx), ids, rdbperms.PermListTestExonerations, rdbperms.PermListTestResults)
			assert.Loosely(t, err, should.BeNil)
		})
	})
}

func TestQueryWorkUnitAccess(t *testing.T) {
	ftt.Run(`QueryWorkUnitAccess`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		rootInvID := rootinvocations.ID("root-inv")
		workUnitID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "work-unit",
		}

		// Insert root invocation for subsequent tests.
		var ms []*spanner.Mutation
		ms = append(ms, insert.RootInvocation(
			rootinvocations.NewBuilder(rootInvID).WithRealm("testproject:root").Build(),
		)...)
		ms = append(ms, insert.WorkUnit(
			workunits.NewBuilder(rootInvID, "work-unit").WithRealm("testproject:workunit").Build(),
		)...)
		testutil.MustApply(ctx, t, ms...)

		opts := QueryWorkUnitAccessOptions{
			Full:                 rdbperms.PermListWorkUnits,
			Limited:              rdbperms.PermListLimitedWorkUnits,
			UpgradeLimitedToFull: rdbperms.PermGetWorkUnit,
		}

		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
		}
		ctx = auth.WithState(ctx, authState)

		ctx, cancel := span.ReadOnlyTransaction(ctx)
		defer cancel()

		t.Run("root invocation not found", func(t *ftt.Test) {
			workUnitID.RootInvocationID = rootinvocations.ID("not-exists")
			_, err := QueryWorkUnitAccess(ctx, workUnitID, opts)
			st, ok := appstatus.Get(err)
			assert.That(t, ok, should.BeTrue)
			assert.That(t, st.Code(), should.Equal(codes.NotFound))
			assert.That(t, st.Message(), should.ContainSubstring("rootInvocations/not-exists not found"))
		})
		t.Run("no access on root invocation", func(t *ftt.Test) {
			level, err := QueryWorkUnitAccess(ctx, workUnitID, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, level, should.Equal(NoAccess))
		})
		t.Run("full access on root invocation", func(t *ftt.Test) {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{Realm: "testproject:root", Permission: rdbperms.PermListWorkUnits},
			}
			level, err := QueryWorkUnitAccess(ctx, workUnitID, opts)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, level, should.Equal(FullAccess))
		})
		t.Run("limited access on root invocation", func(t *ftt.Test) {
			t.Run("baseline (no upgrade permission)", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedWorkUnits},
				}
				level, err := QueryWorkUnitAccess(ctx, workUnitID, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, level, should.Equal(LimitedAccess))
			})
			t.Run("upgrade to full access", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedWorkUnits},
					{Realm: "testproject:workunit", Permission: rdbperms.PermGetWorkUnit},
				}
				level, err := QueryWorkUnitAccess(ctx, workUnitID, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, level, should.Equal(FullAccess))
			})
			t.Run("work unit not found", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: "testproject:root", Permission: rdbperms.PermListLimitedWorkUnits},
				}
				workUnitID.WorkUnitID = "not-exists"
				_, err := QueryWorkUnitAccess(ctx, workUnitID, opts)
				st, ok := appstatus.Get(err)
				assert.That(t, ok, should.BeTrue)
				assert.That(t, st.Code(), should.Equal(codes.NotFound))
				assert.That(t, st.Message(), should.ContainSubstring("rootInvocations/root-inv/workUnits/not-exists not found"))
			})
		})
	})
}
