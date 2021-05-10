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

	"go.chromium.org/luci/common/clock"
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

		past := clock.Now(ctx).Add(-10 * time.Minute)
		future := clock.Now(ctx).Add(10 * time.Minute)

		testutil.MustApply(ctx,
			insert.Invocation("expired", resultpb.Invocation_ACTIVE, map[string]interface{}{"Deadline": past}),
			insert.Invocation("unexpired", resultpb.Invocation_ACTIVE, map[string]interface{}{"Deadline": future}),
		)

		go run(ctx, 100*time.Millisecond)
		for len(sched.Tasks().Payloads()) == 0 {
			time.Sleep(1 * time.Millisecond)
		}

		So(sched.Tasks().Payloads()[0], ShouldResembleProto, &taskspb.TryFinalizeInvocation{InvocationId: "expired"})
		var state resultpb.Invocation_State
		testutil.MustReadRow(ctx, "Invocations", invocations.ID("expired").Key(), map[string]interface{}{
			"State": &state,
		})
		So(state, ShouldEqual, resultpb.Invocation_FINALIZING)
		testutil.MustReadRow(ctx, "Invocations", invocations.ID("unexpired").Key(), map[string]interface{}{
			"State": &state,
		})
		So(state, ShouldEqual, resultpb.Invocation_ACTIVE)
	})
}
