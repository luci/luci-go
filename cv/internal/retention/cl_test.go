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

package retention

import (
	"sort"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
)

func TestScheduleWipeoutCLs(t *testing.T) {
	t.Parallel()

	ftt.Run("Schedule wipeout cls tasks", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		registerWipeoutCLsTask(ct.TQDispatcher)

		// create 1000 CLs with 1 minute interval.
		cls := make([]*changelist.CL, 1000)
		for i := range cls {
			cls[i] = changelist.MustGobID("example.com", int64(i+1000)).MustCreateIfNotExists(ctx)
			ct.Clock.Add(1 * time.Minute)
		}

		// Make half of the CLs eligible for wipeout
		ct.Clock.Set(cls[len(cls)/2].UpdateTime.Add(retentionPeriod))

		assert.Loosely(t, scheduleWipeoutCLTasks(ctx, ct.TQDispatcher), should.BeNil)
		var expectedCLIDs common.CLIDs
		for _, cl := range cls[:len(cls)/2] {
			expectedCLIDs = append(expectedCLIDs, cl.ID)
		}

		var actualCLIDs common.CLIDs
		for _, task := range ct.TQ.Tasks() {
			assert.Loosely(t, task.ETA, should.HappenWithin(wipeoutTasksDistInterval, ct.Clock.Now()))
			ids := task.Payload.(*WipeoutCLsTask).GetIds()
			assert.Loosely(t, len(ids), should.BeLessThanOrEqual(clsPerTask))
			for _, id := range ids {
				actualCLIDs = append(actualCLIDs, common.CLID(id))
			}
		}
		sort.Sort(expectedCLIDs)
		sort.Sort(actualCLIDs)
		assert.Loosely(t, actualCLIDs, should.Resemble(expectedCLIDs))
	})
}
func TestWipeoutCLs(t *testing.T) {
	t.Parallel()

	ftt.Run("Wipeout CLs", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		cl := changelist.MustGobID("example.com", 1111).MustCreateIfNotExists(ctx)
		ct.Clock.Add(2 * retentionPeriod) // make cl eligible for wipeout

		t.Run("Can wipeout CL", func(t *ftt.Test) {
			assert.Loosely(t, wipeoutCLs(ctx, common.CLIDs{cl.ID}), should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, &changelist.CL{ID: cl.ID}), should.ErrLike(datastore.ErrNoSuchEntity))
		})

		t.Run("Don't wipeout CL", func(t *ftt.Test) {
			t.Run("When a run is referencing this CL", func(t *ftt.Test) {
				r := &run.Run{
					ID:  common.MakeRunID("infra", ct.Clock.Now(), 1, []byte("deadbeef")),
					CLs: common.CLIDs{cl.ID},
				}
				rcl := &run.RunCL{
					ID:         1,
					Run:        datastore.KeyForObj(ctx, r),
					IndexedID:  cl.ID,
					ExternalID: cl.ExternalID,
				}
				assert.Loosely(t, datastore.Put(ctx, r, rcl), should.BeNil)
				assert.Loosely(t, wipeoutCLs(ctx, common.CLIDs{cl.ID}), should.BeNil)
				assert.Loosely(t, datastore.Get(ctx, &changelist.CL{ID: cl.ID}), should.BeNil)
			})

			t.Run("When CL doesn't exist", func(t *ftt.Test) {
				assert.Loosely(t, wipeoutCLs(ctx, common.CLIDs{cl.ID + 1}), should.BeNil)
			})
		})
	})
}
