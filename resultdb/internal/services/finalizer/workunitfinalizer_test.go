// Copyright 2025 The LUCI Authors.
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
package finalizer

import (
	"testing"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/workunits"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestSweepWorkUnitsForFinalization(t *testing.T) {
	ftt.Run("SweepWorkUnitsForFinalization", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		rootInvID := rootinvocations.ID("test-root-inv")
		const seq = int64(2)
		testutil.MustApply(ctx, t,
			rootinvocations.InsertForTesting(rootinvocations.NewBuilder(rootInvID).WithFinalizerPending(true).WithFinalizerSequence(seq).Build())...,
		)
		opts := sweepWorkUnitsForFinalizationOptions{writeBatchSizeOverride: 3, readLimitOverride: 10}

		t.Run("Stale task exits early", func(t *ftt.Test) {
			// Execute the sweep with an older sequence number.
			err := sweepWorkUnitsForFinalization(ctx, rootInvID, seq-1, opts)
			assert.Loosely(t, err, should.BeNil)

			taskState, err := rootinvocations.ReadFinalizerTaskState(span.Single(ctx), rootInvID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, taskState.Pending, should.BeTrue) // the pending flag should not reset.
			assert.Loosely(t, taskState.Sequence, should.Equal(seq))
		})

		t.Run("e2e", func(t *ftt.Test) {
			t.Run("a complex tree", func(t *ftt.Test) {
				// A tree as below.
				// root*
				// ├── wu1*
				// │   ├── w11* (c)
				// │   └── w12*
				// │       └── w121* (c)
				// └── wu2* (c)
				//     └── wu21 (active)
				//     └── wu22* (c)
				// wu* means wu is a finalizing state
				// wu (c) means wu is a candidate
				wuroot := workunits.NewBuilder(rootInvID, "root").WithFinalizationState(pb.WorkUnit_FINALIZING).Build()
				wu1 := workunits.NewBuilder(rootInvID, "wu1").
					WithFinalizationState(pb.WorkUnit_FINALIZING).
					WithParentWorkUnitID("root").
					Build()
				wu11 := workunits.NewBuilder(rootInvID, "wu11").
					WithFinalizationState(pb.WorkUnit_FINALIZING).
					WithParentWorkUnitID(wu1.ID.WorkUnitID).
					WithFinalizerCandidateTime(spanner.CommitTimestamp).
					Build()
				wu12 := workunits.NewBuilder(rootInvID, "wu12").
					WithFinalizationState(pb.WorkUnit_FINALIZING).
					WithParentWorkUnitID(wu1.ID.WorkUnitID).
					Build()
				wu121 := workunits.NewBuilder(rootInvID, "wu121").
					WithFinalizationState(pb.WorkUnit_FINALIZING).
					WithParentWorkUnitID(wu12.ID.WorkUnitID).
					WithFinalizerCandidateTime(spanner.CommitTimestamp).
					Build()
				wu2 := workunits.NewBuilder(rootInvID, "wu2").
					WithFinalizationState(pb.WorkUnit_FINALIZING).
					WithParentWorkUnitID("root").
					WithFinalizerCandidateTime(spanner.CommitTimestamp).
					Build()
				wu21 := workunits.NewBuilder(rootInvID, "wu21").
					WithFinalizationState(pb.WorkUnit_ACTIVE).
					WithParentWorkUnitID(wu2.ID.WorkUnitID).
					Build()
				wu22 := workunits.NewBuilder(rootInvID, "wu22").
					WithFinalizationState(pb.WorkUnit_FINALIZING).
					WithParentWorkUnitID(wu2.ID.WorkUnitID).
					WithFinalizerCandidateTime(spanner.CommitTimestamp).
					Build()

				createTime := testutil.MustApply(ctx, t, testutil.CombineMutations(
					workunits.InsertForTesting(wuroot),
					workunits.InsertForTesting(wu1),
					workunits.InsertForTesting(wu2),
					workunits.InsertForTesting(wu11),
					workunits.InsertForTesting(wu12),
					workunits.InsertForTesting(wu121),
					workunits.InsertForTesting(wu21),
					workunits.InsertForTesting(wu22),
				)...)
				verifyWU := func(wu *workunits.WorkUnitRow, expectedState pb.WorkUnit_FinalizationState, hasFinalizerCandidateTime bool) {
					t.Helper()
					readWU, err := workunits.Read(span.Single(ctx), wu.ID, workunits.ExcludeExtendedProperties)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, readWU.FinalizationState, should.Equal(expectedState), truth.LineContext())
					if expectedState == pb.WorkUnit_FINALIZED {
						assert.That(t, readWU.FinalizeTime.Valid, should.BeTrue)
						assert.That(t, readWU.FinalizeTime.Time, should.HappenAfter(createTime), truth.LineContext())
					}
					assert.That(t, readWU.FinalizerCandidateTime.Valid, should.Equal(hasFinalizerCandidateTime), truth.LineContext())
					if hasFinalizerCandidateTime {
						assert.That(t, readWU.FinalizerCandidateTime.Time, should.HappenAfter(createTime), truth.LineContext())
					}
				}
				t.Run("one read", func(t *ftt.Test) {
					// The tree after.
					// root* (c)
					// ├── wu1(finalized)
					// │   ├── w11 (finalized)
					// │   └── w12 (finalized)
					// │       └── w121 (finalized)
					// └── wu2* (c updated)
					//     └── wu21 (active)
					//     └── wu22 (finalized)

					err := sweepWorkUnitsForFinalization(ctx, rootInvID, seq, opts)
					assert.Loosely(t, err, should.BeNil)
					// wuroot can't be finalized, and becomes a candidate.
					verifyWU(wuroot, pb.WorkUnit_FINALIZING, true)
					// Finalize wu1, wu11, wu12, w121
					verifyWU(wu1, pb.WorkUnit_FINALIZED, false)
					verifyWU(wu11, pb.WorkUnit_FINALIZED, false)
					verifyWU(wu12, pb.WorkUnit_FINALIZED, false)
					verifyWU(wu121, pb.WorkUnit_FINALIZED, false)
					// wu2 can't be finalized.
					verifyWU(wu2, pb.WorkUnit_FINALIZING, true)
					verifyWU(wu21, pb.WorkUnit_ACTIVE, false)
					verifyWU(wu22, pb.WorkUnit_FINALIZED, false)

					// Assert root invocation sweep state was reset
					taskState, err := rootinvocations.ReadFinalizerTaskState(span.Single(ctx), rootInvID)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, taskState.Pending, should.BeFalse)
				})

				t.Run("multiple read", func(t *ftt.Test) {
					opts.readLimitOverride = 2

					err := sweepWorkUnitsForFinalization(ctx, rootInvID, seq, opts)
					assert.Loosely(t, err, should.BeNil)
					// wuroot can't be finalized, becomes a candidate when wu1 finalizes
					// but gets reset in later iteration since it is not ready to be finalized.
					verifyWU(wuroot, pb.WorkUnit_FINALIZING, false)
					// Finalize wu1, wu11, wu12, w121
					verifyWU(wu1, pb.WorkUnit_FINALIZED, false)
					verifyWU(wu11, pb.WorkUnit_FINALIZED, false)
					verifyWU(wu12, pb.WorkUnit_FINALIZED, false)
					verifyWU(wu121, pb.WorkUnit_FINALIZED, false)
					// wu2 can't be finalized.
					verifyWU(wu2, pb.WorkUnit_FINALIZING, false)
					verifyWU(wu21, pb.WorkUnit_ACTIVE, false)
					verifyWU(wu22, pb.WorkUnit_FINALIZED, false)

					// Assert root invocation sweep state was reset
					taskState, err := rootinvocations.ReadFinalizerTaskState(span.Single(ctx), rootInvID)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, taskState.Pending, should.BeFalse)
				})
			})
		})
	})
}

