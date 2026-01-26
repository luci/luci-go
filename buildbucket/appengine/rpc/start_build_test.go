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
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock/testclock"
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
	stagepb "go.chromium.org/turboci/proto/go/data/stage/v1"
	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/common"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/internal/turboci"
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
	mockOrch := &turboci.FakeOrchestratorClient{}
	srv := &Builds{}
	ctx := memory.Use(context.Background())
	store := &testsecrets.Store{
		Secrets: map[string]secrets.Secret{
			"key": {Active: []byte("stuff")},
		},
	}
	ctx = secrets.Use(ctx, store)
	ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)
	now := time.Date(2025, 01, 01, 0, 0, 0, 0, time.UTC)
	ctx, _ = testclock.UseTime(ctx, now)
	ctx = turboci.WithTurboCIOrchestratorClient(ctx, mockOrch)

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
			ID:      87654321,
			Project: "project",
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
			StageAttemptID:    "Lplan-id:Sstage-id:A1",
			StageAttemptToken: "attempt-token",
		}
		bk := datastore.KeyForObj(ctx, build)
		infra := &model.BuildInfra{
			Build: bk,
			Proto: &pb.BuildInfra{
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Hostname: "app.appspot.com",
				},
			},
		}
		bs := &model.BuildStatus{
			Build:  bk,
			Status: pb.Status_SCHEDULED,
		}
		assert.Loosely(t, datastore.Put(ctx, build, infra, bs), should.BeNil)

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
				assert.Loosely(t, tsmon.Store(ctx).Get(ctx, metrics.V2.BuildCountStarted, []any{"None"}), should.Equal(1))
				val, err := metrics.GetCustomMetricsData(ctx, base, name, []any{"Linux"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, val, should.Equal(1))

				// TurboCI
				assert.That(t, mockOrch.LastWriteNodesCall.GetToken(), should.Equal("attempt-token"))
				assert.That(t, mockOrch.LastWriteNodesCall.GetCurrentAttempt().GetStateTransition().GetRunning().GetProcessUid(), should.Equal("random"))
				assert.That(t, len(mockOrch.LastWriteNodesCall.GetCurrentAttempt().GetDetails()), should.Equal(2))
				bldDetails := &pb.BuildStageDetails{}
				assert.NoErr(t, mockOrch.LastWriteNodesCall.GetCurrentAttempt().GetDetails()[0].GetValue().UnmarshalTo(bldDetails))
				assert.That(t, bldDetails.GetId(), should.Equal(build.ID))
				commonDetails := &stagepb.CommonStageAttemptDetails{}
				assert.NoErr(t, mockOrch.LastWriteNodesCall.GetCurrentAttempt().GetDetails()[1].GetValue().UnmarshalTo(commonDetails))
				assert.That(t, commonDetails.GetViewUrls()["Buildbucket"].GetUrl(), should.Equal(fmt.Sprintf("https://app.appspot.com/build/%d", build.ID)))
			})

			t.Run("attempt is running", func(t *ftt.Test) {
				infra.Proto.Backend = &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://swarming-host",
						},
					},
				}
				assert.Loosely(t, datastore.Put(ctx, infra), should.BeNil)
				mockOrch.Err = turboci.ErrorWithStageAttemptCurrentState(orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_RUNNING.Enum(), t)

				_, err := srv.StartBuild(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				// TQ tasks for pubsub-notification.
				tasks := sch.Tasks()
				assert.Loosely(t, tasks, should.HaveLength(1))
			})

			t.Run("attempt is cancelling", func(t *ftt.Test) {
				infra.Proto.Backend = &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://swarming-host",
						},
					},
				}
				assert.Loosely(t, datastore.Put(ctx, infra), should.BeNil)
				mockOrch.Err = turboci.ErrorWithStageAttemptCurrentState(orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_CANCELLING.Enum(), t)

				_, err := srv.StartBuild(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				// TQ tasks for pubsub-notification and
				// CancelBuildTask for handling status mismatch from TurboCI.
				tasks := sch.Tasks()
				assert.Loosely(t, tasks, should.HaveLength(2))
			})

			t.Run("TurboCI returns fatal error", func(t *ftt.Test) {
				infra.Proto.Backend = &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://swarming-host",
						},
					},
				}
				assert.Loosely(t, datastore.Put(ctx, infra), should.BeNil)
				mockOrch.Err = status.Error(codes.FailedPrecondition, "fatal error from TurboCI")

				_, err := srv.StartBuild(ctx, req)
				assert.ErrIsLike(t, err, "fatal error from TurboCI")
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

			t.Run("build has ended - TurboCI returns transient error", func(t *ftt.Test) {
				infra.Proto.Backend = &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://swarming-host",
						},
					},
				}
				build.Proto.Status = pb.Status_FAILURE
				assert.Loosely(t, datastore.Put(ctx, infra, build), should.BeNil)

				mockOrch.Err = status.Error(codes.Internal, "transient error from TurboCI")
				_, err := srv.StartBuild(ctx, req)
				assert.Loosely(t, err, should.ErrLike(`transient error from TurboCI`))
			})

			t.Run("build has ended - TurboCI returns fatal error", func(t *ftt.Test) {
				infra.Proto.Backend = &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://swarming-host",
						},
					},
				}
				build.Proto.Status = pb.Status_FAILURE
				assert.Loosely(t, datastore.Put(ctx, infra, build), should.BeNil)

				mockOrch.Err = status.Error(codes.FailedPrecondition, "fatal error from TurboCI")
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
				mockOrch.Err = nil
				res, err := srv.StartBuild(ctx, req)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, res.UpdateBuildToken, should.Equal(tok))
			})

			t.Run("idempotent - attempt is running", func(t *ftt.Test) {
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

				mockOrch.Err = turboci.ErrorWithStageAttemptCurrentState(orchestratorpb.StageAttemptState_STAGE_ATTEMPT_STATE_RUNNING.Enum(), t)

				_, err = srv.StartBuild(ctx, req)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, mockOrch.LastWriteNodesCall, should.NotBeNil)
			})
		})
	})
}
