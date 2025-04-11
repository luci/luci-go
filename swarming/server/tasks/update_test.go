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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tqtasks"
)

func TestUpdateOp(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		now := time.Date(2044, time.February, 3, 4, 5, 0, 0, time.UTC)
		ctx, _ = testclock.UseTime(ctx, now)
		ctx, tqt := tqtasks.TestingContext(ctx)

		taskID := "65aba3a3e6b99200"
		reqKey, err := model.TaskIDToRequestKey(ctx, taskID)
		assert.NoErr(t, err)

		mgr := NewManager(tqt.Tasks, "swarming-proj", "cur-version", nil, false)

		type entities struct {
			trs *model.TaskResultSummary
			trr *model.TaskRunResult
		}
		createTaskEntities := func(state apipb.TaskState) entities {
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
			assert.NoErr(t, datastore.Put(ctx, trs, trr))
			return entities{
				trs: trs,
				trr: trr,
			}
		}

		t.Run("task_already_complete", func(t *ftt.Test) {
			createTaskEntities(apipb.TaskState_BOT_DIED)
			op := &UpdateOp{
				RequestKey: reqKey,
				BotID:      "bot1",
			}
			outcome, err := mgr.UpdateTxn(ctx, op)
			assert.NoErr(t, err)
			assert.That(t, outcome.MustStop, should.BeTrue)
		})
	})
}
