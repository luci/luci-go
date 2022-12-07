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
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/compression"
	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"

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

		Convey("w/o callback", func() {
			txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				return NotifyPubSub(ctx, &model.Build{ID: 123})
			}, nil)
			So(txErr, ShouldBeNil)
			tasks := sch.Tasks()
			So(tasks, ShouldHaveLength, 1)
			So(tasks[0].Payload.(*taskdefs.NotifyPubSub).GetBuildId(), ShouldEqual, 123)
			So(tasks[0].Payload.(*taskdefs.NotifyPubSub).GetCallback(), ShouldBeFalse)
		})

		Convey("w/ callback", func() {
			txErr := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				cb := model.PubSubCallback{
					AuthToken: "token",
					Topic:     "topic",
					UserData:  []byte("user_data"),
				}
				return NotifyPubSub(ctx, &model.Build{ID: 123, PubSubCallback: cb})
			}, nil)
			So(txErr, ShouldBeNil)
			tasks := sch.Tasks()
			So(tasks, ShouldHaveLength, 2)

			n1 := tasks[0].Payload.(*taskdefs.NotifyPubSub)
			n2 := tasks[1].Payload.(*taskdefs.NotifyPubSub)
			So(n1.GetBuildId(), ShouldEqual, 123)
			So(n2.GetBuildId(), ShouldEqual, 123)
			// One w/ callback and one w/o callback.
			So(n1.GetCallback() != n2.GetCallback(), ShouldBeTrue)
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
			err := PublishBuildsV2Notification(ctx, 999)
			So(err, ShouldBeNil)
			tasks := sch.Tasks()
			So(tasks, ShouldHaveLength, 0)
		})

		Convey("success", func() {
			err := PublishBuildsV2Notification(ctx, 123)
			So(err, ShouldBeNil)

			tasks := sch.Tasks()
			So(tasks, ShouldHaveLength, 1)
			So(tasks[0].Message.Attributes["project"], ShouldEqual, "project")
			So(tasks[0].Message.Attributes["is_completed"], ShouldEqual, "true")
			So(tasks[0].Payload.(*taskdefs.BuildsV2PubSub).GetBuild(), ShouldResembleProto, &pb.Build{
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
			So(tasks[0].Payload.(*taskdefs.BuildsV2PubSub).GetBuildLargeFields(), ShouldNotBeNil)
			bLargeBytes := tasks[0].Payload.(*taskdefs.BuildsV2PubSub).GetBuildLargeFields()
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

			err := PublishBuildsV2Notification(ctx, 456)
			So(err, ShouldBeNil)

			tasks := sch.Tasks()
			So(tasks, ShouldHaveLength, 1)
			So(tasks[0].Payload.(*taskdefs.BuildsV2PubSub).GetBuild(), ShouldResembleProto, &pb.Build{
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
			So(tasks[0].Payload.(*taskdefs.BuildsV2PubSub).GetBuildLargeFields(), ShouldNotBeNil)
			bLargeBytes := tasks[0].Payload.(*taskdefs.BuildsV2PubSub).GetBuildLargeFields()
			buildLarge, err := zlibUncompressBuild(bLargeBytes)
			So(err, ShouldBeNil)
			So(buildLarge, ShouldResembleProto, &pb.Build{
				Input:  &pb.Build_Input{},
				Output: &pb.Build_Output{},
			})
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
