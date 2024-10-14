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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
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
	ftt.Run("validateStartBuildRequest", t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run("empty req", func(t *ftt.Test) {
			err := validateStartBuildRequest(ctx, &pb.StartBuildRequest{})
			assert.Loosely(t, err, should.ErrLike(`.request_id: required`))
		})

		t.Run("missing build id", func(t *ftt.Test) {
			req := &pb.StartBuildRequest{
				RequestId: "random",
			}
			err := validateStartBuildRequest(ctx, req)
			assert.Loosely(t, err, should.ErrLike(`.build_id: required`))
		})

		t.Run("missing task id", func(t *ftt.Test) {
			req := &pb.StartBuildRequest{
				RequestId: "random",
				BuildId:   87654321,
			}
			err := validateStartBuildRequest(ctx, req)
			assert.Loosely(t, err, should.ErrLike(`.task_id: required`))
		})

		t.Run("pass", func(t *ftt.Test) {
			req := validStartBuildRequest()
			err := validateStartBuildRequest(ctx, req)
			assert.Loosely(t, err, should.BeNil)
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
	ftt.Run("validate token", t, func(t *ftt.Test) {
		t.Run("token missing", func(t *ftt.Test) {
			_, err := srv.StartBuild(ctx, req)
			assert.Loosely(t, err, should.ErrLike(errBadTokenAuth))
		})

		t.Run("wrong purpose", func(t *ftt.Test) {
			tk, err := buildtoken.GenerateToken(ctx, 87654321, pb.TokenBody_TASK)
			assert.Loosely(t, err, should.BeNil)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk))
			_, err = srv.StartBuild(ctx, req)
			assert.Loosely(t, err, should.ErrLike(buildtoken.ErrBadToken))
		})

		t.Run("wrong build id", func(t *ftt.Test) {
			tk, _ := buildtoken.GenerateToken(ctx, 1, pb.TokenBody_START_BUILD)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk))
			_, err := srv.StartBuild(ctx, req)
			assert.Loosely(t, err, should.ErrLike(buildtoken.ErrBadToken))
		})
	})

	ftt.Run("StartBuild", t, func(t *ftt.Test) {
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx = metrics.WithBuilder(ctx, "project", "bucket", "builder")
		base := pb.CustomMetricBase_CUSTOM_METRIC_BASE_STARTED
		name := "chrome/infra/custom/builds/started"
		globalCfg := &pb.SettingsCfg{
			CustomMetrics: []*pb.CustomMetric{
				{
					Name:        name,
					ExtraFields: []string{"os"},
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
						ExtraFields: map[string]string{
							"os": `build.tags.get_value("os")`,
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
		assert.Loosely(t, datastore.Put(ctx, build, infra, bs), should.BeNil)

		t.Run("build on backend", func(t *ftt.Test) {
			tk, _ := buildtoken.GenerateToken(ctx, 87654321, pb.TokenBody_START_BUILD)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk))

			t.Run("build not on backend", func(t *ftt.Test) {
				_, err := srv.StartBuild(ctx, req)
				assert.Loosely(t, err, should.ErrLike(`the build 87654321 does not run on task backend`))
			})

			t.Run("first StartBuild", func(t *ftt.Test) {
				t.Run("first handshake", func(t *ftt.Test) {
					infra.Proto.Backend = &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Target: "swarming://swarming-host",
							},
						},
					}
					assert.Loosely(t, datastore.Put(ctx, infra), should.BeNil)
					res, err := srv.StartBuild(ctx, req)
					assert.Loosely(t, err, should.BeNil)

					err = datastore.Get(ctx, build, bs)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, build.UpdateToken, should.Equal(res.UpdateBuildToken))
					assert.Loosely(t, build.StartBuildRequestID, should.Equal(req.RequestId))
					assert.Loosely(t, build.Status, should.Equal(pb.Status_STARTED))
					assert.Loosely(t, bs.Status, should.Equal(pb.Status_STARTED))

					err = datastore.Get(ctx, infra)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, infra.Proto.Backend.Task.Id.Id, should.Equal(req.TaskId))

					// TQ tasks for pubsub-notification.
					tasks := sch.Tasks()
					assert.Loosely(t, tasks, should.HaveLength(1))

					// metrics
					assert.Loosely(t, tsmon.Store(ctx).Get(ctx, metrics.V2.BuildCountStarted, time.Time{}, []any{"None"}), should.Equal(1))
					val, err := metrics.GetCustomMetricsData(ctx, base, name, time.Time{}, []any{"Linux"})
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, val, should.Equal(1))
				})

				t.Run("same task", func(t *ftt.Test) {
					infra.Proto.Backend = &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Target: "swarming://swarming-host",
								Id:     req.TaskId,
							},
						},
					}
					assert.Loosely(t, datastore.Put(ctx, infra), should.BeNil)
					res, err := srv.StartBuild(ctx, req)
					assert.Loosely(t, err, should.BeNil)

					build, err = common.GetBuild(ctx, 87654321)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, build.UpdateToken, should.Equal(res.UpdateBuildToken))
					assert.Loosely(t, build.StartBuildRequestID, should.Equal(req.RequestId))
					assert.Loosely(t, build.Status, should.Equal(pb.Status_STARTED))

					// TQ tasks for pubsub-notification.
					tasks := sch.Tasks()
					assert.Loosely(t, tasks, should.HaveLength(1))
				})

				t.Run("after RegisterBuildTask", func(t *ftt.Test) {
					t.Run("duplicated task", func(t *ftt.Test) {
						infra.Proto.Backend = &pb.BuildInfra_Backend{
							Task: &pb.Task{
								Id: &pb.TaskID{
									Target: "swarming://swarming-host",
									Id:     "other",
								},
							},
						}
						assert.Loosely(t, datastore.Put(ctx, infra), should.BeNil)
						_, err := srv.StartBuild(ctx, req)
						assert.Loosely(t, err, should.ErrLike(`build 87654321 has associated with task "other"`))
						assert.Loosely(t, buildbucket.DuplicateTask.In(err), should.BeTrue)
						build, err = common.GetBuild(ctx, 87654321)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, build.UpdateToken, should.BeEmpty)
						assert.Loosely(t, build.StartBuildRequestID, should.BeEmpty)
						assert.Loosely(t, build.Status, should.Equal(pb.Status_SCHEDULED))

						// TQ tasks for pubsub-notification.
						tasks := sch.Tasks()
						assert.Loosely(t, tasks, should.HaveLength(0))
					})

					t.Run("same task", func(t *ftt.Test) {
						infra.Proto.Backend = &pb.BuildInfra_Backend{
							Task: &pb.Task{
								Id: &pb.TaskID{
									Target: "swarming://swarming-host",
									Id:     req.TaskId,
								},
							},
						}
						assert.Loosely(t, datastore.Put(ctx, infra), should.BeNil)
						res, err := srv.StartBuild(ctx, req)
						assert.Loosely(t, err, should.BeNil)

						build, err = common.GetBuild(ctx, 87654321)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, build.UpdateToken, should.Equal(res.UpdateBuildToken))
						assert.Loosely(t, build.StartBuildRequestID, should.Equal(req.RequestId))
						assert.Loosely(t, build.Status, should.Equal(pb.Status_STARTED))
					})
				})

				t.Run("build has started", func(t *ftt.Test) {
					infra.Proto.Backend = &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Target: "swarming://swarming-host",
							},
						},
					}
					build.Proto.Status = pb.Status_STARTED
					assert.Loosely(t, datastore.Put(ctx, infra, build), should.BeNil)
					_, err := srv.StartBuild(ctx, req)
					assert.Loosely(t, err, should.ErrLike(`cannot start started build`))
				})

				t.Run("build has ended", func(t *ftt.Test) {
					infra.Proto.Backend = &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Target: "swarming://swarming-host",
							},
						},
					}
					build.Proto.Status = pb.Status_FAILURE
					assert.Loosely(t, datastore.Put(ctx, infra, build), should.BeNil)
					_, err := srv.StartBuild(ctx, req)
					assert.Loosely(t, err, should.ErrLike(`cannot start ended build`))
				})
			})

			t.Run("subsequent StartBuild", func(t *ftt.Test) {
				t.Run("duplicate task", func(t *ftt.Test) {
					build.StartBuildRequestID = "other request"
					infra.Proto.Backend = &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Target: "swarming://swarming-host",
								Id:     "another",
							},
						},
					}
					assert.Loosely(t, datastore.Put(ctx, []any{build, infra}), should.BeNil)

					_, err := srv.StartBuild(ctx, req)
					assert.Loosely(t, err, should.ErrLike(`build 87654321 has recorded another StartBuild with request id "other request"`))
					assert.Loosely(t, buildbucket.DuplicateTask.In(err), should.BeTrue)
				})

				t.Run("task with collided request id", func(t *ftt.Test) {
					build.StartBuildRequestID = req.RequestId
					var err error
					tok, err := buildtoken.GenerateToken(ctx, build.ID, pb.TokenBody_BUILD)
					assert.Loosely(t, err, should.BeNil)
					build.UpdateToken = tok
					infra.Proto.Backend = &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Target: "swarming://swarming-host",
								Id:     "another",
							},
						},
					}
					assert.Loosely(t, datastore.Put(ctx, []any{build, infra}), should.BeNil)

					_, err = srv.StartBuild(ctx, req)
					assert.Loosely(t, err, should.ErrLike(`build 87654321 has associated with task id "another" with StartBuild request id "random"`))
					assert.Loosely(t, buildbucket.TaskWithCollidedRequestID.In(err), should.BeTrue)
				})

				t.Run("idempotent", func(t *ftt.Test) {
					build.StartBuildRequestID = req.RequestId
					var err error
					tok, err := buildtoken.GenerateToken(ctx, build.ID, pb.TokenBody_BUILD)
					assert.Loosely(t, err, should.BeNil)
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
					assert.Loosely(t, datastore.Put(ctx, []any{build, infra}), should.BeNil)

					res, err := srv.StartBuild(ctx, req)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, res.UpdateBuildToken, should.Equal(tok))
				})
			})
		})

		t.Run("build on swarming", func(t *ftt.Test) {
			t.Run("build token missing", func(t *ftt.Test) {
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, "I am a potato"))
				_, err := srv.StartBuild(ctx, req)
				assert.Loosely(t, err, should.ErrLike(buildtoken.ErrBadToken))
			})

			t.Run("build token mismatch", func(t *ftt.Test) {
				tk, err := buildtoken.GenerateToken(ctx, 123456, pb.TokenBody_BUILD)
				assert.Loosely(t, err, should.BeNil)
				ctx := metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk))

				_, err = srv.StartBuild(ctx, req)
				assert.Loosely(t, err, should.ErrLike(buildtoken.ErrBadToken))
			})

			t.Run("StartBuild", func(t *ftt.Test) {
				tk, err := buildtoken.GenerateToken(ctx, 87654321, pb.TokenBody_BUILD)
				assert.Loosely(t, err, should.BeNil)
				ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk))
				build.UpdateToken = tk
				bs := &model.BuildStatus{
					Build:  datastore.KeyForObj(ctx, build),
					Status: pb.Status_SCHEDULED,
				}
				assert.Loosely(t, datastore.Put(ctx, build, bs), should.BeNil)
				t.Run("build not on swarming", func(t *ftt.Test) {
					_, err := srv.StartBuild(ctx, req)
					assert.Loosely(t, err, should.ErrLike(`the build 87654321 does not run on swarming`))
				})

				t.Run("first StartBuild", func(t *ftt.Test) {
					t.Run("first handshake", func(t *ftt.Test) {
						infra.Proto.Swarming = &pb.BuildInfra_Swarming{
							TaskId: req.TaskId,
						}
						assert.Loosely(t, datastore.Put(ctx, infra), should.BeNil)
						res, err := srv.StartBuild(ctx, req)
						assert.Loosely(t, err, should.BeNil)

						err = datastore.Get(ctx, build, bs)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, build.UpdateToken, should.Equal(res.UpdateBuildToken))
						assert.Loosely(t, build.StartBuildRequestID, should.Equal(req.RequestId))
						assert.Loosely(t, build.Status, should.Equal(pb.Status_STARTED))
						assert.Loosely(t, bs.Status, should.Equal(pb.Status_STARTED))

						// TQ tasks for pubsub-notification.
						tasks := sch.Tasks()
						assert.Loosely(t, tasks, should.HaveLength(1))
					})

					t.Run("first handshake with no task id in datastore", func(t *ftt.Test) {
						infra.Proto.Swarming = &pb.BuildInfra_Swarming{}
						assert.Loosely(t, datastore.Put(ctx, infra), should.BeNil)
						res, err := srv.StartBuild(ctx, req)
						assert.Loosely(t, err, should.BeNil)

						err = datastore.Get(ctx, build, bs, infra)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, build.UpdateToken, should.Equal(res.UpdateBuildToken))
						assert.Loosely(t, build.StartBuildRequestID, should.Equal(req.RequestId))
						assert.Loosely(t, build.Status, should.Equal(pb.Status_STARTED))
						assert.Loosely(t, bs.Status, should.Equal(pb.Status_STARTED))
						assert.Loosely(t, infra.Proto.Swarming.TaskId, should.Equal(req.TaskId))

						// TQ tasks for pubsub-notification.
						tasks := sch.Tasks()
						assert.Loosely(t, tasks, should.HaveLength(1))
					})

					t.Run("duplicated task", func(t *ftt.Test) {
						infra.Proto.Swarming = &pb.BuildInfra_Swarming{
							TaskId: "another",
						}
						assert.Loosely(t, datastore.Put(ctx, infra), should.BeNil)
						_, err := srv.StartBuild(ctx, req)
						assert.Loosely(t, err, should.ErrLike(`build 87654321 has associated with task "another"`))
						assert.Loosely(t, buildbucket.DuplicateTask.In(err), should.BeTrue)

						// TQ tasks for pubsub-notification.
						tasks := sch.Tasks()
						assert.Loosely(t, tasks, should.HaveLength(0))
					})

					t.Run("build has started", func(t *ftt.Test) {
						infra.Proto.Swarming = &pb.BuildInfra_Swarming{
							TaskId: req.TaskId,
						}
						build.Proto.Status = pb.Status_STARTED
						assert.Loosely(t, datastore.Put(ctx, infra, build), should.BeNil)
						res, err := srv.StartBuild(ctx, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, res.UpdateBuildToken, should.Equal(build.UpdateToken))
						// TQ tasks for pubsub-notification.
						tasks := sch.Tasks()
						assert.Loosely(t, tasks, should.HaveLength(0))
					})

					t.Run("build has ended", func(t *ftt.Test) {
						infra.Proto.Swarming = &pb.BuildInfra_Swarming{
							TaskId: req.TaskId,
						}
						build.Proto.Status = pb.Status_FAILURE
						assert.Loosely(t, datastore.Put(ctx, infra, build), should.BeNil)
						_, err := srv.StartBuild(ctx, req)
						assert.Loosely(t, err, should.ErrLike(`cannot start ended build`))
					})
				})

				t.Run("subsequent StartBuild", func(t *ftt.Test) {
					t.Run("duplicate task", func(t *ftt.Test) {
						build.StartBuildRequestID = "other request"
						infra.Proto.Swarming = &pb.BuildInfra_Swarming{
							TaskId: "another",
						}
						assert.Loosely(t, datastore.Put(ctx, []any{build, infra}), should.BeNil)

						_, err := srv.StartBuild(ctx, req)
						assert.Loosely(t, err, should.ErrLike(`build 87654321 has recorded another StartBuild with request id "other request"`))
						assert.Loosely(t, buildbucket.DuplicateTask.In(err), should.BeTrue)
					})

					t.Run("task with collided request id", func(t *ftt.Test) {
						build.StartBuildRequestID = req.RequestId
						infra.Proto.Swarming = &pb.BuildInfra_Swarming{
							TaskId: "another",
						}
						assert.Loosely(t, datastore.Put(ctx, []any{build, infra}), should.BeNil)

						_, err := srv.StartBuild(ctx, req)
						assert.Loosely(t, err, should.ErrLike(`build 87654321 has associated with task id "another" with StartBuild request id "random"`))
						assert.Loosely(t, buildbucket.TaskWithCollidedRequestID.In(err), should.BeTrue)
					})

					t.Run("idempotent", func(t *ftt.Test) {
						build.StartBuildRequestID = req.RequestId
						build.Proto.Status = pb.Status_STARTED
						infra.Proto.Swarming = &pb.BuildInfra_Swarming{
							TaskId: req.TaskId,
						}
						assert.Loosely(t, datastore.Put(ctx, []any{build, infra}), should.BeNil)

						res, err := srv.StartBuild(ctx, req)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, res.UpdateBuildToken, should.Equal(tk))
					})
				})
			})
		})
	})
}
