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
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/resultdb/internal/instructionutil"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestGetWorkUnit(t *testing.T) {
	ftt.Run("GetWorkUnit", t, func(t *ftt.Test) {
		const (
			rootRealm = "testproject:root"
			wuRealm   = "testproject:workunit"
		)
		rootInvID := rootinvocations.ID("root-inv-id")
		rootWorkUnitID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "root",
		}

		ctx := testutil.SpannerTestContext(t)

		// Insert a root invocation and a work unit.
		rootInv := rootinvocations.NewBuilder(rootInvID).WithRealm(rootRealm).Build()
		wu := workunits.NewBuilder(rootInvID, rootWorkUnitID.WorkUnitID).WithRealm(wuRealm).Build()
		testutil.MustApply(ctx, t, insert.RootInvocation(rootInv)...)
		testutil.MustApply(ctx, t, insert.WorkUnit(wu)...)

		// Insert a child work unit.
		childWuID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "child-work-unit-id",
		}
		childWu := workunits.NewBuilder(rootInvID, childWuID.WorkUnitID).
			WithRealm(wuRealm).
			WithParentWorkUnitID(rootWorkUnitID.WorkUnitID).
			Build()
		testutil.MustApply(ctx, t, insert.WorkUnit(childWu)...)

		// Setup authorisation.
		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: rootRealm, Permission: rdbperms.PermGetWorkUnit},
			},
		}
		ctx = auth.WithState(ctx, authState)

		req := &pb.GetWorkUnitRequest{
			Name: rootWorkUnitID.Name(),
		}

		srv := newTestResultDBService()

		t.Run("valid", func(t *ftt.Test) {
			expectedRsp := &pb.WorkUnit{
				Name:              rootWorkUnitID.Name(),
				WorkUnitId:        rootWorkUnitID.WorkUnitID,
				State:             wu.State,
				Realm:             wu.Realm,
				CreateTime:        pbutil.MustTimestampProto(wu.CreateTime),
				Creator:           wu.CreatedBy,
				FinalizeStartTime: pbutil.MustTimestampProto(wu.FinalizeStartTime.Time),
				FinalizeTime:      pbutil.MustTimestampProto(wu.FinalizeTime.Time),
				Deadline:          pbutil.MustTimestampProto(wu.Deadline),
				Parent:            rootInvID.Name(),
				ProducerResource:  wu.ProducerResource,
				Tags:              wu.Tags,
				Properties:        wu.Properties,
				Instructions:      instructionutil.InstructionsWithNames(wu.Instructions, rootWorkUnitID.Name()),
				IsMasked:          false,
			}

			t.Run("full access", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: rootRealm, Permission: rdbperms.PermGetWorkUnit},
				}
				t.Run("default view", func(t *ftt.Test) {
					rsp, err := srv.GetWorkUnit(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, rsp, should.Match(expectedRsp))
				})
				t.Run("basic view", func(t *ftt.Test) {
					req.View = pb.WorkUnitView_WORK_UNIT_VIEW_BASIC

					rsp, err := srv.GetWorkUnit(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, rsp, should.Match(expectedRsp))
				})
				t.Run("full view", func(t *ftt.Test) {
					req.View = pb.WorkUnitView_WORK_UNIT_VIEW_FULL
					expectedRsp.ExtendedProperties = wu.ExtendedProperties

					rsp, err := srv.GetWorkUnit(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, rsp, should.Match(expectedRsp))
				})
				t.Run("masked access upgraded to full", func(t *ftt.Test) {
					authState.IdentityPermissions = []authtest.RealmPermission{
						{Realm: rootRealm, Permission: rdbperms.PermListLimitedWorkUnits},
						{Realm: wuRealm, Permission: rdbperms.PermGetWorkUnit},
					}
					rsp, err := srv.GetWorkUnit(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, rsp, should.Match(expectedRsp))
				})
			})
			t.Run("limited access", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: rootRealm, Permission: rdbperms.PermListLimitedWorkUnits},
				}

				expectedRsp.Tags = nil
				expectedRsp.Properties = nil
				expectedRsp.Instructions = nil
				expectedRsp.ExtendedProperties = nil
				expectedRsp.IsMasked = true

				t.Run("default view", func(t *ftt.Test) {
					rsp, err := srv.GetWorkUnit(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, rsp, should.Match(expectedRsp))
				})
				t.Run("basic view", func(t *ftt.Test) {
					req.View = pb.WorkUnitView_WORK_UNIT_VIEW_BASIC

					rsp, err := srv.GetWorkUnit(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, rsp, should.Match(expectedRsp))
				})
				t.Run("full view", func(t *ftt.Test) {
					req.View = pb.WorkUnitView_WORK_UNIT_VIEW_FULL

					rsp, err := srv.GetWorkUnit(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, rsp, should.Match(expectedRsp))
				})
			})
			t.Run("child work unit", func(t *ftt.Test) {
				req.Name = childWuID.Name()
				rsp, err := srv.GetWorkUnit(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, rsp.Parent, should.Equal(rootWorkUnitID.Name()))
			})
		})

		t.Run("does not exist", func(t *ftt.Test) {
			req.Name = "rootInvocations/root-inv-id/workUnits/non-existent"
			_, err := srv.GetWorkUnit(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.That(t, err, should.ErrLike("rootInvocations/root-inv-id/workUnits/non-existent not found"))
		})

		t.Run("permission denied", func(t *ftt.Test) {
			authState.IdentityPermissions = nil
			_, err := srv.GetWorkUnit(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.That(t, err, should.ErrLike("caller does not have permission resultdb.workUnits.get (or resultdb.workUnits.listLimited) on root invocation rootInvocations/root-inv-id"))
		})

		t.Run("invalid request", func(t *ftt.Test) {
			t.Run("name", func(t *ftt.Test) {
				t.Run("invalid", func(t *ftt.Test) {
					req.Name = "invalid-name"
					_, err := srv.GetWorkUnit(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("name: does not match"))
				})
				t.Run("empty", func(t *ftt.Test) {
					req.Name = ""
					_, err := srv.GetWorkUnit(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("name: unspecified"))
				})
			})
			t.Run("view", func(t *ftt.Test) {
				req.Name = rootWorkUnitID.Name()
				req.View = pb.WorkUnitView(999)
				_, err := srv.GetWorkUnit(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("view: unrecognized view"))
			})
		})
	})
}
