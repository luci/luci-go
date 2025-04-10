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
	"sort"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/metrics"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tasks/taskspb"
	"go.chromium.org/luci/swarming/server/tqtasks"
)

func generateEntities(ctx context.Context, reqKey *datastore.Key, state apipb.TaskState, now time.Time) (*model.TaskRequest, *model.TaskResultSummary) {
	tr := &model.TaskRequest{
		Key: reqKey,
		TaskSlices: []model.TaskSlice{
			{
				Properties: model.TaskProperties{
					Dimensions: model.TaskDimensions{
						"d1": {"v1", "v2"},
					},
				},
			},
		},
		PubSubTopic: "pubsub-topic",
		RBEInstance: "rbe-instance",
	}

	trs := &model.TaskResultSummary{
		Key: model.TaskResultSummaryKey(ctx, reqKey),
		TaskResultCommon: model.TaskResultCommon{
			Modified: now.Add(-time.Minute),
			State:    state,
		},
		Created: now.Add(-2 * time.Hour),
		Tags:    []string{"spec_name:spec_name", "pool:test_pool", "device_type:wobblyeye"},
	}
	return tr, trs
}

func TestCancel(t *testing.T) {
	t.Parallel()

	ftt.Run("TestRun", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		now := time.Date(2024, time.January, 1, 2, 3, 4, 0, time.UTC)
		ctx, _ = testclock.UseTime(ctx, now)
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		globalStore := tsmon.Store(ctx)

		ctx, tqt := tqtasks.TestingContext(ctx)
		mgr := NewManager(tqt.Tasks, "swarming-proj", "cur-version", nil, false)

		tID := "65aba3a3e6b99310"
		reqKey, err := model.TaskIDToRequestKey(ctx, tID)
		assert.NoErr(t, err)
		tr, trs := generateEntities(ctx, reqKey, apipb.TaskState_PENDING, now)
		assert.NoErr(t, datastore.Put(ctx, tr))

		run := func(op *CancelOp) (*CancelOpOutcome, error) {
			var outcome *CancelOpOutcome
			err := datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
				outcome, err = mgr.CancelTxn(ctx, op)
				return
			}, nil)
			return outcome, err
		}

		t.Run("failed to get some entity", func(t *ftt.Test) {
			_, err := run(&CancelOp{TaskRequest: tr})
			assert.Loosely(t, err, should.ErrLike("missing TaskResultSummary for task 65aba3a3e6b99310"))
		})

		t.Run("cancel ended task", func(t *ftt.Test) {
			trs.State = apipb.TaskState_COMPLETED
			assert.NoErr(t, datastore.Put(ctx, trs))
			outcome, err := run(&CancelOp{TaskRequest: tr})
			assert.NoErr(t, err)
			assert.That(t, outcome, should.Match(&CancelOpOutcome{
				Canceled:   false,
				WasRunning: false,
			}))
		})

		t.Run("cancel pending task", func(t *ftt.Test) {
			trs.State = apipb.TaskState_PENDING
			toRunKey, err := model.TaskRequestToToRunKey(ctx, tr, 0)
			assert.NoErr(t, err)
			ttr := &model.TaskToRun{
				Key:            toRunKey,
				QueueNumber:    datastore.NewIndexedOptional(int64(2)),
				Expiration:     datastore.NewIndexedOptional(now.Add(time.Hour)),
				RBEReservation: "reservation",
			}
			assert.NoErr(t, datastore.Put(ctx, trs, ttr))
			outcome, err := run(&CancelOp{TaskRequest: tr})
			assert.NoErr(t, err)
			assert.That(t, outcome, should.Match(&CancelOpOutcome{
				Canceled:   true,
				WasRunning: false,
			}))

			assert.NoErr(t, datastore.Get(ctx, trs, ttr))
			assert.Loosely(t, trs.Abandoned.Get(), should.Match(now))
			assert.Loosely(t, trs.Modified, should.Match(now))
			assert.Loosely(t, trs.State, should.Equal(apipb.TaskState_CANCELED))
			assert.Loosely(t, ttr.IsReapable(), should.BeFalse)
			assert.Loosely(t, tqt.Pending(tqt.CancelRBE), should.Match([]string{"rbe-instance/reservation"}))
			assert.Loosely(t, tqt.Pending(tqt.PubSubNotify), should.Match([]string{"65aba3a3e6b99310"}))
			val := globalStore.Get(ctx, metrics.TaskStatusChangeSchedulerLatency, []any{"test_pool", "spec_name", "User canceled", "wobblyeye"})
			assert.Loosely(t, val.(*distribution.Distribution).Sum(), should.Equal(float64(2*time.Hour.Milliseconds())))
			val = globalStore.Get(ctx, metrics.JobsCompleted, []any{"spec_name", "", "", "test_pool", "none", "success", "User canceled"})
			assert.Loosely(t, val, should.Equal(1))
			// Canceled task doesn't have duration.
			val = globalStore.Get(ctx, metrics.JobsDuration, []any{"spec_name", "", "", "test_pool", "none", "success"})
			assert.That(t, val, should.BeNil)
		})

		t.Run("cancel running task", func(t *ftt.Test) {
			trs.State = apipb.TaskState_RUNNING
			assert.NoErr(t, datastore.Put(ctx, trs))

			t.Run("don't kill running", func(t *ftt.Test) {
				outcome, err := run(&CancelOp{TaskRequest: tr})
				assert.NoErr(t, err)
				assert.That(t, outcome, should.Match(&CancelOpOutcome{
					Canceled:   false,
					WasRunning: true,
				}))
			})

			t.Run("wrong bot id", func(t *ftt.Test) {
				outcome, err := run(&CancelOp{TaskRequest: tr, BotID: "bot"})
				assert.NoErr(t, err)
				assert.That(t, outcome, should.Match(&CancelOpOutcome{
					Canceled:   false,
					WasRunning: true,
				}))
				assert.NoErr(t, datastore.Get(ctx, trs))
				assert.Loosely(t, trs.Modified, should.Match(now.Add(-time.Minute)))
			})

			t.Run("kill running", func(t *ftt.Test) {
				trr := &model.TaskRunResult{
					TaskResultCommon: model.TaskResultCommon{
						State: apipb.TaskState_RUNNING,
					},
					Key: model.TaskRunResultKey(ctx, reqKey),
				}
				assert.NoErr(t, datastore.Put(ctx, trr))

				outcome, err := run(&CancelOp{TaskRequest: tr, KillRunning: true})
				assert.NoErr(t, err)
				assert.That(t, outcome, should.Match(&CancelOpOutcome{
					Canceled:   true,
					WasRunning: true,
				}))

				assert.NoErr(t, datastore.Get(ctx, trr, trs))
				assert.Loosely(t, trs.Abandoned.Get(), should.Match(trr.Abandoned.Get()))
				assert.Loosely(t, trs.Abandoned.Get(), should.Match(now))
				assert.Loosely(t, trs.Modified, should.Match(trr.Modified))
				assert.Loosely(t, trs.Modified, should.Match(now))
				assert.Loosely(t, trr.Killing, should.BeTrue)
				assert.Loosely(t, trs.State, should.Equal(apipb.TaskState_RUNNING))
				assert.Loosely(t, tqt.Pending(tqt.CancelChildren), should.Match([]string{tID}))
			})

			t.Run("kill running does nothing if the task has started cancellation", func(t *ftt.Test) {
				previousModified := now.Add(-time.Minute)
				trr := &model.TaskRunResult{
					TaskResultCommon: model.TaskResultCommon{
						State:    apipb.TaskState_RUNNING,
						Modified: previousModified,
					},
					Key:     model.TaskRunResultKey(ctx, reqKey),
					Killing: true,
				}
				trs.Modified = previousModified
				assert.NoErr(t, datastore.Put(ctx, trr, trs))

				outcome, err := run(&CancelOp{TaskRequest: tr, KillRunning: true})
				assert.NoErr(t, err)
				assert.That(t, outcome, should.Match(&CancelOpOutcome{
					Canceled:   false,
					WasRunning: true,
				}))

				assert.NoErr(t, datastore.Get(ctx, trr, trs))
				assert.Loosely(t, trs.Modified, should.Match(trr.Modified))
				assert.Loosely(t, trs.Modified, should.Match(previousModified))
			})
		})
	})
}

