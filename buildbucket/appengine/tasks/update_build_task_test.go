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

package tasks

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"strconv"
	"testing"

	"google.golang.org/api/pubsub/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/cachingtest"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestValidateBuildTask(t *testing.T) {
	t.Parallel()

	ftt.Run("ValidateBuildTask", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

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
			CreateTime: t0,
		}
		bk := datastore.KeyForObj(ctx, build)
		infra := &model.BuildInfra{
			Build: bk,
			Proto: &pb.BuildInfra{},
		}
		assert.Loosely(t, datastore.Put(ctx, build, infra), should.BeNil)
		t.Run("backend not in build infra", func(t *ftt.Test) {
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming",
					},
				},
			}
			err := validateBuildTask(req, infra)
			assert.Loosely(t, err, should.ErrLike("build 1 does not support task backend"))
		})
		t.Run("task not in build infra", func(t *ftt.Test) {
			infra := &model.BuildInfra{
				Build: bk,
				Proto: &pb.BuildInfra{
					Backend: &pb.BuildInfra_Backend{},
				},
			}
			assert.Loosely(t, datastore.Put(ctx, infra), should.BeNil)
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "2",
						Target: "other",
					},
				},
			}
			err := validateBuildTask(req, infra)
			assert.Loosely(t, err, should.ErrLike("no task is associated with the build"))
		})
		t.Run("task ID target mismatch", func(t *ftt.Test) {
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
			assert.Loosely(t, datastore.Put(ctx, infra), should.BeNil)
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "1",
						Target: "other",
					},
				},
			}
			err := validateBuildTask(req, infra)
			assert.Loosely(t, err, should.ErrLike("does not match TaskID associated with build"))
		})
		t.Run("Run task did not return yet", func(t *ftt.Test) {
			infra := &model.BuildInfra{
				Build: bk,
				Proto: &pb.BuildInfra{
					Backend: &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Target: "swarming",
							},
						},
					},
				},
			}
			assert.Loosely(t, datastore.Put(ctx, infra), should.BeNil)
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming",
					},
					Status: pb.Status_CANCELED,
				},
			}
			err := validateBuildTask(req, infra)
			assert.Loosely(t, err, should.ErrLike("no task is associated with the build"))
		})
		t.Run("task is complete and success", func(t *ftt.Test) {
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
			assert.Loosely(t, datastore.Put(ctx, infra), should.BeNil)
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming",
					},
				},
			}
			err := validateBuildTask(req, infra)
			assert.Loosely(t, err, should.ErrLike("cannot update an ended task"))
		})
		t.Run("task is cancelled", func(t *ftt.Test) {
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
			assert.Loosely(t, datastore.Put(ctx, infra), should.BeNil)
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming",
					},
				},
			}
			err := validateBuildTask(req, infra)
			assert.Loosely(t, err, should.ErrLike("cannot update an ended task"))
		})
		t.Run("task is running", func(t *ftt.Test) {
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
			assert.Loosely(t, datastore.Put(ctx, infra), should.BeNil)
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming",
					},
				},
			}
			err := validateBuildTask(req, infra)
			assert.Loosely(t, err, should.BeNil)
		})

	})
}

