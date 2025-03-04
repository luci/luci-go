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
	"slices"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestScheduleWipeoutTryjobs(t *testing.T) {
	t.Parallel()

	ftt.Run("Schedule wipeout tryjobs tasks", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)
		registerWipeoutTryjobsTask(ct.TQDispatcher)

		// create tryjobs with 1 minute interval.
		tryjobs := make([]*tryjob.Tryjob, 3*tryjobsPerTask)
		for i := range tryjobs {
			tryjobs[i] = tryjob.MustBuildbucketID("bb.example.com", int64(i+1000)).
				MustCreateIfNotExists(ctx)
			ct.Clock.Add(1 * time.Minute)
		}

		// Make half of the tryjobs eligible for wipeout
		ct.Clock.Set(tryjobs[len(tryjobs)/2].EntityUpdateTime.Add(retentionPeriod))

		assert.NoErr(t, scheduleWipeoutTryjobsTasks(ctx, ct.TQDispatcher))
		var expectedTryjobIDs common.TryjobIDs
		for _, tj := range tryjobs[:len(tryjobs)/2] {
			expectedTryjobIDs = append(expectedTryjobIDs, tj.ID)
		}

		var actualTryjobIDs common.TryjobIDs
		for _, task := range ct.TQ.Tasks() {
			assert.Loosely(t, task.ETA, should.HappenWithin(wipeoutTasksDistInterval, ct.Clock.Now()))
			ids := task.Payload.(*WipeoutTryjobsTask).GetIds()
			assert.Loosely(t, len(ids), should.BeLessThanOrEqual(tryjobsPerTask))
			for _, id := range ids {
				actualTryjobIDs = append(actualTryjobIDs, common.TryjobID(id))
			}
		}
		slices.Sort(expectedTryjobIDs)
		slices.Sort(actualTryjobIDs)
		assert.That(t, actualTryjobIDs, should.Match(expectedTryjobIDs))
	})
}
func TestWipeoutTryjobs(t *testing.T) {
	t.Parallel()

	ftt.Run("Wipeout Tryjobs", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		tj := tryjob.MustBuildbucketID("bb.example.com", 12345).
			MustCreateIfNotExists(ctx)
		ct.Clock.Add(2 * retentionPeriod) // make tryjob eligible for wipeout

		t.Run("Can wipeout tryjob", func(t *ftt.Test) {
			t.Run("when there's no watching run", func(t *ftt.Test) {
				assert.NoErr(t, wipeoutTryjobs(ctx, common.TryjobIDs{tj.ID}))
				assert.ErrIsLike(t, datastore.Get(ctx, &tryjob.Tryjob{ID: tj.ID}), datastore.ErrNoSuchEntity)
			})
			t.Run("when all watching run no longer exists", func(t *ftt.Test) {
				tj.LaunchedBy = common.MakeRunID("infra", tj.EntityCreateTime, 1, []byte("deadbeef"))
				tj.ReusedBy = append(tj.ReusedBy, common.MakeRunID("infra", tj.EntityCreateTime.Add(1*time.Minute), 1, []byte("deadbeef")))
				assert.NoErr(t, wipeoutTryjobs(ctx, common.TryjobIDs{tj.ID}))
				assert.ErrIsLike(t, datastore.Get(ctx, &tryjob.Tryjob{ID: tj.ID}), datastore.ErrNoSuchEntity)
			})
		})

		t.Run("Don't wipeout tryjob", func(t *ftt.Test) {
			t.Run("When a run that use this tryjob still exists", func(t *ftt.Test) {
				r := &run.Run{
					ID: common.MakeRunID("infra", tj.EntityCreateTime, 1, []byte("deadbeef")),
				}
				tj.LaunchedBy = r.ID
				tj.ReusedBy = append(tj.ReusedBy, common.MakeRunID("infra", tj.EntityCreateTime.Add(-1*time.Minute), 1, []byte("deadbeef")))
				assert.NoErr(t, datastore.Put(ctx, r, tj))
				assert.NoErr(t, wipeoutTryjobs(ctx, common.TryjobIDs{tj.ID}))
				assert.NoErr(t, datastore.Get(ctx, &tryjob.Tryjob{ID: tj.ID}))
			})

			t.Run("When Tryjob doesn't exist", func(t *ftt.Test) {
				assert.NoErr(t, wipeoutTryjobs(ctx, common.TryjobIDs{tj.ID + 1}))
			})
		})
	})
}
