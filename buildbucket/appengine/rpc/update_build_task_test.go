// Copyright 2022 The LUCI Authors.
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
	"strconv"
	"testing"

	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateBuildTask(t *testing.T) {
	t.Parallel()

	Convey("ValidateBuildTask", t, func() {
		ctx := memory.Use(context.Background())
		ctx = installTestSecret(ctx)

		tk, err := buildtoken.GenerateToken(ctx, 1, pb.TokenBody_TASK)
		So(err, ShouldBeNil)
		t0 := testclock.TestRecentTimeUTC
		build := &model.Build{
			ID: 1,
			Proto: &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_STARTED,
			},
			CreateTime:  t0,
			UpdateToken: tk,
		}
		bk := datastore.KeyForObj(ctx, build)
		infra := &model.BuildInfra{
			Build: bk,
			Proto: &pb.BuildInfra{},
		}
		So(datastore.Put(ctx, build, infra), ShouldBeNil)
		Convey("backend not in build infra", func() {
			req := &pb.UpdateBuildTaskRequest{
				BuildId: "1",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming",
					},
				},
			}
			result, err := validateBuildTask(ctx, req, build)
			expected := &pb.BuildInfra{
				Backend: &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Id:     "1",
							Target: "swarming",
						},
					},
				},
			}
			So(err, ShouldBeNil)
			So(result.Infra, ShouldResembleProto, expected)
		})
		Convey("task not in build infra", func() {
			infra := &model.BuildInfra{
				Build: bk,
				Proto: &pb.BuildInfra{
					Backend: &pb.BuildInfra_Backend{},
				},
			}
			So(datastore.Put(ctx, infra), ShouldBeNil)
			expected := &pb.BuildInfra{
				Backend: &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Id:     "2",
							Target: "other",
						},
					},
				},
			}
			req := &pb.UpdateBuildTaskRequest{
				BuildId: "1",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "2",
						Target: "other",
					},
				},
			}
			result, err := validateBuildTask(ctx, req, build)
			So(err, ShouldBeNil)
			So(result.Infra, ShouldResembleProto, expected)
		})
		Convey("task ID target mismatch", func() {
			infra := &model.BuildInfra{
				Build: bk,
				Proto: &pb.BuildInfra{
					Backend: &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Id:     "2",
								Target: "other",
							},
						},
					},
				},
			}
			So(datastore.Put(ctx, infra), ShouldBeNil)
			req := &pb.UpdateBuildTaskRequest{
				BuildId: "1",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "1",
						Target: "other",
					},
				},
			}
			_, err := validateBuildTask(ctx, req, build)
			So(err, ShouldBeError)
		})
		Convey("task is complete and success", func() {
			infra := &model.BuildInfra{
				Build: bk,
				Proto: &pb.BuildInfra{
					Backend: &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Status: pb.Status_SUCCESS,
							Id: &pb.TaskID{
								Id:     "1",
								Target: "swarming",
							},
						},
					},
				},
			}
			So(datastore.Put(ctx, infra), ShouldBeNil)
			req := &pb.UpdateBuildTaskRequest{
				BuildId: "1",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming",
					},
				},
			}
			_, err := validateBuildTask(ctx, req, build)
			So(err, ShouldBeError)
		})
		Convey("task is cancelled", func() {
			infra := &model.BuildInfra{
				Build: bk,
				Proto: &pb.BuildInfra{
					Backend: &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Status: pb.Status_CANCELED,
							Id: &pb.TaskID{
								Id:     "1",
								Target: "swarming",
							},
						},
					},
				},
			}
			So(datastore.Put(ctx, infra), ShouldBeNil)
			req := &pb.UpdateBuildTaskRequest{
				BuildId: "1",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming",
					},
				},
			}
			_, err := validateBuildTask(ctx, req, build)
			So(err, ShouldBeError)
		})
		Convey("task is running", func() {
			infra := &model.BuildInfra{
				Build: bk,
				Proto: &pb.BuildInfra{
					Backend: &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Status: pb.Status_STARTED,
							Id: &pb.TaskID{
								Id:     "1",
								Target: "swarming",
							},
						},
					},
				},
			}
			So(datastore.Put(ctx, infra), ShouldBeNil)
			req := &pb.UpdateBuildTaskRequest{
				BuildId: "1",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming",
					},
				},
			}
			_, err := validateBuildTask(ctx, req, build)
			So(err, ShouldBeNil)
		})

	})
}

