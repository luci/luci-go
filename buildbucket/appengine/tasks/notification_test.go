// Copyright 2020 The LUCI Authors.
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

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/internal/compression"
	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestNotification(t *testing.T) {
	t.Parallel()

	Convey("notifyPubsub", t, func() {
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{})
		ctx = txndefer.FilterRDS(ctx)
		ctx, sch := tq.TestingContext(ctx, nil)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		sortTasksByClassName := func(tasks tqtesting.TaskList) {
			sort.Slice(tasks, func(i, j int) bool {
				return tasks[i].Class < tasks[j].Class
			})
		}

		Convey("w/o callback", func() {
			txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				return NotifyPubSub(ctx, &model.Build{
					ID: 123,
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
						},
					},
				})
			}, nil)
			So(txErr, ShouldBeNil)
			tasks := sch.Tasks()
			sortTasksByClassName(tasks)
			So(tasks, ShouldHaveLength, 2)
			So(tasks[0].Payload.(*taskdefs.NotifyPubSub).GetBuildId(), ShouldEqual, 123)
			So(tasks[0].Payload.(*taskdefs.NotifyPubSub).GetCallback(), ShouldBeFalse)
			So(tasks[1].Payload.(*taskdefs.NotifyPubSubGoProxy).GetBuildId(), ShouldEqual, 123)
			So(tasks[1].Payload.(*taskdefs.NotifyPubSubGoProxy).GetProject(), ShouldEqual, "project")
		})

		Convey("w/ callback", func() {
			txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				cb := model.PubSubCallback{
					AuthToken: "token",
					Topic:     "topic",
					UserData:  []byte("user_data"),
				}
				return NotifyPubSub(ctx, &model.Build{
					ID:             123,
					PubSubCallback: cb,
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "project",
						},
					},
				})
			}, nil)
			So(txErr, ShouldBeNil)
			tasks := sch.Tasks()
			sortTasksByClassName(tasks)
			So(tasks, ShouldHaveLength, 3)

			n1 := tasks[0].Payload.(*taskdefs.NotifyPubSub)
			n2 := tasks[1].Payload.(*taskdefs.NotifyPubSubGo)
			So(n1.GetBuildId(), ShouldEqual, 123)
			So(n1.GetCallback(), ShouldBeFalse)
			So(n2.GetBuildId(), ShouldEqual, 123)
			So(n2.GetCallback(), ShouldBeTrue)
			So(n2.GetTopic().GetName(), ShouldEqual, "topic")

			So(tasks[2].Payload.(*taskdefs.NotifyPubSubGoProxy).GetBuildId(), ShouldEqual, 123)
			So(tasks[2].Payload.(*taskdefs.NotifyPubSubGoProxy).GetProject(), ShouldEqual, "project")
		})
	})

	Convey("EnqueueNotifyPubSubGo", t, func() {
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{})
		ctx = txndefer.FilterRDS(ctx)
		ctx, sch := tq.TestingContext(ctx, nil)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		So(datastore.Put(ctx, &model.Project{
			ID: "project_with_external_topics",
			CommonConfig: &pb.BuildbucketCfg_CommonConfig{
				BuildsNotificationTopics: []*pb.BuildbucketCfg_Topic{
					{
						Name: "projects/my-cloud-project/topics/my-topic",
					},
				},
			},
		}), ShouldBeNil)

		Convey("no project entity", func() {
			txErr := EnqueueNotifyPubSubGo(ctx, 123, "project_no_external_topics")
			So(txErr, ShouldBeNil)
			tasks := sch.Tasks()
			So(tasks, ShouldHaveLength, 1)
			So(tasks[0].Payload, ShouldResembleProto, &taskdefs.NotifyPubSubGo{
				BuildId: 123,
			})
		})

		Convey("empty project.common_config", func() {
			So(datastore.Put(ctx, &model.Project{
				ID:           "project_empty",
				CommonConfig: &pb.BuildbucketCfg_CommonConfig{},
			}), ShouldBeNil)
			txErr := EnqueueNotifyPubSubGo(ctx, 123, "project_empty")
			So(txErr, ShouldBeNil)
			tasks := sch.Tasks()
			So(tasks, ShouldHaveLength, 1)
			So(tasks[0].Payload, ShouldResembleProto, &taskdefs.NotifyPubSubGo{
				BuildId: 123,
			})
		})

		Convey("has external topics", func() {
			txErr := EnqueueNotifyPubSubGo(ctx, 123, "project_with_external_topics")
			So(txErr, ShouldBeNil)
			tasks := sch.Tasks()
			So(tasks, ShouldHaveLength, 2)
			taskGo0 := tasks[0].Payload.(*taskdefs.NotifyPubSubGo)
			taskGo1 := tasks[1].Payload.(*taskdefs.NotifyPubSubGo)
			So(taskGo0.BuildId, ShouldEqual, 123)
			So(taskGo1.BuildId, ShouldEqual, 123)
			So(taskGo0.Topic.GetName()+taskGo1.Topic.GetName(), ShouldEqual, "projects/my-cloud-project/topics/my-topic")
		})
	})

	Convey("PublishBuildsV2Notification", t, func() {
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{})
		ctx = txndefer.FilterRDS(ctx)
		ctx, sch := tq.TestingContext(ctx, nil)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		b := &model.Build{
			ID: 123,
			Proto: &pb.Build{
				Id: 123,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_CANCELED,
			},
		}
		bk := datastore.KeyForObj(ctx, b)
		bsBytes, err := proto.Marshal(&pb.Build{
			Steps: []*pb.Step{
				{
					Name:            "step",
					SummaryMarkdown: "summary",
					Logs: []*pb.Log{{
						Name:    "log1",
						Url:     "url",
						ViewUrl: "view_url",
					},
					},
				},
			},
		})
		So(err, ShouldBeNil)
		bs := &model.BuildSteps{ID: 1, Build: bk, Bytes: bsBytes}
		bi := &model.BuildInfra{
			ID:    1,
			Build: bk,
			Proto: &pb.BuildInfra{
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Hostname: "hostname",
				},
			},
		}
		bo := &model.BuildOutputProperties{
			Build: bk,
			Proto: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"output": {
						Kind: &structpb.Value_StringValue{
							StringValue: "output value",
						},
					},
				},
			},
		}
		binpProp := &model.BuildInputProperties{
			Build: bk,
			Proto: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"input": {
						Kind: &structpb.Value_StringValue{
							StringValue: "input value",
						},
					},
				},
			},
		}
		So(datastore.Put(ctx, b, bi, bs, bo, binpProp), ShouldBeNil)

		Convey("build not exist", func() {
			err := PublishBuildsV2Notification(ctx, 999, nil, false)
			So(err, ShouldBeNil)
			tasks := sch.Tasks()
			So(tasks, ShouldHaveLength, 0)
		})

		Convey("To internal topic", func() {

			Convey("success", func() {
				err := PublishBuildsV2Notification(ctx, 123, nil, false)
				So(err, ShouldBeNil)

				tasks := sch.Tasks()
				So(tasks, ShouldHaveLength, 1)
				So(tasks[0].Message.Attributes["project"], ShouldEqual, "project")
				So(tasks[0].Message.Attributes["is_completed"], ShouldEqual, "true")
				So(tasks[0].Payload.(*pb.BuildsV2PubSub).GetBuild(), ShouldResembleProto, &pb.Build{
					Id: 123,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_CANCELED,
					Infra: &pb.BuildInfra{
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "hostname",
						},
					},
					Input:  &pb.Build_Input{},
					Output: &pb.Build_Output{},
				})
				So(tasks[0].Payload.(*pb.BuildsV2PubSub).GetBuildLargeFields(), ShouldNotBeNil)
				bLargeBytes := tasks[0].Payload.(*pb.BuildsV2PubSub).GetBuildLargeFields()
				buildLarge, err := zlibUncompressBuild(bLargeBytes)
				So(err, ShouldBeNil)
				So(buildLarge, ShouldResembleProto, &pb.Build{
					Steps: []*pb.Step{
						{
							Name:            "step",
							SummaryMarkdown: "summary",
							Logs: []*pb.Log{{
								Name:    "log1",
								Url:     "url",
								ViewUrl: "view_url",
							},
							},
						},
					},
					Input: &pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"input": {
									Kind: &structpb.Value_StringValue{
										StringValue: "input value",
									},
								},
							},
						},
					},
					Output: &pb.Build_Output{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"output": {
									Kind: &structpb.Value_StringValue{
										StringValue: "output value",
									},
								},
							},
						},
					},
				})
			})

			Convey("success - no large fields", func() {
				b := &model.Build{
					ID: 456,
					Proto: &pb.Build{
						Id: 456,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Status: pb.Status_CANCELED,
					},
				}
				bk := datastore.KeyForObj(ctx, b)
				bi := &model.BuildInfra{
					ID:    1,
					Build: bk,
					Proto: &pb.BuildInfra{
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "hostname",
						},
					},
				}
				So(datastore.Put(ctx, b, bi), ShouldBeNil)

				err := PublishBuildsV2Notification(ctx, 456, nil, false)
				So(err, ShouldBeNil)

				tasks := sch.Tasks()
				So(tasks, ShouldHaveLength, 1)
				So(tasks[0].Payload.(*pb.BuildsV2PubSub).GetBuild(), ShouldResembleProto, &pb.Build{
					Id: 456,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_CANCELED,
					Infra: &pb.BuildInfra{
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "hostname",
						},
					},
					Input:  &pb.Build_Input{},
					Output: &pb.Build_Output{},
				})
				So(tasks[0].Payload.(*pb.BuildsV2PubSub).GetBuildLargeFields(), ShouldNotBeNil)
				bLargeBytes := tasks[0].Payload.(*pb.BuildsV2PubSub).GetBuildLargeFields()
				buildLarge, err := zlibUncompressBuild(bLargeBytes)
				So(err, ShouldBeNil)
				So(buildLarge, ShouldResembleProto, &pb.Build{
					Input:  &pb.Build_Input{},
					Output: &pb.Build_Output{},
				})
			})
		})

		Convey("To external topic (non callback)", func() {
			ctx, psserver, psclient, err := clients.SetupTestPubsub(ctx, "my-cloud-project")
			So(err, ShouldBeNil)
			defer func() {
				psclient.Close()
				psserver.Close()
			}()
			_, err = psclient.CreateTopic(ctx, "my-topic")
			So(err, ShouldBeNil)

			Convey("success (zlib compression)", func() {
				err := PublishBuildsV2Notification(ctx, 123, &pb.BuildbucketCfg_Topic{Name: "projects/my-cloud-project/topics/my-topic"}, false)
				So(err, ShouldBeNil)

				tasks := sch.Tasks()
				So(tasks, ShouldHaveLength, 0)
				So(psserver.Messages(), ShouldHaveLength, 1)
				publishedMsg := psserver.Messages()[0]

				So(publishedMsg.Attributes["project"], ShouldEqual, "project")
				So(publishedMsg.Attributes["bucket"], ShouldEqual, "bucket")
				So(publishedMsg.Attributes["builder"], ShouldEqual, "builder")
				So(publishedMsg.Attributes["is_completed"], ShouldEqual, "true")
				buildMsg := &pb.BuildsV2PubSub{}
				err = protojson.Unmarshal(publishedMsg.Data, buildMsg)
				So(err, ShouldBeNil)
				So(buildMsg.Build, ShouldResembleProto, &pb.Build{
					Id: 123,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_CANCELED,
					Infra: &pb.BuildInfra{
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "hostname",
						},
					},
					Input:  &pb.Build_Input{},
					Output: &pb.Build_Output{},
				})
				So(buildMsg.BuildLargeFields, ShouldNotBeNil)
				buildLarge, err := zlibUncompressBuild(buildMsg.BuildLargeFields)
				So(err, ShouldBeNil)
				So(buildLarge, ShouldResembleProto, &pb.Build{
					Steps: []*pb.Step{
						{
							Name:            "step",
							SummaryMarkdown: "summary",
							Logs: []*pb.Log{{
								Name:    "log1",
								Url:     "url",
								ViewUrl: "view_url",
							},
							},
						},
					},
					Input: &pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"input": {
									Kind: &structpb.Value_StringValue{
										StringValue: "input value",
									},
								},
							},
						},
					},
					Output: &pb.Build_Output{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"output": {
									Kind: &structpb.Value_StringValue{
										StringValue: "output value",
									},
								},
							},
						},
					},
				})
			})

			Convey("success (zstd compression)", func() {
				err := PublishBuildsV2Notification(ctx, 123, &pb.BuildbucketCfg_Topic{
					Name:        "projects/my-cloud-project/topics/my-topic",
					Compression: pb.Compression_ZSTD,
				}, false)
				So(err, ShouldBeNil)

				tasks := sch.Tasks()
				So(tasks, ShouldHaveLength, 0)
				So(psserver.Messages(), ShouldHaveLength, 1)
				publishedMsg := psserver.Messages()[0]

				So(publishedMsg.Attributes["project"], ShouldEqual, "project")
				So(publishedMsg.Attributes["bucket"], ShouldEqual, "bucket")
				So(publishedMsg.Attributes["builder"], ShouldEqual, "builder")
				So(publishedMsg.Attributes["is_completed"], ShouldEqual, "true")
				buildMsg := &pb.BuildsV2PubSub{}
				err = protojson.Unmarshal(publishedMsg.Data, buildMsg)
				So(err, ShouldBeNil)
				So(buildMsg.Build, ShouldResembleProto, &pb.Build{
					Id: 123,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_CANCELED,
					Infra: &pb.BuildInfra{
						Buildbucket: &pb.BuildInfra_Buildbucket{
							Hostname: "hostname",
						},
					},
					Input:  &pb.Build_Input{},
					Output: &pb.Build_Output{},
				})
				So(buildMsg.BuildLargeFields, ShouldNotBeNil)
				So(buildMsg.Compression, ShouldEqual, pb.Compression_ZSTD)
				buildLarge, err := zstdUncompressBuild(buildMsg.BuildLargeFields)
				So(err, ShouldBeNil)
				So(buildLarge, ShouldResembleProto, &pb.Build{
					Steps: []*pb.Step{
						{
							Name:            "step",
							SummaryMarkdown: "summary",
							Logs: []*pb.Log{{
								Name:    "log1",
								Url:     "url",
								ViewUrl: "view_url",
							},
							},
						},
					},
					Input: &pb.Build_Input{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"input": {
									Kind: &structpb.Value_StringValue{
										StringValue: "input value",
									},
								},
							},
						},
					},
					Output: &pb.Build_Output{
						Properties: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"output": {
									Kind: &structpb.Value_StringValue{
										StringValue: "output value",
									},
								},
							},
						},
					},
				})
			})

			Convey("non-exist topic", func() {
				err := PublishBuildsV2Notification(ctx, 123, &pb.BuildbucketCfg_Topic{
					Name: "projects/my-cloud-project/topics/non-exist-topic",
				}, false)
				So(err, ShouldNotBeNil)
				So(transient.Tag.In(err), ShouldBeTrue)
			})
		})

		Convey("To external topic (callback)", func() {
			ctx, psserver, psclient, err := clients.SetupTestPubsub(ctx, "my-cloud-project")
			So(err, ShouldBeNil)
			defer func() {
				psclient.Close()
				psserver.Close()
			}()
			_, err = psclient.CreateTopic(ctx, "callback-topic")
			So(err, ShouldBeNil)

			So(datastore.Put(ctx, &model.Build{
				ID:        999,
				Project:   "project",
				BucketID:  "bucket",
				BuilderID: "builder",
				Proto: &pb.Build{
					Id: 999,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
				},
				PubSubCallback: model.PubSubCallback{
					Topic:    "projects/my-cloud-project/topics/callback-topic",
					UserData: []byte("userdata"),
				},
			}), ShouldBeNil)

			err = PublishBuildsV2Notification(ctx, 999, &pb.BuildbucketCfg_Topic{Name: "projects/my-cloud-project/topics/callback-topic"}, true)
			So(err, ShouldBeNil)

			tasks := sch.Tasks()
			So(tasks, ShouldHaveLength, 0)
			So(psserver.Messages(), ShouldHaveLength, 1)
			publishedMsg := psserver.Messages()[0]

			So(publishedMsg.Attributes["project"], ShouldEqual, "project")
			So(publishedMsg.Attributes["bucket"], ShouldEqual, "bucket")
			So(publishedMsg.Attributes["builder"], ShouldEqual, "builder")
			So(publishedMsg.Attributes["is_completed"], ShouldEqual, "false")
			psCallbackMsg := &pb.PubSubCallBack{}
			err = protojson.Unmarshal(publishedMsg.Data, psCallbackMsg)
			So(err, ShouldBeNil)

			buildLarge, err := zlibUncompressBuild(psCallbackMsg.BuildPubsub.BuildLargeFields)
			So(err, ShouldBeNil)
			So(buildLarge, ShouldResembleProto, &pb.Build{Input: &pb.Build_Input{}, Output: &pb.Build_Output{}})
			So(psCallbackMsg.BuildPubsub.Build, ShouldResembleProto, &pb.Build{
				Id: 999,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Input:  &pb.Build_Input{},
				Output: &pb.Build_Output{},
			})
			So(err, ShouldBeNil)
			So(psCallbackMsg.UserData, ShouldResemble, []byte("userdata"))
		})
	})
}

func zlibUncompressBuild(compressed []byte) (*pb.Build, error) {
	originalData, err := compression.ZlibDecompress(compressed)
	if err != nil {
		return nil, err
	}
	b := &pb.Build{}
	if err := proto.Unmarshal(originalData, b); err != nil {
		return nil, err
	}
	return b, nil
}

func zstdUncompressBuild(compressed []byte) (*pb.Build, error) {
	originalData, err := compression.ZstdDecompress(compressed, nil)
	if err != nil {
		return nil, err
	}
	b := &pb.Build{}
	if err := proto.Unmarshal(originalData, b); err != nil {
		return nil, err
	}
	return b, nil
}