func TestValidateTaskUpdate(t *testing.T) {
	t.Parallel()

	ftt.Run("ValidateTaskUpdate", t, func(t *ftt.Test) {
		t.Run("is valid task", func(t *ftt.Test) {
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_STARTED,
					Id: &pb.TaskID{
						Id:     "one",
						Target: "swarming",
					},
					UpdateId: 1,
				},
			}
			assert.Loosely(t, validateBuildTaskUpdate(req), should.BeNil)
		})
		t.Run("is missing task", func(t *ftt.Test) {
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
			}
			err := validateBuildTaskUpdate(req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("task.id: required"))
		})
		t.Run("is missing build ID", func(t *ftt.Test) {
			req := &pb.BuildTaskUpdate{
				Task: &pb.Task{
					Status: pb.Status_STARTED,
					Id: &pb.TaskID{
						Id:     "one",
						Target: "swarming",
					},
				},
			}
			err := validateBuildTaskUpdate(req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("build_id required"))
		})
		t.Run("is missing task ID", func(t *ftt.Test) {
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_STARTED,
				},
			}
			err := validateBuildTaskUpdate(req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("task.id: required"))
		})
		t.Run("is missing update ID", func(t *ftt.Test) {
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_STARTED,
					Id: &pb.TaskID{
						Id:     "one",
						Target: "swarming",
					},
				},
			}
			err := validateBuildTaskUpdate(req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("task.UpdateId: required"))
		})
		t.Run("is invalid task status: SCHEDULED", func(t *ftt.Test) {
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_SCHEDULED,
					Id: &pb.TaskID{
						Id:     "one",
						Target: "swarming",
					},
					UpdateId: 1,
				},
			}
			err := validateBuildTaskUpdate(req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("invalid status SCHEDULED"))
		})
		t.Run("is invalid task status: ENDED_MASK", func(t *ftt.Test) {
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_ENDED_MASK,
					Id: &pb.TaskID{
						Id:     "one",
						Target: "swarming",
					},
					UpdateId: 1,
				},
			}
			err := validateBuildTaskUpdate(req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("invalid status ENDED_MASK"))
		})
		t.Run("is invalid task status: STATUS_UNSPECIFIED", func(t *ftt.Test) {
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_STATUS_UNSPECIFIED,
					Id: &pb.TaskID{
						Id:     "one",
						Target: "swarming",
					},
					UpdateId: 1,
				},
			}
			err := validateBuildTaskUpdate(req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("invalid status STATUS_UNSPECIFIED"))
		})
		t.Run("is invalid task detail", func(t *ftt.Test) {

			details := make(map[string]*structpb.Value)
			for i := 0; i < 10000; i++ {
				v, _ := structpb.NewValue("my really long detail, but it's not that long.")
				details[strconv.Itoa(i)] = v
			}
			req := &pb.BuildTaskUpdate{
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
					UpdateId: 1,
				},
			}
			err := validateBuildTaskUpdate(req)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, err.Error(), should.ContainSubstring("task.details is greater than 10 kb"))
		})
	})
}

