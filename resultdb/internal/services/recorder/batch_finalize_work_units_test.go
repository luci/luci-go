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

package recorder

import (
	"fmt"
	"testing"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestVerifyBatchFinalizeWorkUnitsPermissions(t *testing.T) {
	t.Parallel()

	ftt.Run(`VerifyBatchFinalizeWorkUnitsPermissions`, t, func(t *ftt.Test) {
		ctx := testutil.TestingContext()

		req := &pb.BatchFinalizeWorkUnitsRequest{
			Requests: []*pb.FinalizeWorkUnitRequest{
				{Name: "rootInvocations/root-inv-id/workUnits/wu"},
				{Name: "rootInvocations/root-inv-id/workUnits/wu:a"},
				{Name: "rootInvocations/root-inv-id/workUnits/wu:b"},
			},
		}

		commonPrefixWorkUnitID := workunits.ID{
			RootInvocationID: "root-inv-id",
			WorkUnitID:       "wu",
		}

		token, err := generateWorkUnitUpdateToken(ctx, commonPrefixWorkUnitID)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		t.Run("valid", func(t *ftt.Test) {
			err := verifyBatchFinalizeWorkUnitsPermissions(ctx, req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("request count is zero", func(t *ftt.Test) {
			req.Requests = []*pb.FinalizeWorkUnitRequest{}
			err := verifyBatchFinalizeWorkUnitsPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("requests: must have at least one request"))
		})

		t.Run("request count is too high", func(t *ftt.Test) {
			req.Requests = make([]*pb.FinalizeWorkUnitRequest, 501)
			for i := 0; i < len(req.Requests); i++ {
				req.Requests[i] = &pb.FinalizeWorkUnitRequest{Name: fmt.Sprintf("rootInvocations/root-inv-id/workUnits/wu:%v", i)}
			}
			err := verifyBatchFinalizeWorkUnitsPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("requests: the number of requests in the batch (501) exceeds 500"))
		})

		t.Run("unspecified name", func(t *ftt.Test) {
			req.Requests[1].Name = ""
			err := verifyBatchFinalizeWorkUnitsPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("requests[1]: name: unspecified"))
		})

		t.Run("invalid name", func(t *ftt.Test) {
			req.Requests[1].Name = "invalid name"
			err := verifyBatchFinalizeWorkUnitsPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("requests[1]: name: does not match"))
		})

		t.Run("name appears more than once", func(t *ftt.Test) {
			req.Requests = []*pb.FinalizeWorkUnitRequest{
				{Name: "rootInvocations/root-inv-id/workUnits/same-name"},
				{Name: "rootInvocations/root-inv-id/workUnits/same-name"},
			}
			err := verifyBatchFinalizeWorkUnitsPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`requests[1]: name: "rootInvocations/root-inv-id/workUnits/same-name" appears more than once in the request`))
		})

		t.Run("request has multiple different root invocations", func(t *ftt.Test) {
			req.Requests = []*pb.FinalizeWorkUnitRequest{
				{Name: "rootInvocations/root-inv-id/workUnits/wu"},
				{Name: "rootInvocations/root-inv-id-2/workUnits/wu"},
			}
			err := verifyBatchFinalizeWorkUnitsPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`requests[1]: name: all work units in a batch must belong to the same root invocation; got "rootInvocations/root-inv-id-2", want "rootInvocations/root-inv-id"`))
		})

		t.Run("request requires multiple different update tokens", func(t *ftt.Test) {
			req.Requests = []*pb.FinalizeWorkUnitRequest{
				{Name: "rootInvocations/root-inv-id/workUnits/wu"},
				{Name: "rootInvocations/root-inv-id/workUnits/wu2"},
			}
			err := verifyBatchFinalizeWorkUnitsPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike(`requests[1]: name: work unit "rootInvocations/root-inv-id/workUnits/wu2" requires a different update token to request[0]'s "rootInvocations/root-inv-id/workUnits/wu", but this RPC only accepts one update token`))
		})

		t.Run("invalid token", func(t *ftt.Test) {
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid"))
			err := verifyBatchFinalizeWorkUnitsPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("invalid update token"))
		})

		t.Run("missing token", func(t *ftt.Test) {
			ctx = metadata.NewIncomingContext(ctx, metadata.MD{})
			err := verifyBatchFinalizeWorkUnitsPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.Unauthenticated))
			assert.Loosely(t, err, should.ErrLike("missing update-token metadata value"))
		})

		t.Run("too many tokens", func(t *ftt.Test) {
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token, pb.UpdateTokenMetadataKey, token))
			err := verifyBatchFinalizeWorkUnitsPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("expected exactly one update-token metadata value, got 2"))
		})
	})
}