func TestValidateTaskUpdate(t *testing.T) {
	t.Parallel()

	Convey("ValidateTaskUpdate", t, func() {
		ctx := memory.Use(context.Background())

		Convey("is valid task", func() {
			req := &pb.UpdateBuildTaskRequest{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_STARTED,
					Id: &pb.TaskID{
						Id:     "one",
						Target: "swarming",
					},
				},
			}
			So(validateUpdateBuildTaskRequest(ctx, req), ShouldBeNil)
		})
		Convey("is missing task", func() {
			req := &pb.UpdateBuildTaskRequest{
				BuildId: "1",
			}
			So(validateUpdateBuildTaskRequest(ctx, req), ShouldBeError)
		})
		Convey("is missing build ID", func() {
			req := &pb.UpdateBuildTaskRequest{
				Task: &pb.Task{
					Status: pb.Status_STARTED,
					Id: &pb.TaskID{
						Id:     "one",
						Target: "swarming",
					},
				},
			}
			So(validateUpdateBuildTaskRequest(ctx, req), ShouldBeError)
		})
		Convey("is missing task ID", func() {
			req := &pb.UpdateBuildTaskRequest{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_STARTED,
				},
			}
			So(validateUpdateBuildTaskRequest(ctx, req), ShouldBeError)
		})
		Convey("is invalid task status: SCHEDULED", func() {
			req := &pb.UpdateBuildTaskRequest{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_SCHEDULED,
					Id: &pb.TaskID{
						Id:     "one",
						Target: "swarming",
					},
				},
			}
			So(validateUpdateBuildTaskRequest(ctx, req), ShouldBeError)
		})
		Convey("is invalid task status: ENDED_MASK", func() {
			req := &pb.UpdateBuildTaskRequest{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_ENDED_MASK,
					Id: &pb.TaskID{
						Id:     "one",
						Target: "swarming",
					},
				},
			}
			So(validateUpdateBuildTaskRequest(ctx, req), ShouldBeError)
		})
		Convey("is invalid task status: STATUS_UNSPECIFIED", func() {
			req := &pb.UpdateBuildTaskRequest{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_STATUS_UNSPECIFIED,
					Id: &pb.TaskID{
						Id:     "one",
						Target: "swarming",
					},
				},
			}
			So(validateUpdateBuildTaskRequest(ctx, req), ShouldBeError)
		})
		Convey("is invalid task detail", func() {

			details := make(map[string]*structpb.Value)
			for i := 0; i < 10000; i++ {
				v, _ := structpb.NewValue("my really long detail, but it's not that long.")
				details[strconv.Itoa(i)] = v
			}
			req := &pb.UpdateBuildTaskRequest{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_STARTED,
					Details: &structpb.Struct{
						Fields: details,
					},
					Id: &pb.TaskID{
						Id:     "one",
						Target: "swarming",
					},
				},
			}
			So(validateUpdateBuildTaskRequest(ctx, req), ShouldBeError)
		})
	})
}