func TestUpdateTaskEntity(t *testing.T) {
	t.Parallel()
	ftt.Run("UpdateTaskEntity", t, func(t *ftt.Test) {
		ctx, sch := tq.TestingContext(memory.Use(context.Background()), nil)
		ctx = txndefer.FilterRDS(ctx)
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx, _ = metrics.WithCustomMetrics(ctx, &pb.SettingsCfg{})

		t0 := testclock.TestRecentTimeUTC

		taskProto := &pb.Task{
			Status: pb.Status_SCHEDULED,
			Id: &pb.TaskID{
				Id:     "1",
				Target: "swarming",
			},
			Link:     "a link",
			UpdateId: 50,
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
			Status: pb.Status_STARTED,
		}
		buildModel := &model.Build{
			ID:         1,
			Proto:      buildProto,
			CreateTime: t0,
			Status:     pb.Status_STARTED,
		}
		bk := datastore.KeyForObj(ctx, buildModel)
		infraModel := &model.BuildInfra{
			Build: bk,
			Proto: infraProto,
		}
		bldr := &model.Builder{
			ID:     "builder",
			Parent: model.BucketKey(ctx, "project", "bucket"),
			Config: &pb.BuilderConfig{
				MaxConcurrentBuilds: 2,
			},
		}
		assert.Loosely(t, datastore.Put(ctx, buildModel, infraModel, bldr), should.BeNil)
		t.Run("normal task save", func(t *ftt.Test) {
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_STARTED,
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming",
					},
					UpdateId: 100,
				},
			}
			err := updateTaskEntity(ctx, req, 1, false)
			assert.Loosely(t, err, should.BeNil)
			bk := datastore.KeyForObj(ctx, &model.Build{ID: 1})
			resultInfraModel := &model.BuildInfra{
				Build: bk,
			}
			result := datastore.Get(ctx, resultInfraModel)
			assert.Loosely(t, result, should.BeNil)
			assert.Loosely(t, resultInfraModel.Proto, should.Resemble(&pb.BuildInfra{
				Backend: &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Status: pb.Status_STARTED,
						Id: &pb.TaskID{
							Id:     "1",
							Target: "swarming",
						},
						Link:     "a link",
						UpdateId: 100,
					},
				},
			}))
		})

		t.Run("old update_id", func(t *ftt.Test) {
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_STARTED,
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming",
					},
					Link:     "a link",
					UpdateId: 2,
				},
			}
			err := updateTaskEntity(ctx, req, 1, false)
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("end a task", func(t *ftt.Test) {
			bs := &model.BuildStatus{
				Build:  bk,
				Status: pb.Status_STARTED,
			}
			b, err := proto.Marshal(&pb.Build{
				Steps: []*pb.Step{
					{
						Name: "step",
					},
				},
			})
			assert.Loosely(t, err, should.BeNil)
			steps := &model.BuildSteps{
				ID:       1,
				Build:    bk,
				IsZipped: false,
				Bytes:    b,
			}
			assert.Loosely(t, datastore.Put(ctx, buildModel, infraModel, bs, steps), should.BeNil)

			endReq := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_INFRA_FAILURE,
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming",
					},
					UpdateId: 200,
				},
			}
			err = updateTaskEntity(ctx, endReq, 1, false)
			assert.Loosely(t, err, should.BeNil)
			resultInfraModel := &model.BuildInfra{
				Build: bk,
			}
			result := datastore.Get(ctx, resultInfraModel, bs, buildModel, steps)
			assert.Loosely(t, result, should.BeNil)
			assert.Loosely(t, resultInfraModel.Proto, should.Resemble(&pb.BuildInfra{
				Backend: &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Status: pb.Status_INFRA_FAILURE,
						Id: &pb.TaskID{
							Id:     "1",
							Target: "swarming",
						},
						UpdateId: 200,
						Link:     "a link",
					},
				},
			}))

			assert.Loosely(t, sch.Tasks(), should.HaveLength(4))
			assert.Loosely(t, bs.Status, should.Equal(pb.Status_INFRA_FAILURE))
			assert.Loosely(t, buildModel.Proto.Status, should.Equal(pb.Status_INFRA_FAILURE))
			stp, err := steps.ToProto(ctx)
			assert.Loosely(t, err, should.BeNil)
			for _, s := range stp {
				assert.Loosely(t, s.Status, should.Equal(pb.Status_CANCELED))
			}
		})
		t.Run("expire a task", func(t *ftt.Test) {
			bs := &model.BuildStatus{
				Build:  bk,
				Status: pb.Status_SCHEDULED,
			}
			assert.Loosely(t, datastore.Put(ctx, buildModel, infraModel, bs), should.BeNil)

			statusDetails := &pb.StatusDetails{
				Timeout: &pb.StatusDetails_Timeout{},
			}
			endReq := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Status:        pb.Status_INFRA_FAILURE,
					StatusDetails: statusDetails,
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming",
					},
					UpdateId: 200,
				},
			}
			err := updateTaskEntity(ctx, endReq, 1, false)
			assert.Loosely(t, err, should.BeNil)
			resultInfraModel := &model.BuildInfra{
				Build: bk,
			}
			result := datastore.Get(ctx, resultInfraModel, bs, buildModel)
			assert.Loosely(t, result, should.BeNil)
			assert.Loosely(t, resultInfraModel.Proto, should.Resemble(&pb.BuildInfra{
				Backend: &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Status:        pb.Status_INFRA_FAILURE,
						StatusDetails: statusDetails,
						Id: &pb.TaskID{
							Id:     "1",
							Target: "swarming",
						},
						UpdateId: 200,
						Link:     "a link",
					},
				},
			}))

			assert.Loosely(t, sch.Tasks(), should.HaveLength(4))
			assert.Loosely(t, bs.Status, should.Equal(pb.Status_INFRA_FAILURE))
			assert.Loosely(t, buildModel.Proto.Status, should.Equal(pb.Status_INFRA_FAILURE))
			assert.Loosely(t, buildModel.Proto.StatusDetails, should.Resemble(statusDetails))
		})
		t.Run("start a task", func(t *ftt.Test) {
			buildModel.Proto.Status = pb.Status_SCHEDULED
			infraModel.Proto.Backend.Task.Status = pb.Status_SCHEDULED
			bs := &model.BuildStatus{
				Build:  bk,
				Status: pb.Status_SCHEDULED,
			}
			assert.Loosely(t, datastore.Put(ctx, buildModel, infraModel, bs), should.BeNil)

			endReq := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_STARTED,
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming",
					},
					UpdateId: 200,
				},
			}
			err := updateTaskEntity(ctx, endReq, 1, false)
			assert.Loosely(t, err, should.BeNil)
			resultInfraModel := &model.BuildInfra{
				Build: bk,
			}
			result := datastore.Get(ctx, resultInfraModel, bs, buildModel)
			assert.Loosely(t, result, should.BeNil)
			assert.Loosely(t, resultInfraModel.Proto.Backend.Task.Status, should.Equal(pb.Status_STARTED))

			assert.Loosely(t, sch.Tasks(), should.HaveLength(0))
			assert.Loosely(t, bs.Status, should.Equal(pb.Status_SCHEDULED))
			assert.Loosely(t, buildModel.Proto.Status, should.Equal(pb.Status_SCHEDULED))
		})

		t.Run("RunTask did not return but UpdateBuildTask called", func(t *ftt.Test) {
			buildModel.Proto.Status = pb.Status_SCHEDULED
			infraModel.Proto.Backend.Task.Id.Id = ""
			infraModel.Proto.Backend.Task.UpdateId = 0
			infraModel.Proto.Backend.Task.Link = ""
			bs := &model.BuildStatus{
				Build:  bk,
				Status: pb.Status_SCHEDULED,
			}
			assert.Loosely(t, datastore.Put(ctx, buildModel, infraModel, bs), should.BeNil)

			endReq := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_STARTED,
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming",
					},
					UpdateId: 200,
				},
			}
			err := updateTaskEntity(ctx, endReq, 1, false)
			assert.Loosely(t, err, should.BeNil)
			resultInfraModel := &model.BuildInfra{
				Build: bk,
			}
			result := datastore.Get(ctx, resultInfraModel, bs, buildModel)
			assert.Loosely(t, result, should.BeNil)
			assert.Loosely(t, resultInfraModel.Proto.Backend.Task.Status, should.Equal(pb.Status_STARTED))

			assert.Loosely(t, sch.Tasks(), should.HaveLength(0))
			assert.Loosely(t, bs.Status, should.Equal(pb.Status_SCHEDULED))
			assert.Loosely(t, buildModel.Proto.Status, should.Equal(pb.Status_SCHEDULED))

			postInfra := &model.BuildInfra{Build: bk}
			assert.Loosely(t, datastore.Get(ctx, postInfra), should.BeNil)
			assert.Loosely(t, postInfra.Proto.Backend.Task, should.Resemble(&pb.Task{
				Status: pb.Status_STARTED,
				Id: &pb.TaskID{
					Id:     "1",
					Target: "swarming",
				},
				UpdateId: 200,
			}))
		})

		t.Run("Backfill task for a canceled build", func(t *ftt.Test) {
			buildModel.Proto.Status = pb.Status_CANCELED
			assert.Loosely(t, datastore.Put(ctx, buildModel), should.BeNil)
			t.Run("bypassing if task has ended", func(t *ftt.Test) {
				infraModel.Proto.Backend.Task.Status = pb.Status_CANCELED
				assert.Loosely(t, datastore.Put(ctx, infraModel), should.BeNil)
				endReq := &pb.BuildTaskUpdate{
					BuildId: "1",
					Task: &pb.Task{
						Status: pb.Status_CANCELED,
						Id: &pb.TaskID{
							Id:     "1",
							Target: "swarming",
						},
						UpdateId: 200,
					},
				}
				err := updateTaskEntity(ctx, endReq, 1, false)
				assert.Loosely(t, err, should.BeNil)
				result := &model.BuildInfra{
					Build: bk,
				}
				assert.Loosely(t, datastore.Get(ctx, result), should.BeNil)
				// No update made.
				assert.Loosely(t, result.Proto.Backend.Task.UpdateId, should.Equal(50))
			})
			t.Run("bypassing intermediate update", func(t *ftt.Test) {
				endReq := &pb.BuildTaskUpdate{
					BuildId: "1",
					Task: &pb.Task{
						Status: pb.Status_STARTED,
						Id: &pb.TaskID{
							Id:     "1",
							Target: "swarming",
						},
						UpdateId: 200,
					},
				}
				err := updateTaskEntity(ctx, endReq, 1, false)
				assert.Loosely(t, err, should.BeNil)
				result := &model.BuildInfra{
					Build: bk,
				}
				assert.Loosely(t, datastore.Get(ctx, result), should.BeNil)
				// No update made.
				assert.Loosely(t, result.Proto.Backend.Task.UpdateId, should.Equal(50))
			})
			t.Run("backfilled", func(t *ftt.Test) {
				endReq := &pb.BuildTaskUpdate{
					BuildId: "1",
					Task: &pb.Task{
						Status: pb.Status_CANCELED,
						Id: &pb.TaskID{
							Id:     "1",
							Target: "swarming",
						},
						UpdateId: 200,
					},
				}
				err := updateTaskEntity(ctx, endReq, 1, false)
				assert.Loosely(t, err, should.BeNil)
				result := &model.BuildInfra{
					Build: bk,
				}
				assert.Loosely(t, datastore.Get(ctx, result), should.BeNil)
				assert.Loosely(t, result.Proto.Backend.Task.UpdateId, should.Equal(200))
				assert.Loosely(t, result.Proto.Backend.Task.Status, should.Equal(pb.Status_CANCELED))
			})
		})

	})
}

