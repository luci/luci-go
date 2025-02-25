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
	"context"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg/prjcfgtest"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
)

func TestScheduleWipeoutRuns(t *testing.T) {
	t.Parallel()

	ftt.Run("Schedule wipeout runs tasks", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		registerWipeoutRunsTask(ct.TQDispatcher, &mockRM{})

		// Test Scenario: Create a lot of runs under 2 LUCI Projects (1 disabled).
		// Making sure the tasks are scheduled for all runs that are out of
		// retention period.

		cfg := &cfgpb.Config{
			ConfigGroups: []*cfgpb.ConfigGroup{{
				Name: "main",
			}},
		}
		const projFoo = "foo"
		const projDisabled = "disabled"
		prjcfgtest.Create(ctx, projFoo, cfg)
		prjcfgtest.Create(ctx, projDisabled, cfg)
		prjcfgtest.Disable(ctx, projDisabled)

		createNRunsBetween := func(proj string, n int, start, end time.Time) []*run.Run {
			assert.Loosely(t, n, should.BeGreaterThan(0))
			assert.Loosely(t, end, should.HappenAfter(start))
			runs := make([]*run.Run, n)
			for i := range runs {
				createTime := start.Add(time.Duration(mathrand.Int63n(ctx, int64(end.Sub(start))))).Truncate(time.Millisecond)
				runs[i] = &run.Run{
					ID:         common.MakeRunID(proj, createTime, 1, []byte("deadbeef")),
					CreateTime: createTime,
				}
			}
			assert.Loosely(t, datastore.Put(ctx, runs), should.BeNil)
			return runs
		}

		cutOff := ct.Clock.Now().UTC().Add(-retentionPeriod)
		var allRuns []*run.Run
		allRuns = append(allRuns, createNRunsBetween(projFoo, 1000, cutOff.Add(-time.Hour), cutOff.Add(time.Hour))...)
		allRuns = append(allRuns, createNRunsBetween(projDisabled, 500, cutOff.Add(-time.Minute), cutOff.Add(time.Minute))...)

		var expectedRuns common.RunIDs
		for _, r := range allRuns {
			if r.CreateTime.Before(cutOff) {
				expectedRuns.InsertSorted(r.ID)
			}
		}

		err := scheduleWipeoutRuns(ctx, ct.TQDispatcher)
		assert.NoErr(t, err)
		var actualRuns common.RunIDs
		for _, task := range ct.TQ.Tasks() {
			assert.Loosely(t, task.ETA, should.HappenWithin(wipeoutTasksDistInterval, ct.Clock.Now()))
			ids := task.Payload.(*WipeoutRunsTask).GetIds()
			assert.Loosely(t, len(ids), should.BeLessThanOrEqual(runsPerTask))
			for _, id := range ids {
				actualRuns.InsertSorted(common.RunID(id))
			}
		}
		assert.Loosely(t, actualRuns, should.Match(expectedRuns))
	})
}

func TestWipeoutRuns(t *testing.T) {
	t.Parallel()

	ftt.Run("Wipeout", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "infra"
		mockRM := &mockRM{}
		makeRun := func(createTime time.Time) *run.Run {
			r := &run.Run{
				ID:         common.MakeRunID(lProject, createTime, 1, []byte("deadbeef")),
				CreateTime: createTime,
				Status:     run.Status_SUCCEEDED,
			}
			assert.Loosely(t, datastore.Put(ctx, r), should.BeNil)
			return r
		}

		t.Run("wipeout Run and children", func(t *ftt.Test) {
			r := makeRun(ct.Clock.Now().Add(-2 * retentionPeriod).UTC())
			cl1 := &run.RunCL{
				ID:  1,
				Run: datastore.KeyForObj(ctx, r),
			}
			cl2 := &run.RunCL{
				ID:  2,
				Run: datastore.KeyForObj(ctx, r),
			}
			t.Run("run and run cls", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, cl1, cl2), should.BeNil)
				assert.Loosely(t, wipeoutRuns(ctx, common.RunIDs{r.ID}, mockRM), should.BeNil)
				assert.Loosely(t, datastore.Get(ctx, r), should.ErrLike(datastore.ErrNoSuchEntity))
				assert.Loosely(t, datastore.Get(ctx, cl1), should.ErrLike(datastore.ErrNoSuchEntity))
				assert.Loosely(t, datastore.Get(ctx, cl2), should.ErrLike(datastore.ErrNoSuchEntity))
			})

			t.Run("with a lot of log", func(t *ftt.Test) {
				logs := make([]*run.RunLog, 5000)
				for i := range logs {
					logs[i] = &run.RunLog{
						ID:  int64(i + 1000),
						Run: datastore.KeyForObj(ctx, r),
					}
				}
				assert.Loosely(t, datastore.Put(ctx, cl1, cl2, logs), should.BeNil)
				assert.Loosely(t, wipeoutRuns(ctx, common.RunIDs{r.ID}, mockRM), should.BeNil)
				for _, log := range logs {
					assert.Loosely(t, datastore.Get(ctx, log), should.ErrLike(datastore.ErrNoSuchEntity))
				}
				assert.Loosely(t, datastore.Get(ctx, r), should.ErrLike(datastore.ErrNoSuchEntity))
				assert.Loosely(t, datastore.Get(ctx, cl1), should.ErrLike(datastore.ErrNoSuchEntity))
				assert.Loosely(t, datastore.Get(ctx, cl2), should.ErrLike(datastore.ErrNoSuchEntity))
			})
		})

		t.Run("handle run doesn't exist", func(t *ftt.Test) {
			createTime := ct.Clock.Now().Add(-2 * retentionPeriod).UTC()
			rid := common.MakeRunID(lProject, createTime, 1, []byte("deadbeef"))
			assert.Loosely(t, wipeoutRuns(ctx, common.RunIDs{rid}, mockRM), should.BeNil)
		})

		t.Run("handle run should still be retained", func(t *ftt.Test) {
			r := makeRun(ct.Clock.Now().Add(-retentionPeriod / 2).UTC())
			assert.Loosely(t, wipeoutRuns(ctx, common.RunIDs{r.ID}, mockRM), should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, r), should.BeNil)
		})

		t.Run("Poke run if it is not ended", func(t *ftt.Test) {
			r := makeRun(ct.Clock.Now().Add(-2 * retentionPeriod).UTC())
			r.Status = run.Status_PENDING
			assert.Loosely(t, datastore.Put(ctx, r), should.BeNil)
			assert.Loosely(t, wipeoutRuns(ctx, common.RunIDs{r.ID}, mockRM), should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, r), should.BeNil)
			assert.Loosely(t, mockRM.called, should.Match(common.RunIDs{r.ID}))
		})
	})
}

type mockRM struct {
	called   common.RunIDs
	calledMu sync.Mutex
}

func (rm *mockRM) PokeNow(ctx context.Context, runID common.RunID) error {
	rm.calledMu.Lock()
	rm.called = append(rm.called, runID)
	rm.calledMu.Unlock()
	return nil
}
