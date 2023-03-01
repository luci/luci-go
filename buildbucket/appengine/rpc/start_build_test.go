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
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func validStartBuildRequest() *pb.StartBuildRequest {
	return &pb.StartBuildRequest{
		RequestId: "random",
		BuildId:   87654321,
		TaskId:    "deadbeef",
	}
}

func TestValidateStartBuildRequest(t *testing.T) {
	t.Parallel()
	Convey("validateStartBuildRequest", t, func() {
		ctx := context.Background()

		Convey("empty req", func() {
			err := validateStartBuildRequest(ctx, &pb.StartBuildRequest{})
			So(err, ShouldErrLike, `.request_id: required`)
		})

		Convey("missing build id", func() {
			req := &pb.StartBuildRequest{
				RequestId: "random",
			}
			err := validateStartBuildRequest(ctx, req)
			So(err, ShouldErrLike, `.build_id: required`)
		})

		Convey("missing task id", func() {
			req := &pb.StartBuildRequest{
				RequestId: "random",
				BuildId:   87654321,
			}
			err := validateStartBuildRequest(ctx, req)
			So(err, ShouldErrLike, `.task_id: required`)
		})

		Convey("pass", func() {
			req := validStartBuildRequest()
			err := validateStartBuildRequest(ctx, req)
			So(err, ShouldBeNil)
		})
	})
}

