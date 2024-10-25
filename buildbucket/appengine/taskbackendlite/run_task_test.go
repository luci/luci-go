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
	"fmt"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/cachingtest"

	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestRunTask(t *testing.T) {
	t.Parallel()

	ftt.Run("RunTask", t, func(t *ftt.Test) {
		ctx := memory.UseWithAppID(context.Background(), "myApp-dev")
		ctx = cachingtest.WithGlobalCache(ctx, map[string]caching.BlobCache{
			"taskbackendlite-run-task": cachingtest.NewBlobCache(),
		})
		ctx = auth.WithState(ctx, &authtest.FakeState{
			Identity:             identity.Identity("project:myProject"),
			PeerIdentityOverride: identity.Identity("user:cr-buildbucket-dev@appspot.gserviceaccount.com"),
		})
		ctx, psserver, psclient, err := clients.SetupTestPubsub(ctx, "myApp-dev")
		assert.Loosely(t, err, should.BeNil)
		defer func() {
			psclient.Close()
			psserver.Close()
		}()
		myTopic, err := psclient.CreateTopic(ctx, fmt.Sprintf(TopicIDFormat, "myProject"))
		assert.Loosely(t, err, should.BeNil)

		srv := &TaskBackendLite{}
		req := &pb.RunTaskRequest{
			BuildId:   "123",
			RequestId: "request_id",
			Target:    "target",
			Secrets: &pb.BuildSecrets{
				StartBuildToken: "token",
			},
			BackendConfig: &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"tags": {
						Kind: &structpb.Value_ListValue{
							ListValue: &structpb.ListValue{
								Values: []*structpb.Value{
									{Kind: &structpb.Value_StringValue{StringValue: "buildbucket_bucket:infra/try"}},
									{Kind: &structpb.Value_StringValue{StringValue: "builder:foo"}},
								},
							},
						},
					},
				},
			},
		}
		t.Run("ok", func(t *ftt.Test) {
			res, err := srv.RunTask(ctx, req)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.RunTaskResponse{
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "123_request_id",
						Target: "target",
					},
					UpdateId: 1,
				},
			}))
			assert.Loosely(t, psserver.Messages(), should.HaveLength(1))
			publishedMsg := psserver.Messages()[0]
			assert.Loosely(t, publishedMsg.Attributes["dummy_task_id"], should.Equal("123_request_id"))
			assert.Loosely(t, publishedMsg.Attributes["project"], should.Equal("infra"))
			assert.Loosely(t, publishedMsg.Attributes["bucket"], should.Equal("try"))
			assert.Loosely(t, publishedMsg.Attributes["builder"], should.Equal("foo"))
			data := &TaskNotification{}
			err = json.Unmarshal(publishedMsg.Data, data)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, data, should.Resemble(&TaskNotification{
				BuildID:         "123",
				StartBuildToken: "token",
			}))
		})

		t.Run("nil BackendConfig", func(t *ftt.Test) {
			req.BackendConfig = nil

			res, err := srv.RunTask(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.RunTaskResponse{
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "123_request_id",
						Target: "target",
					},
					UpdateId: 1,
				},
			}))

			assert.Loosely(t, psserver.Messages(), should.HaveLength(1))
			publishedMsg := psserver.Messages()[0]
			assert.Loosely(t, publishedMsg.Attributes["dummy_task_id"], should.Equal("123_request_id"))
			assert.Loosely(t, publishedMsg.Attributes["project"], should.BeEmpty)
			assert.Loosely(t, publishedMsg.Attributes["bucket"], should.BeEmpty)
			assert.Loosely(t, publishedMsg.Attributes["builder"], should.BeEmpty)
		})

		t.Run("no builder related tags in req", func(t *ftt.Test) {
			req.BackendConfig = &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"tags": {
						Kind: &structpb.Value_ListValue{
							ListValue: &structpb.ListValue{
								Values: []*structpb.Value{
									{Kind: &structpb.Value_NumberValue{NumberValue: 10}},
									{Kind: &structpb.Value_StringValue{StringValue: "any"}},
								},
							},
						},
					},
				},
			}

			res, err := srv.RunTask(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.RunTaskResponse{
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "123_request_id",
						Target: "target",
					},
					UpdateId: 1,
				},
			}))

			assert.Loosely(t, psserver.Messages(), should.HaveLength(1))
			publishedMsg := psserver.Messages()[0]
			assert.Loosely(t, publishedMsg.Attributes["dummy_task_id"], should.Equal("123_request_id"))
			assert.Loosely(t, publishedMsg.Attributes["project"], should.BeEmpty)
			assert.Loosely(t, publishedMsg.Attributes["bucket"], should.BeEmpty)
			assert.Loosely(t, publishedMsg.Attributes["builder"], should.BeEmpty)
		})

		t.Run("duplicate req", func(t *ftt.Test) {
			cache := caching.GlobalCache(ctx, "taskbackendlite-run-task")
			err := cache.Set(ctx, "123_request_id", []byte{1}, DefaultTaskCreationTimeout)
			assert.Loosely(t, err, should.BeNil)

			res, err := srv.RunTask(ctx, req)

			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, res, should.Resemble(&pb.RunTaskResponse{
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "123_request_id",
						Target: "target",
					},
					UpdateId: 1,
				},
			}))
			assert.Loosely(t, psserver.Messages(), should.HaveLength(0))
		})

		t.Run("topic not exist", func(t *ftt.Test) {
			err := myTopic.Delete(ctx)
			assert.Loosely(t, err, should.BeNil)
			res, err := srv.RunTask(ctx, req)
			assert.Loosely(t, res, should.BeNil)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, err, should.ErrLike("topic taskbackendlite-myProject does not exist on Cloud project myApp-dev"))
		})

		t.Run("perm errors", func(t *ftt.Test) {
			t.Run("no access", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:             identity.Identity("project:myProject"),
					PeerIdentityOverride: identity.Identity("user:user1@example.com"),
				})
				res, err := srv.RunTask(ctx, req)
				assert.Loosely(t, res, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`the peer "user:user1@example.com" is not allowed to access this task backend`))
			})

			t.Run("not a project identity", func(t *ftt.Test) {
				ctx = auth.WithState(ctx, &authtest.FakeState{
					Identity:             identity.Identity("user:user1"),
					PeerIdentityOverride: identity.Identity("user:cr-buildbucket-dev@appspot.gserviceaccount.com"),
				})
				res, err := srv.RunTask(ctx, req)
				assert.Loosely(t, res, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike(`The caller's user identity "user:user1" is not a project identity`))
			})
		})
	})
}
