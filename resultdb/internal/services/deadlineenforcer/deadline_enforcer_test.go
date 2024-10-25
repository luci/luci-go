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

	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestExpiredInvocations(t *testing.T) {
	ftt.Run(`ExpiredInvocations`, t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		ctx, sched := tq.TestingContext(ctx, nil)
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		store := tsmon.Store(ctx)

		past := clock.Now(ctx).Add(-10 * time.Minute)
		future := clock.Now(ctx).Add(10 * time.Minute)

		testutil.MustApply(ctx, t,
			insert.Invocation("expired", resultpb.Invocation_ACTIVE, map[string]any{"Deadline": past}),
			insert.Invocation("unexpired", resultpb.Invocation_ACTIVE, map[string]any{"Deadline": future}),
		)

		s, err := invocations.CurrentMaxShard(ctx)
		assert.Loosely(t, err, should.BeNil)
		for i := 0; i < s+1; i++ {
			assert.Loosely(t, enforceOneShard(ctx, i), should.BeNil)
		}

		assert.Loosely(t, sched.Tasks().Payloads(), should.Resemble([]protoreflect.ProtoMessage{
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
}
