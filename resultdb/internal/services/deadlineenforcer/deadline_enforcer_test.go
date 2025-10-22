// Copyright 2021 The LUCI Authors.
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

package deadlineenforcer

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	"go.chromium.org/luci/resultdb/internal/workunits"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestDeadlineEnforcer(t *testing.T) {
	ftt.Run(`Deadline Enforcer`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		ctx, sched := tq.TestingContext(ctx, nil)
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		store := tsmon.Store(ctx)

		// Always use a stable time to prevent flakiness.
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

		past := clock.Now(ctx).Add(-10 * time.Minute)
		future := clock.Now(ctx).Add(10 * time.Minute)

		runEnforcer := func() {
			// Run legacy enforcer.
			s, err := invocations.CurrentMaxShard(ctx)
			assert.Loosely(t, err, should.BeNil)
			for i := 0; i < s+1; i++ {
				assert.Loosely(t, enforceOneShardLegacy(ctx, i), should.BeNil)
			}

			// Run new enforcer.
			for i := 0; i < enforcerShardCount; i++ {
				assert.Loosely(t, enforceOneShard(ctx, i), should.BeNil)
			}
		}

		t.Run(`Expired Invocations`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Invocation("expired", resultpb.Invocation_ACTIVE, map[string]any{"Deadline": past}),
				insert.Invocation("unexpired", resultpb.Invocation_ACTIVE, map[string]any{"Deadline": future}),
			)

			runEnforcer()

			assert.Loosely(t, sched.Tasks().Payloads(), should.Match([]protoreflect.ProtoMessage{
				&taskspb.RunExportNotifications{InvocationId: "expired"},
				&taskspb.TryFinalizeInvocation{InvocationId: "expired"},
			}))
			var state resultpb.Invocation_State
			testutil.MustReadRow(ctx, t, "Invocations", invocations.ID("expired").Key(), map[string]any{
				"State": &state,
			})
			assert.Loosely(t, state, should.Equal(resultpb.Invocation_FINALIZING))
			testutil.MustReadRow(ctx, t, "Invocations", invocations.ID("unexpired").Key(), map[string]any{
				"State": &state,
			})
			assert.Loosely(t, state, should.Equal(resultpb.Invocation_ACTIVE))

			assert.Loosely(t, store.Get(ctx, overdueInvocationsFinalized, []any{insert.TestRealm}), should.Equal(1))
			d := store.Get(ctx, timeInvocationsOverdue, []any{insert.TestRealm}).(*distribution.Distribution)
			// The 10 minute (600 s) delay should fall into bucket 29 (~400k - ~630k ms).
			// allow +/- 1 bucket for clock shenanigans.
			assert.Loosely(t, d.Buckets()[28] == 1 || d.Buckets()[29] == 1 || d.Buckets()[30] == 1, should.BeTrue)
		})
		t.Run(`Expired Root Invocations and Work Units`, func(t *ftt.Test) {
			var ms []*spanner.Mutation
			ms = append(ms, insert.RootInvocationOnly(rootinvocations.NewBuilder("unexpired").WithFinalizationState(resultpb.RootInvocation_ACTIVE).Build())...)
			ms = append(ms, insert.WorkUnit(workunits.NewBuilder("unexpired", "root").WithDeadline(future).WithFinalizationState(resultpb.WorkUnit_ACTIVE).Build())...)
			ms = append(ms, insert.RootInvocationOnly(rootinvocations.NewBuilder("expired").WithFinalizationState(resultpb.RootInvocation_ACTIVE).Build())...)
			ms = append(ms, insert.WorkUnit(workunits.NewBuilder("expired", "root").WithDeadline(past).WithFinalizationState(resultpb.WorkUnit_ACTIVE).Build())...)
			testutil.MustApply(ctx, t, ms...)

			const expiredRootInvocationID = rootinvocations.ID("expired")
			const unexpiredRootInvocationID = rootinvocations.ID("unexpired")
			expiredWorkUnitID := workunits.ID{RootInvocationID: expiredRootInvocationID, WorkUnitID: "root"}
			unexpiredWorkUnitID := workunits.ID{RootInvocationID: unexpiredRootInvocationID, WorkUnitID: "root"}

			// A expired child work unit in the unexpired root invocation.
			expiredChildWorkUnit := workunits.NewBuilder(unexpiredRootInvocationID, "expiredChild").WithDeadline(past).WithFinalizationState(resultpb.WorkUnit_ACTIVE).Build()
			testutil.MustApply(ctx, t, workunits.InsertForTesting(expiredChildWorkUnit)...)

			runEnforcer()

			expectedTasks := []protoreflect.ProtoMessage{
				&taskspb.SweepWorkUnitsForFinalization{RootInvocationId: string(expiredRootInvocationID), SequenceNumber: 1},
				&taskspb.SweepWorkUnitsForFinalization{RootInvocationId: string(unexpiredRootInvocationID), SequenceNumber: 1},
			}
			assert.That(t, sched.Tasks().Payloads(), should.Match(expectedTasks))
			// Finalizer task state updated on root invocation.
			taskState, err := rootinvocations.ReadFinalizerTaskState(span.Single(ctx), expiredRootInvocationID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, taskState, should.Match(rootinvocations.FinalizerTaskState{Pending: true, Sequence: 1}))
			taskState, err = rootinvocations.ReadFinalizerTaskState(span.Single(ctx), unexpiredRootInvocationID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, taskState, should.Match(rootinvocations.FinalizerTaskState{Pending: true, Sequence: 1}))

			invState, err := rootinvocations.ReadFinalizationState(span.Single(ctx), expiredRootInvocationID)
			assert.That(t, invState, should.Equal(resultpb.RootInvocation_FINALIZING))

			invState, err = rootinvocations.ReadFinalizationState(span.Single(ctx), unexpiredRootInvocationID)
			assert.That(t, invState, should.Equal(resultpb.RootInvocation_ACTIVE))

			readWU, err := workunits.Read(span.Single(ctx), expiredWorkUnitID, workunits.AllFields)
			assert.That(t, readWU.FinalizationState, should.Match(resultpb.WorkUnit_FINALIZING))
			assert.That(t, readWU.FinalizerCandidateTime.Valid, should.BeTrue)

			readWU, err = workunits.Read(span.Single(ctx), unexpiredWorkUnitID, workunits.AllFields)
			assert.That(t, readWU.FinalizationState, should.Match(resultpb.WorkUnit_ACTIVE))
			assert.That(t, readWU.FinalizerCandidateTime.Valid, should.BeFalse)

			readWU, err = workunits.Read(span.Single(ctx), expiredChildWorkUnit.ID, workunits.AllFields)
			assert.That(t, readWU.FinalizationState, should.Match(resultpb.WorkUnit_FINALIZING))
			assert.That(t, readWU.FinalizerCandidateTime.Valid, should.BeTrue)

			assert.Loosely(t, store.Get(ctx, overdueWorkUnitsFinalized, []any{insert.TestRealm}), should.Equal(2))
			d := store.Get(ctx, timeWorkUnitsOverdue, []any{insert.TestRealm}).(*distribution.Distribution)

			// The 10 minute (600 s) delay should fall into bucket 29 (~400k - ~630k ms).
			// allow +/- 1 bucket for clock shenanigans.
			var bucketSum int64
			for i, b := range d.Buckets() {
				if i >= 28 && i <= 30 {
					bucketSum += b
				}
			}
			assert.Loosely(t, bucketSum, should.Equal(2))

			t.Run(`Enforcer is idempotent`, func(t *ftt.Test) {
				runEnforcer()

				// No further tasks have been enqueued.
				assert.That(t, sched.Tasks().Payloads(), should.Match(expectedTasks))

				// Counts remain the same.
				assert.Loosely(t, store.Get(ctx, overdueWorkUnitsFinalized, []any{insert.TestRealm}), should.Equal(2))
			})
		})
	})
}
