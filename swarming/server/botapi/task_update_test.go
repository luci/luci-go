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

	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"

	internalspb "go.chromium.org/luci/swarming/proto/internals"
	"go.chromium.org/luci/swarming/server/botsrv"
	"go.chromium.org/luci/swarming/server/model"
)

func TestProcessTaskUpdate(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	srv := BotAPIServer{}

	taskID := "65aba3a3e6b99201"

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
				name: "wrong_task_id",
				req: &TaskUpdateRequest{
					TaskID: "65aba3a3e6b99101",
				},
				err: `wrong task ID "65aba3a3e6b99101": the bot is not executing this task`,
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
				name: "missing_duration_for_performance_stats",
				req: &TaskUpdateRequest{
					TaskID:      taskID,
					BotOverhead: &positive,
				},
				err: `expected to have both duration and bot overhead or neither`,
			},
			{
				name: "missing_performance_stats_for_duration",
				req: &TaskUpdateRequest{
					TaskID:   taskID,
					Duration: &positive,
					ExitCode: &zero,
				},
				err: `expected to have both duration and bot overhead or neither`,
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
				_, err := srv.processTaskUpdate(ctx, tc.req, r)
				assert.That(t, err, should.ErrLike(tc.err))
			})
		}
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
		assert.That(t, model.RequestKeyToTaskID(update.RequestKey, model.AsRunResult), should.Equal(taskID))
		assert.That(t, update.Output, should.Match([]byte("output")))
		assert.Loosely(t, update.PerformanceStats, should.BeNil)
	})
	t.Run("process_completed_task", func(t *testing.T) {
		zerof := 0.0
		var zero int64 = 0

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

		update, err := srv.processTaskUpdate(ctx, req, &botsrv.Request{
			CurrentTaskID: taskID,
			Session: &internalspb.Session{
				BotId:     "some-bot",
				SessionId: "session-id",
			},
		})
		assert.NoErr(t, err)
		assert.That(t, model.RequestKeyToTaskID(update.RequestKey, model.AsRunResult), should.Equal(taskID))
		assert.That(t, update.Output, should.Match([]byte("output")))
		assert.That(t, update.PerformanceStats, should.Match(perfStats))
	})
}
