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

		past := clock.Now(ctx).Add(-10 * time.Minute)
		future := clock.Now(ctx).Add(10 * time.Minute)

		t.Run(`Expired Invocations`, func(t *ftt.Test) {
			testutil.MustApply(ctx, t,
				insert.Invocation("expired", resultpb.Invocation_ACTIVE, map[string]any{"Deadline": past}),
				insert.Invocation("unexpired", resultpb.Invocation_ACTIVE, map[string]any{"Deadline": future}),
			)

			s, err := invocations.CurrentMaxShard(ctx)
			assert.Loosely(t, err, should.BeNil)
			for i := 0; i < s+1; i++ {
				assert.Loosely(t, enforceOneShard(ctx, i), should.BeNil)
			}

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
			d := store.Get(ctx, timeOverdue, []any{insert.TestRealm}).(*distribution.Distribution)
			// The 10 minute (600 s) delay should fall into bucket 29 (~400k - ~630k ms).
			// allow +/- 1 bucket for clock shenanigans.
			assert.Loosely(t, d.Buckets()[28] == 1 || d.Buckets()[29] == 1 || d.Buckets()[30] == 1, should.BeTrue)
		})
		t.Run(`Expired Root Invocations and Work Units`, func(t *ftt.Test) {
			var ms []*spanner.Mutation
			ms = append(ms, insert.RootInvocationWithRootWorkUnit(rootinvocations.NewBuilder("expired").WithDeadline(past).WithState(resultpb.RootInvocation_ACTIVE).Build())...)
			ms = append(ms, insert.RootInvocationWithRootWorkUnit(rootinvocations.NewBuilder("unexpired").WithDeadline(future).WithState(resultpb.RootInvocation_ACTIVE).Build())...)
			testutil.MustApply(ctx, t, ms...)

			const expiredRootInvocationID = rootinvocations.ID("expired")
			const unexpiredRootInvocationID = rootinvocations.ID("unexpired")
			expiredWorkUnitID := workunits.ID{RootInvocationID: expiredRootInvocationID, WorkUnitID: "root"}
			unexpiredWorkUnitID := workunits.ID{RootInvocationID: unexpiredRootInvocationID, WorkUnitID: "root"}

			s, err := invocations.CurrentMaxShard(ctx)
			assert.Loosely(t, err, should.BeNil)
			for i := 0; i < s+1; i++ {
				assert.Loosely(t, enforceOneShard(ctx, i), should.BeNil)
			}

			expectedTasks := []protoreflect.ProtoMessage{
				&taskspb.RunExportNotifications{InvocationId: string(expiredWorkUnitID.LegacyInvocationID())},
				&taskspb.TryFinalizeInvocation{InvocationId: string(expiredWorkUnitID.LegacyInvocationID())},
				&taskspb.RunExportNotifications{InvocationId: string(expiredRootInvocationID.LegacyInvocationID())},
				&taskspb.TryFinalizeInvocation{InvocationId: string(expiredRootInvocationID.LegacyInvocationID())},
			}
			assert.That(t, sched.Tasks().Payloads(), should.Match(expectedTasks))

			invState, err := rootinvocations.ReadState(span.Single(ctx), expiredRootInvocationID)
			assert.That(t, invState, should.Equal(resultpb.RootInvocation_FINALIZING))

			invState, err = rootinvocations.ReadState(span.Single(ctx), unexpiredRootInvocationID)
			assert.That(t, invState, should.Equal(resultpb.RootInvocation_ACTIVE))

			state, err := workunits.ReadState(span.Single(ctx), expiredWorkUnitID)
			assert.That(t, state, should.Equal(resultpb.WorkUnit_FINALIZING))

			state, err = workunits.ReadState(span.Single(ctx), unexpiredWorkUnitID)
			assert.That(t, state, should.Equal(resultpb.WorkUnit_ACTIVE))

			assert.Loosely(t, store.Get(ctx, overdueInvocationsFinalized, []any{insert.TestRealm}), should.Equal(2))
			d := store.Get(ctx, timeOverdue, []any{insert.TestRealm}).(*distribution.Distribution)

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
				s, err := invocations.CurrentMaxShard(ctx)
				assert.Loosely(t, err, should.BeNil)
				for i := 0; i < s+1; i++ {
					assert.Loosely(t, enforceOneShard(ctx, i), should.BeNil)
				}

				// No further tasks have been enqueued.
				assert.That(t, sched.Tasks().Payloads(), should.Match(expectedTasks))

				// Counts remain the same.
				assert.Loosely(t, store.Get(ctx, overdueInvocationsFinalized, []any{insert.TestRealm}), should.Equal(2))
			})
		})
	})
}
