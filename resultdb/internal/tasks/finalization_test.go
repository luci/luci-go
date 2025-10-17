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

package tasks

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/internal/testutil"
)

func TestNextMinuteBoundaryWithOffset(t *testing.T) {
	t.Parallel()

	ftt.Run("nextMinuteBoundaryWithOffset", t, func(t *ftt.Test) {
		baseTime := time.Date(2044, time.April, 4, 4, 4, 0, 0, time.UTC)
		ctx := context.Background()

		id1 := rootinvocations.ID("test-id-1")
		offset1 := 34610 * time.Millisecond
		t.Run("now is before offset", func(t *ftt.Test) {
			ctx, _ = testclock.UseTime(ctx, baseTime.Add(34*time.Second))

			result := nextMinuteBoundaryWithOffset(ctx, id1)
			assert.Loosely(t, result, should.Match(baseTime.Add(offset1)))
		})

		t.Run("now is after offset", func(t *ftt.Test) {
			ctx, _ = testclock.UseTime(ctx, baseTime.Add(35*time.Second))

			result := nextMinuteBoundaryWithOffset(ctx, id1)
			assert.Loosely(t, result, should.Match(baseTime.Add(offset1).Add(time.Minute)))
		})

		t.Run("now is exactly at offset", func(t *ftt.Test) {
			ctx, _ = testclock.UseTime(ctx, baseTime.Add(offset1))

			result := nextMinuteBoundaryWithOffset(ctx, id1)
			assert.Loosely(t, result, should.Match(baseTime.Add(offset1).Add(time.Minute)))
		})
	})
}

func TestScheduleWorkUnitsFinalization(t *testing.T) {
	ftt.Run("ScheduleWorkUnitsFinalization", t, func(t *ftt.Test) {
		ctx := testutil.SpannerTestContext(t)
		ctx, sched := tq.TestingContext(ctx, nil)
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		rootInvID := rootinvocations.ID("root-inv-id")
		t.Run("have pending task", func(t *ftt.Test) {
			rootInv := rootinvocations.NewBuilder(rootInvID).WithFinalizerPending(true).WithFinalizerSequence(1).Build()
			testutil.MustApply(ctx, t, rootinvocations.InsertForTesting(rootInv)...)

			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				err := ScheduleWorkUnitsFinalization(ctx, rootInvID)
				if err != nil {
					return err
				}
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
			// no task is scheduled.
			assert.Loosely(t, sched.Tasks().Payloads(), should.HaveLength(0))
			// No update to root invocation.
			readRootInv, err := rootinvocations.Read(span.Single(ctx), rootInvID)
			assert.That(t, readRootInv, should.Match(rootInv))
		})
		t.Run("no pending task", func(t *ftt.Test) {
			rootInv := rootinvocations.NewBuilder(rootInvID).WithFinalizerPending(false).WithFinalizerSequence(1).Build()
			testutil.MustApply(ctx, t, rootinvocations.InsertForTesting(rootInv)...)

			_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
				err := ScheduleWorkUnitsFinalization(ctx, rootInvID)
				if err != nil {
					return err
				}
				return nil
			})
			assert.Loosely(t, err, should.BeNil)
			// Check pending and sequence are set.
			readRootInv, err := rootinvocations.Read(span.Single(ctx), rootInvID)
			assert.Loosely(t, err, should.BeNil)
			expectedRootInv := rootInv.Clone()
			expectedRootInv.FinalizerPending = true
			expectedRootInv.FinalizerSequence = 2
			assert.That(t, readRootInv, should.Match(expectedRootInv))
			// Check task is scheduled.
			expectedTasks := []protoreflect.ProtoMessage{
				&taskspb.SweepWorkUnitsForFinalization{RootInvocationId: string(rootInvID), SequenceNumber: 2},
			}
			assert.Loosely(t, sched.Tasks().Payloads(), should.Match(expectedTasks))
		})
	})
}
