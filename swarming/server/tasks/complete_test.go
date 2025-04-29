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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/metrics"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/resultdb"
	"go.chromium.org/luci/swarming/server/tqtasks"
)

func TestCompleteOp(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		now := time.Date(2044, time.February, 3, 4, 5, 0, 0, time.UTC)
		ctx, _ = testclock.UseTime(ctx, now)
		ctx, tqt := tqtasks.TestingContext(ctx)
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		globalStore := tsmon.Store(ctx)

		taskID := "65aba3a3e6b99200"
		runID := "65aba3a3e6b99201"
		reqKey, err := model.TaskIDToRequestKey(ctx, taskID)
		assert.NoErr(t, err)

		mgr := NewManager(tqt.Tasks, "swarming-proj", "cur-version", nil, true).(*managerImpl)

		type entities struct {
			tr  *model.TaskRequest
			trs *model.TaskResultSummary
			trr *model.TaskRunResult
		}
		createTaskEntities := func(state apipb.TaskState) entities {
			tr := &model.TaskRequest{
				Key:                 reqKey,
				PubSubTopic:         "pubsub-topic",
				ResultDBUpdateToken: "token",
				Realm:               "project:realm",
			}
			trc := model.TaskResultCommon{
				State: state,
				ResultDBInfo: model.ResultDBInfo{
					Hostname:   "resultdb.example.com",
					Invocation: "invocations/task-65aba3a3e6b99201",
				},
				ServerVersions: []string{"v1"},
				Started:        datastore.NewIndexedNullable(now.Add(-time.Hour)),
			}

			trs := &model.TaskResultSummary{
				Key:              model.TaskResultSummaryKey(ctx, reqKey),
				TaskResultCommon: trc,
				RequestRealm:     "project:realm",
			}
			trs.ServerVersions = addServerVersion(trs.ServerVersions, "v2")

			trr := &model.TaskRunResult{
				Key:              model.TaskRunResultKey(ctx, reqKey),
				TaskResultCommon: trc,
				BotID:            "bot1",
				DeadAfter:        datastore.NewUnindexedOptional(now.Add(time.Hour)),
			}
			assert.NoErr(t, datastore.Put(ctx, tr, trs, trr))
			return entities{
				tr:  tr,
				trs: trs,
				trr: trr,
			}
		}

		t.Run("task_missing", func(t *ftt.Test) {
			op := &CompleteOp{
				Request: &model.TaskRequest{
					Key: reqKey,
				},
			}
			_, err := mgr.CompleteTxn(ctx, op)
			assert.That(t, err, should.ErrLike(`task "65aba3a3e6b99201" not found`))
			assert.That(t, err, grpccode.ShouldBe(codes.NotFound))
		})
		t.Run("task_get_update_from_wrong_bot", func(t *ftt.Test) {
			ents := createTaskEntities(apipb.TaskState_RUNNING)
			op := &CompleteOp{
				Request: ents.tr,
				BotID:   "bot2",
			}
			_, err := mgr.CompleteTxn(ctx, op)
			assert.That(t, err, should.ErrLike(`task "65aba3a3e6b99201" is not running by bot "bot2"`))
			assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("task_not_to_complete", func(t *ftt.Test) {
			ents := createTaskEntities(apipb.TaskState_RUNNING)
			op := &CompleteOp{
				Request: ents.tr,
				BotID:   "bot1",
			}
			_, err := mgr.CompleteTxn(ctx, op)
			assert.That(t, err, should.ErrLike(`expect to complete task "65aba3a3e6b99201", not only update`))
			assert.That(t, err, grpccode.ShouldBe(codes.Internal))
		})
		t.Run("task_already_complete", func(t *ftt.Test) {
			t.Run("retry_on_normal_completion", func(t *ftt.Test) {
				ents := createTaskEntities(apipb.TaskState_COMPLETED)
				var exitcode int64 = 1
				var duration float64 = 3600
				ents.trs.ExitCode.Set(exitcode)
				ents.trs.DurationSecs.Set(duration)
				assert.NoErr(t, datastore.Put(ctx, ents.trs))
				op := &CompleteOp{
					Request:  ents.tr,
					BotID:    "bot1",
					ExitCode: &exitcode,
					Duration: &duration,
				}
				outcome, err := mgr.CompleteTxn(ctx, op)
				assert.NoErr(t, err)
				assert.That(t, outcome.Updated, should.BeFalse)
			})
			t.Run("retry_on_abnormal_completion", func(t *ftt.Test) {
				ents := createTaskEntities(apipb.TaskState_TIMED_OUT)
				var exitcode int64 = -1
				var duration float64 = 3600
				ents.trs.ExitCode.Set(exitcode)
				ents.trs.DurationSecs.Set(duration)
				assert.NoErr(t, datastore.Put(ctx, ents.trs))
				op := &CompleteOp{
					Request:     ents.tr,
					BotID:       "bot1",
					HardTimeout: true,
				}
				outcome, err := mgr.CompleteTxn(ctx, op)
				assert.NoErr(t, err)
				assert.That(t, outcome.Updated, should.BeFalse)
			})
			t.Run("different_exit_code", func(t *ftt.Test) {
				ents := createTaskEntities(apipb.TaskState_COMPLETED)
				var exitcode int64 = 1
				var duration float64 = 3600
				var anotherExitCode int64 = -1
				ents.trs.ExitCode.Set(exitcode)
				ents.trs.DurationSecs.Set(duration)
				assert.NoErr(t, datastore.Put(ctx, ents.trs))
				op := &CompleteOp{
					Request:  ents.tr,
					BotID:    "bot1",
					ExitCode: &anotherExitCode,
					Duration: &duration,
				}
				res, err := mgr.CompleteTxn(ctx, op)
				assert.That(t, res.Updated, should.BeFalse)
				assert.NoErr(t, err)
			})

			t.Run("different_end_state", func(t *ftt.Test) {
				ents := createTaskEntities(apipb.TaskState_COMPLETED)
				var exitcode int64 = 1
				var duration float64 = 3600
				ents.trs.ExitCode.Set(exitcode)
				ents.trs.DurationSecs.Set(duration)
				assert.NoErr(t, datastore.Put(ctx, ents.trs))
				op := &CompleteOp{
					Request:     ents.tr,
					BotID:       "bot1",
					HardTimeout: true,
				}
				res, err := mgr.CompleteTxn(ctx, op)
				assert.That(t, res.Updated, should.BeFalse)
				assert.NoErr(t, err)
			})
		})

		run := func(op *CompleteOp) (*CompleteTxnOutcome, error) {
			var outcome *CompleteTxnOutcome
			err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				var err error
				outcome, err = mgr.CompleteTxn(ctx, op)
				return err
			}, nil)
			return outcome, err
		}

		t.Run("OK-complete", func(t *ftt.Test) {
			ents := createTaskEntities(apipb.TaskState_RUNNING)
			var exitcode int64 = 1
			var duration float64 = 3600
			perfStats := &model.PerformanceStats{
				BotOverheadSecs: 100,
				CacheTrim: model.OperationStats{
					DurationSecs: 10,
				},
				PackageInstallation: model.OperationStats{
					DurationSecs: 20,
				},
				NamedCachesInstall: model.OperationStats{
					DurationSecs: 30,
				},
				NamedCachesUninstall: model.OperationStats{
					DurationSecs: 40,
				},
				Cleanup: model.OperationStats{
					DurationSecs: 50,
				},
			}
			casOutput := model.CASReference{
				CASInstance: "instance",
				Digest: model.CASDigest{
					Hash:      "hash",
					SizeBytes: 3,
				},
			}
			cipdPins := model.CIPDInput{
				ClientPackage: model.CIPDPackage{
					PackageName: "name",
					Version:     "version",
				},
				Packages: []model.CIPDPackage{{
					PackageName: "name",
					Version:     "version",
					Path:        "path",
				}},
			}
			op := &CompleteOp{
				Request:          ents.tr,
				BotID:            "bot1",
				ExitCode:         &exitcode,
				Duration:         &duration,
				PerformanceStats: perfStats,
				CostUSD:          0.35,
				CASOutputRoot:    casOutput,
				CIPDPins:         cipdPins,
			}
			outcome, err := run(op)
			assert.NoErr(t, err)
			assert.That(t, outcome.Updated, should.BeTrue)
			assert.That(t, outcome.BotEventType, should.Equal(model.BotEventTaskCompleted))

			trs := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, reqKey)}
			assert.NoErr(t, datastore.Get(ctx, trs))
			assert.That(t, trs.ExitCode.Get(), should.Equal(exitcode))
			assert.That(t, trs.DurationSecs.Get(), should.Equal(duration))
			assert.That(t, trs.State, should.Equal(apipb.TaskState_COMPLETED))
			assert.That(t, trs.Failure, should.BeTrue)
			assert.That(t, trs.Modified, should.Match(now))
			assert.That(t, trs.Completed.Get(), should.Match(now))
			assert.That(t, trs.CASOutputRoot, should.Match(casOutput))
			assert.That(t, trs.CIPDPins, should.Match(cipdPins))
			assert.That(t, trs.CostUSD, should.Equal(0.35))
			assert.That(t, trs.ServerVersions, should.Match(
				[]string{"cur-version", "v1", "v2"}))

			trr := &model.TaskRunResult{
				Key: model.TaskRunResultKey(ctx, reqKey),
			}
			savedPerfStats := &model.PerformanceStats{
				Key: model.PerformanceStatsKey(ctx, reqKey),
			}
			assert.NoErr(t, datastore.Get(ctx, trr, savedPerfStats))

			assert.That(t, trr.ServerVersions, should.Match(
				[]string{"cur-version", "v1"}))
			assert.That(t, trr.DeadAfter.IsSet(), should.BeFalse)

			saved, err := savedPerfStats.ToProto()
			assert.NoErr(t, err)
			expected, err := perfStats.ToProto()
			assert.NoErr(t, err)
			assert.That(t, saved, should.Match(expected))

			assert.That(t, tqt.Pending(tqt.PubSubNotify), should.Match([]string{taskID}))
			assert.That(t, tqt.Pending(tqt.FinalizeTask), should.Match([]string{runID}))

			val := globalStore.Get(ctx, metrics.JobsCompleted, []any{"", "", "", "", "none", "failure", "Completed"})
			assert.Loosely(t, val, should.Equal(1))

			dur := globalStore.Get(ctx, metrics.JobsDuration, []any{"", "", "", "", "none", "failure"})
			assert.Loosely(t, dur.(*distribution.Distribution).Sum(), should.Equal(duration))
		})

		t.Run("OK-complete-build-task", func(t *ftt.Test) {
			ents := createTaskEntities(apipb.TaskState_RUNNING)
			ents.tr.HasBuildTask = true
			assert.NoErr(t, datastore.Put(ctx, ents.tr))

			var exitcode int64 = 0
			var duration float64 = 3600

			op := &CompleteOp{
				Request:  ents.tr,
				BotID:    "bot1",
				ExitCode: &exitcode,
				Duration: &duration,
			}
			outcome, err := run(op)
			assert.NoErr(t, err)
			assert.That(t, outcome.Updated, should.BeTrue)
			assert.That(t, outcome.BotEventType, should.Equal(model.BotEventTaskCompleted))

			trs := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, reqKey)}
			assert.NoErr(t, datastore.Get(ctx, trs))
			assert.That(t, trs.ExitCode.Get(), should.Equal(exitcode))
			assert.That(t, trs.DurationSecs.Get(), should.Equal(duration))
			assert.That(t, trs.State, should.Equal(apipb.TaskState_COMPLETED))
			assert.That(t, trs.Failure, should.BeFalse)
			assert.That(t, trs.Modified, should.Match(now))
			assert.That(t, trs.Completed.Get(), should.Match(now))

			assert.That(t, tqt.Pending(tqt.PubSubNotify), should.Match([]string{taskID}))
			assert.That(t, tqt.Pending(tqt.BuildbucketNotify), should.Match([]string{taskID}))
			assert.That(t, tqt.Pending(tqt.FinalizeTask), should.Match([]string{runID}))
		})

		t.Run("OK-timeout", func(t *ftt.Test) {
			ents := createTaskEntities(apipb.TaskState_RUNNING)

			op := &CompleteOp{
				Request:     ents.tr,
				BotID:       "bot1",
				HardTimeout: true,
			}
			outcome, err := run(op)
			assert.NoErr(t, err)
			assert.That(t, outcome.Updated, should.BeTrue)
			assert.That(t, outcome.BotEventType, should.Equal(model.BotEventTaskCompleted))

			trs := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, reqKey)}
			assert.NoErr(t, datastore.Get(ctx, trs))
			assert.That(t, trs.ExitCode.Get(), should.Equal(int64(-1)))
			assert.That(t, trs.DurationSecs.Get(), should.Equal(now.Sub(ents.trr.Started.Get()).Seconds()))
			assert.That(t, trs.State, should.Equal(apipb.TaskState_TIMED_OUT))
			assert.That(t, trs.Failure, should.BeTrue)
		})

		t.Run("OK-killing", func(t *ftt.Test) {
			t.Run("Canceled", func(t *ftt.Test) {
				ents := createTaskEntities(apipb.TaskState_RUNNING)
				ents.trr.Killing = true
				assert.NoErr(t, datastore.Put(ctx, ents.trr))
				op := &CompleteOp{
					Request:  ents.tr,
					BotID:    "bot1",
					Canceled: true,
				}
				outcome, err := run(op)
				assert.NoErr(t, err)
				assert.That(t, outcome.Updated, should.BeTrue)
				assert.That(t, outcome.BotEventType, should.Equal(model.BotEventTaskCompleted))

				trs := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, reqKey)}
				assert.NoErr(t, datastore.Get(ctx, trs))
				assert.That(t, trs.ExitCode.IsSet(), should.BeFalse)
				assert.That(t, trs.DurationSecs.IsSet(), should.BeFalse)
				assert.That(t, trs.State, should.Equal(apipb.TaskState_CANCELED))

				trr := ents.trr
				assert.NoErr(t, datastore.Get(ctx, trr))
				assert.That(t, trr.Killing, should.BeFalse)

			})

			t.Run("killed", func(t *ftt.Test) {
				ents := createTaskEntities(apipb.TaskState_RUNNING)
				ents.trr.Killing = true
				assert.NoErr(t, datastore.Put(ctx, ents.trr))
				var exitcode int64 = -9
				var duration float64 = 3600
				op := &CompleteOp{
					Request:  ents.tr,
					BotID:    "bot1",
					ExitCode: &exitcode,
					Duration: &duration,
				}
				outcome, err := run(op)
				assert.NoErr(t, err)
				assert.That(t, outcome.Updated, should.BeTrue)
				assert.That(t, outcome.BotEventType, should.Equal(model.BotEventTaskKilled))

				trs := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, reqKey)}
				assert.NoErr(t, datastore.Get(ctx, trs))
				assert.That(t, trs.ExitCode.Get(), should.Equal(exitcode))
				assert.That(t, trs.DurationSecs.Get(), should.Equal(duration))
				assert.That(t, trs.State, should.Equal(apipb.TaskState_KILLED))
				assert.That(t, trs.Failure, should.BeTrue)

				trr := ents.trr
				assert.NoErr(t, datastore.Get(ctx, trr))
				assert.That(t, trr.Killing, should.BeFalse)
			})

			t.Run("timedout_during_cancellation", func(t *ftt.Test) {
				ents := createTaskEntities(apipb.TaskState_RUNNING)
				ents.trr.Killing = true
				assert.NoErr(t, datastore.Put(ctx, ents.trr))
				op := &CompleteOp{
					Request:   ents.tr,
					BotID:     "bot1",
					IOTimeout: true,
				}
				outcome, err := run(op)
				assert.NoErr(t, err)
				assert.That(t, outcome.Updated, should.BeTrue)
				assert.That(t, outcome.BotEventType, should.Equal(model.BotEventTaskCompleted))

				trs := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, reqKey)}
				assert.NoErr(t, datastore.Get(ctx, trs))
				assert.That(t, trs.ExitCode.Get(), should.Equal(int64(-1)))
				assert.That(t, trs.DurationSecs.Get(), should.Equal(now.Sub(ents.trr.Started.Get()).Seconds()))
				assert.That(t, trs.State, should.Equal(apipb.TaskState_TIMED_OUT))
				assert.That(t, trs.Failure, should.BeTrue)

				trr := ents.trr
				assert.NoErr(t, datastore.Get(ctx, trr))
				assert.That(t, trr.Killing, should.BeFalse)
			})
		})

		t.Run("abandon", func(t *ftt.Test) {
			t.Run("normal_abandon", func(t *ftt.Test) {
				ents := createTaskEntities(apipb.TaskState_RUNNING)
				assert.NoErr(t, datastore.Put(ctx, ents.trr))
				op := &CompleteOp{
					Request:   ents.tr,
					BotID:     "bot1",
					Abandoned: true,
				}
				outcome, err := run(op)
				assert.NoErr(t, err)
				assert.That(t, outcome.Updated, should.BeTrue)
				assert.Loosely(t, outcome.BotEventType, should.Equal(""))

				trs := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, reqKey)}
				assert.NoErr(t, datastore.Get(ctx, trs))
				assert.That(t, trs.ExitCode.Get(), should.Equal(int64(-1)))
				assert.That(t, trs.DurationSecs.Get(), should.Equal(now.Sub(ents.trr.Started.Get()).Seconds()))
				assert.That(t, trs.State, should.Equal(apipb.TaskState_BOT_DIED))
				assert.That(t, trs.InternalFailure, should.BeTrue)
				assert.That(t, trs.Abandoned.Get(), should.Match(now))
				assert.That(t, tqt.Pending(tqt.PubSubNotify), should.Match([]string{taskID}))
				assert.That(t, tqt.Pending(tqt.FinalizeTask), should.Match([]string{runID}))
			})

			t.Run("abandon_during_cancellation", func(t *ftt.Test) {
				ents := createTaskEntities(apipb.TaskState_RUNNING)
				ents.trr.Killing = true
				ents.trr.Abandoned = datastore.NewIndexedNullable(now.Add(-time.Minute))
				ents.trs.Abandoned = datastore.NewIndexedNullable(now.Add(-time.Minute))
				assert.NoErr(t, datastore.Put(ctx, ents.trr, ents.trs))
				op := &CompleteOp{
					Request:   ents.tr,
					BotID:     "bot1",
					Abandoned: true,
				}
				outcome, err := run(op)
				assert.NoErr(t, err)
				assert.That(t, outcome.Updated, should.BeTrue)
				assert.Loosely(t, outcome.BotEventType, should.Equal(""))

				trs := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, reqKey)}
				assert.NoErr(t, datastore.Get(ctx, trs))
				assert.That(t, trs.ExitCode.Get(), should.Equal(int64(-1)))
				assert.That(t, trs.DurationSecs.Get(), should.Equal(now.Sub(ents.trr.Started.Get()).Seconds()))
				assert.That(t, trs.State, should.Equal(apipb.TaskState_KILLED))
				assert.That(t, trs.InternalFailure, should.BeTrue)
				assert.That(t, trs.Abandoned.Get(), should.Match(now.Add(-time.Minute)))
				assert.That(t, tqt.Pending(tqt.PubSubNotify), should.Match([]string{taskID}))
				assert.That(t, tqt.Pending(tqt.FinalizeTask), should.Match([]string{runID}))
			})

			t.Run("skip_abandon_since_not_allowed", func(t *ftt.Test) {
				mgr.allowAbandoningTasks = false
				op := &CompleteOp{
					Request:   nil,
					BotID:     "bot1",
					Abandoned: true,
				}
				outcome, err := run(op)
				assert.NoErr(t, err)
				assert.That(t, outcome.Updated, should.BeFalse)
			})
		})

		t.Run("task_error", func(t *ftt.Test) {
			t.Run("client_error", func(t *ftt.Test) {
				ents := createTaskEntities(apipb.TaskState_RUNNING)
				assert.NoErr(t, datastore.Put(ctx, ents.trr))
				op := &CompleteOp{
					Request: ents.tr,
					BotID:   "bot1",
					ClientError: &ClientError{
						MissingCAS: []model.CASReference{
							{
								CASInstance: "instance",
								Digest: model.CASDigest{
									Hash:      "hash",
									SizeBytes: int64(3),
								},
							},
						},
						MissingCIPD: []model.CIPDPackage{
							{
								PackageName: "package",
								Version:     "version",
								Path:        "path",
							},
						},
					},
				}
				outcome, err := run(op)
				assert.NoErr(t, err)
				assert.That(t, outcome.Updated, should.BeTrue)
				assert.Loosely(t, outcome.BotEventType, should.Equal(""))

				trs := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, reqKey)}
				assert.NoErr(t, datastore.Get(ctx, trs))
				assert.That(t, trs.ExitCode.Get(), should.Equal(int64(-1)))
				assert.That(t, trs.DurationSecs.Get(), should.Equal(now.Sub(ents.trr.Started.Get()).Seconds()))
				assert.That(t, trs.State, should.Equal(apipb.TaskState_CLIENT_ERROR))
				assert.That(t, trs.InternalFailure, should.BeTrue)
				assert.That(t, trs.Abandoned.Get(), should.Match(now))
				assert.That(t, trs.MissingCAS, should.Match(op.ClientError.MissingCAS))
				assert.That(t, trs.MissingCIPD, should.Match(op.ClientError.MissingCIPD))
				assert.That(t, tqt.Pending(tqt.PubSubNotify), should.Match([]string{taskID}))
				assert.That(t, tqt.Pending(tqt.FinalizeTask), should.Match([]string{runID}))
			})

			t.Run("task_error", func(t *ftt.Test) {
				ents := createTaskEntities(apipb.TaskState_RUNNING)
				assert.NoErr(t, datastore.Put(ctx, ents.trr))
				op := &CompleteOp{
					Request:   ents.tr,
					BotID:     "bot1",
					TaskError: true,
				}
				outcome, err := run(op)
				assert.NoErr(t, err)
				assert.That(t, outcome.Updated, should.BeTrue)
				assert.Loosely(t, outcome.BotEventType, should.Equal(""))

				trs := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, reqKey)}
				assert.NoErr(t, datastore.Get(ctx, trs))
				assert.That(t, trs.ExitCode.Get(), should.Equal(int64(-1)))
				assert.That(t, trs.DurationSecs.Get(), should.Equal(now.Sub(ents.trr.Started.Get()).Seconds()))
				assert.That(t, trs.State, should.Equal(apipb.TaskState_BOT_DIED))
				assert.That(t, trs.InternalFailure, should.BeTrue)
				assert.That(t, trs.Abandoned.Get(), should.Match(now))
				assert.That(t, tqt.Pending(tqt.PubSubNotify), should.Match([]string{taskID}))
				assert.That(t, tqt.Pending(tqt.FinalizeTask), should.Match([]string{runID}))
			})
		})
	})
}

