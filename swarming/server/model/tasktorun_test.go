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

package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestTaskToRuns(t *testing.T) {
	t.Parallel()

	ftt.Run("TestTaskToRuns", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		now := testclock.TestRecentTimeUTC
		ctx, _ = testclock.UseTime(ctx, now)

		buildTaskSlices := func(vals []string, taskReq *datastore.Key) []TaskSlice {
			res := make([]TaskSlice, len(vals))
			for i, val := range vals {
				res[i] = TaskSlice{
					Properties: TaskProperties{
						Dimensions: TaskDimensions{
							"d1": {"v1", "v2"},
							"d2": {val},
						},
					},
					ExpirationSecs: 60,
				}
			}
			return res
		}

		buildToRuns := func(taskReq *TaskRequest) []*TaskToRun {
			res := make([]*TaskToRun, len(taskReq.TaskSlices))
			for i := range len(taskReq.TaskSlices) {
				res[i], _ = NewTaskToRun(ctx, "swarming", taskReq, i)
			}
			return res
		}

		key, err := TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		assert.NoErr(t, err)

		taskSlices := buildTaskSlices([]string{"v3", "v4", "v5"}, key)
		taskReq := &TaskRequest{
			Key:        key,
			TaskSlices: taskSlices,
			Created:    now.Add(-time.Hour),
		}
		expirations := []int{60, 120, 180}
		toRuns := buildToRuns(taskReq)
		toPut := []any{taskReq}
		for i, toRun := range toRuns {
			toPut = append(toPut, toRun)
			if i == 0 {
				assert.That(t, toRun.Created, should.Match(taskReq.Created))
			} else {
				assert.That(t, toRun.Created, should.Match(now))
			}
			assert.That(t, toRun.RBEReservation,
				should.Equal(fmt.Sprintf("swarming-65aba3a3e6b99310-%d-0", i)))
			assert.That(t, toRun.Expiration.Get(),
				should.Match(taskReq.Created.Add(time.Duration(expirations[i])*time.Second)))
			assert.That(t, toRun.Dimensions, should.Match(taskSlices[i].Properties.Dimensions))
		}
		assert.NoErr(t, datastore.Put(ctx, toPut...))

		t.Run("Consume", func(t *ftt.Test) {
			claimID := "claim_id"
			toRuns[0].Consume(claimID)
			assert.Loosely(t, toRuns[0].ClaimID.Get(), should.Equal(claimID))
			assert.Loosely(t, toRuns[0].Expiration.Get().IsZero(), should.BeTrue)
			assert.Loosely(t, toRuns[0].QueueNumber.Get(), should.BeZero)
		})

		t.Run("TaskSliceIndex", func(t *ftt.Test) {
			for i := range len(toRuns) {
				assert.That(t, toRuns[i].TaskSliceIndex(), should.Equal(i))
			}
		})

		t.Run("MustShardIndex", func(t *ftt.Test) {
			for i := range len(toRuns) {
				assert.That(t, toRuns[i].MustShardIndex(),
					should.Equal(sliceToToRunShardIndex(taskSlices[i])))
			}
		})

		t.Run("TaskRequestToToRunKey errors", func(t *ftt.Test) {
			t.Run("fail", func(t *ftt.Test) {
				_, err := TaskRequestToToRunKey(ctx, taskReq, -1)
				assert.Loosely(t, err, should.ErrLike("sliceIndex -1 out of range: [0, 3)"))
				_, err = TaskRequestToToRunKey(ctx, taskReq, 5)
				assert.Loosely(t, err, should.ErrLike("sliceIndex 5 out of range: [0, 3)"))
			})
			t.Run("pass", func(t *ftt.Test) {
				key, err := TaskRequestToToRunKey(ctx, taskReq, 0)
				assert.NoErr(t, err)
				toRun := &TaskToRun{Key: key}
				assert.NoErr(t, datastore.Get(ctx, toRun))
				assert.Loosely(t, toRun.Dimensions["d2"], should.Match([]string{"v3"}))
			})
		})
	})
}
