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
	"time"

	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/common"
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
	store := &testsecrets.Store{
		Secrets: map[string]secrets.Secret{
			"key": {Active: []byte("stuff")},
		},
	}
	ctx = secrets.Use(ctx, store)
	ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

	req := validStartBuildRequest()
	Convey("validate token", t, func() {
		Convey("token missing", func() {
			_, err := srv.StartBuild(ctx, req)
			So(err, ShouldErrLike, errBadTokenAuth)
		})

		Convey("wrong purpose", func() {
			tk, err := buildtoken.GenerateToken(ctx, 87654321, pb.TokenBody_TASK)
			So(err, ShouldBeNil)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk))
			_, err = srv.StartBuild(ctx, req)
			So(err, ShouldErrLike, buildtoken.ErrBadToken)
		})

		Convey("wrong build id", func() {
			tk, _ := buildtoken.GenerateToken(ctx, 1, pb.TokenBody_START_BUILD)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk))
			_, err := srv.StartBuild(ctx, req)
			So(err, ShouldErrLike, buildtoken.ErrBadToken)
		})
	})

	Convey("StartBuild", t, func() {
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx = metrics.WithBuilder(ctx, "project", "bucket", "builder")
		base := pb.CustomMetricDefinitionBase_CUSTOM_BUILD_METRIC_BASE_STARTED
		name := "chrome/infra/custom/builds/started"
		globalCfg := &pb.SettingsCfg{
			CustomMetrics: []*pb.CustomMetric{
				{
					Name:   name,
					Fields: []string{"experiments", "os"},
					Class: &pb.CustomMetric_MetricBase{
						MetricBase: base,
					},
				},
			},
		}
		ctx, _ = metrics.WithCustomMetrics(ctx, globalCfg)
		ctx = txndefer.FilterRDS(ctx)
		var sch *tqtesting.Scheduler
		ctx, sch = tq.TestingContext(ctx, nil)

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
			Tags:   []string{"os:Linux"},
			CustomMetrics: []model.CustomMetric{
				{
					Base: base,
					Metric: &pb.CustomMetricDefinition{
						Name:       name,
						Predicates: []string{`build.tags.get_value("os")!=""`},
						Fields: map[string]string{
							"experiments": "build.input.experiments.to_string()",
							"os":          `build.tags.get_value("os")`,
						},
					},
				},
			},
		}
		bk := datastore.KeyForObj(ctx, build)
		infra := &model.BuildInfra{
			Build: bk,
			Proto: &pb.BuildInfra{},
		}
		bs := &model.BuildStatus{
			Build:  bk,
			Status: pb.Status_SCHEDULED,
		}
		So(datastore.Put(ctx, build, infra, bs), ShouldBeNil)

		Convey("build on backend", func() {
			tk, _ := buildtoken.GenerateToken(ctx, 87654321, pb.TokenBody_START_BUILD)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk))

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

					err = datastore.Get(ctx, build, bs)
					So(err, ShouldBeNil)
					So(build.UpdateToken, ShouldEqual, res.UpdateBuildToken)
					So(build.StartBuildRequestID, ShouldEqual, req.RequestId)
					So(build.Status, ShouldEqual, pb.Status_STARTED)
					So(bs.Status, ShouldEqual, pb.Status_STARTED)

					err = datastore.Get(ctx, infra)
					So(err, ShouldBeNil)
					So(infra.Proto.Backend.Task.Id.Id, ShouldEqual, req.TaskId)

					// TQ tasks for pubsub-notification.
					tasks := sch.Tasks()
					So(tasks, ShouldHaveLength, 2)

					// metrics
					So(tsmon.Store(ctx).Get(ctx, metrics.V2.BuildCountStarted, time.Time{}, []any{"None"}), ShouldEqual, 1)
					val, err := metrics.GetCustomMetricsData(ctx, base, name, time.Time{}, []any{"None", "Linux"})
					So(err, ShouldBeNil)
					So(val, ShouldEqual, 1)
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

					build, err = common.GetBuild(ctx, 87654321)
					So(err, ShouldBeNil)
					So(build.UpdateToken, ShouldEqual, res.UpdateBuildToken)
					So(build.StartBuildRequestID, ShouldEqual, req.RequestId)
					So(build.Status, ShouldEqual, pb.Status_STARTED)

					// TQ tasks for pubsub-notification.
					tasks := sch.Tasks()
					So(tasks, ShouldHaveLength, 2)
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
						build, err = common.GetBuild(ctx, 87654321)
						So(err, ShouldBeNil)
						So(build.UpdateToken, ShouldEqual, "")
						So(build.StartBuildRequestID, ShouldEqual, "")
						So(build.Status, ShouldEqual, pb.Status_SCHEDULED)

						// TQ tasks for pubsub-notification.
						tasks := sch.Tasks()
						So(tasks, ShouldHaveLength, 0)
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

						build, err = common.GetBuild(ctx, 87654321)
						So(err, ShouldBeNil)
						So(build.UpdateToken, ShouldEqual, res.UpdateBuildToken)
						So(build.StartBuildRequestID, ShouldEqual, req.RequestId)
						So(build.Status, ShouldEqual, pb.Status_STARTED)
					})
				})

				Convey("build has started", func() {
					infra.Proto.Backend = &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Target: "swarming://swarming-host",
							},
						},
					}
					build.Proto.Status = pb.Status_STARTED
					So(datastore.Put(ctx, infra, build), ShouldBeNil)
					_, err := srv.StartBuild(ctx, req)
					So(err, ShouldErrLike, `cannot start started build`)
				})

				Convey("build has ended", func() {
					infra.Proto.Backend = &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Target: "swarming://swarming-host",
							},
						},
					}
					build.Proto.Status = pb.Status_FAILURE
					So(datastore.Put(ctx, infra, build), ShouldBeNil)
					_, err := srv.StartBuild(ctx, req)
					So(err, ShouldErrLike, `cannot start ended build`)
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
					tok, err := buildtoken.GenerateToken(ctx, build.ID, pb.TokenBody_BUILD)
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
					tok, err := buildtoken.GenerateToken(ctx, build.ID, pb.TokenBody_BUILD)
					So(err, ShouldBeNil)
					build.UpdateToken = tok
					build.Proto.Status = pb.Status_STARTED
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

		Convey("build on swarming", func() {
			Convey("build token missing", func() {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, "I am a potato"))
				_, err := srv.StartBuild(ctx, req)
				So(err, ShouldErrLike, buildtoken.ErrBadToken)
			})

			Convey("build token mismatch", func() {
				tk, err := buildtoken.GenerateToken(ctx, 123456, pb.TokenBody_BUILD)
				So(err, ShouldBeNil)
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk))

				_, err = srv.StartBuild(ctx, req)
				So(err, ShouldErrLike, buildtoken.ErrBadToken)
			})

			Convey("StartBuild", func() {
				tk, err := buildtoken.GenerateToken(ctx, 87654321, pb.TokenBody_BUILD)
				So(err, ShouldBeNil)
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk))
				build.UpdateToken = tk
				bs := &model.BuildStatus{
					Build:  datastore.KeyForObj(ctx, build),
					Status: pb.Status_SCHEDULED,
				}
				So(datastore.Put(ctx, build, bs), ShouldBeNil)
				Convey("build not on swarming", func() {
					_, err := srv.StartBuild(ctx, req)
					So(err, ShouldErrLike, `the build 87654321 does not run on swarming`)
				})

				Convey("first StartBuild", func() {
					Convey("first handshake", func() {
						infra.Proto.Swarming = &pb.BuildInfra_Swarming{
							TaskId: req.TaskId,
						}
						So(datastore.Put(ctx, infra), ShouldBeNil)
						res, err := srv.StartBuild(ctx, req)
						So(err, ShouldBeNil)

						err = datastore.Get(ctx, build, bs)
						So(err, ShouldBeNil)
						So(build.UpdateToken, ShouldEqual, res.UpdateBuildToken)
						So(build.StartBuildRequestID, ShouldEqual, req.RequestId)
						So(build.Status, ShouldEqual, pb.Status_STARTED)
						So(bs.Status, ShouldEqual, pb.Status_STARTED)

						// TQ tasks for pubsub-notification.
						tasks := sch.Tasks()
						So(tasks, ShouldHaveLength, 2)
					})

					Convey("first handshake with no task id in datastore", func() {
						infra.Proto.Swarming = &pb.BuildInfra_Swarming{}
						So(datastore.Put(ctx, infra), ShouldBeNil)
						res, err := srv.StartBuild(ctx, req)
						So(err, ShouldBeNil)

						err = datastore.Get(ctx, build, bs, infra)
						So(err, ShouldBeNil)
						So(build.UpdateToken, ShouldEqual, res.UpdateBuildToken)
						So(build.StartBuildRequestID, ShouldEqual, req.RequestId)
						So(build.Status, ShouldEqual, pb.Status_STARTED)
						So(bs.Status, ShouldEqual, pb.Status_STARTED)
						So(infra.Proto.Swarming.TaskId, ShouldEqual, req.TaskId)

						// TQ tasks for pubsub-notification.
						tasks := sch.Tasks()
						So(tasks, ShouldHaveLength, 2)
					})

					Convey("duplicated task", func() {
						infra.Proto.Swarming = &pb.BuildInfra_Swarming{
							TaskId: "another",
						}
						So(datastore.Put(ctx, infra), ShouldBeNil)
						_, err := srv.StartBuild(ctx, req)
						So(err, ShouldErrLike, `build 87654321 has associated with task "another"`)
						So(buildbucket.DuplicateTask.In(err), ShouldBeTrue)

						// TQ tasks for pubsub-notification.
						tasks := sch.Tasks()
						So(tasks, ShouldHaveLength, 0)
					})

					Convey("build has started", func() {
						infra.Proto.Swarming = &pb.BuildInfra_Swarming{
							TaskId: req.TaskId,
						}
						build.Proto.Status = pb.Status_STARTED
						So(datastore.Put(ctx, infra, build), ShouldBeNil)
						res, err := srv.StartBuild(ctx, req)
						So(err, ShouldBeNil)
						So(res.UpdateBuildToken, ShouldEqual, build.UpdateToken)
						// TQ tasks for pubsub-notification.
						tasks := sch.Tasks()
						So(tasks, ShouldHaveLength, 0)
					})

					Convey("build has ended", func() {
						infra.Proto.Swarming = &pb.BuildInfra_Swarming{
							TaskId: req.TaskId,
						}
						build.Proto.Status = pb.Status_FAILURE
						So(datastore.Put(ctx, infra, build), ShouldBeNil)
						_, err := srv.StartBuild(ctx, req)
						So(err, ShouldErrLike, `cannot start ended build`)
					})
				})

				Convey("subsequent StartBuild", func() {
					Convey("duplicate task", func() {
						build.StartBuildRequestID = "other request"
						infra.Proto.Swarming = &pb.BuildInfra_Swarming{
							TaskId: "another",
						}
						So(datastore.Put(ctx, []any{build, infra}), ShouldBeNil)

						_, err := srv.StartBuild(ctx, req)
						So(err, ShouldErrLike, `build 87654321 has recorded another StartBuild with request id "other request"`)
						So(buildbucket.DuplicateTask.In(err), ShouldBeTrue)
					})

					Convey("task with collided request id", func() {
						build.StartBuildRequestID = req.RequestId
						infra.Proto.Swarming = &pb.BuildInfra_Swarming{
							TaskId: "another",
						}
						So(datastore.Put(ctx, []any{build, infra}), ShouldBeNil)

						_, err := srv.StartBuild(ctx, req)
						So(err, ShouldErrLike, `build 87654321 has associated with task id "another" with StartBuild request id "random"`)
						So(buildbucket.TaskWithCollidedRequestID.In(err), ShouldBeTrue)
					})

					Convey("idempotent", func() {
						build.StartBuildRequestID = req.RequestId
						build.Proto.Status = pb.Status_STARTED
						infra.Proto.Swarming = &pb.BuildInfra_Swarming{
							TaskId: req.TaskId,
						}
						So(datastore.Put(ctx, []any{build, infra}), ShouldBeNil)

						res, err := srv.StartBuild(ctx, req)
						So(err, ShouldBeNil)
						So(res.UpdateBuildToken, ShouldEqual, tk)
					})
				})
			})
		})
	})
}
