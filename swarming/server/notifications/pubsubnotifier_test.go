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

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsub/pstest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/notifications/taskspb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestHandlePubSubNotifyTask(t *testing.T) {
	t.Parallel()

	Convey("send pubsub", t, func() {
		ctx := context.Background()
		psServer, psClient, err := setupTestPubsub(ctx, "foo")
		So(err, ShouldBeNil)
		defer func() {
			_ = psClient.Close()
			_ = psServer.Close()
		}()

		notifier := &PubSubNotifier{
			client: psClient,
		}
		fooTopic, err := psClient.CreateTopic(ctx, "swarming-updates")
		So(err, ShouldBeNil)

		psTask := &taskspb.PubSubNofityTask{
			TaskId:    "task_id_0",
			Topic:     "projects/foo/topics/swarming-updates",
			AuthToken: "auth_token",
			Userdata:  "user_data",
		}

		Convey("invalid topic", func() {
			psTask.Topic = "any"
			err := notifier.handlePubSubNotifyTask(ctx, psTask)
			So(err, ShouldErrLike, `topic "any" does not match "^projects/(.*)/topics/(.*)$"`)
		})

		Convey("topic not exist", func() {
			So(fooTopic.Delete(ctx), ShouldBeNil)
			err := notifier.handlePubSubNotifyTask(ctx, psTask)
			So(err, ShouldErrLike, `failed to publish the msg to projects/foo/topics/swarming-updates`, "NotFound")
		})

		Convey("ok", func() {
			err := notifier.handlePubSubNotifyTask(ctx, psTask)
			So(err, ShouldBeNil)
			So(psServer.Messages(), ShouldHaveLength, 1)
			publishedMsg := psServer.Messages()[0]
			So(publishedMsg.Attributes["auth_token"], ShouldEqual, "auth_token")
			data := &PubSubNotification{}
			err = json.Unmarshal(publishedMsg.Data, data)
			So(err, ShouldBeNil)
			So(data, ShouldResemble, &PubSubNotification{
				TaskID:   "task_id_0",
				Userdata: "user_data",
			})
		})
	})
}

// setupTestPubsub creates a new fake Pub/Sub server and the client connection
// to the server.
func setupTestPubsub(ctx context.Context, cloudProject string) (*pstest.Server, *pubsub.Client, error) {
	srv := pstest.NewServer()
	conn, err := grpc.Dial(srv.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
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

	Convey("handleBBNotifyTask", t, func() {
		ctx := memory.Use(context.Background())
		psServer, psClient, err := setupTestPubsub(ctx, "bb")
		So(err, ShouldBeNil)
		defer func() {
			_ = psClient.Close()
			_ = psServer.Close()
		}()

		notifier := &PubSubNotifier{
			client:       psClient,
			cloudProject: "app",
		}
		bbTopic, err := psClient.CreateTopic(ctx, "bb-updates")
		So(err, ShouldBeNil)

		reqKey, err := model.TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		So(err, ShouldBeNil)

		buildTask := &model.BuildTask{
			Key:              model.BuildTaskKey(ctx, reqKey),
			BuildID:          "1",
			BuildbucketHost:  "bb-host",
			UpdateID:         100,
			LatestTaskStatus: bbpb.Status_SCHEDULED,
			PubSubTopic:      "projects/bb/topics/bb-updates",
		}
		resultSummary := &model.TaskResultSummary{
			Key: model.TaskResultSummaryKey(ctx, reqKey),
			TaskResultCommon: model.TaskResultCommon{
				Failure:       false,
				BotDimensions: model.BotDimensions{"dim": []string{"a", "b"}},
			},
		}
		So(datastore.Put(ctx, buildTask, resultSummary), ShouldBeNil)

		Convey("build task not exist", func() {
			psTask := &taskspb.BuildbucketNofityTask{
				TaskId:   "65aba3a3e6b00000",
				State:    apipb.TaskState_RUNNING,
				UpdateId: 101,
			}
			err := notifier.handleBBNotifyTask(ctx, psTask)
			So(err, ShouldErrLike, "cannot find BuildTask")
			So(tq.Fatal.In(err), ShouldBeTrue)
		})

		Convey("prior update id", func() {
			psTask := &taskspb.BuildbucketNofityTask{
				TaskId:   "65aba3a3e6b99310",
				State:    apipb.TaskState_RUNNING,
				UpdateId: 99,
			}
			err := notifier.handleBBNotifyTask(ctx, psTask)
			So(err, ShouldBeNil)
			So(psServer.Messages(), ShouldHaveLength, 0)
		})

		Convey("no state change", func() {
			psTask := &taskspb.BuildbucketNofityTask{
				TaskId:   "65aba3a3e6b99310",
				State:    apipb.TaskState_PENDING,
				UpdateId: 101,
			}
			err := notifier.handleBBNotifyTask(ctx, psTask)
			So(err, ShouldBeNil)
			So(psServer.Messages(), ShouldHaveLength, 0)
		})

		Convey("state change", func() {
			psTask := &taskspb.BuildbucketNofityTask{
				TaskId:   "65aba3a3e6b99310",
				State:    apipb.TaskState_RUNNING,
				UpdateId: 101,
			}

			err := notifier.handleBBNotifyTask(ctx, psTask)

			So(err, ShouldBeNil)
			So(psServer.Messages(), ShouldHaveLength, 1)
			bbTopic.Stop()
			publishedMsg := psServer.Messages()[0]
			sentBBUpdate := &bbpb.BuildTaskUpdate{}
			err = proto.Unmarshal(publishedMsg.Data, sentBBUpdate)
			So(err, ShouldBeNil)
			So(sentBBUpdate, ShouldResembleProto, &bbpb.BuildTaskUpdate{
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
			})

			updatedBuildTask := &model.BuildTask{Key: model.BuildTaskKey(ctx, reqKey)}
			So(datastore.Get(ctx, updatedBuildTask), ShouldBeNil)
			So(updatedBuildTask.LatestTaskStatus, ShouldEqual, bbpb.Status_STARTED)
			So(updatedBuildTask.BotDimensions, ShouldEqual, resultSummary.BotDimensions)
		})
	})
}