func TestUpdateBuildTask(t *testing.T) {
	t.Parallel()

	ftt.Run("pubsub handler", t, func(t *ftt.Test) {
		ctx := memory.UseWithAppID(context.Background(), "dev~app-id")
		ctx = cachingtest.WithGlobalCache(ctx, map[string]caching.BlobCache{
			"update-build-task-pubsub-msg-id": cachingtest.NewBlobCache(),
		})
		assert.Loosely(t, config.SetTestSettingsCfg(ctx, &pb.SettingsCfg{
			Backends: []*pb.BackendSetting{
				{
					Target:   "swarming://chromium-swarm",
					Hostname: "chromium-swarm.appspot.com",
					Mode: &pb.BackendSetting_FullMode_{
						FullMode: &pb.BackendSetting_FullMode{
							PubsubId: "chromium-swarm-backend",
						},
					},
				},
				{
					Target:   "foo://foo-backend",
					Hostname: "foo.com",
					Mode:     &pb.BackendSetting_LiteMode_{},
				},
			},
		}), should.BeNil)

		t0 := testclock.TestRecentTimeUTC

		// Create a "scheduled" build.
		build := &model.Build{
			ID: 1,
			Proto: &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_SCHEDULED,
			},
			CreateTime: t0,
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
				Backend: &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://chromium-swarm",
						},
						UpdateId: 0,
					},
				},
			},
		}
		t.Run("full mode", func(t *ftt.Test) {
			t.Run("ok", func(t *ftt.Test) {
				// Update the backend task as if RunTask had responded.
				infra.Proto.Backend.Task.Id.Id = "one"
				infra.Proto.Backend.Task.UpdateId = 1
				infra.Proto.Backend.Task.Status = pb.Status_SCHEDULED
				assert.Loosely(t, datastore.Put(ctx, build, infra), should.BeNil)
				req := &pb.BuildTaskUpdate{
					BuildId: "1",
					Task: &pb.Task{
						Status: pb.Status_STARTED,
						Id: &pb.TaskID{
							Id:     "one",
							Target: "swarming://chromium-swarm",
						},
						UpdateId:        2,
						SummaryMarkdown: "imo, html is ugly to read",
					},
				}
				body := makeUpdateBuildTaskPubsubMsg(req, "msg_id_1", "chromium-swarm-backend")
				assert.Loosely(t, UpdateBuildTask(ctx, body), should.BeNil)

				expectedBuildInfra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, &model.Build{ID: 1})}
				assert.Loosely(t, datastore.Get(ctx, expectedBuildInfra), should.BeNil)
				assert.Loosely(t, expectedBuildInfra.Proto.Backend.Task.Status, should.Equal(pb.Status_STARTED))
				assert.Loosely(t, expectedBuildInfra.Proto.Backend.Task.UpdateId, should.Equal(2))
				assert.Loosely(t, expectedBuildInfra.Proto.Backend.Task.SummaryMarkdown, should.Equal("imo, html is ugly to read"))
			})

			t.Run("task is not registered", func(t *ftt.Test) {
				assert.Loosely(t, datastore.Put(ctx, build, infra), should.BeNil)
				req := &pb.BuildTaskUpdate{
					BuildId: "1",
					Task: &pb.Task{
						Status: pb.Status_STARTED,
						Id: &pb.TaskID{
							Id:     "one",
							Target: "swarming://chromium-swarm",
						},
						UpdateId:        2,
						SummaryMarkdown: "imo, html is ugly to read",
					},
				}
				body := makeUpdateBuildTaskPubsubMsg(req, "msg_id_1", "chromium-swarm-backend")
				assert.Loosely(t, UpdateBuildTask(ctx, body), should.ErrLike("no task is associated with the build"))
			})

			t.Run("subscription id mismatch", func(t *ftt.Test) {
				req := &pb.BuildTaskUpdate{
					BuildId: "1",
					Task: &pb.Task{
						Status: pb.Status_STARTED,
						Id: &pb.TaskID{
							Id:     "one",
							Target: "swarming://chromium-swarm",
						},
						UpdateId:        2,
						SummaryMarkdown: "imo, html is ugly to read",
					},
				}
				body := makeUpdateBuildTaskPubsubMsg(req, "msg_id_1", "chromium-swarm-backend-v2")
				assert.Loosely(t, UpdateBuildTask(ctx, body), should.ErrLike("pubsub subscription projects/app-id/subscriptions/chromium-swarm-backend-v2 did not match the one configured for target swarming://chromium-swarm"))
			})
		})
		t.Run("lite mode", func(t *ftt.Test) {
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_STARTED,
					Id: &pb.TaskID{
						Id:     "one",
						Target: "foo://foo-backend",
					},
					UpdateId: 2,
				},
			}
			body := makeUpdateBuildTaskPubsubMsg(req, "msg_id_1", "foo")
			assert.Loosely(t, UpdateBuildTask(ctx, body), should.ErrLike("backend target foo://foo-backend is in lite mode. The task update isn't supported"))
		})
	})
}

func makeUpdateBuildTaskPubsubMsg(req *pb.BuildTaskUpdate, msgID, subID string) io.Reader {
	data, err := proto.Marshal(req)
	if err != nil {
		return nil
	}
	msg := &pushRequest{
		Message: pubsub.PubsubMessage{
			Data:      base64.StdEncoding.EncodeToString(data),
			MessageId: msgID,
		},
		Subscription: "projects/app-id/subscriptions/" + subID,
	}
	jmsg, _ := json.Marshal(msg)
	return bytes.NewReader(jmsg)
}
