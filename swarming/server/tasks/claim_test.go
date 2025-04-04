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

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tqtasks"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(
		datastore.Nullable[int64, datastore.Indexed]{},
		datastore.Nullable[string, datastore.Indexed]{},
		datastore.Nullable[time.Time, datastore.Indexed]{},
		datastore.Optional[[]byte, datastore.Indexed]{},
		datastore.Optional[float64, datastore.Unindexed]{},
		datastore.Optional[int64, datastore.Indexed]{},
		datastore.Optional[int64, datastore.Unindexed]{},
		datastore.Optional[string, datastore.Unindexed]{},
		datastore.Optional[time.Time, datastore.Indexed]{},
		datastore.Optional[time.Time, datastore.Unindexed]{},
	))
}

func TestClaimOp(t *testing.T) {
	t.Parallel()

	var (
		testTime   = time.Date(2044, time.February, 3, 4, 5, 0, 0, time.UTC)
		createTime = testTime.Add(-20 * time.Minute)
	)

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx, _ = testclock.UseTime(ctx, testTime)
		ctx, tqt := tqtasks.TestingContext(ctx)

		mgr := NewManager(tqt.Tasks, "swarming-proj", "cur-version", nil, false)

		taskID := "65aba3a3e6b99310"
		reqKey, err := model.TaskIDToRequestKey(ctx, taskID)
		assert.NoErr(t, err)

		dims := model.TaskDimensions{"pool": {"some-pool"}, "os": {"Linux"}}

		req := &model.TaskRequest{
			Key:                  reqKey,
			Created:              createTime,
			Tags:                 []string{"k1:v1", "k2:v2"},
			Name:                 "task-name",
			User:                 "some-user",
			PubSubTopic:          "pubsub-topic",
			HasBuildTask:         true,
			BotPingToleranceSecs: 120,
			TaskSlices: []model.TaskSlice{
				{
					Properties: model.TaskProperties{Dimensions: dims},
				},
				{
					Properties: model.TaskProperties{Dimensions: dims},
				},
			},
		}
		trs := &model.TaskResultSummary{
			Key:     model.TaskResultSummaryKey(ctx, reqKey),
			Created: createTime,
			Tags:    req.Tags,
			TaskResultCommon: model.TaskResultCommon{
				State:          apipb.TaskState_PENDING,
				Modified:       createTime,
				ServerVersions: []string{"prev-version"},
				ResultDBInfo: model.ResultDBInfo{
					Hostname:   "hostname",
					Invocation: "inv",
				},
			},
		}
		assert.NoErr(t, datastore.Put(ctx, req, trs))

		createTTR := func(claimID *string) *datastore.Key {
			var exp datastore.Optional[time.Time, datastore.Indexed]
			var clID datastore.Optional[string, datastore.Unindexed]
			if claimID == nil {
				exp.Set(testTime.Add(time.Hour))
			} else {
				clID.Set(*claimID)
			}
			ttr := &model.TaskToRun{
				Key:        model.TaskToRunKey(ctx, reqKey, 1, model.TaskToRunID(1)),
				Created:    createTime,
				Dimensions: dims,
				Expiration: exp,
				ClaimID:    clID,
			}
			assert.NoErr(t, datastore.Put(ctx, ttr))
			return ttr.Key
		}

		claimOp := &ClaimOp{
			Request: req,
			ClaimID: "bot:claim-id",
			BotDimensions: map[string][]string{
				"id":   {"bot-id"},
				"pool": {"some-pool"},
				"os":   {"Linux", "Ubuntu"},
			},
			BotVersion:          "bot-version",
			BotLogsCloudProject: "bot-logs",
			BotIdleSince:        testTime.Add(-time.Hour),
		}

		run := func(ttrKey *datastore.Key) *ClaimOpOutcome {
			var outcome *ClaimOpOutcome
			err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				op := *claimOp
				op.TaskToRunKey = ttrKey
				var err error
				outcome, err = mgr.ClaimTxn(ctx, &op)
				return err
			}, nil)
			assert.NoErr(t, err)
			return outcome
		}

		t.Run("Success", func(t *ftt.Test) {
			ttrKey := createTTR(nil)
			outcome := run(ttrKey)
			assert.That(t, outcome.Claimed, should.BeTrue)
			assert.That(t, tqt.Pending(tqt.PubSubNotify), should.Match([]string{taskID}))
			assert.That(t, tqt.Pending(tqt.BuildbucketNotify), should.Match([]string{taskID}))

			trr := &model.TaskRunResult{Key: model.TaskRunResultKey(ctx, reqKey)}
			assert.NoErr(t, datastore.Get(ctx, trr))
			assert.That(t, trr, should.Match(&model.TaskRunResult{
				Key:            trr.Key,
				RequestCreated: createTime,
				RequestTags:    []string{"k1:v1", "k2:v2"},
				RequestName:    "task-name",
				RequestUser:    "some-user",
				BotID:          "bot-id",
				DeadAfter:      datastore.NewUnindexedOptional(testTime.Add(120 * time.Second)),
				TaskResultCommon: model.TaskResultCommon{
					State:               apipb.TaskState_RUNNING,
					Modified:            testTime,
					BotVersion:          claimOp.BotVersion,
					BotDimensions:       claimOp.BotDimensions,
					BotIdleSince:        datastore.NewUnindexedOptional(claimOp.BotIdleSince),
					BotLogsCloudProject: claimOp.BotLogsCloudProject,
					ServerVersions:      []string{"cur-version"},
					CurrentTaskSlice:    1,
					Started:             datastore.NewIndexedNullable(testTime),
					ResultDBInfo: model.ResultDBInfo{
						Hostname:   "hostname",
						Invocation: "inv",
					},
				},
			}))

			updatedTRS := &model.TaskResultSummary{Key: trs.Key}
			assert.NoErr(t, datastore.Get(ctx, updatedTRS))
			assert.That(t, updatedTRS, should.Match(&model.TaskResultSummary{
				Key:       updatedTRS.Key,
				Created:   trs.Created,
				Tags:      trs.Tags,
				BotID:     datastore.NewUnindexedOptional("bot-id"),
				TryNumber: datastore.NewIndexedNullable(int64(1)),
				TaskResultCommon: model.TaskResultCommon{
					State:               apipb.TaskState_RUNNING,
					Modified:            testTime,
					BotVersion:          claimOp.BotVersion,
					BotDimensions:       claimOp.BotDimensions,
					BotIdleSince:        datastore.NewUnindexedOptional(claimOp.BotIdleSince),
					BotLogsCloudProject: claimOp.BotLogsCloudProject,
					ServerVersions:      []string{"cur-version", "prev-version"},
					CurrentTaskSlice:    1,
					Started:             datastore.NewIndexedNullable(testTime),
					ResultDBInfo: model.ResultDBInfo{
						Hostname:   "hostname",
						Invocation: "inv",
					},
				},
			}))

			updatedTTR := &model.TaskToRun{Key: ttrKey}
			assert.NoErr(t, datastore.Get(ctx, updatedTTR))
			assert.That(t, updatedTTR, should.Match(&model.TaskToRun{
				Key:        ttrKey,
				Created:    createTime,
				Dimensions: dims,
				ClaimID:    datastore.NewUnindexedOptional("bot:claim-id"),
			}))
		})

		t.Run("Missing task", func(t *ftt.Test) {
			// Note: skip calling createTTR, we want it missing.
			outcome := run(model.TaskToRunKey(ctx, reqKey, 1, model.TaskToRunID(1)))
			assert.That(t, outcome.Claimed, should.BeFalse)
			assert.That(t, outcome.Unavailable, should.Equal("No such task"))
			assert.That(t, tqt.Pending(tqt.PubSubNotify), should.Match([]string(nil)))
			assert.That(t, tqt.Pending(tqt.BuildbucketNotify), should.Match([]string(nil)))
		})

		t.Run("Expired TTR", func(t *ftt.Test) {
			emptyClaimID := ""
			outcome := run(createTTR(&emptyClaimID))
			assert.That(t, outcome.Claimed, should.BeFalse)
			assert.That(t, outcome.Unavailable, should.Equal("The task slice has expired"))
			assert.That(t, tqt.Pending(tqt.PubSubNotify), should.Match([]string(nil)))
			assert.That(t, tqt.Pending(tqt.BuildbucketNotify), should.Match([]string(nil)))
		})

		t.Run("Already claimed by us", func(t *ftt.Test) {
			existingClaimID := "bot:claim-id"
			outcome := run(createTTR(&existingClaimID))
			assert.That(t, outcome.Claimed, should.BeFalse)
			assert.That(t, outcome.Unavailable, should.Equal(""))
			assert.That(t, tqt.Pending(tqt.PubSubNotify), should.Match([]string(nil)))
			assert.That(t, tqt.Pending(tqt.BuildbucketNotify), should.Match([]string(nil)))
		})

		t.Run("Already claimed by someone else", func(t *ftt.Test) {
			existingClaimID := "bot:another-claim-id"
			outcome := run(createTTR(&existingClaimID))
			assert.That(t, outcome.Claimed, should.BeFalse)
			assert.That(t, outcome.Unavailable, should.Equal(`Already claimed by "bot:another-claim-id"`))
			assert.That(t, tqt.Pending(tqt.PubSubNotify), should.Match([]string(nil)))
			assert.That(t, tqt.Pending(tqt.BuildbucketNotify), should.Match([]string(nil)))
		})
	})
}
