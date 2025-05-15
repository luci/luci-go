// Copyright 2024 The LUCI Authors.
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

package scan

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tasks"
)

func TestSliceExpirationEnforcer(t *testing.T) {
	t.Parallel()

	submitTime := time.Date(2044, time.February, 3, 4, 5, 0, 0, time.UTC)
	currentTime := submitTime.Add(5 * time.Minute)

	ctx, tc := testclock.UseTime(context.Background(), submitTime)
	ctx = memory.Use(ctx)

	var testTasks []*model.TaskResultSummary

	prepTask := func(name string, exp time.Time, running bool) {
		tr := &model.TaskRequest{
			Key:     model.NewTaskRequestKey(ctx),
			Name:    name,
			Created: submitTime,
			TaskSlices: []model.TaskSlice{
				{
					ExpirationSecs: int64(exp.Sub(submitTime).Seconds()),
				},
			},
		}
		assert.NoErr(t, datastore.Put(ctx, tr))
		trs := &model.TaskResultSummary{
			Key: model.TaskResultSummaryKey(ctx, tr.Key),
		}
		if running {
			trs.State = apipb.TaskState_RUNNING
		} else {
			trs.State = apipb.TaskState_PENDING
			ttr, err := model.NewTaskToRun(ctx, "proj", tr, 0)
			assert.NoErr(t, err)
			trs.ActivateTaskToRun(ttr)
			assert.NoErr(t, datastore.Put(ctx, ttr))
		}
		assert.NoErr(t, datastore.Put(ctx, trs))

		testTasks = append(testTasks, trs)
	}

	prepTask("expired-recently-1", currentTime.Add(-59*time.Second), false) // will be ignored, too soon per GracePeriod
	prepTask("expired-recently-2", currentTime.Add(-time.Second), false)    // will be ignored, too soon per GracePeriod
	prepTask("expired-recently-2", currentTime.Add(time.Second), false)     // will be ignored, not expired at all

	prepTask("expired-not-recently-1", currentTime.Add(-61*time.Second), false) // will be processed
	prepTask("expired-not-recently-2", currentTime.Add(-61*time.Second), false) // will be processed

	prepTask("not-pending", currentTime.Add(-61*time.Second), true) // will be ignored, not pending
	prepTask("fail-to-expiry", currentTime.Add(-61*time.Second), false)
	prepTask("skippped-by-txn", currentTime.Add(-61*time.Second), false)

	var mu sync.Mutex
	var expired []string

	visitor := &SliceExpirationEnforcer{
		GracePeriod: time.Minute,
		TasksManager: &tasks.MockedManager{
			ExpireSliceTxnMock: func(_ context.Context, op *tasks.ExpireSliceOp) (*tasks.ExpireSliceTxnOutcome, error) {
				switch op.Request.Name {
				case "fail-to-expiry":
					return nil, errors.New("BOOM")
				case "skippped-by-txn":
					return &tasks.ExpireSliceTxnOutcome{Expired: false}, nil
				default:
					mu.Lock()
					expired = append(expired, op.Request.Name)
					mu.Unlock()
					return &tasks.ExpireSliceTxnOutcome{Expired: true}, nil
				}
			},
		},
	}

	tc.Set(currentTime)

	visitor.Prepare(ctx)
	for _, t := range testTasks {
		visitor.Visit(ctx, t)
	}
	err := visitor.Finalize(ctx, nil)

	// There was one failure.
	assert.That(t, err, should.ErrLike("failed to expire some task slices"))

	assert.That(t, visitor.expiredCount.Load(), should.Equal(int64(2)))
	assert.That(t, visitor.failedCount.Load(), should.Equal(int64(1)))
	assert.That(t, visitor.skippedCount.Load(), should.Equal(int64(1)))

	slices.Sort(expired)
	assert.That(t, expired, should.Match([]string{
		"expired-not-recently-1",
		"expired-not-recently-2",
	}))
}