func TestFindWorkUnitsReadyForFinalization(t *testing.T) {
	ftt.Run("findWorkUnitsReadyForFinalization", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		rootInvID := rootinvocations.ID("test-root-inv")
		wuroot := workunits.NewBuilder(rootInvID, "root").WithFinalizationState(pb.WorkUnit_FINALIZING).Build()
		testutil.MustApply(ctx, t, testutil.CombineMutations(
			rootinvocations.InsertForTesting(rootinvocations.NewBuilder(rootInvID).Build()),
			workunits.InsertForTesting(wuroot),
		)...)
		opts := findWorkUnitsReadyForFinalizationOptions{}
		t.Run("propagate from child to parent", func(t *ftt.Test) {
			parentBuilder := workunits.NewBuilder(rootInvID, "parent")
			childBuilder := workunits.NewBuilder(rootInvID, "child").WithParentWorkUnitID("parent")

			t.Run("finalizing parent", func(t *ftt.Test) {
				parent := parentBuilder.WithFinalizationState(pb.WorkUnit_FINALIZING).Build()
				testutil.MustApply(ctx, t, workunits.InsertForTesting(parent)...)
				t.Run("has finalizing candidate child", func(t *ftt.Test) {
					child := childBuilder.WithFinalizationState(pb.WorkUnit_FINALIZING).WithFinalizerCandidateTime(spanner.CommitTimestamp).Build()
					testutil.MustApply(ctx, t, workunits.InsertForTesting(child)...)

					ineligible, toFinalize, moreToRead, err := findWorkUnitsReadyForFinalization(ctx, rootInvID, opts)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, moreToRead, should.BeFalse)
					assert.Loosely(t, toFinalize, should.Match([]workUnitWithParent{
						{ID: child.ID, Parent: parent.ID},
						{ID: parent.ID, Parent: wuroot.ID},
						{ID: wuroot.ID, Parent: workunits.ID{}},
					}))
					assert.Loosely(t, ineligible, should.HaveLength(0))
				})
				t.Run("has finalizing child (finalizerCandidateTime not set)", func(t *ftt.Test) {
					child := childBuilder.WithFinalizationState(pb.WorkUnit_FINALIZING).Build()
					testutil.MustApply(ctx, t, workunits.InsertForTesting(child)...)

					ineligible, toFinalize, moreToRead, err := findWorkUnitsReadyForFinalization(ctx, rootInvID, opts)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, moreToRead, should.BeFalse)
					assert.Loosely(t, toFinalize, should.HaveLength(0))
					assert.Loosely(t, ineligible, should.HaveLength(0))
				})
				t.Run("has finalized child with finalizerCandidateTime set", func(t *ftt.Test) {
					// Finalized work unit shouldn't be picked a candidate even with finalizerCandidateTime set.
					child := childBuilder.WithFinalizationState(pb.WorkUnit_FINALIZED).WithFinalizerCandidateTime(spanner.CommitTimestamp).Build()
					testutil.MustApply(ctx, t, workunits.InsertForTesting(child)...)

					ineligible, toFinalize, moreToRead, err := findWorkUnitsReadyForFinalization(ctx, rootInvID, opts)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, moreToRead, should.BeFalse)
					assert.Loosely(t, toFinalize, should.HaveLength(0))
					assert.Loosely(t, ineligible, should.HaveLength(0))
				})
				t.Run("active child with finalizerCandidateTime set", func(t *ftt.Test) {
					// Active work unit shouldn't be picked a candidate even with finalizerCandidateTime set.
					child := childBuilder.WithFinalizationState(pb.WorkUnit_ACTIVE).WithFinalizerCandidateTime(spanner.CommitTimestamp).Build()
					testutil.MustApply(ctx, t, workunits.InsertForTesting(child)...)

					ineligible, toFinalize, moreToRead, err := findWorkUnitsReadyForFinalization(ctx, rootInvID, opts)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, moreToRead, should.BeFalse)
					assert.Loosely(t, toFinalize, should.HaveLength(0))
					assert.Loosely(t, ineligible, should.HaveLength(0))
				})
			})
			t.Run("finalizing candidate parent", func(t *ftt.Test) {
				parent := parentBuilder.WithFinalizationState(pb.WorkUnit_FINALIZING).WithFinalizerCandidateTime(spanner.CommitTimestamp).Build()
				parentCT := testutil.MustApply(ctx, t, workunits.InsertForTesting(parent)...)
				t.Run("has finalizing candidate child", func(t *ftt.Test) {
					child := childBuilder.WithFinalizationState(pb.WorkUnit_FINALIZING).WithFinalizerCandidateTime(spanner.CommitTimestamp).Build()
					testutil.MustApply(ctx, t, workunits.InsertForTesting(child)...)

					ineligible, toFinalize, moreToRead, err := findWorkUnitsReadyForFinalization(ctx, rootInvID, opts)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, moreToRead, should.BeFalse)
					assert.Loosely(t, toFinalize, should.Match([]workUnitWithParent{
						{ID: child.ID, Parent: parent.ID},
						{ID: parent.ID, Parent: wuroot.ID},
						{ID: wuroot.ID, Parent: workunits.ID{}},
					}))
					assert.Loosely(t, ineligible, should.HaveLength(0))
				})
				t.Run("has finalizing child (finalizerCandidateTime not set)", func(t *ftt.Test) {
					child := childBuilder.WithFinalizationState(pb.WorkUnit_FINALIZING).Build()
					testutil.MustApply(ctx, t, workunits.InsertForTesting(child)...)

					ineligible, toFinalize, moreToRead, err := findWorkUnitsReadyForFinalization(ctx, rootInvID, opts)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, moreToRead, should.BeFalse)
					assert.Loosely(t, toFinalize, should.HaveLength(0))
					assert.Loosely(t, ineligible, should.Match([]workunits.FinalizerCandidate{
						{ID: parent.ID, FinalizerCandidateTime: parentCT},
					}))
				})
				t.Run("has finalized child", func(t *ftt.Test) {
					child := childBuilder.WithFinalizationState(pb.WorkUnit_FINALIZED).Build()
					testutil.MustApply(ctx, t, workunits.InsertForTesting(child)...)

					ineligible, toFinalize, moreToRead, err := findWorkUnitsReadyForFinalization(ctx, rootInvID, opts)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, moreToRead, should.BeFalse)
					assert.Loosely(t, toFinalize, should.Match([]workUnitWithParent{
						{ID: parent.ID, Parent: wuroot.ID},
						{ID: wuroot.ID, Parent: workunits.ID{}},
					}))
					assert.Loosely(t, ineligible, should.HaveLength(0))
				})
				t.Run("has active child", func(t *ftt.Test) {
					child := childBuilder.WithFinalizationState(pb.WorkUnit_ACTIVE).Build()
					testutil.MustApply(ctx, t, workunits.InsertForTesting(child)...)

					ineligible, toFinalize, moreToRead, err := findWorkUnitsReadyForFinalization(ctx, rootInvID, opts)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, moreToRead, should.BeFalse)
					assert.Loosely(t, toFinalize, should.HaveLength(0))
					assert.Loosely(t, ineligible, should.Match([]workunits.FinalizerCandidate{
						{ID: parent.ID, FinalizerCandidateTime: parentCT},
					}))
				})
			})
			t.Run("active parent", func(t *ftt.Test) {
				parent := parentBuilder.WithFinalizationState(pb.WorkUnit_ACTIVE).Build()
				testutil.MustApply(ctx, t, workunits.InsertForTesting(parent)...)
				t.Run("has finalizing candidate child", func(t *ftt.Test) {
					child := childBuilder.WithFinalizationState(pb.WorkUnit_FINALIZING).WithFinalizerCandidateTime(spanner.CommitTimestamp).Build()
					testutil.MustApply(ctx, t, workunits.InsertForTesting(child)...)

					ineligible, toFinalize, moreToRead, err := findWorkUnitsReadyForFinalization(ctx, rootInvID, opts)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, moreToRead, should.BeFalse)
					assert.Loosely(t, toFinalize, should.Match([]workUnitWithParent{
						{ID: child.ID, Parent: parent.ID},
					}))
					assert.Loosely(t, ineligible, should.HaveLength(0))
				})
				// There is need to test when parent has finalizing (no finalizerCandidateTime), finalized, active child
				// because previous test cases already confirmed that these work unit won't be picked as candidate.
			})
		})
		t.Run("all work units are ready to be finalized in a multi-level tree", func(t *ftt.Test) {
			// Tree structure:
			// root*
			// ├── wu1* (c)
			// │   ├── w11* (c)
			// │   └── w12*
			// │       └── w121* (c)
			// └── wu2*
			//     └── wu21* (c)
			//     └── wu22* (c)
			// wu* means wu is a finalizing state
			// wu (c) means wu is a candidate
			wu1 := workunits.NewBuilder(rootInvID, "wu1").
				WithFinalizationState(pb.WorkUnit_FINALIZING).
				WithParentWorkUnitID("root").
				WithFinalizerCandidateTime(spanner.CommitTimestamp).
				Build()
			wu11 := workunits.NewBuilder(rootInvID, "wu11").
				WithFinalizationState(pb.WorkUnit_FINALIZING).
				WithParentWorkUnitID(wu1.ID.WorkUnitID).
				WithFinalizerCandidateTime(spanner.CommitTimestamp).
				Build()
			wu12 := workunits.NewBuilder(rootInvID, "wu12").
				WithFinalizationState(pb.WorkUnit_FINALIZING).
				WithParentWorkUnitID(wu1.ID.WorkUnitID).
				Build()
			wu121 := workunits.NewBuilder(rootInvID, "wu121").
				WithFinalizationState(pb.WorkUnit_FINALIZING).
				WithParentWorkUnitID(wu12.ID.WorkUnitID).
				WithFinalizerCandidateTime(spanner.CommitTimestamp).
				Build()
			wu2 := workunits.NewBuilder(rootInvID, "wu2").
				WithFinalizationState(pb.WorkUnit_FINALIZING).
				WithParentWorkUnitID("root").
				Build()
			wu21 := workunits.NewBuilder(rootInvID, "wu21").
				WithFinalizationState(pb.WorkUnit_FINALIZING).
				WithParentWorkUnitID(wu2.ID.WorkUnitID).
				WithFinalizerCandidateTime(spanner.CommitTimestamp).
				Build()
			wu22 := workunits.NewBuilder(rootInvID, "wu22").
				WithFinalizationState(pb.WorkUnit_FINALIZING).
				WithParentWorkUnitID(wu2.ID.WorkUnitID).
				WithFinalizerCandidateTime(spanner.CommitTimestamp).
				Build()

			ct := testutil.MustApply(ctx, t, testutil.CombineMutations(
				workunits.InsertForTesting(wu1),
				workunits.InsertForTesting(wu2),
				workunits.InsertForTesting(wu11),
				workunits.InsertForTesting(wu12),
				workunits.InsertForTesting(wu121),
				workunits.InsertForTesting(wu21),
				workunits.InsertForTesting(wu22),
			)...)

			t.Run("total number of results below limit", func(t *ftt.Test) {
				ineligible, toFinalize, moreToRead, err := findWorkUnitsReadyForFinalization(ctx, rootInvID, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, moreToRead, should.BeFalse)
				assert.Loosely(t, toFinalize, should.Match([]workUnitWithParent{
					// First round: w121, w11, w21,22 are ready.
					{ID: wu11.ID, Parent: wu1.ID},
					{ID: wu121.ID, Parent: wu12.ID},
					{ID: wu21.ID, Parent: wu2.ID},
					{ID: wu22.ID, Parent: wu2.ID},
					// Second round: w2,w12 is ready.
					{ID: wu12.ID, Parent: wu1.ID},
					{ID: wu2.ID, Parent: wuroot.ID},
					// Third round: w1 is ready.
					{ID: wu1.ID, Parent: wuroot.ID},
					// Third round: wuroot is ready.
					{ID: wuroot.ID, Parent: workunits.ID{}},
				}))
				assert.Loosely(t, ineligible, should.HaveLength(0))
			})

			t.Run("limit works", func(t *ftt.Test) {
				opts.limitOverride = 3
				ineligible, toFinalize, moreToRead, err := findWorkUnitsReadyForFinalization(ctx, rootInvID, opts)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, moreToRead, should.BeTrue)
				assert.Loosely(t, toFinalize, should.Match([]workUnitWithParent{
					// First round: w121, w22 are ready.
					{ID: wu121.ID, Parent: wu12.ID},
					{ID: wu22.ID, Parent: wu2.ID},
					// Second round: w12 is ready.
					{ID: wu12.ID, Parent: wu1.ID},
				}))
				assert.Loosely(t, ineligible, should.Match([]workunits.FinalizerCandidate{
					{ID: wu1.ID, FinalizerCandidateTime: ct},
				}))
			})
		})
	})
}
