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

package bbtaskbackend

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/auth/identity"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	configpb "go.chromium.org/luci/swarming/proto/config"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/model"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestFetchTasks(t *testing.T) {
	t.Parallel()

	Convey("FetchTasks", t, func() {
		caller := identity.Identity("user:someone@example.com")
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
			Identity: caller,
			FakeDB: authtest.NewFakeDB(
				authtest.MockPermission(caller, "project:visible-realm", acls.PermTasksGet),
			),
		})

		srv := TaskBackend{
			bbTarget:    "target",
			cfgProvider: mockCfgProvider(ctx),
		}

		reqKey, err := model.TaskIDToRequestKey(ctx, "65aba3a3e6b99310")
		So(err, ShouldBeNil)

		buildTask := &model.BuildTask{
			Key:              model.BuildTaskKey(ctx, reqKey),
			BuildID:          "1",
			BuildbucketHost:  "bb-host",
			UpdateID:         100,
			LatestTaskStatus: apipb.TaskState_RUNNING,
		}
		resultSummary := &model.TaskResultSummary{
			Key:          model.TaskResultSummaryKey(ctx, reqKey),
			RequestRealm: "project:visible-realm",
			RequestPool:  "visible",
			TaskResultCommon: model.TaskResultCommon{
				State:         apipb.TaskState_RUNNING,
				Failure:       false,
				BotDimensions: model.BotDimensions{"dim": []string{"a", "b"}},
			},
		}
		So(datastore.Put(ctx, buildTask, resultSummary), ShouldBeNil)

		Convey("empty req", func() {
			req := &bbpb.FetchTasksRequest{}
			res, err := srv.FetchTasks(ctx, req)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &bbpb.FetchTasksResponse{})
		})

		Convey("bad task id format", func() {
			req := &bbpb.FetchTasksRequest{
				TaskIds: []*bbpb.TaskID{
					{
						Target: "target",
						Id:     "zzz",
					},
				},
			}
			res, err := srv.FetchTasks(ctx, req)
			So(err, ShouldBeNil)
			errPb, ok := res.Responses[0].Response.(*bbpb.FetchTasksResponse_Response_Error)
			So(ok, ShouldBeTrue)
			So(errPb.Error.Code, ShouldEqual, codes.InvalidArgument)
			So(errPb.Error.Message, ShouldContainSubstring, "bad task ID")
		})

		Convey("BuildTask not found", func() {
			So(datastore.Delete(ctx, buildTask), ShouldBeNil)
			req := &bbpb.FetchTasksRequest{
				TaskIds: []*bbpb.TaskID{
					{
						Target: "target",
						Id:     "65aba3a3e6b99310",
					},
				},
			}

			res, err := srv.FetchTasks(ctx, req)
			So(err, ShouldBeNil)
			errPb, ok := res.Responses[0].Response.(*bbpb.FetchTasksResponse_Response_Error)
			So(ok, ShouldBeTrue)
			So(errPb.Error.Code, ShouldEqual, codes.NotFound)
			So(errPb.Error.Message, ShouldContainSubstring, "BuildTask not found 65aba3a3e6b99310")
		})

		Convey("TaskResultSummary not found", func() {
			So(datastore.Delete(ctx, resultSummary), ShouldBeNil)
			req := &bbpb.FetchTasksRequest{
				TaskIds: []*bbpb.TaskID{
					{
						Target: "target",
						Id:     "65aba3a3e6b99310",
					},
				},
			}

			res, err := srv.FetchTasks(ctx, req)
			So(err, ShouldBeNil)
			errPb, ok := res.Responses[0].Response.(*bbpb.FetchTasksResponse_Response_Error)
			So(ok, ShouldBeTrue)
			So(errPb.Error.Code, ShouldEqual, codes.NotFound)
			So(errPb.Error.Message, ShouldContainSubstring, "TaskResultSummary not found 65aba3a3e6b99310")
		})

		Convey("no perm", func() {
			resultSummary.RequestRealm = "project:hidden-realm"
			resultSummary.RequestPool = "hidden"
			So(datastore.Put(ctx, resultSummary), ShouldBeNil)
			req := &bbpb.FetchTasksRequest{
				TaskIds: []*bbpb.TaskID{
					{
						Target: "target",
						Id:     "65aba3a3e6b99310",
					},
				},
			}

			res, err := srv.FetchTasks(ctx, req)
			So(err, ShouldBeNil)
			errPb, ok := res.Responses[0].Response.(*bbpb.FetchTasksResponse_Response_Error)
			So(ok, ShouldBeTrue)
			So(errPb.Error.Code, ShouldEqual, codes.PermissionDenied)
			So(errPb.Error.Message, ShouldContainSubstring, "doesn't have permission")
		})

		Convey("ok - one", func() {
			req := &bbpb.FetchTasksRequest{
				TaskIds: []*bbpb.TaskID{
					{
						Target: "target",
						Id:     "65aba3a3e6b99310",
					},
				},
			}
			res, err := srv.FetchTasks(ctx, req)
			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &bbpb.FetchTasksResponse{
				Responses: []*bbpb.FetchTasksResponse_Response{
					{
						Response: &bbpb.FetchTasksResponse_Response_Task{
							Task: &bbpb.Task{
								Status: bbpb.Status_STARTED,
								Id: &bbpb.TaskID{
									Id:     "65aba3a3e6b99310",
									Target: "target",
								},
								UpdateId: 100,
								Details: &structpb.Struct{
									Fields: map[string]*structpb.Value{
										"bot_dimensions": {
											Kind: &structpb.Value_StructValue{
												StructValue: resultSummary.BotDimensions.ToStructPB(),
											},
										},
									},
								},
							},
						},
					},
				},
			})
		})

		Convey("ok - multiple", func() {
			key, err := model.TaskIDToRequestKey(ctx, "65aba3a3e6b99320")
			So(err, ShouldBeNil)
			bTask := &model.BuildTask{
				Key:              model.BuildTaskKey(ctx, key),
				BuildID:          "2",
				BuildbucketHost:  "bb-host",
				UpdateID:         200,
				LatestTaskStatus: apipb.TaskState_COMPLETED,
			}
			trs := &model.TaskResultSummary{
				Key:          model.TaskResultSummaryKey(ctx, key),
				RequestRealm: "project:visible-realm",
				RequestPool:  "visible",
				TaskResultCommon: model.TaskResultCommon{
					State:         apipb.TaskState_COMPLETED,
					Failure:       false,
					BotDimensions: model.BotDimensions{"dim": []string{"a", "b"}},
				},
			}
			So(datastore.Put(ctx, bTask, trs), ShouldBeNil)

			req := &bbpb.FetchTasksRequest{
				TaskIds: []*bbpb.TaskID{
					{
						Target: "target",
						Id:     "65aba3a3e6b99310",
					},
					{
						Target: "target",
						Id:     "zzz",
					},
					{
						Target: "target",
						Id:     "65aba3a3e6b99320",
					},
					{
						Target: "target",
						Id:     "65aba3a3e6b99330",
					},
				},
			}

			res, err := srv.FetchTasks(ctx, req)
			So(err, ShouldBeNil)
			// the response should be [found, bad_id, found, not_found]
			responses := res.Responses
			So(responses[0].Response.(*bbpb.FetchTasksResponse_Response_Task).Task.Id.Id, ShouldEqual, "65aba3a3e6b99310")
			So(responses[1].Response.(*bbpb.FetchTasksResponse_Response_Error).Error.Code, ShouldEqual, codes.InvalidArgument)
			So(responses[2].Response.(*bbpb.FetchTasksResponse_Response_Task).Task.Id.Id, ShouldEqual, "65aba3a3e6b99320")
			So(responses[3].Response.(*bbpb.FetchTasksResponse_Response_Error).Error.Code, ShouldEqual, codes.NotFound)
		})
	})
}

func mockCfgProvider(ctx context.Context) *cfg.Provider {
	poolsPb := &configpb.PoolsCfg{
		Pool: []*configpb.Pool{
			{
				Name:  []string{"visible"},
				Realm: "project:visible-realm",
			},
		}}

	files := make(cfgmem.Files)
	putPb := func(path string, msg proto.Message) {
		if msg != nil {
			blob, err := prototext.Marshal(msg)
			if err != nil {
				panic(err)
			}
			files[path] = string(blob)
		}
	}

	putPb("pools.cfg", poolsPb)

	// Load them back in a queriable form.
	err := cfg.UpdateConfigs(cfgclient.Use(ctx, cfgmem.New(map[config.Set]cfgmem.Files{
		"services/${appid}": files,
	})))
	if err != nil {
		panic(err)
	}

	p, err := cfg.NewProvider(ctx)
	if err != nil {
		panic(err)
	}
	return p
}