func TestFinalizeInvocation(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx, tqt := tqtasks.TestingContext(ctx)

		taskID := "65aba3a3e6b99200"
		reqKey, err := model.TaskIDToRequestKey(ctx, taskID)
		assert.NoErr(t, err)

		token := "token for 65aba3a3e6b99201"

		mgr := NewManager(tqt.Tasks, "swarming-proj", "cur-version", nil, false).(*managerImpl)

		t.Run("task_not_have_invocation", func(t *ftt.Test) {
			tr := &model.TaskRequest{
				Key: reqKey,
			}
			assert.NoErr(t, datastore.Put(ctx, tr))
			assert.NoErr(t, mgr.finalizeResultDBInvocation(ctx, taskID))
		})

		t.Run("request_and_result_mismatch", func(t *ftt.Test) {
			tr := &model.TaskRequest{
				Key:                 reqKey,
				ResultDBUpdateToken: token,
			}
			trs := &model.TaskResultSummary{
				Key: model.TaskResultSummaryKey(ctx, reqKey),
			}
			assert.NoErr(t, datastore.Put(ctx, tr, trs))
			assert.That(t, mgr.finalizeResultDBInvocation(ctx, taskID), should.ErrLike(`task result "65aba3a3e6b99200" misses resultdb info`))
		})

		t.Run("OK", func(t *ftt.Test) {
			tr := &model.TaskRequest{
				Key:                 reqKey,
				ResultDBUpdateToken: token,
			}
			invName := "invocations/task-example.appspot.com-65aba3a3e6b99201"

			trs := &model.TaskResultSummary{
				Key: model.TaskResultSummaryKey(ctx, reqKey),
				TaskResultCommon: model.TaskResultCommon{
					ResultDBInfo: model.ResultDBInfo{
						Hostname:   "host",
						Invocation: invName,
					},
				},
				RequestRealm: "project:realm",
			}
			assert.NoErr(t, datastore.Put(ctx, tr, trs))

			inv := &rdbpb.Invocation{
				Name: invName,
			}
			rdb := resultdb.NewMockRecorderClientFactory(nil, inv, nil, token)
			mgr.rdb = rdb
			assert.NoErr(t, mgr.finalizeResultDBInvocation(ctx, taskID))
		})
	})
}
