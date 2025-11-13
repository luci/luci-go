// Copyright 2025 The LUCI Authors.
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
	"strings"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestQueryWorkUnits(t *testing.T) {
	ftt.Run("QueryWorkUnits", t, func(t *ftt.Test) {
		const (
			rootInvRealm = "testproject:root_inv"
			fullRealm    = "testproject:full_access"
			limitedRealm = "testproject:limited_access"
		)
		rootInvID := rootinvocations.ID("root-inv-id")
		rootWuID := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "root"}
		gpWuID := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "grandparent"}
		pWuID := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "parent"}
		cWuID := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "child"}

		ctx := testutil.SpannerTestContext(t)

		// Insert a root invocation and hierarchy of work units.
		// Root -> Grandparent -> Parent -> Child
		rootInv := rootinvocations.NewBuilder(rootInvID).WithRealm(rootInvRealm).Build()

		// Root WU gets FULL access realm.
		rootWu := workunits.NewBuilder(rootInvID, rootWuID.WorkUnitID).WithRealm(fullRealm).Build()

		extraProps, _ := structpb.NewStruct(map[string]any{"key": "value"})
		// Grandparent gets LIMITED access realm.
		gpWu := workunits.NewBuilder(rootInvID, gpWuID.WorkUnitID).
			WithRealm(limitedRealm).
			WithParentWorkUnitID(rootWuID.WorkUnitID).
			WithExtendedProperties(map[string]*structpb.Struct{"ns": extraProps}).
			Build()
		// Parent gets LIMITED access realm.
		pWu := workunits.NewBuilder(rootInvID, pWuID.WorkUnitID).
			WithRealm(limitedRealm).
			WithParentWorkUnitID(gpWuID.WorkUnitID).
			Build()
		// Child gets LIMITED access realm.
		cWu := workunits.NewBuilder(rootInvID, cWuID.WorkUnitID).
			WithRealm(limitedRealm).
			WithParentWorkUnitID(pWuID.WorkUnitID).
			Build()

		testutil.MustApply(ctx, t, insert.RootInvocationOnly(rootInv)...)
		testutil.MustApply(ctx, t, testutil.CombineMutations(
			insert.WorkUnit(rootWu),
			insert.WorkUnit(gpWu),
			insert.WorkUnit(pWu),
			insert.WorkUnit(cWu),
		)...)

		// Default authorisation: Full access to everything via Root Invocation realm.
		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: rootInvRealm, Permission: rdbperms.PermListLimitedWorkUnits},
				{Realm: rootInvRealm, Permission: rdbperms.PermGetWorkUnit},
			},
		}
		ctx = auth.WithState(ctx, authState)

		srv := newTestResultDBService()
		baseReq := &pb.QueryWorkUnitsRequest{
			Parent: rootInvID.Name(),
			Predicate: &pb.WorkUnitPredicate{
				AncestorsOf: cWuID.Name(),
			},
		}

		t.Run("happy path", func(t *ftt.Test) {
			t.Run("full access", func(t *ftt.Test) {
				req := baseReq
				t.Run("default view", func(t *ftt.Test) {
					rsp, err := srv.QueryWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp.WorkUnits, should.HaveLength(3))
					// Order: Parent, Grandparent, Root
					assert.Loosely(t, rsp.WorkUnits[0].Name, should.Equal(pWuID.Name()))
					assert.Loosely(t, rsp.WorkUnits[1].Name, should.Equal(gpWuID.Name()))
					assert.Loosely(t, rsp.WorkUnits[2].Name, should.Equal(rootWuID.Name()))
				})

				t.Run("basic view", func(t *ftt.Test) {
					req.View = pb.WorkUnitView_WORK_UNIT_VIEW_BASIC
					rsp, err := srv.QueryWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp.WorkUnits, should.HaveLength(3))
					// Grandparent has ExtendedProperties, verify they are masked in Basic view.
					assert.Loosely(t, rsp.WorkUnits[1].Name, should.Equal(gpWuID.Name()))
					assert.Loosely(t, rsp.WorkUnits[1].ExtendedProperties, should.BeNil)
				})

				t.Run("full view", func(t *ftt.Test) {
					req.View = pb.WorkUnitView_WORK_UNIT_VIEW_FULL
					rsp, err := srv.QueryWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rsp.WorkUnits, should.HaveLength(3))
					// Grandparent has ExtendedProperties, verify they are present in Full view.
					assert.Loosely(t, rsp.WorkUnits[1].Name, should.Equal(gpWuID.Name()))
					assert.Loosely(t, rsp.WorkUnits[1].ExtendedProperties, should.NotBeNil)
				})
			})

			t.Run("limited access", func(t *ftt.Test) {
				// Remove PermGetWorkUnit from rootInvRealm so individual WU realms matter.
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: rootInvRealm, Permission: rdbperms.PermListLimitedWorkUnits},
					{Realm: fullRealm, Permission: rdbperms.PermGetWorkUnit},
					// No PermGetWorkUnit for limitedRealm.
				}
				ctx = auth.WithState(ctx, authState)

				req := baseReq
				req.View = pb.WorkUnitView_WORK_UNIT_VIEW_FULL
				rsp, err := srv.QueryWorkUnits(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, rsp.WorkUnits, should.HaveLength(3))

				// Parent (index 0) is in limitedRealm. Masked even if Full view requested.
				assert.Loosely(t, rsp.WorkUnits[0].Name, should.Equal(pWuID.Name()))
				assert.Loosely(t, rsp.WorkUnits[0].IsMasked, should.BeTrue)

				// Grandparent (index 1) is in limitedRealm. ExtendedProperties masked.
				assert.Loosely(t, rsp.WorkUnits[1].Name, should.Equal(gpWuID.Name()))
				assert.Loosely(t, rsp.WorkUnits[1].IsMasked, should.BeTrue)
				assert.Loosely(t, rsp.WorkUnits[1].ExtendedProperties, should.BeNil)

				// Root (index 2) is in fullRealm. Not masked.
				assert.Loosely(t, rsp.WorkUnits[2].Name, should.Equal(rootWuID.Name()))
				assert.Loosely(t, rsp.WorkUnits[2].IsMasked, should.BeFalse)
			})

			t.Run("summary markdown truncation", func(t *ftt.Test) {
				// Use new hierarchy dedicated to this test to avoid conflicts.
				longParentID := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "long-parent"}
				longParent := workunits.NewBuilder(rootInvID, longParentID.WorkUnitID).
					WithRealm(limitedRealm). // Limited realm to trigger masking
					WithParentWorkUnitID(rootWuID.WorkUnitID).
					WithSummaryMarkdown(strings.Repeat("a", 4096)). // Long enough to be truncated
					Build()
				childID := workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "child-of-long"}
				child := workunits.NewBuilder(rootInvID, childID.WorkUnitID).
					WithRealm(limitedRealm).
					WithParentWorkUnitID(longParentID.WorkUnitID).
					Build()

				testutil.MustApply(ctx, t, testutil.CombineMutations(
					insert.WorkUnit(longParent),
					insert.WorkUnit(child),
				)...)

				req := baseReq
				req.Predicate.AncestorsOf = childID.Name()
				// Set Limited access to trigger truncation logic.
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: rootInvRealm, Permission: rdbperms.PermListLimitedWorkUnits},
					{Realm: limitedRealm, Permission: rdbperms.PermListLimitedWorkUnits},
				}
				ctx = auth.WithState(ctx, authState)

				rsp, err := srv.QueryWorkUnits(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				// Verify long parent is truncated and masked.
				assert.Loosely(t, rsp.WorkUnits[0].Name, should.Equal(longParentID.Name()))
				assert.Loosely(t, rsp.WorkUnits[0].IsMasked, should.BeTrue)
				assert.Loosely(t, len(rsp.WorkUnits[0].SummaryMarkdown), should.BeLessThan(4096))
				assert.Loosely(t, rsp.WorkUnits[0].SummaryMarkdown, should.HaveSuffix("..."))
			})
		})

		t.Run("start node does not exist", func(t *ftt.Test) {
			req := baseReq
			req.Predicate.AncestorsOf = workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "non-existent"}.Name()
			_, err := srv.QueryWorkUnits(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.That(t, err, should.ErrLike(`"rootInvocations/root-inv-id/workUnits/non-existent" not found`))
		})

		t.Run("request authorization", func(t *ftt.Test) {
			req := baseReq
			authState.IdentityPermissions = nil
			ctx = auth.WithState(ctx, authState)
			_, err := srv.QueryWorkUnits(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.That(t, err, should.ErrLike(`caller does not have permission resultdb.workUnits.listLimited in realm of root invocation "rootInvocations/root-inv-id"`))
		})

		t.Run("request validation", func(t *ftt.Test) {
			t.Run("parent", func(t *ftt.Test) {
				t.Run("empty", func(t *ftt.Test) {
					req := baseReq
					req.Parent = ""
					_, err := srv.QueryWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("parent: unspecified"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					req := baseReq
					req.Parent = "invalid"
					_, err := srv.QueryWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("parent: does not match pattern"))
				})
			})

			t.Run("view", func(t *ftt.Test) {
				req := baseReq
				req.View = pb.WorkUnitView(999)
				_, err := srv.QueryWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("view: unrecognized view"))
			})

			t.Run("predicate", func(t *ftt.Test) {
				req := baseReq
				t.Run("nil", func(t *ftt.Test) {
					req.Predicate = nil
					_, err := srv.QueryWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("predicate: unspecified"))
				})
				t.Run("ancestors_of unspecified", func(t *ftt.Test) {
					req.Predicate = &pb.WorkUnitPredicate{}
					_, err := srv.QueryWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("predicate: ancestors_of: unspecified"))
				})
				t.Run("ancestors_of invalid format", func(t *ftt.Test) {
					req.Predicate = &pb.WorkUnitPredicate{AncestorsOf: "invalid"}
					_, err := srv.QueryWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("predicate: ancestors_of: does not match pattern"))
				})
				t.Run("ancestors_of wrong root", func(t *ftt.Test) {
					req.Predicate = &pb.WorkUnitPredicate{
						AncestorsOf: workunits.ID{RootInvocationID: "other-root", WorkUnitID: "child"}.Name(),
					}
					_, err := srv.QueryWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("does not belong to the parent root invocation"))
				})
			})
		})
	})
}
