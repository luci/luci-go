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

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/cfg/cfgtest"
	"go.chromium.org/luci/swarming/server/metrics"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tqtasks"
)

func TestExpireSliceTxn(t *testing.T) {
	t.Parallel()

	var (
		testTime   = time.Date(2025, time.February, 3, 4, 5, 0, 0, time.UTC)
		createTime = testTime.Add(-21 * time.Minute)
	)

	ftt.Run("ExpireSliceTxn", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx, _ = testclock.UseTime(ctx, testTime)
		ctx, tqt := tqtasks.TestingContext(ctx)
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		globalStore := tsmon.Store(ctx)

		mgr := NewManager(tqt.Tasks, "swarming-proj", "cur-version", nil, false).(*managerImpl)

		mockCfg := &cfgtest.MockedConfigs{
			Settings: &configpb.SettingsCfg{},
			Pools: &configpb.PoolsCfg{
				Pool: []*configpb.Pool{
					{
						Name:  []string{"pool"},
						Realm: "project:realm",
						RbeMigration: &configpb.Pool_RBEMigration{
							RbeInstance: "rbe-instance",
							BotModeAllocation: []*configpb.Pool_RBEMigration_BotModeAllocation{
								{Mode: configpb.Pool_RBEMigration_BotModeAllocation_RBE, Percent: 100},
							},
						},
					},
				},
			},
		}
		p := cfgtest.MockConfigs(ctx, mockCfg)
		cfg := p.Cached(ctx)

		taskID := "65aba3a3e6b99310"
		reqKey, err := model.TaskIDToRequestKey(ctx, taskID)
		assert.NoErr(t, err)

		req := &model.TaskRequest{
			Key:     reqKey,
			Created: createTime,
			Name:    "task-name",
			Tags:    []string{"pool:pool", "spec_name:spec"},
			TaskSlices: []model.TaskSlice{
				{ // Slice 0
					Properties: model.TaskProperties{
						Dimensions: model.TaskDimensions{"pool": {"pool"}},
					},
					ExpirationSecs: 600,
				},
				{ // Slice 1
					Properties: model.TaskProperties{
						Dimensions: model.TaskDimensions{"pool": {"pool"}},
					},
					ExpirationSecs: 600,
				},
			},
			Expiration:  createTime.Add(1200 * time.Second),
			RBEInstance: "rbe-instance",
			PubSubTopic: "pubsub-topic",
		}
		assert.NoErr(t, datastore.Put(ctx, req))

		createEntities := func(sliceIdx int, reapable bool) (*model.TaskResultSummary, *model.TaskToRun) {
			trs := &model.TaskResultSummary{
				Key:     model.TaskResultSummaryKey(ctx, reqKey),
				Created: createTime,
				Tags:    req.Tags,
				TaskResultCommon: model.TaskResultCommon{
					State:            apipb.TaskState_PENDING,
					Modified:         createTime,
					ServerVersions:   []string{"prev-version"},
					CurrentTaskSlice: int64(sliceIdx),
				},
				RequestPriority: 20,
			}

			ttrKey, err := model.TaskRequestToToRunKey(ctx, req, sliceIdx)
			assert.NoErr(t, err)
			ttr := &model.TaskToRun{
				Key:            ttrKey,
				Created:        createTime.Add(time.Duration(sliceIdx) * 10 * time.Minute),
				Dimensions:     req.TaskSlices[sliceIdx].Properties.Dimensions,
				RBEReservation: model.NewReservationID("swarming-proj", reqKey, sliceIdx, 0),
			}
			if reapable {
				exp := createTime.Add(time.Duration(req.TaskSlices[sliceIdx].ExpirationSecs) * time.Second)
				if sliceIdx > 0 {
					exp = createTime.Add(time.Duration(req.TaskSlices[0].ExpirationSecs+req.TaskSlices[1].ExpirationSecs) * time.Second)
				}
				ttr.Expiration.Set(exp)
			}

			assert.NoErr(t, datastore.Put(ctx, trs, ttr))
			return trs, ttr
		}

		run := func(ctx context.Context, op *ExpireSliceOp) error {
			return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				return mgr.ExpireSliceTxn(ctx, op)
			}, nil)
		}

		t.Run("Task Not Found", func(t *ftt.Test) {
			op := &ExpireSliceOp{
				Request:  req,
				ToRunKey: model.TaskToRunKey(ctx, reqKey, 0, model.TaskToRunID(0)),
				Reason:   Expired,
				Config:   cfg,
			}
			err := run(ctx, op)
			assert.That(t, err, should.ErrLike(`task "65aba3a3e6b99310" not found`))
		})

		t.Run("Slice Already Consumed", func(t *ftt.Test) {
			_, ttr := createEntities(0, false)
			op := &ExpireSliceOp{
				Request:  req,
				ToRunKey: ttr.Key,
				Reason:   Expired,
				Config:   cfg,
			}
			err := run(ctx, op)
			assert.NoErr(t, err)

			loadedTRS := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, reqKey)}
			loadedTTR := &model.TaskToRun{Key: ttr.Key}
			assert.NoErr(t, datastore.Get(ctx, loadedTRS, loadedTTR))
			assert.That(t, loadedTRS.Modified, should.Match(createTime))
			assert.That(t, loadedTTR.IsReapable(), should.BeFalse)
		})

		t.Run("Expire Last Slice - Expired", func(t *ftt.Test) {
			trs, ttr := createEntities(1, true)
			op := &ExpireSliceOp{
				Request:  req,
				ToRunKey: ttr.Key,
				Reason:   Expired,
				Config:   cfg,
			}
			err := run(ctx, op)
			assert.NoErr(t, err)

			loadedTRS := &model.TaskResultSummary{Key: trs.Key}
			loadedTTR := &model.TaskToRun{Key: ttr.Key}
			assert.NoErr(t, datastore.Get(ctx, loadedTRS, loadedTTR))

			assert.That(t, loadedTRS.State, should.Equal(apipb.TaskState_EXPIRED))
			assert.That(t, loadedTRS.Modified, should.Match(testTime))
			assert.That(t, loadedTRS.Completed.Get(), should.Match(testTime))
			assert.That(t, loadedTRS.Abandoned.Get(), should.Match(testTime))
			assert.That(t, loadedTRS.InternalFailure, should.BeFalse)
			assert.That(t, loadedTRS.ExpirationDelay.Get(), should.Equal(time.Minute.Seconds()))
			assert.That(t, loadedTRS.ServerVersions, should.Match([]string{"cur-version", "prev-version"}))

			assert.That(t, loadedTTR.IsReapable(), should.BeFalse)
			assert.That(t, loadedTTR.ExpirationDelay.Get(), should.Equal(time.Minute.Seconds()))

			assert.That(t, tqt.Pending(tqt.PubSubNotify), should.Match([]string{taskID}))
			assert.That(t, tqt.Pending(tqt.FinalizeTask), should.Match([]string{taskID}))

			val := globalStore.Get(ctx, metrics.JobsCompleted, []any{"spec", "", "", "pool", "none", "success", "Expired"})
			assert.Loosely(t, val, should.Equal(1))

			val = globalStore.Get(ctx, metrics.TasksExpired, []any{"spec", "", "", "pool", "none", 20})
			assert.Loosely(t, val, should.Equal(1))

			sliceExp := globalStore.Get(ctx, metrics.TaskSliceExpirationDelay, []any{"", "none", 1, "expired"})
			assert.Loosely(t, sliceExp.(*distribution.Distribution).Sum(), should.Equal(time.Minute.Seconds()))

			taskExp := globalStore.Get(ctx, metrics.TaskExpirationDelay, []any{"", "none"})
			assert.Loosely(t, taskExp.(*distribution.Distribution).Sum(), should.Equal(time.Minute.Seconds()))
		})

		t.Run("Expire Last Slice - NoResource", func(t *ftt.Test) {
			trs, ttr := createEntities(1, true)
			op := &ExpireSliceOp{
				Request:  req,
				ToRunKey: ttr.Key,
				Reason:   NoResource,
				Config:   cfg,
			}
			err := run(ctx, op)
			assert.NoErr(t, err)

			loadedTRS := &model.TaskResultSummary{Key: trs.Key}
			loadedTTR := &model.TaskToRun{Key: ttr.Key}
			assert.NoErr(t, datastore.Get(ctx, loadedTRS, loadedTTR))

			assert.That(t, loadedTRS.State, should.Equal(apipb.TaskState_NO_RESOURCE))
			assert.That(t, loadedTRS.Modified, should.Match(testTime))
			assert.That(t, loadedTRS.Completed.Get(), should.Match(testTime))
			assert.That(t, loadedTRS.Abandoned.Get(), should.Match(testTime))
			assert.That(t, loadedTRS.InternalFailure, should.BeFalse)
			assert.That(t, loadedTRS.ExpirationDelay.IsSet(), should.BeFalse)

			assert.That(t, loadedTTR.IsReapable(), should.BeFalse)
			assert.That(t, loadedTTR.ExpirationDelay.IsSet(), should.BeFalse)

			val := globalStore.Get(ctx, metrics.JobsCompleted, []any{"spec", "", "", "pool", "none", "success", "No resource available"})
			assert.Loosely(t, val, should.Equal(1))

			// NoResource task is not reported to metrics.TasksExpired.
			val = globalStore.Get(ctx, metrics.TasksExpired, []any{"spec", "", "", "pool", "none", 20})
			assert.Loosely(t, val, should.BeNil)

			sliceExp := globalStore.Get(ctx, metrics.TaskSliceExpirationDelay, []any{"", "none", 1, "expired"})
			assert.That(t, sliceExp, should.BeNil)

			taskExp := globalStore.Get(ctx, metrics.TaskExpirationDelay, []any{"", "none"})
			assert.That(t, taskExp, should.BeNil)
		})

		t.Run("Expire Last Slice - BotInternalError", func(t *ftt.Test) {
			trs, ttr := createEntities(1, true)
			culpritBot := "culprit-bot-1"
			op := &ExpireSliceOp{
				Request:      req,
				ToRunKey:     ttr.Key,
				Reason:       BotInternalError,
				CulpritBotID: culpritBot,
				Config:       cfg,
			}
			err := run(ctx, op)
			assert.NoErr(t, err)

			loadedTRS := &model.TaskResultSummary{Key: trs.Key}
			loadedTTR := &model.TaskToRun{Key: ttr.Key}
			assert.NoErr(t, datastore.Get(ctx, loadedTRS, loadedTTR))

			assert.That(t, loadedTRS.State, should.Equal(apipb.TaskState_BOT_DIED))
			assert.That(t, loadedTRS.Modified, should.Match(testTime))
			assert.That(t, loadedTRS.Completed.Get(), should.Match(testTime))
			assert.That(t, loadedTRS.Abandoned.Get(), should.Match(testTime))
			assert.That(t, loadedTRS.InternalFailure, should.BeTrue)
			assert.That(t, loadedTRS.BotID.Get(), should.Equal(culpritBot))
			assert.That(t, loadedTRS.ExpirationDelay.IsSet(), should.BeFalse)

			assert.That(t, loadedTTR.IsReapable(), should.BeFalse)
			assert.That(t, loadedTTR.ExpirationDelay.IsSet(), should.BeFalse)

			val := globalStore.Get(ctx, metrics.JobsCompleted, []any{"spec", "", "", "pool", "none", "infra_failure", "Bot died"})
			assert.Loosely(t, val, should.Equal(1))
		})

		t.Run("Expire Last Slice - InvalidArgument", func(t *ftt.Test) {
			now := createTime.Add(5 * time.Minute)
			ctx, _ := testclock.UseTime(ctx, now)
			trs, ttr := createEntities(1, true)
			op := &ExpireSliceOp{
				Request:  req,
				ToRunKey: ttr.Key,
				Reason:   Expired,
				Config:   cfg,
			}
			err := run(ctx, op)
			assert.NoErr(t, err)

			loadedTRS := &model.TaskResultSummary{Key: trs.Key}
			loadedTTR := &model.TaskToRun{Key: ttr.Key}
			assert.NoErr(t, datastore.Get(ctx, loadedTRS, loadedTTR))

			assert.That(t, loadedTRS.State, should.Equal(apipb.TaskState_EXPIRED))
			assert.That(t, loadedTRS.Modified, should.Match(now))
			assert.That(t, loadedTRS.Completed.Get(), should.Match(now))
			assert.That(t, loadedTRS.Abandoned.Get(), should.Match(now))
			assert.That(t, loadedTRS.InternalFailure, should.BeFalse)
			// The task is not really expired.
			assert.That(t, loadedTRS.ExpirationDelay.Get(), should.Equal(0.0))
			assert.That(t, loadedTRS.ServerVersions, should.Match([]string{"cur-version", "prev-version"}))

			assert.That(t, loadedTTR.IsReapable(), should.BeFalse)
			// The slice is not really expired.
			assert.That(t, loadedTTR.ExpirationDelay.Get(), should.Equal(0.0))

			val := globalStore.Get(ctx, metrics.JobsCompleted, []any{"spec", "", "", "pool", "none", "success", "Expired"})
			assert.Loosely(t, val, should.Equal(1))
		})

		t.Run("Expire Intermediate Slice", func(t *ftt.Test) {
			trs, ttr := createEntities(0, true)
			op := &ExpireSliceOp{
				Request:  req,
				ToRunKey: ttr.Key,
				Reason:   Expired,
				Config:   cfg,
			}
			err := run(ctx, op)
			assert.NoErr(t, err)

			loadedTRS := &model.TaskResultSummary{Key: trs.Key}
			loadedTTR := &model.TaskToRun{Key: ttr.Key}
			nextTTRKey, _ := model.TaskRequestToToRunKey(ctx, req, 1)
			nextTTR := &model.TaskToRun{Key: nextTTRKey}
			assert.NoErr(t, datastore.Get(ctx, loadedTRS, loadedTTR, nextTTR))

			assert.That(t, loadedTRS.State, should.Equal(apipb.TaskState_PENDING)) // Still pending
			assert.That(t, loadedTRS.Modified, should.Match(testTime))
			assert.That(t, loadedTRS.CurrentTaskSlice, should.Equal(int64(1))) // Moved to next slice
			assert.That(t, loadedTRS.Completed.IsSet(), should.BeFalse)
			assert.That(t, loadedTRS.Abandoned.IsSet(), should.BeFalse)
			assert.That(t, loadedTRS.ServerVersions, should.Match([]string{"cur-version", "prev-version"}))

			assert.That(t, loadedTTR.IsReapable(), should.BeFalse) // First slice consumed
			assert.That(t, loadedTTR.ExpirationDelay.Get(), should.Equal(11*time.Minute.Seconds()))

			assert.That(t, nextTTR.IsReapable(), should.BeTrue) // Next slice created and reapable
			assert.That(t, nextTTR.TaskSliceIndex(), should.Equal(1))
			assert.That(t, nextTTR.RBEReservation, should.Equal(model.NewReservationID("swarming-proj", reqKey, 1, 0)))

			// Check TQ tasks
			assert.That(t, tqt.Pending(tqt.EnqueueRBE), should.Match([]string{
				"rbe-instance/" + nextTTR.RBEReservation,
			}))
			assert.Loosely(t, tqt.Pending(tqt.PubSubNotify), should.HaveLength(0)) // No notification for intermediate slice expiry
			assert.Loosely(t, tqt.Pending(tqt.FinalizeTask), should.HaveLength(0))

			// Check metrics (should not report completion and task expiration)
			val := globalStore.Get(ctx, metrics.JobsCompleted, []any{"spec", "", "", "pool", "rbe-instance", "infra_failure", "Expired"})
			assert.Loosely(t, val, should.BeNil)

			val = globalStore.Get(ctx, metrics.TasksExpired, []any{"spec", "", "", "pool", "none", 20})
			assert.Loosely(t, val, should.BeNil)

			sliceExp := globalStore.Get(ctx, metrics.TaskSliceExpirationDelay, []any{"", "none", 0, "expired"})
			assert.Loosely(t, sliceExp.(*distribution.Distribution).Sum(), should.Equal(11*time.Minute.Seconds()))

			taskExp := globalStore.Get(ctx, metrics.TaskExpirationDelay, []any{"", "none"})
			assert.That(t, taskExp, should.BeNil)
		})
	})
}
