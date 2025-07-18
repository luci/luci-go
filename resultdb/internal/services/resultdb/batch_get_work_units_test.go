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

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

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
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func TestBatchGetWorkUnits(t *testing.T) {
	ftt.Run("BatchGetWorkUnits", t, func(t *ftt.Test) {
		const (
			rootRealm = "testproject:root"
			wu1Realm  = "testproject:workunit1"
			wu2Realm  = "testproject:workunit2"
		)
		rootInvID := rootinvocations.ID("root-inv-id")

		ctx := testutil.SpannerTestContext(t)

		// Insert a root invocation and work units.
		rootInv := rootinvocations.NewBuilder(rootInvID).WithRealm(rootRealm).Build()
		wu1 := workunits.NewBuilder(rootInvID, "wu1").WithRealm(wu1Realm).Build()
		wu2 := workunits.NewBuilder(rootInvID, "wu2").WithRealm(wu2Realm).WithMinimalFields().Build()
		wuChild1 := workunits.NewBuilder(rootInvID, "wu11").WithParentWorkUnitID("wu1").WithRealm(wu2Realm).Build()

		var ms []*spanner.Mutation
		ms = append(ms, insert.RootInvocationWithRootWorkUnit(rootInv)...)
		ms = append(ms, insert.WorkUnit(wu1)...)
		ms = append(ms, insert.WorkUnit(wu2)...)
		ms = append(ms, insert.WorkUnit(wuChild1)...)
		testutil.MustApply(ctx, t, ms...)

		// Setup authorisation.
		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
			IdentityPermissions: []authtest.RealmPermission{
				{Realm: rootRealm, Permission: rdbperms.PermGetWorkUnit},
			},
		}
		ctx = auth.WithState(ctx, authState)

		req := &pb.BatchGetWorkUnitsRequest{
			Parent: "rootInvocations/root-inv-id",
			Names: []string{
				"rootInvocations/root-inv-id/workUnits/wu1",
				"rootInvocations/root-inv-id/workUnits/wu2",
			},
		}

		srv := newTestResultDBService()

		t.Run("happy path", func(t *ftt.Test) {
			expectedWu1 := &pb.WorkUnit{
				Name:              wu1.ID.Name(),
				WorkUnitId:        wu1.ID.WorkUnitID,
				State:             wu1.State,
				Realm:             wu1.Realm,
				CreateTime:        pbutil.MustTimestampProto(wu1.CreateTime),
				Creator:           wu1.CreatedBy,
				FinalizeStartTime: pbutil.MustTimestampProto(wu1.FinalizeStartTime.Time),
				FinalizeTime:      pbutil.MustTimestampProto(wu1.FinalizeTime.Time),
				Deadline:          pbutil.MustTimestampProto(wu1.Deadline),
				Parent:            workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "root"}.Name(),
				ChildWorkUnits: []string{
					workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "wu11"}.Name(),
				},
				ProducerResource: wu1.ProducerResource,
				Tags:             wu1.Tags,
				Properties:       wu1.Properties,
				Instructions:     wu1.Instructions,
				IsMasked:         false,
			}
			expectedWu2 := &pb.WorkUnit{
				Name:             wu2.ID.Name(),
				WorkUnitId:       wu2.ID.WorkUnitID,
				State:            wu2.State,
				Realm:            wu2.Realm,
				CreateTime:       pbutil.MustTimestampProto(wu2.CreateTime),
				Creator:          wu2.CreatedBy,
				Deadline:         pbutil.MustTimestampProto(wu2.Deadline),
				Parent:           workunits.ID{RootInvocationID: rootInvID, WorkUnitID: "root"}.Name(),
				ProducerResource: wu2.ProducerResource,
				Tags:             wu2.Tags,
				Properties:       wu2.Properties,
				Instructions:     wu2.Instructions,
				IsMasked:         false,
			}

			t.Run("full access", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: rootRealm, Permission: rdbperms.PermGetWorkUnit},
				}
				t.Run("default view", func(t *ftt.Test) {
					rsp, err := srv.BatchGetWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, rsp.WorkUnits, should.Match([]*pb.WorkUnit{expectedWu1, expectedWu2}))
				})
				t.Run("full view", func(t *ftt.Test) {
					req.View = pb.WorkUnitView_WORK_UNIT_VIEW_FULL
					expectedWu1.ExtendedProperties = wu1.ExtendedProperties
					expectedWu2.ExtendedProperties = wu2.ExtendedProperties

					rsp, err := srv.BatchGetWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, rsp.WorkUnits, should.Match([]*pb.WorkUnit{expectedWu1, expectedWu2}))
				})
			})
			t.Run("limited access", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: rootRealm, Permission: rdbperms.PermListLimitedWorkUnits},
				}

				expectedWu1.Tags = nil
				expectedWu1.Properties = nil
				expectedWu1.Instructions = nil
				expectedWu1.ExtendedProperties = nil
				expectedWu1.IsMasked = true

				expectedWu2.Tags = nil
				expectedWu2.Properties = nil
				expectedWu2.Instructions = nil
				expectedWu2.ExtendedProperties = nil
				expectedWu2.IsMasked = true

				t.Run("default view", func(t *ftt.Test) {
					rsp, err := srv.BatchGetWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, rsp.WorkUnits, should.Match([]*pb.WorkUnit{expectedWu1, expectedWu2}))
				})
				t.Run("full view", func(t *ftt.Test) {
					req.View = pb.WorkUnitView_WORK_UNIT_VIEW_FULL

					// No additional fields should be exposed because we have limited access.
					rsp, err := srv.BatchGetWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, rsp.WorkUnits, should.Match([]*pb.WorkUnit{expectedWu1, expectedWu2}))
				})
			})
			t.Run("mixed access", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{Realm: rootRealm, Permission: rdbperms.PermListLimitedWorkUnits},
					{Realm: wu1Realm, Permission: rdbperms.PermGetWorkUnit},
				}

				expectedWu2.Tags = nil
				expectedWu2.Properties = nil
				expectedWu2.Instructions = nil
				expectedWu2.ExtendedProperties = nil
				expectedWu2.IsMasked = true

				t.Run("default view", func(t *ftt.Test) {
					rsp, err := srv.BatchGetWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, rsp.WorkUnits, should.Match([]*pb.WorkUnit{expectedWu1, expectedWu2}))
				})
				t.Run("full view", func(t *ftt.Test) {
					req.View = pb.WorkUnitView_WORK_UNIT_VIEW_FULL
					expectedWu1.ExtendedProperties = wu1.ExtendedProperties

					rsp, err := srv.BatchGetWorkUnits(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, rsp.WorkUnits, should.Match([]*pb.WorkUnit{expectedWu1, expectedWu2}))
				})
			})
		})

		t.Run("does not exist", func(t *ftt.Test) {
			req.Names = []string{wu1.ID.Name(), "rootInvocations/root-inv-id/workUnits/non-existent"}
			_, err := srv.BatchGetWorkUnits(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.That(t, err, should.ErrLike(`"rootInvocations/root-inv-id/workUnits/non-existent" not found`))
		})

		t.Run("permission denied", func(t *ftt.Test) {
			authState.IdentityPermissions = nil
			_, err := srv.BatchGetWorkUnits(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.That(t, err, should.ErrLike(`caller does not have permission resultdb.workUnits.get (or resultdb.workUnits.listLimited) on root invocation "rootInvocations/root-inv-id"`))
		})

		t.Run("invalid request", func(t *ftt.Test) {
			t.Run("parent", func(t *ftt.Test) {
				t.Run("invalid", func(t *ftt.Test) {
					req.Parent = "invalid-name"
					_, err := srv.BatchGetWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("parent: does not match"))
				})
				t.Run("empty", func(t *ftt.Test) {
					req.Parent = ""
					_, err := srv.BatchGetWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("parent: unspecified"))
				})
			})
			t.Run("names", func(t *ftt.Test) {
				t.Run("invalid", func(t *ftt.Test) {
					req.Names = []string{"invalid-name"}
					_, err := srv.BatchGetWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("names[0]: does not match"))
				})
				t.Run("empty item", func(t *ftt.Test) {
					req.Names = []string{""}
					_, err := srv.BatchGetWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("names[0]: unspecified"))
				})
				t.Run("empty list", func(t *ftt.Test) {
					req.Names = []string{}
					_, err := srv.BatchGetWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("names: must have at least one request"))
				})
				t.Run("different root invocations", func(t *ftt.Test) {
					req.Names = []string{wu1.ID.Name(), "rootInvocations/other-root/workUnits/wu"}
					_, err := srv.BatchGetWorkUnits(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike(`names[1]: does not match parent root invocation "rootInvocations/root-inv-id"`))
				})
			})
			t.Run("view", func(t *ftt.Test) {
				req.View = pb.WorkUnitView(999)
				_, err := srv.BatchGetWorkUnits(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("view: unrecognized view"))
			})
		})
	})
}
