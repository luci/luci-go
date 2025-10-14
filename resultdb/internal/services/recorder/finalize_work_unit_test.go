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
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestVerifyFinalizeWorkUnitPermissions(t *testing.T) {
	t.Parallel()

	ftt.Run("VerifyFinalizeWorkUnitPermissions", t, func(t *ftt.Test) {
		ctx := testutil.TestingContext()
		id := workunits.ID{
			RootInvocationID: "root-inv-id",
			WorkUnitID:       "work-unit-id",
		}
		token, err := generateWorkUnitUpdateToken(ctx, id)
		assert.Loosely(t, err, should.BeNil)

		req := &pb.FinalizeWorkUnitRequest{
			Name: id.Name(),
		}

		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		t.Run("valid", func(t *ftt.Test) {
			err := verifyFinalizeWorkUnitPermissions(ctx, req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("empty name", func(t *ftt.Test) {
			req.Name = ""
			err := verifyFinalizeWorkUnitPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("name: unspecified"))
		})

		t.Run("invalid name", func(t *ftt.Test) {
			req.Name = "invalid name"
			err := verifyFinalizeWorkUnitPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("name: does not match"))
		})

		t.Run("invalid token", func(t *ftt.Test) {
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid"))
			err := verifyFinalizeWorkUnitPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("invalid update token"))
		})

		t.Run("missing token", func(t *ftt.Test) {
			ctx = metadata.NewIncomingContext(ctx, metadata.MD{})
			err := verifyFinalizeWorkUnitPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.Unauthenticated))
			assert.Loosely(t, err, should.ErrLike("missing update-token metadata value"))
		})

		t.Run("too many tokens", func(t *ftt.Test) {
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token, pb.UpdateTokenMetadataKey, token))
			err := verifyFinalizeWorkUnitPermissions(ctx, req)
			assert.Loosely(t, appstatus.Code(err), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("expected exactly one update-token metadata value, got 2"))
		})
	})
}

func TestFinalizeWorkUnit(t *testing.T) {
	ftt.Run("FinalizeWorkUnit", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, sched := tq.TestingContext(ctx, nil)
		recorder := newTestRecorderServer()

		rootInvID := rootinvocations.ID("root-inv-id")
		wuID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "work-unit-id",
		}

		token, err := generateWorkUnitUpdateToken(ctx, wuID)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		req := &pb.FinalizeWorkUnitRequest{Name: wuID.Name()}

		t.Run("invalid request", func(t *ftt.Test) {
			// Do not need to test all cases, the tests for VerifyFinalizeWorkUnitPermissions
			// verifies that. We simply want to check that method is called.
			req.Name = "invalid name"
			_, err := recorder.FinalizeWorkUnit(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.That(t, err, should.ErrLike("name: does not match"))
		})

		t.Run("permission denied", func(t *ftt.Test) {
			// Do not need to test all cases, the tests for VerifyFinalizeWorkUnitPermissions
			// verifies that. We simply want to check that method is called.
			badCtx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid"))
			_, err := recorder.FinalizeWorkUnit(badCtx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.That(t, err, should.ErrLike("invalid update token"))
		})

		t.Run("not found", func(t *ftt.Test) {
			// This would normally only happen if a row was manually deleted from
			// the underlying table as the work unit update token could not have
			// been minted otherwise.
			_, err := recorder.FinalizeWorkUnit(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.That(t, err, should.ErrLike(`"rootInvocations/root-inv-id/workUnits/work-unit-id" not found`))
		})

		t.Run("success", func(t *ftt.Test) {
			// Insert a root invocation and the work unit.
			rootInv := rootinvocations.NewBuilder(rootInvID).WithRealm("testproject:testrealm").Build()
			wu := workunits.NewBuilder(rootInvID, wuID.WorkUnitID).WithFinalizationState(pb.WorkUnit_ACTIVE).Build()
			testutil.MustApply(ctx, t, insert.RootInvocationWithRootWorkUnit(rootInv)...)
			testutil.MustApply(ctx, t, insert.WorkUnit(wu)...)

			wuProto, err := recorder.FinalizeWorkUnit(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, wuProto.FinalizationState, should.Equal(pb.WorkUnit_FINALIZING))
			assert.Loosely(t, wuProto.FinalizeStartTime, should.NotBeNil)
			finalizeTime := wuProto.FinalizeStartTime.AsTime()
			assert.Loosely(t, wuProto.LastUpdated.AsTime(), should.Match(finalizeTime))

			// Read the work unit from Spanner to confirm it's really FINALIZING.
			wuRow, err := workunits.Read(span.Single(ctx), wuID, workunits.AllFields)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, wuRow.FinalizationState, should.Equal(pb.WorkUnit_FINALIZING))
			assert.Loosely(t, wuRow.FinalizeStartTime, should.Match(spanner.NullTime{Valid: true, Time: finalizeTime}))
			assert.Loosely(t, wuRow.LastUpdated, should.Match(finalizeTime))

			// Enqueued the finalization task for the legacy invocation.
			expectedTasks := []protoreflect.ProtoMessage{
				&taskspb.RunExportNotifications{InvocationId: string(wuID.LegacyInvocationID())},
				&taskspb.TryFinalizeInvocation{InvocationId: string(wuID.LegacyInvocationID())},
			}
			assert.Loosely(t, sched.Tasks().Payloads(), should.Match(expectedTasks))

			t.Run("idempotent", func(t *ftt.Test) {
				wuProto2, err := recorder.FinalizeWorkUnit(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, wuProto2, should.Match(wuProto))

				wuRow2, err := workunits.Read(span.Single(ctx), wuID, workunits.AllFields)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, wuRow2, should.Match(wuRow))

				// No new tasks should be enqueued.
				assert.Loosely(t, sched.Tasks().Payloads(), should.Match(expectedTasks))
			})
		})
	})
}
