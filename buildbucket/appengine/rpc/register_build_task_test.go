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

	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func validRegisterBuildTaskRequest() *pb.RegisterBuildTaskRequest {
	return &pb.RegisterBuildTaskRequest{
		RequestId: "random",
		BuildId:   87654321,
		Task: &pb.Task{
			Id: &pb.TaskID{
				Target: "swarming://swarming-host",
				Id:     "deadbeef",
			},
		},
	}
}

func TestValidateRegisterBuildTaskRequest(t *testing.T) {
	t.Parallel()
	Convey("validateRegisterBuildTaskRequest", t, func() {
		ctx := context.Background()

		Convey("empty req", func() {
			err := validateRegisterBuildTaskRequest(ctx, &pb.RegisterBuildTaskRequest{})
			So(err, ShouldErrLike, `.request_id: required`)
		})

		Convey("empty task", func() {
			req := &pb.RegisterBuildTaskRequest{
				RequestId: "random",
				BuildId:   87654321,
				Task:      &pb.Task{},
			}
			err := validateRegisterBuildTaskRequest(ctx, req)
			So(err, ShouldErrLike, `.task.id: required`)
		})

		Convey("empty task id", func() {
			req := &pb.RegisterBuildTaskRequest{
				RequestId: "random",
				BuildId:   87654321,
				Task: &pb.Task{
					Id: &pb.TaskID{},
				},
			}
			err := validateRegisterBuildTaskRequest(ctx, req)
			So(err, ShouldErrLike, `.task.id.target: required`)
		})

		Convey("pass", func() {
			req := validRegisterBuildTaskRequest()
			err := validateRegisterBuildTaskRequest(ctx, req)
			So(err, ShouldBeNil)
		})
	})
}

func TestRegisterBuildTask(t *testing.T) {
	srv := &Builds{}
	ctx := memory.Use(context.Background())
	req := validRegisterBuildTaskRequest()
	Convey("validate token", t, func() {
		Convey("token missing", func() {
			_, err := srv.RegisterBuildTask(ctx, req)
			So(err, ShouldErrLike, `token is missing`)
		})

		Convey("wrong purpose", func() {
			tk, _ := buildtoken.GenerateToken(ctx, 87654321, pb.TokenBody_TASK)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk))
			_, err := srv.RegisterBuildTask(ctx, req)
			So(err, ShouldErrLike, `token is for purpose TASK, but expected REGISTER_TASK`)
		})

		Convey("wrong build id", func() {
			tk, _ := buildtoken.GenerateToken(ctx, 1, pb.TokenBody_REGISTER_TASK)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk))
			_, err := srv.RegisterBuildTask(ctx, req)
			So(err, ShouldErrLike, `token is for build 1, but expected 87654321`)
		})
	})

	Convey("RegisterBuildTask", t, func() {
		tk, _ := buildtoken.GenerateToken(ctx, 87654321, pb.TokenBody_REGISTER_TASK)
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
			_, err := srv.RegisterBuildTask(ctx, req)
			So(err, ShouldErrLike, `the build 87654321 does not run on task backend`)
		})

		Convey("first RegisterBuildTask", func() {
			Convey("different target as required", func() {
				infra.Proto.Backend = &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming",
						},
					},
				}
				So(datastore.Put(ctx, infra), ShouldBeNil)
				_, err := srv.RegisterBuildTask(ctx, req)
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
				res, err := srv.RegisterBuildTask(ctx, req)
				So(err, ShouldBeNil)

				build, err = getBuild(ctx, 87654321)
				So(err, ShouldBeNil)
				So(build.BackendTaskToken, ShouldEqual, res.UpdateBuildTaskToken)
				So(build.RegisterTaskRequestID, ShouldEqual, req.RequestId)

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
					_, err := srv.RegisterBuildTask(ctx, req)
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
					res, err := srv.RegisterBuildTask(ctx, req)
					So(err, ShouldBeNil)

					build, err = getBuild(ctx, 87654321)
					So(err, ShouldBeNil)
					So(res.UpdateBuildTaskToken, ShouldEqual, build.BackendTaskToken)
					So(build.RegisterTaskRequestID, ShouldEqual, req.RequestId)

					err = datastore.Get(ctx, infra)
					So(err, ShouldBeNil)
					So(infra.Proto.Backend.Task, ShouldResembleProto, req.Task)
				})
			})
		})

		Convey("subsequent RegisterBuildTask", func() {
			Convey("duplicate task", func() {
				build.RegisterTaskRequestID = "other request"
				infra.Proto.Backend = &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://swarming-host",
							Id:     "another",
						},
					},
				}
				So(datastore.Put(ctx, []any{build, infra}), ShouldBeNil)

				_, err := srv.RegisterBuildTask(ctx, req)
				So(err, ShouldErrLike, `build 87654321 has recorded another RegisterBuildTask with request id "other request"`)
				So(buildbucket.DuplicateTask.In(err), ShouldBeTrue)
			})

			Convey("task with collided request id", func() {
				build.RegisterTaskRequestID = req.RequestId
				var err error
				tok, err := buildtoken.GenerateToken(ctx, build.ID, pb.TokenBody_TASK)
				So(err, ShouldBeNil)
				build.BackendTaskToken = tok
				infra.Proto.Backend = &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://swarming-host",
							Id:     "another",
						},
					},
				}
				So(datastore.Put(ctx, []any{build, infra}), ShouldBeNil)

				_, err = srv.RegisterBuildTask(ctx, req)
				So(err, ShouldErrLike, `another`)
				So(buildbucket.TaskWithCollidedRequestID.In(err), ShouldBeTrue)
			})

			Convey("idempotent", func() {
				build.RegisterTaskRequestID = req.RequestId
				var err error
				tok, err := buildtoken.GenerateToken(ctx, build.ID, pb.TokenBody_TASK)
				So(err, ShouldBeNil)
				build.BackendTaskToken = tok
				infra.Proto.Backend = &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://swarming-host",
							Id:     req.Task.Id.Id,
						},
					},
				}
				So(datastore.Put(ctx, []any{build, infra}), ShouldBeNil)

				res, err := srv.RegisterBuildTask(ctx, req)
				So(err, ShouldBeNil)
				So(res, ShouldResembleProto, &pb.RegisterBuildTaskResponse{UpdateBuildTaskToken: tok})
			})
		})
	})
}