func TestUpdateBuildTask(t *testing.T) {
	t.Parallel()

	updateContextForNewBuildToken := func(ctx context.Context, buildID int64) (string, context.Context) {
		newToken, _ := buildtoken.GenerateToken(ctx, buildID, pb.TokenBody_TASK)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, newToken))
		return newToken, ctx
	}

	Convey("UpdateBuildTask", t, func() {
		srv := &Builds{}
		ctx := memory.Use(context.Background())
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx = installTestSecret(ctx)

		tk, ctx := updateContextForNewBuildToken(ctx, 1)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		ctx = txndefer.FilterRDS(ctx)

		t0 := testclock.TestRecentTimeUTC

		// Helper function to call UpdateBuild.
		updateBuildTask := func(ctx context.Context, req *pb.UpdateBuildTaskRequest) error {
			_, err := srv.UpdateBuildTask(ctx, req)
			return err
		}

		// Create and save a sample build in the datastore.
		build := &model.Build{
			ID: 1,
			Proto: &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_STARTED,
			},
			CreateTime:  t0,
			UpdateToken: tk,
		}
		bk := datastore.KeyForObj(ctx, build)
		infra := &model.BuildInfra{
			Build: bk,
			Proto: &pb.BuildInfra{
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Hostname: "bbhost",
					Agent: &pb.BuildInfra_Buildbucket_Agent{
						Input: &pb.BuildInfra_Buildbucket_Agent_Input{
							Data: map[string]*pb.InputDataRef{},
						},
					},
				},
			},
		}
		So(datastore.Put(ctx, build, infra), ShouldBeNil)

		Convey("tokenValidation", func() {
			Convey("buildID matches token", func() {
				req := &pb.UpdateBuildTaskRequest{
					BuildId: "1",
					Task: &pb.Task{
						Status: pb.Status_STARTED,
						Id: &pb.TaskID{
							Id:     "one",
							Target: "swarming",
						},
					},
				}
				So(updateBuildTask(ctx, req), ShouldBeRPCOK)
			})
			Convey("buildID does not match token", func() {
				req := &pb.UpdateBuildTaskRequest{
					BuildId: "2",
					Task: &pb.Task{
						Status: pb.Status_STARTED,
						Id: &pb.TaskID{
							Id:     "two",
							Target: "swarming",
						},
					},
				}
				So(updateBuildTask(ctx, req), ShouldBeRPCUnknown)
			})
		})
	})
}

func TestUpdateTaskEntity(t *testing.T) {
	t.Parallel()
	Convey("UpdateTaskEntity", t, func() {
		ctx := memory.Use(context.Background())
		ctx = installTestSecret(ctx)

		tk, _ := buildtoken.GenerateToken(ctx, 1, pb.TokenBody_TASK)
		t0 := testclock.TestRecentTimeUTC

		taskProto := &pb.Task{
			Status: pb.Status_STARTED,
			Id: &pb.TaskID{
				Id:     "1",
				Target: "swarming",
			},
		}
		req := &pb.UpdateBuildTaskRequest{
			BuildId: "1",
			Task:    taskProto,
		}
		infraProto := &pb.BuildInfra{
			Backend: &pb.BuildInfra_Backend{
				Task: taskProto,
			},
		}
		buildProto := &pb.Build{
			Id: 1,
			Builder: &pb.BuilderID{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
			},
			Infra: infraProto,
		}
		buildModel := &model.Build{
			ID:          1,
			Proto:       buildProto,
			CreateTime:  t0,
			UpdateToken: tk,
		}

		Convey("normal task save", func() {
			So(datastore.Put(ctx, buildModel), ShouldBeNil)
			err := updateTaskEntity(ctx, req, buildProto)
			So(err, ShouldBeNil)
			bk := datastore.KeyForObj(ctx, &model.Build{ID: 1})
			resultInfraModel := &model.BuildInfra{
				Build: bk,
			}
			result := datastore.Get(ctx, resultInfraModel)
			So(result, ShouldBeNil)
			So(resultInfraModel.Proto, ShouldResembleProto, &pb.BuildInfra{
				Backend: &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Status: pb.Status_STARTED,
						Id: &pb.TaskID{
							Id:     "1",
							Target: "swarming",
						},
					},
				},
			})
		})
		Convey("empty build proto", func() {
			emptyBuildProto := &pb.Build{}
			err := updateTaskEntity(ctx, req, emptyBuildProto)
			So(err, ShouldBeError)
		})
	})
}
