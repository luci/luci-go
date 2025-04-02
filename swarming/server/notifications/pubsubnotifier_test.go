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

package notifications

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsub/pstest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/metrics"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/notifications/taskspb"
)

func TestHandlePubSubNotifyTask(t *testing.T) {
	t.Parallel()

	ftt.Run("send pubsub", t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		now := time.Date(2024, time.January, 1, 1, 1, 1, 1, time.UTC)
		ctx, _ = testclock.UseTime(ctx, now)
		psServer, psClient, err := setupTestPubsub(ctx, "foo")
		assert.NoErr(t, err)
		defer func() {
			_ = psClient.Close()
			_ = psServer.Close()
		}()

		notifier := &PubSubNotifier{
			client: psClient,
		}
		fooTopic, err := psClient.CreateTopic(ctx, "swarming-updates")
		assert.NoErr(t, err)

		psTask := &taskspb.PubSubNotifyTask{
			TaskId:    "task_id_0",
			Topic:     "projects/foo/topics/swarming-updates",
			AuthToken: "auth_token",
			Userdata:  "user_data",
			State:     apipb.TaskState_BOT_DIED,
			StartTime: timestamppb.New(now.Add(-100 * time.Millisecond).UTC()), // make the latency to be 100ms.
		}

		t.Run("invalid topic", func(t *ftt.Test) {
			psTask.Topic = "any"
			err := notifier.handlePubSubNotifyTask(ctx, psTask)
			assert.Loosely(t, err, should.ErrLike(`topic "any" does not match "^projects/(.*)/topics/(.*)$"`))
		})

		t.Run("topic not exist", func(t *ftt.Test) {
			assert.NoErr(t, fooTopic.Delete(ctx))
			err := notifier.handlePubSubNotifyTask(ctx, psTask)
			assert.Loosely(t, err, should.ErrLike(`failed to publish the msg to projects/foo/topics/swarming-updates`))
			assert.Loosely(t, err, should.ErrLike("NotFound"))
			pushedTsmonData := metrics.TaskStatusChangePubsubLatency.Get(ctx, "", model.TaskStateString(psTask.State), 404)
			assert.Loosely(t, pushedTsmonData.Count(), should.Equal(1))
			assert.Loosely(t, pushedTsmonData.Sum(), should.AlmostEqual(float64(100)))
		})

		t.Run("ok", func(t *ftt.Test) {
			err := notifier.handlePubSubNotifyTask(ctx, psTask)
			assert.NoErr(t, err)
			assert.Loosely(t, psServer.Messages(), should.HaveLength(1))
			publishedMsg := psServer.Messages()[0]
			assert.Loosely(t, publishedMsg.Attributes["auth_token"], should.Equal("auth_token"))
			data := &PubSubNotification{}
			err = json.Unmarshal(publishedMsg.Data, data)
			assert.NoErr(t, err)
			assert.Loosely(t, data, should.Match(&PubSubNotification{
				TaskID:   "task_id_0",
				Userdata: "user_data",
			}))
			pushedTsmonData := metrics.TaskStatusChangePubsubLatency.Get(ctx, "", model.TaskStateString(psTask.State), 200)
			assert.Loosely(t, pushedTsmonData.Count(), should.Equal(1))
			assert.Loosely(t, pushedTsmonData.Sum(), should.AlmostEqual(float64(100)))
		})
	})
}

// setupTestPubsub creates a new fake Pub/Sub server and the client connection
// to the server.
func setupTestPubsub(ctx context.Context, cloudProject string) (*pstest.Server, *pubsub.Client, error) {
	srv := pstest.NewServer()
	conn, err := grpc.NewClient(srv.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, nil, err
	}

	client, err := pubsub.NewClient(ctx, cloudProject, option.WithGRPCConn(conn))
	if err != nil {
		return nil, nil, err
	}
	return srv, client, nil
}

