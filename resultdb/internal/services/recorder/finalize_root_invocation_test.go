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

func TestVerifyFinalizeRootInvocationPermissions(t *testing.T) {
	t.Parallel()

	ftt.Run("VerifyFinalizeRootInvocationPermissions", t, func(t *ftt.Test) {
		ctx := testutil.TestingContext()
		rootInvID := rootinvocations.ID("finalize-inv-id")
		rootWorkUnitID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       workunits.RootWorkUnitID,
		}

		req := &pb.FinalizeRootInvocationRequest{
			Name: rootInvID.Name(),
		}

		token, err := generateWorkUnitUpdateToken(ctx, rootWorkUnitID)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		t.Run("valid", func(t *ftt.Test) {
			err := verifyFinalizeRootInvocationPermissions(ctx, req)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("empty name", func(t *ftt.Test) {
			req.Name = ""
			err := verifyFinalizeRootInvocationPermissions(ctx, req)
			st, ok := appstatus.Get(err)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, st.Code(), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, st.Err(), should.ErrLike("name: unspecified"))
		})

		t.Run("invalid name", func(t *ftt.Test) {
			req.Name = "invalid name"
			err := verifyFinalizeRootInvocationPermissions(ctx, req)
			st, ok := appstatus.Get(err)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, st.Code(), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, st.Err(), should.ErrLike("name: does not match"))
		})

		t.Run("invalid token", func(t *ftt.Test) {
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid"))
			err := verifyFinalizeRootInvocationPermissions(ctx, req)
			st, ok := appstatus.Get(err)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, st.Code(), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, st.Err(), should.ErrLike("invalid update token"))
		})

		t.Run("missing token", func(t *ftt.Test) {
			ctx = metadata.NewIncomingContext(ctx, metadata.MD{})
			err := verifyFinalizeRootInvocationPermissions(ctx, req)
			st, ok := appstatus.Get(err)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, st.Code(), should.Equal(codes.Unauthenticated))
			assert.Loosely(t, st.Err(), should.ErrLike("missing update-token metadata value"))
		})

		t.Run("too many tokens", func(t *ftt.Test) {
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token, pb.UpdateTokenMetadataKey, token))
			err := verifyFinalizeRootInvocationPermissions(ctx, req)
			st, ok := appstatus.Get(err)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, st.Code(), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, st.Err(), should.ErrLike("expected exactly one update-token metadata value, got 2"))
		})
	})
}