func TestCancelChildren(t *testing.T) {
	t.Parallel()

	ftt.Run("TestCancelChildren", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		ctx, tqt := tqtasks.TestingContext(ctx)
		mgr := NewManager(tqt.Tasks, "swarming-proj", "cur-version", nil, false).(*managerImpl)

		pID := "65aba3a3e6b99310"
		pReqKey, err := model.TaskIDToRequestKey(ctx, pID)
		assert.NoErr(t, err)

		t.Run("task has no children", func(t *ftt.Test) {
			assert.NoErr(t, mgr.queryToCancel(ctx, 300, pID))
			assert.Loosely(t, tqt.Pending(tqt.BatchCancel), should.HaveLength(0))
		})

		t.Run("task has no active children to cancel", func(t *ftt.Test) {
			cID := "65aba3a3e6b99100"
			cReqKey, err := model.TaskIDToRequestKey(ctx, cID)
			assert.NoErr(t, err)
			childReq := &model.TaskRequest{
				Key:          cReqKey,
				ParentTaskID: datastore.NewIndexedNullable(model.RequestKeyToTaskID(pReqKey, model.AsRunResult)),
			}
			childRes := &model.TaskResultSummary{
				Key: model.TaskResultSummaryKey(ctx, cReqKey),
				TaskResultCommon: model.TaskResultCommon{
					State: apipb.TaskState_COMPLETED,
				},
			}
			assert.NoErr(t, datastore.Put(ctx, childReq, childRes))
			assert.NoErr(t, mgr.queryToCancel(ctx, 300, pID))
			assert.Loosely(t, tqt.Pending(tqt.BatchCancel), should.HaveLength(0))
		})

		t.Run("task has active children to cancel", func(t *ftt.Test) {
			cIDs := []string{"65aba3a3e6b99100", "65aba3a3e6b99200", "65aba3a3e6b99300", "65aba3a3e6b99400"}
			for i, cID := range cIDs {
				cReqKey, _ := model.TaskIDToRequestKey(ctx, cID)
				childReq := &model.TaskRequest{
					Key:          cReqKey,
					ParentTaskID: datastore.NewIndexedNullable(model.RequestKeyToTaskID(pReqKey, model.AsRunResult)),
				}
				childRes := &model.TaskResultSummary{
					Key: model.TaskResultSummaryKey(ctx, cReqKey),
					TaskResultCommon: model.TaskResultCommon{
						State: apipb.TaskState_RUNNING,
					},
				}
				if i == 1 {
					childRes.State = apipb.TaskState_COMPLETED
				}
				assert.NoErr(t, datastore.Put(ctx, childReq, childRes))
			}

			t.Run("one task for all children", func(t *ftt.Test) {
				assert.NoErr(t, mgr.queryToCancel(ctx, 300, pID))
				assert.Loosely(t, tqt.Pending(tqt.BatchCancel), should.Match([]string{
					`["65aba3a3e6b99100" "65aba3a3e6b99300" "65aba3a3e6b99400"], purpose: cancel children for 65aba3a3e6b99310 batch 0, retry # 0`,
				}))
			})

			t.Run("multiple tasks", func(t *ftt.Test) {
				assert.NoErr(t, mgr.queryToCancel(ctx, 2, pID))
				assert.Loosely(t, tqt.Pending(tqt.BatchCancel), should.Match([]string{
					`["65aba3a3e6b99300" "65aba3a3e6b99400"], purpose: cancel children for 65aba3a3e6b99310 batch 0, retry # 0`,
					`["65aba3a3e6b99100"], purpose: cancel children for 65aba3a3e6b99310 batch 1, retry # 0`,
				}))
			})
		})
	})
}