func TestHandleBBNotifyTask(t *testing.T) {
	t.Parallel()

	ftt.Run("handleBBNotifyTask", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		psServer, psClient, err := setupTestPubsub(ctx, "bb")
		assert.NoErr(t, err)
		defer func() {
			_ = psClient.Close()
			_ = psServer.Close()
		}()

		notifier := &PubSubNotifier{
			client:       psClient,
			cloudProject: "app",
		}
		bbTopic, err := psClient.CreateTopic(ctx, "bb-updates")
		assert.NoErr(t, err)

		reqKey, err := model.TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		assert.NoErr(t, err)

		buildTask := &model.BuildTask{
			Key:              model.BuildTaskKey(ctx, reqKey),
			BuildID:          "1",
			BuildbucketHost:  "bb-host",
			UpdateID:         100,
			LatestTaskStatus: apipb.TaskState_PENDING,
			PubSubTopic:      "projects/bb/topics/bb-updates",
		}
		resultSummary := &model.TaskResultSummary{
			Key: model.TaskResultSummaryKey(ctx, reqKey),
			TaskResultCommon: model.TaskResultCommon{
				Failure:       false,
				BotDimensions: model.BotDimensions{"dim": []string{"a", "b"}},
			},
		}
		assert.NoErr(t, datastore.Put(ctx, buildTask, resultSummary))

		t.Run("build task not exist", func(t *ftt.Test) {
			psTask := &taskspb.BuildbucketNotifyTask{
				TaskId:   "65aba3a3e6b00000",
				State:    apipb.TaskState_RUNNING,
				UpdateId: 101,
			}
			err := notifier.handleBBNotifyTask(ctx, psTask)
			assert.Loosely(t, err, should.ErrLike("cannot find BuildTask"))
			assert.Loosely(t, tq.Fatal.In(err), should.BeTrue)
		})

		t.Run("prior update id", func(t *ftt.Test) {
			psTask := &taskspb.BuildbucketNotifyTask{
				TaskId:   "65aba3a3e6b99310",
				State:    apipb.TaskState_RUNNING,
				UpdateId: 99, // update id < prior update id
			}
			assert.NoErr(t, notifier.handleBBNotifyTask(ctx, psTask))
			psTask.UpdateId = 100 // update id == prior update id
			assert.NoErr(t, notifier.handleBBNotifyTask(ctx, psTask))
			assert.Loosely(t, psServer.Messages(), should.HaveLength(0))
		})

		t.Run("no state change", func(t *ftt.Test) {
			psTask := &taskspb.BuildbucketNotifyTask{
				TaskId:   "65aba3a3e6b99310",
				State:    apipb.TaskState_PENDING,
				UpdateId: 101,
			}
			err := notifier.handleBBNotifyTask(ctx, psTask)
			assert.NoErr(t, err)
			assert.Loosely(t, psServer.Messages(), should.HaveLength(0))
		})

		t.Run("state change", func(t *ftt.Test) {
			psTask := &taskspb.BuildbucketNotifyTask{
				TaskId:   "65aba3a3e6b99310",
				State:    apipb.TaskState_RUNNING,
				UpdateId: 101,
			}

			err := notifier.handleBBNotifyTask(ctx, psTask)

			assert.NoErr(t, err)
			assert.Loosely(t, psServer.Messages(), should.HaveLength(1))
			bbTopic.Stop()
			publishedMsg := psServer.Messages()[0]
			sentBBUpdate := &bbpb.BuildTaskUpdate{}
			err = proto.Unmarshal(publishedMsg.Data, sentBBUpdate)
			assert.NoErr(t, err)
			assert.Loosely(t, sentBBUpdate, should.Match(&bbpb.BuildTaskUpdate{
				BuildId: "1",
				Task: &bbpb.Task{
					Status: bbpb.Status_STARTED,
					Id: &bbpb.TaskID{
						Id:     "65aba3a3e6b99310",
						Target: "swarming://app",
					},
					UpdateId: 101,
					Details: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"bot_dimensions": {
								Kind: &structpb.Value_StructValue{
									StructValue: &structpb.Struct{
										Fields: map[string]*structpb.Value{
											"dim": {
												Kind: &structpb.Value_ListValue{
													ListValue: &structpb.ListValue{
														Values: []*structpb.Value{
															{Kind: &structpb.Value_StringValue{StringValue: "a"}},
															{Kind: &structpb.Value_StringValue{StringValue: "b"}},
														},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}))

			updatedBuildTask := &model.BuildTask{Key: model.BuildTaskKey(ctx, reqKey)}
			assert.NoErr(t, datastore.Get(ctx, updatedBuildTask))
			assert.Loosely(t, updatedBuildTask.LatestTaskStatus, should.Equal(apipb.TaskState_RUNNING))
			assert.Loosely(t, updatedBuildTask.BotDimensions, should.Match(resultSummary.BotDimensions))
		})
	})
}

func TestEnqueueNotificationTasks(t *testing.T) {
	t.Parallel()

	ctx := memory.Use(context.Background())
	ctx = txndefer.FilterRDS(ctx)
	now := testclock.TestRecentTimeUTC
	ctx, _ = testclock.UseTime(ctx, now)
	ctx, sch := tq.TestingContext(ctx, nil)
	psServer, psClient, err := setupTestPubsub(ctx, "bb")
	assert.NoErr(t, err)
	defer func() {
		_ = psClient.Close()
		_ = psServer.Close()
	}()
	notifier := &PubSubNotifier{
		client: psClient,
	}
	notifier.RegisterTQTasks(&tq.Default)

	ftt.Run("SendOnTaskUpdate", t, func(t *ftt.Test) {
		tID := "65aba3a3e6b99310"
		reqKey, err := model.TaskIDToRequestKey(ctx, tID)
		assert.NoErr(t, err)
		tr := &model.TaskRequest{
			Key:             reqKey,
			PubSubTopic:     "projects/bb/topics/swarming-updates",
			PubSubAuthToken: "token",
			PubSubUserData:  "data",
		}
		trs := &model.TaskResultSummary{
			Key:  model.TaskResultSummaryKey(ctx, reqKey),
			Tags: []string{"tag1", "tag2"},
			TaskResultCommon: model.TaskResultCommon{
				State: apipb.TaskState_COMPLETED,
			},
		}

		t.Run("no pubsub topic", func(t *ftt.Test) {
			tr.PubSubTopic = ""
			txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				return SendOnTaskUpdate(ctx, tr, trs)
			}, nil)
			assert.NoErr(t, txErr)
			assert.Loosely(t, sch.Tasks(), should.BeEmpty)
		})

		t.Run("not a build task", func(t *ftt.Test) {
			txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				return SendOnTaskUpdate(ctx, tr, trs)
			}, nil)
			assert.NoErr(t, txErr)
			assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
		})

		t.Run("build task", func(t *ftt.Test) {
			tr.HasBuildTask = true
			txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				return SendOnTaskUpdate(ctx, tr, trs)
			}, nil)
			assert.NoErr(t, txErr)
			// 1 from above test, 2 from this one.
			assert.Loosely(t, sch.Tasks(), should.HaveLength(3))

			// Test added tasks are what the handlers expect.
			_, err := psClient.CreateTopic(ctx, "swarming-updates")
			assert.NoErr(t, err)
			task := sch.Tasks()[1].Payload.(*taskspb.PubSubNotifyTask)
			err = notifier.handlePubSubNotifyTask(ctx, task)
			assert.NoErr(t, err)

			_, err = psClient.CreateTopic(ctx, "bb-updates")
			assert.NoErr(t, err)
			buildTask := &model.BuildTask{
				Key:              model.BuildTaskKey(ctx, reqKey),
				BuildID:          "1",
				BuildbucketHost:  "bb-host",
				UpdateID:         100,
				LatestTaskStatus: apipb.TaskState_PENDING,
				PubSubTopic:      "projects/bb/topics/bb-updates",
			}
			resultSummary := &model.TaskResultSummary{
				Key: model.TaskResultSummaryKey(ctx, reqKey),
				TaskResultCommon: model.TaskResultCommon{
					Failure:       false,
					BotDimensions: model.BotDimensions{"dim": []string{"a", "b"}},
				},
			}
			assert.NoErr(t, datastore.Put(ctx, buildTask, resultSummary))
			bbTask := sch.Tasks()[0].Payload.(*taskspb.BuildbucketNotifyTask)
			err = notifier.handleBBNotifyTask(ctx, bbTask)
			assert.NoErr(t, err)
		})
	})
}