func TestStartBuild(t *testing.T) {
	srv := &Builds{}
	ctx := memory.Use(context.Background())
	req := validStartBuildRequest()
	Convey("validate token", t, func() {
		Convey("token missing", func() {
			_, err := srv.StartBuild(ctx, req)
			So(err, ShouldErrLike, `token is missing`)
		})

		Convey("wrong purpose", func() {
			tk, _ := buildtoken.GenerateToken(ctx, 87654321, pb.TokenBody_TASK)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk))
			_, err := srv.StartBuild(ctx, req)
			So(err, ShouldErrLike, `token is for purpose TASK, but expected START_BUILD`)
		})

		Convey("wrong build id", func() {
			tk, _ := buildtoken.GenerateToken(ctx, 1, pb.TokenBody_START_BUILD)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk))
			_, err := srv.StartBuild(ctx, req)
			So(err, ShouldErrLike, `token is for build 1, but expected 87654321`)
		})
	})

	Convey("StartBuild", t, func() {
		tk, _ := buildtoken.GenerateToken(ctx, 87654321, pb.TokenBody_START_BUILD)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk))
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")

		build := &model.Build{
			ID: 87654321,
			Proto: &pb.Build{
				Id: 87654321,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_SCHEDULED,
			},
			Status: pb.Status_SCHEDULED,
		}
		bk := datastore.KeyForObj(ctx, build)
		infra := &model.BuildInfra{
			Build: bk,
			Proto: &pb.BuildInfra{},
		}
		So(datastore.Put(ctx, build, infra), ShouldBeNil)

		Convey("build on backend", func() {

			Convey("build not on backend", func() {
				_, err := srv.StartBuild(ctx, req)
				So(err, ShouldErrLike, `the build 87654321 does not run on task backend`)
			})

			Convey("first StartBuild", func() {
				Convey("first handshake", func() {
					infra.Proto.Backend = &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Target: "swarming://swarming-host",
							},
						},
					}
					So(datastore.Put(ctx, infra), ShouldBeNil)
					res, err := srv.StartBuild(ctx, req)
					So(err, ShouldBeNil)

					build, err = getBuild(ctx, 87654321)
					So(err, ShouldBeNil)
					So(build.UpdateToken, ShouldEqual, res.UpdateBuildToken)
					So(build.StartBuildRequestID, ShouldEqual, req.RequestId)
					So(build.Status, ShouldEqual, pb.Status_STARTED)

					err = datastore.Get(ctx, infra)
					So(err, ShouldBeNil)
					So(infra.Proto.Backend.Task.Id.Id, ShouldEqual, req.TaskId)
				})

				Convey("same task", func() {
					infra.Proto.Backend = &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Target: "swarming://swarming-host",
								Id:     req.TaskId,
							},
						},
					}
					So(datastore.Put(ctx, infra), ShouldBeNil)
					res, err := srv.StartBuild(ctx, req)
					So(err, ShouldBeNil)

					build, err = getBuild(ctx, 87654321)
					So(err, ShouldBeNil)
					So(build.UpdateToken, ShouldEqual, res.UpdateBuildToken)
					So(build.StartBuildRequestID, ShouldEqual, req.RequestId)
					So(build.Status, ShouldEqual, pb.Status_STARTED)
				})

				Convey("after RegisterBuildTask", func() {
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
						_, err := srv.StartBuild(ctx, req)
						So(err, ShouldErrLike, `build 87654321 has associated with task "other"`)
						So(buildbucket.DuplicateTask.In(err), ShouldBeTrue)
						build, err = getBuild(ctx, 87654321)
						So(err, ShouldBeNil)
						So(build.UpdateToken, ShouldEqual, "")
						So(build.StartBuildRequestID, ShouldEqual, "")
						So(build.Status, ShouldEqual, pb.Status_SCHEDULED)
					})

					Convey("same task", func() {
						infra.Proto.Backend = &pb.BuildInfra_Backend{
							Task: &pb.Task{
								Id: &pb.TaskID{
									Target: "swarming://swarming-host",
									Id:     req.TaskId,
								},
							},
						}
						So(datastore.Put(ctx, infra), ShouldBeNil)
						res, err := srv.StartBuild(ctx, req)
						So(err, ShouldBeNil)

						build, err = getBuild(ctx, 87654321)
						So(err, ShouldBeNil)
						So(build.UpdateToken, ShouldEqual, res.UpdateBuildToken)
						So(build.StartBuildRequestID, ShouldEqual, req.RequestId)
						So(build.Status, ShouldEqual, pb.Status_STARTED)
					})
				})
			})

			Convey("subsequent StartBuild", func() {
				Convey("duplicate task", func() {
					build.StartBuildRequestID = "other request"
					infra.Proto.Backend = &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Target: "swarming://swarming-host",
								Id:     "another",
							},
						},
					}
					So(datastore.Put(ctx, []any{build, infra}), ShouldBeNil)

					_, err := srv.StartBuild(ctx, req)
					So(err, ShouldErrLike, `build 87654321 has recorded another StartBuild with request id "other request"`)
					So(buildbucket.DuplicateTask.In(err), ShouldBeTrue)
				})

				Convey("task with collided request id", func() {
					build.StartBuildRequestID = req.RequestId
					var err error
					tok, err := buildtoken.GenerateToken(ctx, build.ID, pb.TokenBody_TASK)
					So(err, ShouldBeNil)
					build.UpdateToken = tok
					infra.Proto.Backend = &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Target: "swarming://swarming-host",
								Id:     "another",
							},
						},
					}
					So(datastore.Put(ctx, []any{build, infra}), ShouldBeNil)

					_, err = srv.StartBuild(ctx, req)
					So(err, ShouldErrLike, `build 87654321 has associated with task id "another" with StartBuild request id "random"`)
					So(buildbucket.TaskWithCollidedRequestID.In(err), ShouldBeTrue)
				})

				Convey("idempotent", func() {
					build.StartBuildRequestID = req.RequestId
					var err error
					tok, err := buildtoken.GenerateToken(ctx, build.ID, pb.TokenBody_TASK)
					So(err, ShouldBeNil)
					build.UpdateToken = tok
					infra.Proto.Backend = &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Target: "swarming://swarming-host",
								Id:     req.TaskId,
							},
						},
					}
					So(datastore.Put(ctx, []any{build, infra}), ShouldBeNil)

					res, err := srv.StartBuild(ctx, req)
					So(err, ShouldBeNil)
					So(res.UpdateBuildToken, ShouldEqual, tok)
				})
			})
		})
	})
}