func TestBatchCancellation(t *testing.T) {
	t.Parallel()

	ftt.Run("TestBatchCancellation", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		now := time.Date(2024, time.January, 1, 2, 3, 4, 0, time.UTC)
		ctx, _ = testclock.UseTime(ctx, now)
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		ctx, tqt := tqtasks.TestingContext(ctx)
		mgr := NewManager(tqt.Tasks, "swarming-proj", "cur-version", nil, false).(*managerImpl)

		t.Run("no tasks to cancel", func(t *ftt.Test) {
			err := mgr.runBatchCancellation(ctx, 10, &taskspb.BatchCancelTask{})
			assert.Loosely(t, err, should.ErrLike("no tasks specified for cancellation"))
			assert.Loosely(t, tqt.Pending(tqt.PubSubNotify), should.HaveLength(0))
		})

		tID1, tID2 := "65aba3a3e6b99100", "65aba3a3e6b99200"
		reqKey1, _ := model.TaskIDToRequestKey(ctx, tID1)
		tr1, trs1 := generateEntities(ctx, reqKey1, apipb.TaskState_PENDING, now)
		reqKey2, _ := model.TaskIDToRequestKey(ctx, tID2)
		tr2, trs2 := generateEntities(ctx, reqKey2, apipb.TaskState_RUNNING, now)
		assert.NoErr(t, datastore.Put(ctx, tr1, tr2, trs1, trs2))

		t.Run("all fail for unknown tasks, no retry", func(t *ftt.Test) {
			assert.NoErr(t, mgr.runBatchCancellation(ctx, 10, &taskspb.BatchCancelTask{
				Tasks:       []string{tID1, tID2},
				KillRunning: true,
			}))
			// No new retry is enqueued.
			assert.Loosely(t, tqt.Pending(tqt.BatchCancel), should.HaveLength(0))
		})

		toRunKey, err := model.TaskRequestToToRunKey(ctx, tr1, 0)
		assert.NoErr(t, err)
		ttr := &model.TaskToRun{
			Key:            toRunKey,
			QueueNumber:    datastore.NewIndexedOptional(int64(2)),
			Expiration:     datastore.NewIndexedOptional(now.Add(time.Hour)),
			RBEReservation: "reservation",
		}
		trr := &model.TaskRunResult{
			TaskResultCommon: model.TaskResultCommon{
				State: apipb.TaskState_RUNNING,
			},
			Key: model.TaskRunResultKey(ctx, reqKey2),
		}
		assert.NoErr(t, datastore.Put(ctx, ttr, trr))

		t.Run("all failures are retriable", func(t *ftt.Test) {
			mgr.testingPostCancelTxn = func(tid string) error {
				return errors.Reason("boom").Err()
			}
			assert.NoErr(t, mgr.runBatchCancellation(ctx, 10, &taskspb.BatchCancelTask{
				Tasks:       []string{tID1, tID2},
				KillRunning: true,
				Purpose:     "CancelTasks request",
			}))
			// A new task is enqueued to retry both tasks.
			assert.Loosely(t, tqt.Pending(tqt.BatchCancel), should.Match([]string{
				`["65aba3a3e6b99100" "65aba3a3e6b99200"], purpose: CancelTasks request, retry # 1`,
			}))
		})

		t.Run("partial fail", func(t *ftt.Test) {
			mgr.testingPostCancelTxn = func(tid string) error {
				if tid == tID1 {
					return errors.Reason("boom").Err()
				}
				return nil
			}
			assert.NoErr(t, mgr.runBatchCancellation(ctx, 10, &taskspb.BatchCancelTask{
				Tasks:       []string{tID1, tID2},
				KillRunning: true,
				Purpose:     "CancelTasks request",
			}))
			// A new task is enqueued to retry t1 only.
			assert.Loosely(t, tqt.Pending(tqt.BatchCancel), should.Match([]string{
				`["65aba3a3e6b99100"], purpose: CancelTasks request, retry # 1`,
			}))
			// t2 is fine.
			assert.Loosely(t, tqt.Pending(tqt.CancelChildren), should.Match([]string{
				tID2,
			}))
			assert.NoErr(t, datastore.Get(ctx, trr, trs2))
			assert.Loosely(t, trs2.Modified, should.Match(trr.Modified))
			assert.Loosely(t, trs2.Modified, should.Match(now))
			assert.Loosely(t, trr.Killing, should.BeTrue)
		})

		t.Run("give up after sufficient retries", func(t *ftt.Test) {
			mgr.testingPostCancelTxn = func(tid string) error {
				if tid == tID1 {
					return errors.Reason("boom").Err()
				}
				return nil
			}
			assert.NoErr(t, mgr.runBatchCancellation(ctx, 10, &taskspb.BatchCancelTask{
				Tasks:       []string{tID1},
				KillRunning: true,
				Retries:     maxBatchCancellationRetries,
			}))
			// No new retry is enqueued.
			assert.Loosely(t, tqt.Pending(tqt.BatchCancel), should.HaveLength(0))
		})

		t.Run("success", func(t *ftt.Test) {
			assert.NoErr(t, mgr.runBatchCancellation(ctx, 10, &taskspb.BatchCancelTask{
				Tasks:       []string{tID1, tID2},
				KillRunning: true,
				Purpose:     "CancelTasks request",
			}))

			assert.Loosely(t, tqt.Pending(tqt.CancelRBE), should.Match([]string{
				"rbe-instance/reservation",
			}))

			res := tqt.Pending(tqt.PubSubNotify)
			sort.Strings(res)
			assert.Loosely(t, res, should.Match([]string{tID1, tID2}))
			assert.Loosely(t, tqt.Pending(tqt.CancelChildren), should.Match([]string{
				tID2,
			}))

			assert.NoErr(t, datastore.Get(ctx, ttr, trr, trs1, trs2))
			assert.Loosely(t, trs1.Modified, should.Match(now))
			assert.Loosely(t, trs1.State, should.Equal(apipb.TaskState_CANCELED))
			assert.Loosely(t, ttr.IsReapable(), should.BeFalse)
			assert.Loosely(t, trs2.Modified, should.Match(trr.Modified))
			assert.Loosely(t, trs2.Modified, should.Match(now))
			assert.Loosely(t, trr.Killing, should.BeTrue)
		})
	})
}
