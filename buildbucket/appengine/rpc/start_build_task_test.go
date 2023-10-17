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

package rpc

import (
	"context"
	"testing"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/common"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
	"google.golang.org/grpc/metadata"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func validStartBuildTaskRequest() *pb.StartBuildTaskRequest {
	return &pb.StartBuildTaskRequest{
		RequestId: "random",
		BuildId:   87654321,
		Task: &pb.Task{
			Id: &pb.TaskID{
				Target: "swarming://swarming-host",
				Id:     "deadbeef",
			},
			Link:     "www.wwwdotcom.com",
			Status:   pb.Status_STARTED,
			UpdateId: 1234,
		},
	}
}

func TestValidateStartBuildTaskRequest(t *testing.T) {
	t.Parallel()
	Convey("validateStartBuildTaskRequest", t, func() {
		ctx := context.Background()

		Convey("empty req", func() {
			err := validateStartBuildTaskRequest(ctx, &pb.StartBuildTaskRequest{})
			So(err, ShouldErrLike, `.request_id: required`)
		})

		Convey("empty task", func() {
			req := &pb.StartBuildTaskRequest{
				RequestId: "random",
				BuildId:   87654321,
				Task:      &pb.Task{},
			}
			err := validateStartBuildTaskRequest(ctx, req)
			So(err, ShouldErrLike, `.task.id: required`)
		})

		Convey("empty task id", func() {
			req := &pb.StartBuildTaskRequest{
				RequestId: "random",
				BuildId:   87654321,
				Task: &pb.Task{
					Id: &pb.TaskID{},
				},
			}
			err := validateStartBuildTaskRequest(ctx, req)
			So(err, ShouldErrLike, `.task.id.target: required`)
		})

		Convey("empty updateId", func() {
			req := &pb.StartBuildTaskRequest{
				RequestId: "random",
				BuildId:   87654321,
				Task: &pb.Task{
					Id: &pb.TaskID{
						Target: "swarming://swarming-host",
						Id:     "deadbeef",
					},
					Link:   "www.wwwdotcom.com",
					Status: pb.Status_STARTED,
				},
			}
			err := validateStartBuildTaskRequest(ctx, req)
			So(err, ShouldErrLike, `.task.update_id: required`)
		})

		Convey("empty status", func() {
			req := &pb.StartBuildTaskRequest{
				RequestId: "random",
				BuildId:   87654321,
				Task: &pb.Task{
					Id: &pb.TaskID{
						Target: "swarming://swarming-host",
						Id:     "deadbeef",
					},
					Link: "www.wwwdotcom.com",
				},
			}
			err := validateStartBuildTaskRequest(ctx, req)
			So(err, ShouldErrLike, `.task.status: required`)
		})

		Convey("pass", func() {
			req := validStartBuildTaskRequest()
			err := validateStartBuildTaskRequest(ctx, req)
			So(err, ShouldBeNil)
		})
	})
}

func TestStartBuildTask(t *testing.T) {
	srv := &Builds{}
	ctx := memory.Use(context.Background())
	store := &testsecrets.Store{
		Secrets: map[string]secrets.Secret{
			"key": {Active: []byte("stuff")},
		},
	}
	ctx = secrets.Use(ctx, store)
	ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

	req := validStartBuildTaskRequest()
	Convey("validate token", t, func() {
		Convey("token missing", func() {
			_, err := srv.StartBuildTask(ctx, req)
			So(err, ShouldErrLike, errBadTokenAuth)
		})

		Convey("wrong purpose", func() {
			tk, _ := buildtoken.GenerateToken(ctx, 87654321, pb.TokenBody_BUILD)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk))
			_, err := srv.StartBuildTask(ctx, req)
			So(err, ShouldErrLike, errBadTokenAuth)
		})

		Convey("wrong build id", func() {
			tk, _ := buildtoken.GenerateToken(ctx, 1, pb.TokenBody_START_BUILD_TASK)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk))
			_, err := srv.StartBuildTask(ctx, req)
			So(err, ShouldErrLike, errBadTokenAuth)
		})
	})

	Convey("StartBuildTask", t, func() {
		tk, _ := buildtoken.GenerateToken(ctx, 87654321, pb.TokenBody_START_BUILD_TASK)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk))

		build := &model.Build{
			ID: 87654321,
			Proto: &pb.Build{
				Id: 87654321,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
			},
		}
		bk := datastore.KeyForObj(ctx, build)
		infra := &model.BuildInfra{
			Build: bk,
			Proto: &pb.BuildInfra{},
		}
		So(datastore.Put(ctx, build, infra), ShouldBeNil)

		Convey("build not on backend", func() {
			_, err := srv.StartBuildTask(ctx, req)
			So(err, ShouldErrLike, `the build 87654321 does not run on task backend`)
		})

		Convey("first StartBuildTask", func() {
			Convey("different target as required", func() {
				infra.Proto.Backend = &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming",
						},
					},
				}
				So(datastore.Put(ctx, infra), ShouldBeNil)
				_, err := srv.StartBuildTask(ctx, req)
				So(err, ShouldErrLike, `build 87654321 requires task target "swarming", got "swarming://swarming-host"`)
			})

			Convey("first handshake", func() {
				infra.Proto.Backend = &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://swarming-host",
						},
					},
				}
				So(datastore.Put(ctx, infra), ShouldBeNil)
				res, err := srv.StartBuildTask(ctx, req)
				So(err, ShouldBeNil)

				build, err = common.GetBuild(ctx, 87654321)
				So(err, ShouldBeNil)
				So(build.StartBuildToken, ShouldEqual, res.Secrets.StartBuildToken)
				So(build.StartBuildTaskRequestID, ShouldEqual, req.RequestId)

				err = datastore.Get(ctx, infra)
				So(err, ShouldBeNil)
				So(infra.Proto.Backend.Task, ShouldResembleProto, req.Task)
			})

			Convey("after StartBuild", func() {
				Convey("duplicated task", func() {
					infra.Proto.Backend = &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Target: "swarming://swarming-host",
								Id:     "other",
							},
						},
					}
					So(datastore.Put(ctx, infra), ShouldBeNil)
					_, err := srv.StartBuildTask(ctx, req)
					So(err, ShouldErrLike, `build 87654321 has associated with task "other"`)
					So(buildbucket.DuplicateTask.In(err), ShouldBeTrue)
				})

				Convey("same task", func() {
					infra.Proto.Backend = &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Target: "swarming://swarming-host",
								Id:     req.Task.Id.Id,
							},
						},
					}
					So(datastore.Put(ctx, infra), ShouldBeNil)
					res, err := srv.StartBuildTask(ctx, req)
					So(err, ShouldBeNil)

					build, err = common.GetBuild(ctx, 87654321)
					So(err, ShouldBeNil)
					So(res.Secrets.StartBuildToken, ShouldEqual, build.StartBuildToken)
					So(build.StartBuildTaskRequestID, ShouldEqual, req.RequestId)

					err = datastore.Get(ctx, infra)
					So(err, ShouldBeNil)
					So(infra.Proto.Backend.Task, ShouldResembleProto, req.Task)
				})
			})
		})

		Convey("subsequent StartBuildTask", func() {
			Convey("duplicate task", func() {
				build.StartBuildTaskRequestID = "other request"
				infra.Proto.Backend = &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://swarming-host",
							Id:     "another",
						},
					},
				}
				So(datastore.Put(ctx, []any{build, infra}), ShouldBeNil)

				_, err := srv.StartBuildTask(ctx, req)
				So(err, ShouldErrLike, `build 87654321 has recorded another StartBuildTask with request id "other request"`)
				So(buildbucket.DuplicateTask.In(err), ShouldBeTrue)
			})

			Convey("task with collided request id", func() {
				build.StartBuildTaskRequestID = req.RequestId
				var err error
				tok, err := buildtoken.GenerateToken(ctx, build.ID, pb.TokenBody_TASK)
				So(err, ShouldBeNil)
				build.StartBuildToken = tok
				infra.Proto.Backend = &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://swarming-host",
							Id:     "another",
						},
					},
				}
				So(datastore.Put(ctx, []any{build, infra}), ShouldBeNil)

				_, err = srv.StartBuildTask(ctx, req)
				So(err, ShouldErrLike, `another`)
				So(buildbucket.TaskWithCollidedRequestID.In(err), ShouldBeTrue)
			})

			Convey("idempotent", func() {
				build.StartBuildTaskRequestID = req.RequestId
				var err error
				tok, err := buildtoken.GenerateToken(ctx, build.ID, pb.TokenBody_START_BUILD_TASK)
				So(err, ShouldBeNil)
				build.StartBuildToken = tok
				infra.Proto.Backend = &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://swarming-host",
							Id:     req.Task.Id.Id,
						},
					},
				}
				So(datastore.Put(ctx, []any{build, infra}), ShouldBeNil)

				res, err := srv.StartBuildTask(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.StartBuildTaskResponse{Secrets: &pb.BuildSecrets{StartBuildToken: tok}})
			})
		})
	})
}
