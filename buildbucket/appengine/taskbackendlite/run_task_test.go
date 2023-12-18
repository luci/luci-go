// Copyright 2023 The LUCI Authors.
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

package main

import (
	"context"
	"encoding/json"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/cachingtest"

	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRunTask(t *testing.T) {
	t.Parallel()

	Convey("RunTask", t, func() {
		ctx := memory.UseWithAppID(context.Background(), "myApp-dev")
		ctx = cachingtest.WithGlobalCache(ctx, map[string]caching.BlobCache{
			"taskbackendlite-run-task": cachingtest.NewBlobCache(),
		})
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:             identity.Identity("project:myProject"),
			PeerIdentityOverride: identity.Identity("user:cr-buildbucket-dev@appspot.gserviceaccount.com"),
		})
		ctx, psserver, psclient, err := clients.SetupTestPubsub(ctx, "myApp-dev")
		So(err, ShouldBeNil)
		defer func() {
			psclient.Close()
			psserver.Close()
		}()
		topicFoo, err := psclient.CreateTopic(ctx, "foo")
		So(err, ShouldBeNil)

		srv := &TaskBackendLite{}
		req := &pb.RunTaskRequest{
			BuildId:   "123",
			RequestId: "request_id",
			Target:    "target",
			BackendConfig: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"topic_id": {
						Kind: &structpb.Value_StringValue{
							StringValue: "foo",
						},
					},
				},
			},
			Secrets: &pb.BuildSecrets{
				StartBuildToken: "token",
			},
		}
		Convey("ok", func() {
			res, err := srv.RunTask(ctx, req)

			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.RunTaskResponse{
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "123_request_id",
						Target: "target",
					},
					UpdateId: 1,
				},
			})
			So(psserver.Messages(), ShouldHaveLength, 1)
			publishedMsg := psserver.Messages()[0]
			So(publishedMsg.Attributes["dummy_task_id"], ShouldEqual, "123_request_id")
			data := &TaskNotification{}
			err = json.Unmarshal(publishedMsg.Data, data)
			So(err, ShouldBeNil)
			So(data, ShouldResemble, &TaskNotification{
				BuildID:         "123",
				StartBuildToken: "token",
			})
		})

		Convey("duplicate req", func() {
			cache := caching.GlobalCache(ctx, "taskbackendlite-run-task")
			err := cache.Set(ctx, "123_request_id", []byte{1}, DefaultTaskCreationTimeout)
			So(err, ShouldBeNil)

			res, err := srv.RunTask(ctx, req)

			So(err, ShouldBeNil)
			So(res, ShouldResembleProto, &pb.RunTaskResponse{
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "123_request_id",
						Target: "target",
					},
					UpdateId: 1,
				},
			})
			So(psserver.Messages(), ShouldHaveLength, 0)
		})

		Convey("no topic_id in backend_config", func() {
			res, err := srv.RunTask(ctx, &pb.RunTaskRequest{
				BuildId:       "123",
				BackendConfig: &structpb.Struct{},
			})
			So(res, ShouldBeNil)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, "'topic_id'in req.BackendConfig is not specified or it's not a string")
		})

		Convey("topic_id is not a string", func() {
			res, err := srv.RunTask(ctx, &pb.RunTaskRequest{
				BuildId: "123",
				BackendConfig: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"topic_id": {
							Kind: &structpb.Value_NumberValue{
								NumberValue: 1,
							},
						},
					},
				},
			})
			So(res, ShouldBeNil)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, "'topic_id'in req.BackendConfig is not specified or it's not a string")
		})

		Convey("topic not exist", func() {
			err := topicFoo.Delete(ctx)
			So(err, ShouldBeNil)
			res, err := srv.RunTask(ctx, req)
			So(res, ShouldBeNil)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, "topic foo does not exist on Cloud project myApp-dev")
		})

		Convey("perm errors", func() {
			Convey("no access", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:             identity.Identity("project:myProject"),
					PeerIdentityOverride: identity.Identity("user:user1@example.com"),
				})
				res, err := srv.RunTask(ctx, req)
				So(res, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.PermissionDenied, `the peer "user:user1@example.com" is not allowed to access this task backend`)
			})

			Convey("not a project identity", func() {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:             identity.Identity("user:user1"),
					PeerIdentityOverride: identity.Identity("user:cr-buildbucket-dev@appspot.gserviceaccount.com"),
				})
				res, err := srv.RunTask(ctx, req)
				So(res, ShouldBeNil)
				So(err, ShouldHaveGRPCStatus, codes.PermissionDenied, `The caller's user identity "user:user1" is not a project identity`)
			})
		})
	})
}