func TestFinalizeRootInvocation(t *testing.T) {
	ftt.Run("FinalizeRootInvocation", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, sched := tq.TestingContext(ctx, nil)
		recorder := newTestRecorderServer()

		rootInvID := rootinvocations.ID("finalize-inv-id")
		rootWorkUnitID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       workunits.RootWorkUnitID,
		}

		token, err := generateWorkUnitUpdateToken(ctx, rootWorkUnitID)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		req := &pb.FinalizeRootInvocationRequest{
			Name:              rootInvID.Name(),
			FinalizationScope: pb.FinalizeRootInvocationRequest_INCLUDE_ROOT_WORK_UNIT,
		}

		t.Run("invalid request", func(t *ftt.Test) {
			// Do not need to test all cases, the tests for VerifyFinalizeWorkUnitPermissions
			// verifies that. We simply want to check that method is called.
			t.Run("name", func(t *ftt.Test) {
				req.Name = "invalid name"
				_, err := recorder.FinalizeRootInvocation(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("name: does not match"))
			})
			t.Run("finalization scope", func(t *ftt.Test) {
				req.FinalizationScope = pb.FinalizeRootInvocationRequest_FinalizationScope(100)
				_, err := recorder.FinalizeRootInvocation(ctx, req)
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
				assert.That(t, err, should.ErrLike("finalization_scope: invalid value (100)"))
			})
		})

		t.Run("permission denied", func(t *ftt.Test) {
			// Do not need to test all cases, the tests for VerifyFinalizeWorkUnitPermissions
			// verifies that. We simply want to check that method is called.
			badCtx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid"))
			_, err := recorder.FinalizeRootInvocation(badCtx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.That(t, err, should.ErrLike("invalid update token"))
		})

		t.Run("not found", func(t *ftt.Test) {
			// This would normally only happen if a row was manually deleted from
			// the underlying table as the update token could not have
			// been minted otherwise.
			_, err := recorder.FinalizeRootInvocation(ctx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.That(t, err, should.ErrLike(`"rootInvocations/finalize-inv-id" not found`))
		})

		t.Run("success", func(t *ftt.Test) {
			// Insert a root invocation and the work unit.
			rootInv := rootinvocations.NewBuilder(rootInvID).WithRealm("testproject:testrealm").WithState(pb.RootInvocation_ACTIVE).Build()
			testutil.MustApply(ctx, t, insert.RootInvocationWithRootWorkUnit(rootInv)...)

			t.Run("include root work unit", func(t *ftt.Test) {
				req.FinalizationScope = pb.FinalizeRootInvocationRequest_INCLUDE_ROOT_WORK_UNIT

				riProto, err := recorder.FinalizeRootInvocation(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, riProto.State, should.Equal(pb.WorkUnit_FINALIZING))
				assert.Loosely(t, riProto.FinalizeStartTime, should.NotBeNil)
				finalizeTime := riProto.FinalizeStartTime.AsTime()
				assert.Loosely(t, riProto.LastUpdated.AsTime(), should.Match(finalizeTime))

				expectedRootInv := *rootInv
				expectedRootInv.State = pb.RootInvocation_FINALIZING
				expectedRootInv.LastUpdated = finalizeTime
				expectedRootInv.FinalizeStartTime = spanner.NullTime{Valid: true, Time: finalizeTime}

				// Read the root invocation and work unit from Spanner to confirm they have been updated.
				riRow, err := rootinvocations.Read(span.Single(ctx), rootInvID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, riRow, should.Match(&expectedRootInv))

				wuRow, err := workunits.Read(span.Single(ctx), rootWorkUnitID, workunits.AllFields)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, wuRow.State, should.Match(pb.WorkUnit_FINALIZING))
				assert.Loosely(t, wuRow.LastUpdated, should.Match(finalizeTime))
				assert.Loosely(t, wuRow.FinalizeStartTime, should.Match(spanner.NullTime{Valid: true, Time: finalizeTime}))

				// Enqueued the finalization task for the legacy invocation.
				expectedTasks := []protoreflect.ProtoMessage{
					&taskspb.RunExportNotifications{InvocationId: string(rootWorkUnitID.LegacyInvocationID())},
					&taskspb.TryFinalizeInvocation{InvocationId: string(rootWorkUnitID.LegacyInvocationID())},
					&taskspb.RunExportNotifications{InvocationId: string(rootInvID.LegacyInvocationID())},
					&taskspb.TryFinalizeInvocation{InvocationId: string(rootInvID.LegacyInvocationID())},
				}
				assert.Loosely(t, sched.Tasks().Payloads(), should.Match(expectedTasks))

				t.Run("idempotent", func(t *ftt.Test) {
					riProto2, err := recorder.FinalizeRootInvocation(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, riProto2, should.Match(riProto))

					// Read the root invocation and work unit from Spanner to confirm they are unchanged.
					riRow, err := rootinvocations.Read(span.Single(ctx), rootInvID)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, riRow, should.Match(&expectedRootInv))

					wuRow2, err := workunits.Read(span.Single(ctx), rootWorkUnitID, workunits.AllFields)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, wuRow2, should.Match(wuRow))

					// No new tasks should be enqueued.
					assert.Loosely(t, sched.Tasks().Payloads(), should.Match(expectedTasks))
				})
			})
			t.Run("exclude root work unit", func(t *ftt.Test) {
				req.FinalizationScope = pb.FinalizeRootInvocationRequest_EXCLUDE_ROOT_WORK_UNIT

				riProto, err := recorder.FinalizeRootInvocation(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, riProto.State, should.Equal(pb.WorkUnit_FINALIZING))
				assert.Loosely(t, riProto.FinalizeStartTime, should.NotBeNil)
				finalizeTime := riProto.FinalizeStartTime.AsTime()
				assert.Loosely(t, riProto.LastUpdated.AsTime(), should.Match(finalizeTime))

				expectedRootInv := *rootInv
				expectedRootInv.State = pb.RootInvocation_FINALIZING
				expectedRootInv.LastUpdated = finalizeTime
				expectedRootInv.FinalizeStartTime = spanner.NullTime{Valid: true, Time: finalizeTime}

				// Read the root invocation and work unit from Spanner to confirm they have been updated.
				riRow, err := rootinvocations.Read(span.Single(ctx), rootInvID)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, riRow, should.Match(&expectedRootInv))

				wuRow, err := workunits.Read(span.Single(ctx), rootWorkUnitID, workunits.AllFields)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, wuRow.State, should.Match(pb.WorkUnit_ACTIVE))
				assert.Loosely(t, wuRow.FinalizeStartTime, should.Match(spanner.NullTime{Valid: false}))

				// Enqueued the finalization task for the legacy invocation.
				expectedTasks := []protoreflect.ProtoMessage{
					&taskspb.RunExportNotifications{InvocationId: string(rootInvID.LegacyInvocationID())},
					&taskspb.TryFinalizeInvocation{InvocationId: string(rootInvID.LegacyInvocationID())},
				}
				assert.Loosely(t, sched.Tasks().Payloads(), should.Match(expectedTasks))

				t.Run("idempotent", func(t *ftt.Test) {
					riProto2, err := recorder.FinalizeRootInvocation(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, riProto2, should.Match(riProto))

					// Read the root invocation and work unit from Spanner to confirm they are unchanged.
					riRow, err := rootinvocations.Read(span.Single(ctx), rootInvID)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, riRow, should.Match(&expectedRootInv))

					wuRow2, err := workunits.Read(span.Single(ctx), rootWorkUnitID, workunits.AllFields)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, wuRow2, should.Match(wuRow))

					// No new tasks should be enqueued.
					assert.Loosely(t, sched.Tasks().Payloads(), should.Match(expectedTasks))
				})
			})
		})
	})
}
