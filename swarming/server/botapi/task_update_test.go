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

package botapi

import (
	"context"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/botinfo"
	"go.chromium.org/luci/swarming/server/botsrv"
	"go.chromium.org/luci/swarming/server/cfg/cfgtest"
	"go.chromium.org/luci/swarming/server/hmactoken"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tasks"
)

func TestProcessTaskUpdate(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	srv := BotAPIServer{}

	taskID := "65aba3a3e6b99201"
	reqKey, err := model.TaskIDToRequestKey(ctx, taskID)
	assert.NoErr(t, err)
	tr := &model.TaskRequest{
		Key: reqKey,
	}
	assert.NoErr(t, datastore.Put(ctx, tr))

	t.Run("Validation", func(t *testing.T) {
		negative := -1.0
		positive := 1.0
		var zero int64 = 0

		testCases := []struct {
			name        string
			req         *TaskUpdateRequest
			taskIDInBot string
			err         any
		}{
			{
				name: "no_task_id",
				req:  &TaskUpdateRequest{},
				err:  "task ID is required",
			},
			{
				name: "invalid_task_id",
				req: &TaskUpdateRequest{
					TaskID: "invalid",
				},
				taskIDInBot: "invalid",
				err:         `invalid task ID "invalid"`,
			},
			{
				name: "negative_duration",
				req: &TaskUpdateRequest{
					TaskID:   taskID,
					Duration: &negative,
				},
				err: `negative duration -1.000000`,
			},
			{
				name: "negative_cost",
				req: &TaskUpdateRequest{
					TaskID:  taskID,
					CostUSD: -1.0,
				},
				err: `negative cost -1.000000`,
			},
			{
				name: "duration_for_running_task",
				req: &TaskUpdateRequest{
					TaskID:   taskID,
					Duration: &positive,
				},
				err: `expected to have both duration and exit code or neither`,
			},
			{
				name: "missing_duration_for_success_task",
				req: &TaskUpdateRequest{
					TaskID:   taskID,
					ExitCode: &zero,
				},
				err: `expected to have both duration and exit code or neither`,
			},
			{
				name: "missing_duration_for_bot_overhead",
				req: &TaskUpdateRequest{
					TaskID:      taskID,
					BotOverhead: &positive,
				},
				err: `duration must be set when bot overhead is set`,
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				taskIDInBot := tc.taskIDInBot
				if taskIDInBot == "" {
					taskIDInBot = taskID
				}
				r := &botsrv.Request{
					CurrentTaskID: taskIDInBot,
				}
				_, err := validateTaskUpdateRequest(ctx, tc.req, r)
				assert.That(t, err, should.ErrLike(tc.err))
			})
		}
	})

	t.Run("update_wrong_task_id", func(t *testing.T) {
		req := &TaskUpdateRequest{
			TaskID: "wrong-task-id",
		}
		r := &botsrv.Request{
			CurrentTaskID: taskID,
		}
		res, err := srv.updateTask(ctx, req, r)
		assert.NoErr(t, err)
		assert.That(t, res, should.Match(&TaskUpdateResponse{MustStop: true, OK: false, StopReason: "the bot is not associated with this task on the server"}))
	})

	t.Run("complete_wrong_task_id", func(t *testing.T) {
		req := &TaskUpdateRequest{
			TaskID: "wrong-task-id",
		}
		r := &botsrv.Request{
			CurrentTaskID: taskID,
		}
		res, err := srv.completeTask(ctx, req, r)
		assert.NoErr(t, err)
		assert.That(t, res, should.Match(&TaskUpdateResponse{OK: false}))
	})

	t.Run("process_running_task", func(t *testing.T) {
		req := &TaskUpdateRequest{
			TaskID:           taskID,
			Output:           []byte("output"),
			OutputChunkStart: 3,
		}

		update, err := srv.processTaskUpdate(ctx, req, &botsrv.Request{
			CurrentTaskID: taskID,
			Session: &internalspb.Session{
				BotId:     "some-bot",
				SessionId: "session-id",
			},
		})
		assert.NoErr(t, err)
		assert.That(t, model.RequestKeyToTaskID(update.Request.Key, model.AsRunResult), should.Equal(taskID))
		assert.That(t, update.Output, should.Match([]byte("output")))
	})
	t.Run("process_completed_task", func(t *testing.T) {
		zerof := 0.0
		var zero int64 = 0

		reqKey, err := model.TaskIDToRequestKey(ctx, taskID)
		assert.NoErr(t, err)
		tr := &model.TaskRequest{
			Key: reqKey,
		}
		assert.NoErr(t, datastore.Put(ctx, tr))

		req := &TaskUpdateRequest{
			TaskID:      taskID,
			BotOverhead: &zerof,
			CacheTrimStats: model.OperationStats{
				DurationSecs: 2.0,
			},
			CIPDStats: model.OperationStats{
				DurationSecs: 0.0,
			},
			IsolatedStats: IsolatedStats{
				Download: model.CASOperationStats{
					DurationSecs: 1.0,
					InitialItems: 10,
					InitialSize:  10,
					ItemsCold:    []byte("cold"),
				},
				Upload: model.CASOperationStats{
					DurationSecs: 0.0,
					ItemsHot:     []byte("hot"),
				},
			},
			NamedCachesStats: NamedCachesStats{
				Install: model.OperationStats{
					DurationSecs: 0.1,
				},
				Uninstall: model.OperationStats{
					DurationSecs: 0.1,
				},
			},
			CleanupStats: model.OperationStats{
				DurationSecs: 0.1,
			},
			Duration: &zerof,
			ExitCode: &zero,
			CostUSD:  0.0,
			CASOutputRoot: model.CASReference{
				CASInstance: "instance",
				Digest: model.CASDigest{
					Hash:      "hash",
					SizeBytes: 3,
				},
			},
			CIPDPins: model.CIPDInput{
				ClientPackage: model.CIPDPackage{
					PackageName: "name",
					Version:     "version",
				},
				Packages: []model.CIPDPackage{{
					PackageName: "name",
					Version:     "version",
					Path:        "path",
				}},
			},
			Output:           []byte("output"),
			OutputChunkStart: 3,
		}

		perfStats := &model.PerformanceStats{
			BotOverheadSecs:      0,
			CacheTrim:            req.CacheTrimStats,
			PackageInstallation:  req.CIPDStats,
			NamedCachesInstall:   req.NamedCachesStats.Install,
			NamedCachesUninstall: req.NamedCachesStats.Uninstall,
			Cleanup:              req.CleanupStats,
			IsolatedDownload: model.CASOperationStats{
				DurationSecs: 1.0,
				InitialItems: 10,
				InitialSize:  10,
				ItemsCold:    []byte("cold"),
			},
			IsolatedUpload: model.CASOperationStats{
				DurationSecs: 0.0,
				ItemsHot:     []byte("hot"),
			},
		}

		complete, err := srv.processTaskCompletion(ctx, req, &botsrv.Request{
			CurrentTaskID: taskID,
			Session: &internalspb.Session{
				BotId:     "some-bot",
				SessionId: "session-id",
			},
		})
		assert.NoErr(t, err)
		assert.That(t, model.RequestKeyToTaskID(complete.Request.Key, model.AsRunResult), should.Equal(taskID))
		assert.That(t, complete.Output, should.Match([]byte("output")))
		assert.That(t, complete.PerformanceStats, should.Match(perfStats))
	})

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		now := time.Date(2044, time.February, 3, 4, 5, 0, 0, time.UTC)
		ctx := memory.Use(context.Background())
		ctx, _ = testclock.UseTime(ctx, now)
		ctx, _ = tsmon.WithDummyInMemory(ctx)

		secret := hmactoken.NewStaticSecret(secrets.Secret{
			Active: []byte("secret"),
		})

		srv := BotAPIServer{
			cfg:        cfgtest.MockConfigs(ctx, cfgtest.NewMockedConfigs()),
			hmacSecret: secret,
			version:    "server-ver",
		}

		botID := "bot-id"
		taskID := "65aba3a3e6b99200"
		runID := "65aba3a3e6b99201"
		reqKey, err := model.TaskIDToRequestKey(ctx, taskID)
		assert.NoErr(t, err)
		tr := &model.TaskRequest{
			Key: reqKey,
		}
		assert.NoErr(t, datastore.Put(ctx, tr))

		call := func(req *TaskUpdateRequest, botLastSeen time.Time) (*TaskUpdateResponse, error) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "bot:bot-id",
			})

			resp, err := srv.TaskUpdate(ctx, req, &botsrv.Request{
				Session: &internalspb.Session{
					BotId: botID,
					BotConfig: &internalspb.BotConfig{
						LogsCloudProject: "logs-cloud-project",
					},
					SessionId: "session-id",
				},
				CurrentTaskID: taskID,
				BotLastSeen:   botLastSeen,
			})
			if err != nil {
				return nil, err
			}
			return resp.(*TaskUpdateResponse), nil
		}

		t.Run("completeTask", func(t *ftt.Test) {
			reqKey, err := model.TaskIDToRequestKey(ctx, taskID)
			assert.NoErr(t, err)
			tr := &model.TaskRequest{
				Key: reqKey,
			}
			assert.NoErr(t, datastore.Put(ctx, tr))

			submit := func(outcome *tasks.CompleteTxnOutcome, err error, proceed bool) {
				srv.submitUpdate = func(ctx context.Context, u *botinfo.Update) error {
					return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						res, err := u.Prepare(ctx, &model.BotInfo{
							Key: model.BotInfoKey(ctx, u.BotID),
						})
						if res != nil {
							assert.That(t, res.Proceed, should.BeTrue)
							assert.That(t, res.EventType, should.Equal(outcome.BotEventType))

						}
						return err
					}, nil)

				}

				srv.tasksManager = &tasks.MockedManager{
					CompleteTxnMock: func(ctx context.Context, op *tasks.CompleteOp) (*tasks.CompleteTxnOutcome, error) {
						return outcome, err
					},
				}
			}
			t.Run("normal-completion", func(t *ftt.Test) {
				submit(&tasks.CompleteTxnOutcome{Updated: true, BotEventType: model.BotEventTaskCompleted}, nil, true)
				req := &TaskUpdateRequest{
					TaskID:      taskID,
					ExitCode:    int64Ptr(1),
					Duration:    float64Ptr(3600),
					BotOverhead: float64Ptr(100),
				}
				resp, err := call(req, now.Add(-time.Second))
				assert.NoErr(t, err)
				assert.That(t, resp.OK, should.BeTrue)
				assert.That(t, resp.MustStop, should.BeFalse)
			})
			t.Run("task-canceled", func(t *ftt.Test) {
				submit(&tasks.CompleteTxnOutcome{Updated: true, BotEventType: model.BotEventTaskCompleted}, nil, true)
				req := &TaskUpdateRequest{
					TaskID:   taskID,
					Canceled: true,
				}
				resp, err := call(req, now.Add(-time.Second))
				assert.NoErr(t, err)
				assert.That(t, resp.OK, should.BeTrue)
				assert.That(t, resp.MustStop, should.BeFalse)
			})

			t.Run("task-killed", func(t *ftt.Test) {
				submit(&tasks.CompleteTxnOutcome{Updated: true, BotEventType: model.BotEventTaskKilled}, nil, true)
				req := &TaskUpdateRequest{
					TaskID:      taskID,
					ExitCode:    int64Ptr(1),
					Duration:    float64Ptr(3600),
					BotOverhead: float64Ptr(100),
				}
				resp, err := call(req, now.Add(-time.Second))
				assert.NoErr(t, err)
				assert.That(t, resp.OK, should.BeTrue)
				assert.That(t, resp.MustStop, should.BeFalse)
			})

			t.Run("failed_to_complete", func(t *ftt.Test) {
				submit(nil, status.Errorf(codes.InvalidArgument, "task %q is not running by bot %q", runID, botID), false)
				req := &TaskUpdateRequest{
					TaskID:      taskID,
					ExitCode:    int64Ptr(1),
					Duration:    float64Ptr(3600),
					BotOverhead: float64Ptr(100),
				}
				_, err := call(req, now.Add(-time.Second))
				assert.That(t, err, should.ErrLike(`task "65aba3a3e6b99201" is not running by bot "bot-id"`))
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			})

			t.Run("retry_on_normal_completion", func(t *ftt.Test) {
				submit(&tasks.CompleteTxnOutcome{Updated: false}, nil, false)
				req := &TaskUpdateRequest{
					TaskID:      taskID,
					ExitCode:    int64Ptr(1),
					Duration:    float64Ptr(3600),
					BotOverhead: float64Ptr(100),
				}
				resp, err := call(req, now.Add(-time.Second))
				assert.NoErr(t, err)
				assert.That(t, resp.OK, should.BeTrue)
				assert.That(t, resp.MustStop, should.BeFalse)
			})
		})

		t.Run("updateTask", func(t *ftt.Test) {
			updateTask := func(outcome *tasks.UpdateTxnOutcome, err error) {
				srv.tasksManager = &tasks.MockedManager{
					UpdateTxnMock: func(ctx context.Context, op *tasks.UpdateOp) (*tasks.UpdateTxnOutcome, error) {
						return outcome, err
					},
				}
			}

			t.Run("fail_to_update_task", func(t *ftt.Test) {
				updateTask(nil, status.Errorf(codes.InvalidArgument, "task %q is not running by bot %q", runID, botID))
				req := &TaskUpdateRequest{
					TaskID: taskID,
				}
				_, err := call(req, now.Add(-time.Second))
				assert.That(t, err, should.ErrLike(`task "65aba3a3e6b99201" is not running by bot "bot-id"`))
				assert.That(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			})
			t.Run("OK_update_bot_last_seen", func(t *ftt.Test) {
				updateTask(&tasks.UpdateTxnOutcome{MustStop: false}, nil)
				srv.submitUpdate = func(ctx context.Context, u *botinfo.Update) error {
					return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						botInfo := &model.BotInfo{
							Key: model.BotInfoKey(ctx, u.BotID),
						}
						assert.NoErr(t, datastore.Get(ctx, botInfo))
						botInfo.LastSeen.Set(now)
						assert.NoErr(t, datastore.Put(ctx, botInfo))
						return nil
					}, nil)

				}

				lastSeen := now.Add(-time.Minute)
				botInfo := &model.BotInfo{
					Key: model.BotInfoKey(ctx, botID),
					BotCommon: model.BotCommon{
						LastSeen: datastore.NewUnindexedOptional(lastSeen),
					},
				}
				assert.NoErr(t, datastore.Put(ctx, botInfo))
				req := &TaskUpdateRequest{
					TaskID: taskID,
				}
				resp, err := call(req, lastSeen)
				assert.NoErr(t, err)
				assert.That(t, resp.OK, should.BeTrue)
				assert.That(t, resp.MustStop, should.BeFalse)

				assert.NoErr(t, datastore.Get(ctx, botInfo))
				assert.That(t, botInfo.LastSeen.Get(), should.Match(now))
			})

			t.Run("OK_skip_update_bot_last_seen", func(t *ftt.Test) {
				updateTask(&tasks.UpdateTxnOutcome{MustStop: true, StopReason: "Killing task"}, nil)
				// srv.submitUpdate is not called.
				lastSeen := now.Add(-10 * time.Second)
				botInfo := &model.BotInfo{
					Key: model.BotInfoKey(ctx, botID),
					BotCommon: model.BotCommon{
						LastSeen: datastore.NewUnindexedOptional(lastSeen),
					},
				}
				assert.NoErr(t, datastore.Put(ctx, botInfo))
				req := &TaskUpdateRequest{
					TaskID: taskID,
				}
				resp, err := call(req, lastSeen)
				assert.NoErr(t, err)
				assert.That(t, resp.OK, should.BeTrue)
				assert.That(t, resp.MustStop, should.BeTrue)
				assert.That(t, resp.StopReason, should.Equal("Killing task"))

				assert.NoErr(t, datastore.Get(ctx, botInfo))
				assert.That(t, botInfo.LastSeen.Get(), should.Match(lastSeen))
			})
		})
	})
}

func int64Ptr(i int64) *int64 { return &i }

func float64Ptr(f float64) *float64 { return &f }
