// Copyright 2026 The LUCI Authors.
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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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

func TestFinalizeWorkUnitDescendants(t *testing.T) {
	ftt.Run("FinalizeWorkUnitDescendants", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)

		ctx, sched := tq.TestingContext(ctx, nil)
		recorder := newTestRecorderServer()

		rootInvID := rootinvocations.ID("root-inv-id")
		baseID := workunits.ID{
			RootInvocationID: rootInvID,
			WorkUnitID:       "base",
		}

		token, err := generateWorkUnitUpdateToken(ctx, baseID)
		assert.Loosely(t, err, should.BeNil)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, token))

		req := &pb.FinalizeWorkUnitDescendantsRequest{
			Name:              baseID.Name(),
			State:             pb.WorkUnit_CANCELLED,
			SummaryMarkdown:   "Presubmit run cancelled",
			FinalizationScope: pb.FinalizeWorkUnitDescendantsRequest_PREFIXED_DESCENDANTS_ONLY,
		}

		t.Run("invalid request", func(t *ftt.Test) {
			t.Run("name", func(t *ftt.Test) {
				t.Run("unspecified", func(t *ftt.Test) {
					req.Name = ""
					_, err := recorder.FinalizeWorkUnitDescendants(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("name: unspecified"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.Name = "invalid name"
					_, err := recorder.FinalizeWorkUnitDescendants(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("name: does not match"))
				})
				t.Run("prefixed", func(t *ftt.Test) {
					req.Name = "rootInvocations/root-inv-id/workUnits/base:prefixed"
					_, err := recorder.FinalizeWorkUnitDescendants(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike(`name: "base:prefixed" is a prefixed work unit ID, expected non-prefixed work unit ID`))
				})
			})
			t.Run("state", func(t *ftt.Test) {
				t.Run("unspecified", func(t *ftt.Test) {
					req.State = pb.WorkUnit_STATE_UNSPECIFIED
					_, err := recorder.FinalizeWorkUnitDescendants(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("state: unspecified"))
				})
				t.Run("not a terminal state", func(t *ftt.Test) {
					req.State = pb.WorkUnit_PENDING
					_, err := recorder.FinalizeWorkUnitDescendants(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("state: must be a terminal state"))
				})
				t.Run("invalid state", func(t *ftt.Test) {
					req.State = pb.WorkUnit_State(999)
					_, err := recorder.FinalizeWorkUnitDescendants(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("state: unknown state 999"))
				})
			})
			t.Run("summary markdown", func(t *ftt.Test) {
				t.Run("invalid UTF-8", func(t *ftt.Test) {
					// Not valid UTF-8.
					req.SummaryMarkdown = "\xFF"
					_, err := recorder.FinalizeWorkUnitDescendants(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("summary_markdown: not valid UTF-8"))
				})
			})
			t.Run("finalization scope", func(t *ftt.Test) {
				t.Run("unspecified", func(t *ftt.Test) {
					req.FinalizationScope = pb.FinalizeWorkUnitDescendantsRequest_FINALIZATION_SCOPE_UNSPECIFIED
					_, err := recorder.FinalizeWorkUnitDescendants(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("finalization_scope: unspecified"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					req.FinalizationScope = pb.FinalizeWorkUnitDescendantsRequest_FinalizationScope(999)
					_, err := recorder.FinalizeWorkUnitDescendants(ctx, req)
					assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.That(t, err, should.ErrLike("finalization_scope: unknown finalization scope 999"))
				})
			})
		})

		t.Run("permission denied", func(t *ftt.Test) {
			badCtx := metadata.NewIncomingContext(ctx, metadata.Pairs(pb.UpdateTokenMetadataKey, "invalid"))
			_, err := recorder.FinalizeWorkUnitDescendants(badCtx, req)
			assert.That(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.That(t, err, should.ErrLike("invalid update token"))
		})

		t.Run("success", func(t *ftt.Test) {
			// Insert a root invocation.
			rootInv := rootinvocations.NewBuilder(rootInvID).WithRealm("testproject:testrealm").Build()
			testutil.MustApply(ctx, t, insert.RootInvocationWithRootWorkUnit(rootInv)...)

			// Insert base work unit and descendants.
			// base
			// base:child1
			// base:child2
			// base:grandchild
			// other

			base := workunits.NewBuilder(rootInvID, "base").
				WithFinalizationState(pb.WorkUnit_ACTIVE).WithState(pb.WorkUnit_RUNNING).WithSummaryMarkdown("Running...").Build()
			child1 := workunits.NewBuilder(rootInvID, "base:child1").WithFinalizationState(pb.WorkUnit_FINALIZED).
				WithState(pb.WorkUnit_SUCCEEDED).WithSummaryMarkdown("10 tests run.").Build()
			child2 := workunits.NewBuilder(rootInvID, "base:child2").WithFinalizationState(pb.WorkUnit_ACTIVE).Build()
			grandchild := workunits.NewBuilder(rootInvID, "base:grandchild").WithFinalizationState(pb.WorkUnit_ACTIVE).Build()
			other := workunits.NewBuilder(rootInvID, "other").WithFinalizationState(pb.WorkUnit_ACTIVE).Build()

			testutil.MustApply(ctx, t, insert.WorkUnit(base)...)
			testutil.MustApply(ctx, t, insert.WorkUnit(child1)...)
			testutil.MustApply(ctx, t, insert.WorkUnit(child2)...)
			testutil.MustApply(ctx, t, insert.WorkUnit(grandchild)...)
			testutil.MustApply(ctx, t, insert.WorkUnit(other)...)

			_, err := recorder.FinalizeWorkUnitDescendants(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			type expectedState struct {
				workUnitID        string
				state             pb.WorkUnit_State
				finalizationState pb.WorkUnit_FinalizationState
				markdown          string
			}
			var expectedStates []expectedState = []expectedState{
				{
					// Should be unchanged.
					workUnitID:        "base",
					state:             pb.WorkUnit_RUNNING,
					finalizationState: pb.WorkUnit_ACTIVE,
					markdown:          "Running...",
				},
				{
					// Should be unchanged, as it is already finalizing/finalized.
					workUnitID:        "base:child1",
					state:             pb.WorkUnit_SUCCEEDED,
					finalizationState: pb.WorkUnit_FINALIZED,
					markdown:          "10 tests run.",
				},
				{
					// Should be updated.
					workUnitID:        "base:child2",
					state:             pb.WorkUnit_CANCELLED,
					finalizationState: pb.WorkUnit_FINALIZING,
					markdown:          "Presubmit run cancelled",
				},
				{
					// Should be updated.
					workUnitID:        "base:child2",
					state:             pb.WorkUnit_CANCELLED,
					finalizationState: pb.WorkUnit_FINALIZING,
					markdown:          "Presubmit run cancelled",
				},
				{
					// Should be updated.
					workUnitID:        "base:grandchild",
					state:             pb.WorkUnit_CANCELLED,
					finalizationState: pb.WorkUnit_FINALIZING,
					markdown:          "Presubmit run cancelled",
				},
				{
					// Should be unchanged.
					workUnitID:        "other",
					state:             other.State,
					finalizationState: other.FinalizationState,
					markdown:          other.SummaryMarkdown,
				},
			}

			verifyExpectedStates := func() {
				for _, id := range expectedStates {
					wuRow, err := workunits.Read(span.Single(ctx), workunits.ID{RootInvocationID: rootInvID, WorkUnitID: id.workUnitID}, workunits.AllFields)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, wuRow.FinalizationState, should.Equal(id.finalizationState))
					assert.Loosely(t, wuRow.State, should.Equal(id.state))
					assert.Loosely(t, wuRow.SummaryMarkdown, should.Equal(id.markdown))
					if id.finalizationState == pb.WorkUnit_FINALIZING {
						assert.Loosely(t, wuRow.FinalizerCandidateTime.Valid, should.BeTrue)
					}
				}
			}
			verifyExpectedStates()

			// Verify task scheduled.
			expectedTasks := []protoreflect.ProtoMessage{
				&taskspb.SweepWorkUnitsForFinalization{RootInvocationId: string(rootInvID), SequenceNumber: 1},
			}
			assert.Loosely(t, sched.Tasks().Payloads(), should.Match(expectedTasks))

			t.Run("idempotent", func(t *ftt.Test) {
				_, err := recorder.FinalizeWorkUnitDescendants(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				// States should not have changed.
				verifyExpectedStates()

				// No new tasks should be enqueued.
				assert.Loosely(t, sched.Tasks().Payloads(), should.Match(expectedTasks))
			})
		})
	})
}
