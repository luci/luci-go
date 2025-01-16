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

package tasks

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/filter/featureBreaker/flaky"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/cron"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/swarming/server/model"
)

func TestTasksCleanup(t *testing.T) {
	t.Parallel()

	var testTime = time.Date(2024, time.March, 3, 4, 5, 6, 0, time.UTC)
	const testMaxAge = time.Hour

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := gologger.StdConfig.Use(context.Background())
		ctx, _ = testclock.UseTime(ctx, testTime)
		ctx = memory.Use(ctx)
		ctx, fb := featureBreaker.FilterRDS(ctx, nil)
		datastore.GetTestable(ctx).Consistent(true)

		tqdisp := tq.Dispatcher{}
		ctx, sched := tq.TestingContext(ctx, &tqdisp)

		tc := &tasksCleaner{
			cron:                 &cron.Dispatcher{},
			tq:                   &tqdisp,
			maxTaskAge:           testMaxAge,
			maxCronRunTime:       time.Minute,
			cleanupTaskBatchSize: 3,
		}
		tc.register()

		var counter int64
		createGroup := func(age time.Duration) {
			counter += 1
			key, err := model.TimestampToRequestKey(ctx, testTime.Add(-age), counter)
			if err != nil {
				panic(err)
			}
			err = datastore.Put(ctx,
				&model.TaskRequest{
					Key:     key,
					Created: testTime.Add(-age),
					Name:    age.String(),
				},
				// Create a bunch of child entities as well (their kind doesn't matter).
				&model.TaskToRun{Key: model.TaskToRunKey(ctx, key, 1, 1)},
				&model.TaskToRun{Key: model.TaskToRunKey(ctx, key, 2, 2)},
				&model.TaskToRun{Key: model.TaskToRunKey(ctx, key, 3, 3)},
				&model.TaskToRun{Key: model.TaskToRunKey(ctx, key, 4, 4)},
			)
			if err != nil {
				panic(err)
			}
		}

		remainingGroups := func() []string {
			var ents []*model.TaskRequest
			if err := datastore.GetAll(ctx, datastore.NewQuery("TaskRequest"), &ents); err != nil {
				panic(err)
			}
			var names []string
			for _, ent := range ents {
				names = append(names, ent.Name)
			}
			return names
		}

		t.Run("Happy path", func(t *ftt.Test) {
			// Create a mix of old and new entity groups.
			for m := -5 * time.Minute; m < 5*time.Minute; m += time.Minute {
				createGroup(testMaxAge + m)
			}

			// All initial entity groups.
			assert.Loosely(t, remainingGroups(), should.Match([]string{
				"55m0s",
				"56m0s",
				"57m0s",
				"58m0s",
				"59m0s",
				"1h0m0s", // this one and below are "old"
				"1h1m0s",
				"1h2m0s",
				"1h3m0s",
				"1h4m0s",
			}))

			err := tc.triggerCleanupTasks(ctx)
			assert.NoErr(t, err)
			assert.Loosely(t, sched.Tasks(), should.HaveLength(2))
			sched.Run(ctx, tqtesting.StopWhenDrained())

			// Only "new" ones remained.
			assert.Loosely(t, remainingGroups(), should.Match([]string{
				"55m0s",
				"56m0s",
				"57m0s",
				"58m0s",
				"59m0s",
			}))
		})

		t.Run("Makes progress in presence of errors", func(t *ftt.Test) {
			// Create a whole bunch of "old" entity groups.
			for m := time.Minute; m < 100*time.Minute; m += time.Minute {
				createGroup(testMaxAge + m)
			}

			// Submits a bunch of cleanup tasks.
			err := tc.triggerCleanupTasks(ctx)
			assert.NoErr(t, err)

			// All cleanup tasks eventually succeed and delete everything.
			withBrokenDS(fb, func() {
				sched.Run(ctx, tqtesting.StopWhenDrained())
			})
			assert.Loosely(t, remainingGroups(), should.HaveLength(0))
		})
	})
}

func TestCleanupEntityGroup(t *testing.T) {
	t.Parallel()

	var testTime = time.Date(2024, time.March, 3, 4, 5, 6, 0, time.UTC)

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx, fb := featureBreaker.FilterRDS(ctx, nil)
		datastore.GetTestable(ctx).Consistent(true)

		newEntityGroup := func(ts time.Time) *datastore.Key {
			key, err := model.TimestampToRequestKey(ctx, ts, 1)
			if err != nil {
				panic(err)
			}
			err = datastore.Put(ctx,
				&model.TaskRequest{Key: key},
				&model.SecretBytes{Key: model.SecretBytesKey(ctx, key)},
				&model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, key)},
				&model.TaskOutputChunk{Key: model.TaskOutputChunkKey(ctx, key, 1)},
				&model.TaskOutputChunk{Key: model.TaskOutputChunkKey(ctx, key, 2)},
				&model.TaskToRun{Key: model.TaskToRunKey(ctx, key, 1, 1)},
				&model.TaskToRun{Key: model.TaskToRunKey(ctx, key, 2, 2)},
			)
			if err != nil {
				panic(err)
			}
			return key
		}

		fetchGroup := func(key *datastore.Key) []*datastore.Key {
			var keys []*datastore.Key
			err := datastore.GetAll(ctx, datastore.NewQuery("").Ancestor(key).KeysOnly(true), &keys)
			if err != nil {
				panic(err)
			}
			return keys
		}

		t.Run("Works", func(t *ftt.Test) {
			key1 := newEntityGroup(testTime)
			key2 := newEntityGroup(testTime.Add(time.Second))

			assert.That(t, cleanupEntityGroup(ctx, key1, 4), should.BeTrue)

			assert.Loosely(t, fetchGroup(key1), should.HaveLength(0))
			assert.Loosely(t, fetchGroup(key2), should.HaveLength(7))
		})

		t.Run("Handles errors", func(t *ftt.Test) {
			key := newEntityGroup(testTime)

			withBrokenDS(fb, func() {
				assert.That(t, cleanupEntityGroup(ctx, key, 4), should.BeFalse)
			})

			// Some entities remained, and TaskRequest is among them.
			remained := fetchGroup(key)
			assert.Loosely(t, remained, should.HaveLength(5))
			assert.That(t, key.Equal(remained[0]), should.BeTrue)
		})
	})
}

func withBrokenDS(fb featureBreaker.FeatureBreaker, cb func()) {
	// Makes datastore faulty.
	fb.BreakFeaturesWithCallback(
		flaky.Errors(flaky.Params{
			DeadlineProbability: 0.3,
		}),
		featureBreaker.DatastoreFeatures...,
	)

	cb()

	// "Fixes" datastore, letting us examine it.
	fb.BreakFeaturesWithCallback(
		func(context.Context, string) error { return nil },
		featureBreaker.DatastoreFeatures...,
	)
}
