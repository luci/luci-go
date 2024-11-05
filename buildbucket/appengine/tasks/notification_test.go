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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
)

func TestNotification(t *testing.T) {
	t.Parallel()

	ftt.Run("notifyPubsub", t, func(t *ftt.Test) {
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

		t.Run("w/o callback", func(t *ftt.Test) {
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
			assert.Loosely(t, txErr, should.BeNil)
			tasks := sch.Tasks()
			sortTasksByClassName(tasks)
			assert.Loosely(t, tasks, should.HaveLength(1))
			assert.Loosely(t, tasks[0].Payload.(*taskdefs.NotifyPubSubGoProxy).GetBuildId(), should.Equal(123))
			assert.Loosely(t, tasks[0].Payload.(*taskdefs.NotifyPubSubGoProxy).GetProject(), should.Equal("project"))
		})

		t.Run("w/ callback", func(t *ftt.Test) {
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
			assert.Loosely(t, txErr, should.BeNil)
			tasks := sch.Tasks()
			sortTasksByClassName(tasks)
			assert.Loosely(t, tasks, should.HaveLength(2))

			n := tasks[0].Payload.(*taskdefs.NotifyPubSubGo)
			assert.Loosely(t, n.GetBuildId(), should.Equal(123))
			assert.Loosely(t, n.GetCallback(), should.BeTrue)
			assert.Loosely(t, n.GetTopic().GetName(), should.Equal("topic"))

			assert.Loosely(t, tasks[1].Payload.(*taskdefs.NotifyPubSubGoProxy).GetBuildId(), should.Equal(123))
			assert.Loosely(t, tasks[1].Payload.(*taskdefs.NotifyPubSubGoProxy).GetProject(), should.Equal("project"))
		})
	})

	ftt.Run("EnqueueNotifyPubSubGo", t, func(t *ftt.Test) {
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{})
		ctx = txndefer.FilterRDS(ctx)
		ctx, sch := tq.TestingContext(ctx, nil)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		assert.Loosely(t, datastore.Put(ctx, &model.Project{
			ID: "project_with_external_topics",
			CommonConfig: &pb.BuildbucketCfg_CommonConfig{
				BuildsNotificationTopics: []*pb.BuildbucketCfg_Topic{
					{
						Name: "projects/my-cloud-project/topics/my-topic",
					},
				},
			},
		}), should.BeNil)

		t.Run("no project entity", func(t *ftt.Test) {
			txErr := EnqueueNotifyPubSubGo(ctx, 123, "project_no_external_topics")
			assert.Loosely(t, txErr, should.BeNil)
			tasks := sch.Tasks()
			assert.Loosely(t, tasks, should.HaveLength(1))
			assert.Loosely(t, tasks[0].Payload, should.Resemble(&taskdefs.NotifyPubSubGo{
				BuildId: 123,
			}))
		})

		t.Run("empty project.common_config", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.Project{
				ID:           "project_empty",
				CommonConfig: &pb.BuildbucketCfg_CommonConfig{},
			}), should.BeNil)
			txErr := EnqueueNotifyPubSubGo(ctx, 123, "project_empty")
			assert.Loosely(t, txErr, should.BeNil)
			tasks := sch.Tasks()
			assert.Loosely(t, tasks, should.HaveLength(1))
			assert.Loosely(t, tasks[0].Payload, should.Resemble(&taskdefs.NotifyPubSubGo{
				BuildId: 123,
			}))
		})

		t.Run("has external topics", func(t *ftt.Test) {
			txErr := EnqueueNotifyPubSubGo(ctx, 123, "project_with_external_topics")
			assert.Loosely(t, txErr, should.BeNil)
			tasks := sch.Tasks()
			assert.Loosely(t, tasks, should.HaveLength(2))
			taskGo0 := tasks[0].Payload.(*taskdefs.NotifyPubSubGo)
			taskGo1 := tasks[1].Payload.(*taskdefs.NotifyPubSubGo)
			assert.Loosely(t, taskGo0.BuildId, should.Equal(123))
			assert.Loosely(t, taskGo1.BuildId, should.Equal(123))
			assert.Loosely(t, taskGo0.Topic.GetName()+taskGo1.Topic.GetName(), should.Equal("projects/my-cloud-project/topics/my-topic"))
		})
	})

	ftt.Run("PublishBuildsV2Notification", t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)
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
		assert.Loosely(t, datastore.Put(ctx, b, bi, bs, bo, binpProp), should.BeNil)

		t.Run("build not exist", func(t *ftt.Test) {
			err := PublishBuildsV2Notification(ctx, 999, nil, false, maxLargeBytesSize)
			assert.Loosely(t, err, should.BeNil)
			tasks := sch.Tasks()
			assert.Loosely(t, tasks, should.HaveLength(0))
		})

		t.Run("To internal topic", func(t *ftt.Test) {

			t.Run("success", func(t *ftt.Test) {
				err := PublishBuildsV2Notification(ctx, 123, nil, false, maxLargeBytesSize)
				assert.Loosely(t, err, should.BeNil)

				tasks := sch.Tasks()
				assert.Loosely(t, tasks, should.HaveLength(1))
				assert.Loosely(t, tasks[0].Message.Attributes["project"], should.Equal("project"))
				assert.Loosely(t, tasks[0].Message.Attributes["is_completed"], should.Equal("true"))
				assert.Loosely(t, tasks[0].Payload.(*pb.BuildsV2PubSub).GetBuild(), should.Resemble(&pb.Build{
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
				}))
				assert.Loosely(t, tasks[0].Payload.(*pb.BuildsV2PubSub).GetBuildLargeFields(), should.NotBeNil)
				bLargeBytes := tasks[0].Payload.(*pb.BuildsV2PubSub).GetBuildLargeFields()
				buildLarge, err := zlibUncompressBuild(bLargeBytes)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, buildLarge, should.Resemble(&pb.Build{
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
				}))
			})

			t.Run("success - no large fields", func(t *ftt.Test) {
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
				assert.Loosely(t, datastore.Put(ctx, b, bi), should.BeNil)

				err := PublishBuildsV2Notification(ctx, 456, nil, false, maxLargeBytesSize)
				assert.Loosely(t, err, should.BeNil)

				tasks := sch.Tasks()
				assert.Loosely(t, tasks, should.HaveLength(1))
				assert.Loosely(t, tasks[0].Payload.(*pb.BuildsV2PubSub).GetBuild(), should.Resemble(&pb.Build{
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
				}))
				assert.Loosely(t, tasks[0].Payload.(*pb.BuildsV2PubSub).GetBuildLargeFields(), should.NotBeNil)
				bLargeBytes := tasks[0].Payload.(*pb.BuildsV2PubSub).GetBuildLargeFields()
				buildLarge, err := zlibUncompressBuild(bLargeBytes)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, buildLarge, should.Resemble(&pb.Build{
					Input:  &pb.Build_Input{},
					Output: &pb.Build_Output{},
				}))
			})

			t.Run("success - too large fields are dropped", func(t *ftt.Test) {
				b := &model.Build{
					ID: 789,
					Proto: &pb.Build{
						Id: 789,
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
				bo := &model.BuildOutputProperties{
					Build: bk,
					Proto: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"output": {
								Kind: &structpb.Value_StringValue{
									StringValue: "large value ;)",
								},
							},
						},
					},
				}
				assert.Loosely(t, datastore.Put(ctx, b, bi, bo), should.BeNil)

				err := PublishBuildsV2Notification(ctx, 789, nil, false, 1)
				assert.Loosely(t, err, should.BeNil)

				tasks := sch.Tasks()
				assert.Loosely(t, tasks, should.HaveLength(1))
				assert.Loosely(t, tasks[0].Payload.(*pb.BuildsV2PubSub).GetBuild(), should.Match(&pb.Build{
					Id: 789,
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
				}))
				assert.Loosely(t, tasks[0].Payload.(*pb.BuildsV2PubSub).GetBuildLargeFields(), should.BeNil)
				assert.Loosely(t, tasks[0].Payload.(*pb.BuildsV2PubSub).GetBuildLargeFieldsDropped(), should.BeTrue)
			})
		})

		t.Run("To external topic (non callback)", func(t *ftt.Test) {
			ctx, psserver, psclient, err := clients.SetupTestPubsub(ctx, "my-cloud-project")
			assert.Loosely(t, err, should.BeNil)
			defer func() {
				psclient.Close()
				psserver.Close()
			}()
			_, err = psclient.CreateTopic(ctx, "my-topic")
			assert.Loosely(t, err, should.BeNil)

			t.Run("success (zlib compression)", func(t *ftt.Test) {
				err := PublishBuildsV2Notification(
					ctx, 123,
					&pb.BuildbucketCfg_Topic{Name: "projects/my-cloud-project/topics/my-topic"},
					false, maxLargeBytesSize)
				assert.Loosely(t, err, should.BeNil)

				tasks := sch.Tasks()
				assert.Loosely(t, tasks, should.HaveLength(0))
				assert.Loosely(t, psserver.Messages(), should.HaveLength(1))
				publishedMsg := psserver.Messages()[0]

				assert.Loosely(t, publishedMsg.Attributes["project"], should.Equal("project"))
				assert.Loosely(t, publishedMsg.Attributes["bucket"], should.Equal("bucket"))
				assert.Loosely(t, publishedMsg.Attributes["builder"], should.Equal("builder"))
				assert.Loosely(t, publishedMsg.Attributes["is_completed"], should.Equal("true"))
				buildMsg := &pb.BuildsV2PubSub{}
				err = protojson.Unmarshal(publishedMsg.Data, buildMsg)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, buildMsg.Build, should.Resemble(&pb.Build{
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
				}))
				assert.Loosely(t, buildMsg.BuildLargeFields, should.NotBeNil)
				buildLarge, err := zlibUncompressBuild(buildMsg.BuildLargeFields)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, buildLarge, should.Resemble(&pb.Build{
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
				}))
			})

			t.Run("success (zstd compression)", func(t *ftt.Test) {
				err := PublishBuildsV2Notification(ctx, 123, &pb.BuildbucketCfg_Topic{
					Name:        "projects/my-cloud-project/topics/my-topic",
					Compression: pb.Compression_ZSTD,
				}, false, maxLargeBytesSize)
				assert.Loosely(t, err, should.BeNil)

				tasks := sch.Tasks()
				assert.Loosely(t, tasks, should.HaveLength(0))
				assert.Loosely(t, psserver.Messages(), should.HaveLength(1))
				publishedMsg := psserver.Messages()[0]

				assert.Loosely(t, publishedMsg.Attributes["project"], should.Equal("project"))
				assert.Loosely(t, publishedMsg.Attributes["bucket"], should.Equal("bucket"))
				assert.Loosely(t, publishedMsg.Attributes["builder"], should.Equal("builder"))
				assert.Loosely(t, publishedMsg.Attributes["is_completed"], should.Equal("true"))
				buildMsg := &pb.BuildsV2PubSub{}
				err = protojson.Unmarshal(publishedMsg.Data, buildMsg)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, buildMsg.Build, should.Resemble(&pb.Build{
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
				}))
				assert.Loosely(t, buildMsg.BuildLargeFields, should.NotBeNil)
				assert.Loosely(t, buildMsg.Compression, should.Equal(pb.Compression_ZSTD))
				buildLarge, err := zstdUncompressBuild(buildMsg.BuildLargeFields)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, buildLarge, should.Resemble(&pb.Build{
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
				}))
			})

			t.Run("non-exist topic", func(t *ftt.Test) {
				err := PublishBuildsV2Notification(ctx, 123, &pb.BuildbucketCfg_Topic{
					Name: "projects/my-cloud-project/topics/non-exist-topic",
				}, false, maxLargeBytesSize)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
			})
		})

		t.Run("To external topic (callback)", func(t *ftt.Test) {
			ctx, psserver, psclient, err := clients.SetupTestPubsub(ctx, "my-cloud-project")
			assert.Loosely(t, err, should.BeNil)
			defer func() {
				psclient.Close()
				psserver.Close()
			}()
			_, err = psclient.CreateTopic(ctx, "callback-topic")
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
			}), should.BeNil)

			err = PublishBuildsV2Notification(
				ctx, 999,
				&pb.BuildbucketCfg_Topic{Name: "projects/my-cloud-project/topics/callback-topic"},
				true, maxLargeBytesSize)
			assert.Loosely(t, err, should.BeNil)

			tasks := sch.Tasks()
			assert.Loosely(t, tasks, should.HaveLength(0))
			assert.Loosely(t, psserver.Messages(), should.HaveLength(1))
			publishedMsg := psserver.Messages()[0]

			assert.Loosely(t, publishedMsg.Attributes["project"], should.Equal("project"))
			assert.Loosely(t, publishedMsg.Attributes["bucket"], should.Equal("bucket"))
			assert.Loosely(t, publishedMsg.Attributes["builder"], should.Equal("builder"))
			assert.Loosely(t, publishedMsg.Attributes["is_completed"], should.Equal("false"))
			psCallbackMsg := &pb.PubSubCallBack{}
			err = protojson.Unmarshal(publishedMsg.Data, psCallbackMsg)
			assert.Loosely(t, err, should.BeNil)

			buildLarge, err := zlibUncompressBuild(psCallbackMsg.BuildPubsub.BuildLargeFields)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, buildLarge, should.Resemble(&pb.Build{Input: &pb.Build_Input{}, Output: &pb.Build_Output{}}))
			assert.Loosely(t, psCallbackMsg.BuildPubsub.Build, should.Resemble(&pb.Build{
				Id: 999,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Input:  &pb.Build_Input{},
				Output: &pb.Build_Output{},
			}))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, psCallbackMsg.UserData, should.Resemble([]byte("userdata")))
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
