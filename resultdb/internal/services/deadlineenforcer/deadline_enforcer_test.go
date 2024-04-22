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
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
	"go.chromium.org/luci/resultdb/internal/testutil/insert"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestExpiredInvocations(t *testing.T) {
	Convey(`ExpiredInvocations`, t, func() {
		ctx := testutil.SpannerTestContext(t)
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()

		ctx, sched := tq.TestingContext(ctx, nil)
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		store := tsmon.Store(ctx)

		past := clock.Now(ctx).Add(-10 * time.Minute)
		future := clock.Now(ctx).Add(10 * time.Minute)

		testutil.MustApply(ctx,
			insert.Invocation("expired", resultpb.Invocation_ACTIVE, map[string]any{"Deadline": past}),
			insert.Invocation("unexpired", resultpb.Invocation_ACTIVE, map[string]any{"Deadline": future}),
		)

		s, err := invocations.CurrentMaxShard(ctx)
		So(err, ShouldBeNil)
		for i := 0; i < s+1; i++ {
			So(enforceOneShard(ctx, i), ShouldBeNil)
		}

		So(sched.Tasks().Payloads(), ShouldResembleProto, []protoreflect.ProtoMessage{
			&taskspb.RunExportNotifications{InvocationId: "expired"},
			&taskspb.TryFinalizeInvocation{InvocationId: "expired"},
		})
		var state resultpb.Invocation_State
		testutil.MustReadRow(ctx, "Invocations", invocations.ID("expired").Key(), map[string]any{
			"State": &state,
		})
		So(state, ShouldEqual, resultpb.Invocation_FINALIZING)
		testutil.MustReadRow(ctx, "Invocations", invocations.ID("unexpired").Key(), map[string]any{
			"State": &state,
		})
		So(state, ShouldEqual, resultpb.Invocation_ACTIVE)

		So(store.Get(ctx, overdueInvocationsFinalized, time.Time{}, []any{insert.TestRealm}), ShouldEqual, 1)
		d := store.Get(ctx, timeOverdue, time.Time{}, []any{insert.TestRealm}).(*distribution.Distribution)
		// The 10 minute (600 s) delay should fall into bucket 29 (~400k - ~630k ms).
		// allow +/- 1 bucket for clock shenanigans.
		So(d.Buckets()[28] == 1 || d.Buckets()[29] == 1 || d.Buckets()[30] == 1, ShouldBeTrue)
	})
}
