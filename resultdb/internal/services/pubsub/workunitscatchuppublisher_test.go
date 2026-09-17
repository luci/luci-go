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

package pubsub

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/workunits"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestHandleWorkUnitsCatchUpPublisher(t *testing.T) {
	ftt.Run("HandleWorkUnitsCatchUpPublisher", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = memory.Use(ctx)
		ctx, sched := tq.TestingContext(ctx, nil)

		runCatchUp := func(rootInvID rootinvocations.ID, pageSize int) error {
			task := &taskspb.PublishWorkUnitsCatchUpTask{
				RootInvocationId: string(rootInvID),
			}
			p := &workUnitsCatchUpPublisher{
				task:     task,
				pageSize: pageSize,
			}
			return p.handleWorkUnitsCatchUpPublisher(ctx)
		}

		filterTasks := func(class string) tqtesting.TaskList {
			var res tqtesting.TaskList
			for _, task := range sched.Tasks() {
				if task.Class == class {
					res = append(res, task)
				}
			}
			return res
		}

		t.Run("Happy Path - Enqueues Tasks", func(t *ftt.Test) {
			rootInvID := rootinvocations.ID("test-root-inv-catchup")
			cutoffTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

			// Insert root invocation with METADATA_FINAL and MetadataFinalizedTime.
			rootInv := rootinvocations.NewBuilder(rootInvID).
				WithFinalizationState(pb.RootInvocation_ACTIVE).
				WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).
				WithMetadataFinalizedTime(cutoffTime).
				Build()
			muts := rootinvocations.InsertForTesting(rootInv)

			rootWU := workunits.NewBuilder(rootInvID, "root").WithMinimalFields().WithFinalizationState(pb.WorkUnit_FINALIZED).WithFinalizeTime(cutoffTime.Add(-2 * time.Hour)).Build()
			wu1 := workunits.NewBuilder(rootInvID, "wu1").WithMinimalFields().WithParentWorkUnitID("root").WithFinalizationState(pb.WorkUnit_FINALIZED).WithFinalizeTime(cutoffTime.Add(-1 * time.Hour)).Build()
			wu2 := workunits.NewBuilder(rootInvID, "wu2").WithMinimalFields().WithParentWorkUnitID("root").WithFinalizationState(pb.WorkUnit_FINALIZED).WithFinalizeTime(cutoffTime.Add(-30 * time.Minute)).Build()
			// Active (unfinalized) work unit should not be picked up.
			wuActive := workunits.NewBuilder(rootInvID, "wu-active").WithMinimalFields().WithParentWorkUnitID("root").WithFinalizationState(pb.WorkUnit_ACTIVE).Build()

			muts = append(muts, workunits.InsertForTesting(rootWU)...)
			muts = append(muts, workunits.InsertForTesting(wu1)...)
			muts = append(muts, workunits.InsertForTesting(wu2)...)
			muts = append(muts, workunits.InsertForTesting(wuActive)...)
			testutil.MustApply(ctx, t, muts...)

			err := runCatchUp(rootInvID, 10)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, len(sched.Tasks()), should.Equal(2)) // 1 for WU, 1 for TR
			wuTasks := filterTasks("publish-work-units")
			trTasks := filterTasks("publish-test-results")

			assert.Loosely(t, wuTasks, should.HaveLength(1))
			assert.Loosely(t, trTasks, should.HaveLength(1))

			wuPayload := wuTasks[0].Payload.(*taskspb.PublishWorkUnitsTask)
			assert.Loosely(t, wuPayload.RootInvocationId, should.Equal(string(rootInvID)))
			assert.Loosely(t, wuPayload.WorkUnitIds, should.HaveLength(3)) // root, wu1, wu2

			trPayload := trTasks[0].Payload.(*taskspb.PublishTestResultsTask)
			assert.Loosely(t, trPayload.RootInvocationId, should.Equal(string(rootInvID)))
			assert.Loosely(t, trPayload.WorkUnitIds, should.HaveLength(3))
		})

		t.Run("Gap 1 - Finalized with WAIT_FOR_METADATA", func(t *ftt.Test) {
			rootInvID := rootinvocations.ID("test-root-inv-catchup-gap1")
			baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			cutoffTime := baseTime.Add(2 * time.Hour)

			// Insert root invocation that finalized without ever setting METADATA_FINAL.
			rootInv := rootinvocations.NewBuilder(rootInvID).
				WithStreamingExportState(pb.RootInvocation_WAIT_FOR_METADATA).
				WithFinalizationState(pb.RootInvocation_FINALIZED).
				WithFinalizeTime(cutoffTime).
				Build()
			muts := rootinvocations.InsertForTesting(rootInv)

			rootWU := workunits.NewBuilder(rootInvID, "root").WithMinimalFields().WithFinalizationState(pb.WorkUnit_FINALIZED).WithFinalizeTime(baseTime).Build()
			wu1 := workunits.NewBuilder(rootInvID, "wu1").WithMinimalFields().WithParentWorkUnitID("root").WithFinalizationState(pb.WorkUnit_FINALIZED).WithFinalizeTime(baseTime.Add(1 * time.Hour)).Build()
			wu2 := workunits.NewBuilder(rootInvID, "wu2").WithMinimalFields().WithParentWorkUnitID("root").WithFinalizationState(pb.WorkUnit_FINALIZED).WithFinalizeTime(cutoffTime).Build()
			// Unfinalized work unit should not be included.
			wuActive := workunits.NewBuilder(rootInvID, "wu-active").WithMinimalFields().WithParentWorkUnitID("root").WithFinalizationState(pb.WorkUnit_ACTIVE).Build()

			muts = append(muts, workunits.InsertForTesting(rootWU)...)
			muts = append(muts, workunits.InsertForTesting(wu1)...)
			muts = append(muts, workunits.InsertForTesting(wu2)...)
			muts = append(muts, workunits.InsertForTesting(wuActive)...)
			testutil.MustApply(ctx, t, muts...)

			err := runCatchUp(rootInvID, 10)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, len(sched.Tasks()), should.Equal(2)) // 1 WU, 1 TR
			wuTasks := filterTasks("publish-work-units")
			trTasks := filterTasks("publish-test-results")

			assert.Loosely(t, wuTasks, should.HaveLength(1))
			assert.Loosely(t, trTasks, should.HaveLength(1))

			wuPayload := wuTasks[0].Payload.(*taskspb.PublishWorkUnitsTask)
			// Gap 1 catches up all finalized work units under the root invocation.
			assert.Loosely(t, wuPayload.WorkUnitIds, should.HaveLength(3))
			expectedIDs := map[string]bool{"root": true, "wu1": true, "wu2": true}
			for _, id := range wuPayload.WorkUnitIds {
				assert.Loosely(t, expectedIDs[id], should.BeTrue)
			}
		})

		t.Run("Errors if not ready for catch-up", func(t *ftt.Test) {
			rootInvID := rootinvocations.ID("test-root-inv-catchup-skip")

			// Insert root invocation with WAIT_FOR_METADATA and ACTIVE (not finalized).
			rootInv := rootinvocations.NewBuilder(rootInvID).
				WithStreamingExportState(pb.RootInvocation_WAIT_FOR_METADATA).
				WithFinalizationState(pb.RootInvocation_ACTIVE).
				Build()
			testutil.MustApply(ctx, t, rootinvocations.InsertForTesting(rootInv)...)

			err := runCatchUp(rootInvID, 10)
			assert.Loosely(t, err, should.ErrLike("not ready for catch-up"))
			assert.Loosely(t, len(sched.Tasks()), should.BeZero)
		})

		t.Run("Pagination - Enqueues Continuation", func(t *ftt.Test) {
			rootInvID := rootinvocations.ID("test-root-inv-catchup-pag")
			cutoffTime := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

			rootInv := rootinvocations.NewBuilder(rootInvID).
				WithFinalizationState(pb.RootInvocation_ACTIVE).
				WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).
				WithMetadataFinalizedTime(cutoffTime).
				Build()
			muts := rootinvocations.InsertForTesting(rootInv)

			rootWU := workunits.NewBuilder(rootInvID, "root").WithMinimalFields().WithFinalizationState(pb.WorkUnit_FINALIZED).WithFinalizeTime(cutoffTime.Add(-2 * time.Hour)).Build()
			wu1 := workunits.NewBuilder(rootInvID, "wu1").WithMinimalFields().WithParentWorkUnitID("root").WithFinalizationState(pb.WorkUnit_FINALIZED).WithFinalizeTime(cutoffTime.Add(-1 * time.Hour)).Build()
			wu2 := workunits.NewBuilder(rootInvID, "wu2").WithMinimalFields().WithParentWorkUnitID("root").WithFinalizationState(pb.WorkUnit_FINALIZED).WithFinalizeTime(cutoffTime.Add(-30 * time.Minute)).Build()

			muts = append(muts, workunits.InsertForTesting(rootWU)...)
			muts = append(muts, workunits.InsertForTesting(wu1)...)
			muts = append(muts, workunits.InsertForTesting(wu2)...)
			testutil.MustApply(ctx, t, muts...)

			err := runCatchUp(rootInvID, 2)
			assert.Loosely(t, err, should.BeNil)

			// Should have 3 tasks: 1 WU, 1 TR, 1 Continuation
			assert.Loosely(t, len(sched.Tasks()), should.Equal(3))
			wuTasks := filterTasks("publish-work-units")
			trTasks := filterTasks("publish-test-results")
			contTasks := filterTasks("publish-work-units-catch-up")

			assert.Loosely(t, wuTasks, should.HaveLength(1))
			assert.Loosely(t, trTasks, should.HaveLength(1))
			assert.Loosely(t, contTasks, should.HaveLength(1))

			wuPayload := wuTasks[0].Payload.(*taskspb.PublishWorkUnitsTask)
			assert.Loosely(t, wuPayload.WorkUnitIds, should.HaveLength(2))

			trPayload := trTasks[0].Payload.(*taskspb.PublishTestResultsTask)
			assert.Loosely(t, trPayload.WorkUnitIds, should.HaveLength(2))

			contPayload := contTasks[0].Payload.(*taskspb.PublishWorkUnitsCatchUpTask)
			assert.Loosely(t, contPayload.RootInvocationId, should.Equal(string(rootInvID)))
			assert.Loosely(t, contPayload.PageToken, should.NotBeEmpty)
		})

		t.Run("Gap 2 - Filters by MaxMetadataFinalizedTime", func(t *ftt.Test) {
			rootInvID := rootinvocations.ID("test-root-inv-catchup-opt")
			baseTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			cutoffTime := baseTime.Add(2 * time.Hour)

			// Insert root invocation with MetadataFinalizedTime.
			rootInv := rootinvocations.NewBuilder(rootInvID).
				WithStreamingExportState(pb.RootInvocation_METADATA_FINAL).
				WithMetadataFinalizedTime(cutoffTime).
				Build()
			muts := rootinvocations.InsertForTesting(rootInv)

			rootWU := workunits.NewBuilder(rootInvID, "root").WithMinimalFields().WithFinalizationState(pb.WorkUnit_FINALIZED).WithFinalizeTime(baseTime).Build()
			wu1 := workunits.NewBuilder(rootInvID, "wu1").WithMinimalFields().WithParentWorkUnitID("root").WithFinalizationState(pb.WorkUnit_FINALIZED).WithFinalizeTime(baseTime.Add(1 * time.Hour)).Build()
			wu2 := workunits.NewBuilder(rootInvID, "wu2").WithMinimalFields().WithParentWorkUnitID("root").WithFinalizationState(pb.WorkUnit_FINALIZED).WithFinalizeTime(cutoffTime).Build()
			wu3 := workunits.NewBuilder(rootInvID, "wu3").WithMinimalFields().WithParentWorkUnitID("root").WithFinalizationState(pb.WorkUnit_FINALIZED).WithFinalizeTime(cutoffTime.Add(1 * time.Hour)).Build()

			muts = append(muts, workunits.InsertForTesting(rootWU)...)
			muts = append(muts, workunits.InsertForTesting(wu1)...)
			muts = append(muts, workunits.InsertForTesting(wu2)...)
			muts = append(muts, workunits.InsertForTesting(wu3)...)
			testutil.MustApply(ctx, t, muts...)

			err := runCatchUp(rootInvID, 10)
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, len(sched.Tasks()), should.Equal(2))
			wuTasks := filterTasks("publish-work-units")

			assert.Loosely(t, wuTasks, should.HaveLength(1))
			wuPayload := wuTasks[0].Payload.(*taskspb.PublishWorkUnitsTask)

			// Should only include root and wu1 (strictly before cutoffTime).
			// wu2 (at cutoffTime) is handled by direct flow and wu3 is after cutoffTime.
			assert.Loosely(t, wuPayload.WorkUnitIds, should.HaveLength(2))

			expectedIDs := map[string]bool{"root": true, "wu1": true}
			for _, id := range wuPayload.WorkUnitIds {
				assert.Loosely(t, expectedIDs[id], should.BeTrue)
			}
		})
	})
}