func TestBatchFinalizeWorkUnits(t *testing.T) {
	ftt.Run("TestBatchFinalizeWorkUnits", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, sched := tq.TestingContext(ctx, nil)
		recorder := newTestRecorderServer()

		rootInvID := rootinvocations.ID("root-inv-id")
		wuID1 := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "work-unit-id:child1",
		}
		wuID2 := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "work-unit-id:child2",
		}

		token, err := generateWorkUnitUpdateToken(ctx, wuID1)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		req := &pb.BatchFinalizeWorkUnitsRequest{
			Requests: []*pb.FinalizeWorkUnitRequest{
				{Name: wuID1.Name()},
				{Name: wuID2.Name()},
			},
		}

		// Mock auth such that test-unit-id-2 has no permissions.
		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
		}
		ctx = auth.WithState(ctx, authState)

		t.Run("invalid request", func(t *ftt.Test) {
			// Do not need to test all cases, the tests for VerifyBatchFinalizeWorkUnitsPermissions
			// verifies that. We simply want to check that method is called.
			req.Requests[0].Name = "invalid name"
			_, err := recorder.BatchFinalizeWorkUnits(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.That(t, err, should.ErrLike("name: does not match"))
		})

		t.Run("permission denied", func(t *ftt.Test) {
			// Do not need to test all cases, the tests for VerifyBatchFinalizeWorkUnitsPermissions
			// verifies that. We simply want to check that method is called.
			badCtx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid"))
			_, err := recorder.BatchFinalizeWorkUnits(badCtx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.That(t, err, should.ErrLike("invalid update token"))
		})

		t.Run("not found", func(t *ftt.Test) {
			// This would normally only happen if a row was manually deleted from
			// the underlying table as the work unit update token could not have
			// been minted otherwise.
			_, err := recorder.BatchFinalizeWorkUnits(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.That(t, err, should.ErrLike(`"rootInvocations/root-inv-id/workUnits/work-unit-id:child1" not found`))
		})

		t.Run("success", func(t *ftt.Test) {
			// Insert a root invocation and the work unit.
			rootInv := rootinvocations.NewBuilder(rootInvID).WithRealm("testproject:testrealm").Build()
			wu1 := workunits.NewBuilder(rootInvID, wuID1.WorkUnitID).WithState(pb.WorkUnit_ACTIVE).Build()
			wu2 := workunits.NewBuilder(rootInvID, wuID2.WorkUnitID).WithState(pb.WorkUnit_ACTIVE).Build()

			testutil.MustApply(ctx, t, insert.RootInvocationWithRootWorkUnit(rootInv)...)
			testutil.MustApply(ctx, t, insert.WorkUnit(wu1)...)
			testutil.MustApply(ctx, t, insert.WorkUnit(wu2)...)

			resp, err := recorder.BatchFinalizeWorkUnits(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp.WorkUnits[0].State, should.Equal(pb.WorkUnit_FINALIZING))
			assert.Loosely(t, resp.WorkUnits[0].FinalizeStartTime, should.NotBeNil)
			finalizeTime := resp.WorkUnits[0].FinalizeStartTime.AsTime()
			assert.Loosely(t, resp.WorkUnits[0].LastUpdated.AsTime(), should.Match(finalizeTime))
			assert.Loosely(t, resp.WorkUnits[1].State, should.Equal(pb.WorkUnit_FINALIZING))
			assert.Loosely(t, resp.WorkUnits[1].FinalizeStartTime.AsTime(), should.Match(finalizeTime))
			assert.Loosely(t, resp.WorkUnits[1].LastUpdated.AsTime(), should.Match(finalizeTime))

			// Read the work unit from Spanner to confirm it's really FINALIZING.
			wuRow1, err := workunits.Read(span.Single(ctx), wuID1, workunits.AllFields)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, wuRow1.State, should.Equal(pb.WorkUnit_FINALIZING))
			assert.Loosely(t, wuRow1.LastUpdated, should.Match(finalizeTime))
			assert.Loosely(t, wuRow1.FinalizeStartTime, should.Match(spanner.NullTime{Valid: true, Time: finalizeTime}))

			wuRow2, err := workunits.Read(span.Single(ctx), wuID2, workunits.AllFields)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, wuRow2.State, should.Equal(pb.WorkUnit_FINALIZING))
			assert.Loosely(t, wuRow1.LastUpdated, should.Match(finalizeTime))
			assert.Loosely(t, wuRow2.FinalizeStartTime, should.Match(spanner.NullTime{Valid: true, Time: finalizeTime}))

			// Enqueued the finalization task for the legacy invocation.
			expectedTasks := []protoreflect.ProtoMessage{
				&taskspb.RunExportNotifications{InvocationId: string(wuID2.LegacyInvocationID())},
				&taskspb.TryFinalizeInvocation{InvocationId: string(wuID2.LegacyInvocationID())},
				&taskspb.RunExportNotifications{InvocationId: string(wuID1.LegacyInvocationID())},
				&taskspb.TryFinalizeInvocation{InvocationId: string(wuID1.LegacyInvocationID())},
			}
			assert.Loosely(t, sched.Tasks().Payloads(), should.Match(expectedTasks))

			t.Run("idempotent", func(t *ftt.Test) {
				resp2, err := recorder.BatchFinalizeWorkUnits(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, resp2.WorkUnits[0], should.Match(resp.WorkUnits[0]))
				assert.Loosely(t, resp2.WorkUnits[1], should.Match(resp.WorkUnits[1]))

				wuRow1b, err := workunits.Read(span.Single(ctx), wuID1, workunits.AllFields)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, wuRow1b, should.Match(wuRow1))

				wuRow2b, err := workunits.Read(span.Single(ctx), wuID2, workunits.AllFields)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, wuRow2b, should.Match(wuRow2))

				// No new tasks should be enqueued.
				assert.Loosely(t, sched.Tasks().Payloads(), should.Match(expectedTasks))
			})
		})
	})
}
