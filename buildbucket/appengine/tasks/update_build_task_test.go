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

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateBuildTask(t *testing.T) {
	t.Parallel()

	Convey("ValidateBuildTask", t, func() {
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
		So(datastore.Put(ctx, build, infra), ShouldBeNil)
		Convey("backend not in build infra", func() {
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming",
					},
				},
			}
			err := validateBuildTask(ctx, req, infra)
			So(err, ShouldBeNil)
		})
		Convey("task not in build infra", func() {
			infra := &model.BuildInfra{
				Build: bk,
				Proto: &pb.BuildInfra{
					Backend: &pb.BuildInfra_Backend{},
				},
			}
			So(datastore.Put(ctx, infra), ShouldBeNil)
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "2",
						Target: "other",
					},
				},
			}
			err := validateBuildTask(ctx, req, infra)
			So(err, ShouldBeNil)
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
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "1",
						Target: "other",
					},
				},
			}
			err := validateBuildTask(ctx, req, infra)
			So(err, ShouldBeError)
		})
		Convey("Run task did not return yet", func() {
			infra := &model.BuildInfra{
				Build: bk,
				Proto: &pb.BuildInfra{
					Backend: &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Status: pb.Status_SCHEDULED,
							Id: &pb.TaskID{
								Id:     "",
								Target: "swarming",
							},
							Link: "www.website.com",
						},
					},
				},
			}
			So(datastore.Put(ctx, infra), ShouldBeNil)
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
			err := validateBuildTask(ctx, req, infra)
			So(err, ShouldBeNil)
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
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming",
					},
				},
			}
			err := validateBuildTask(ctx, req, infra)
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
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming",
					},
				},
			}
			err := validateBuildTask(ctx, req, infra)
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
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming",
					},
				},
			}
			err := validateBuildTask(ctx, req, infra)
			So(err, ShouldBeNil)
		})

	})
}

func TestValidateTaskUpdate(t *testing.T) {
	t.Parallel()

	Convey("ValidateTaskUpdate", t, func() {
		ctx := memory.Use(context.Background())

		Convey("is valid task", func() {
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
			So(validateBuildTaskUpdate(ctx, req), ShouldBeNil)
		})
		Convey("is missing task", func() {
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
			}
			err := validateBuildTaskUpdate(ctx, req)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "task.id: required")
		})
		Convey("is missing build ID", func() {
			req := &pb.BuildTaskUpdate{
				Task: &pb.Task{
					Status: pb.Status_STARTED,
					Id: &pb.TaskID{
						Id:     "one",
						Target: "swarming",
					},
				},
			}
			err := validateBuildTaskUpdate(ctx, req)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "build_id required")
		})
		Convey("is missing task ID", func() {
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_STARTED,
				},
			}
			err := validateBuildTaskUpdate(ctx, req)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "task.id: required")
		})
		Convey("is missing update ID", func() {
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
			err := validateBuildTaskUpdate(ctx, req)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "task.UpdateId: required")
		})
		Convey("is invalid task status: SCHEDULED", func() {
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
			err := validateBuildTaskUpdate(ctx, req)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "invalid status SCHEDULED")
		})
		Convey("is invalid task status: ENDED_MASK", func() {
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
			err := validateBuildTaskUpdate(ctx, req)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "invalid status ENDED_MASK")
		})
		Convey("is invalid task status: STATUS_UNSPECIFIED", func() {
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
			err := validateBuildTaskUpdate(ctx, req)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "invalid status STATUS_UNSPECIFIED")
		})
		Convey("is invalid task detail", func() {

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
			err := validateBuildTaskUpdate(ctx, req)
			So(err, ShouldNotBeNil)
			So(err.Error(), ShouldContainSubstring, "task.details is greater than 10 kb")
		})
	})
}

func TestUpdateTaskEntity(t *testing.T) {
	t.Parallel()
	Convey("UpdateTaskEntity", t, func() {
		ctx, sch := tq.TestingContext(memory.Use(context.Background()), nil)
		ctx = txndefer.FilterRDS(ctx)
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")

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
		So(datastore.Put(ctx, buildModel, infraModel), ShouldBeNil)
		Convey("normal task save", func() {
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
			err := updateTaskEntity(ctx, req, 1)
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
						Link:     "a link",
						UpdateId: 100,
					},
				},
			})
		})

		Convey("old update_id", func() {
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
			err := updateTaskEntity(ctx, req, 1)
			So(err, ShouldBeNil)
		})

		Convey("end a task", func() {
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
			So(err, ShouldBeNil)
			steps := &model.BuildSteps{
				ID:       1,
				Build:    bk,
				IsZipped: false,
				Bytes:    b,
			}
			So(datastore.Put(ctx, buildModel, infraModel, bs, steps), ShouldBeNil)

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
			err = updateTaskEntity(ctx, endReq, 1)
			So(err, ShouldBeNil)
			resultInfraModel := &model.BuildInfra{
				Build: bk,
			}
			result := datastore.Get(ctx, resultInfraModel, bs, buildModel, steps)
			So(result, ShouldBeNil)
			So(resultInfraModel.Proto, ShouldResembleProto, &pb.BuildInfra{
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
			})

			So(sch.Tasks(), ShouldHaveLength, 4)
			So(bs.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
			So(buildModel.Proto.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
			stp, err := steps.ToProto(ctx)
			So(err, ShouldBeNil)
			for _, s := range stp {
				So(s.Status, ShouldEqual, pb.Status_CANCELED)
			}
		})
		Convey("start a task", func() {
			buildModel.Proto.Status = pb.Status_SCHEDULED
			infraModel.Proto.Backend.Task.Status = pb.Status_SCHEDULED
			bs := &model.BuildStatus{
				Build:  bk,
				Status: pb.Status_SCHEDULED,
			}
			So(datastore.Put(ctx, buildModel, infraModel, bs), ShouldBeNil)

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
			err := updateTaskEntity(ctx, endReq, 1)
			So(err, ShouldBeNil)
			resultInfraModel := &model.BuildInfra{
				Build: bk,
			}
			result := datastore.Get(ctx, resultInfraModel, bs, buildModel)
			So(result, ShouldBeNil)
			So(resultInfraModel.Proto.Backend.Task.Status, ShouldEqual, pb.Status_STARTED)

			So(sch.Tasks(), ShouldHaveLength, 0)
			So(bs.Status, ShouldEqual, pb.Status_SCHEDULED)
			So(buildModel.Proto.Status, ShouldEqual, pb.Status_SCHEDULED)
		})

		Convey("RunTask did not return but UpdateBuildTask called", func() {
			buildModel.Proto.Status = pb.Status_SCHEDULED
			infraModel.Proto.Backend.Task.Id.Id = ""
			infraModel.Proto.Backend.Task.UpdateId = 0
			infraModel.Proto.Backend.Task.Link = ""
			bs := &model.BuildStatus{
				Build:  bk,
				Status: pb.Status_SCHEDULED,
			}
			So(datastore.Put(ctx, buildModel, infraModel, bs), ShouldBeNil)

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
			err := updateTaskEntity(ctx, endReq, 1)
			So(err, ShouldBeNil)
			resultInfraModel := &model.BuildInfra{
				Build: bk,
			}
			result := datastore.Get(ctx, resultInfraModel, bs, buildModel)
			So(result, ShouldBeNil)
			So(resultInfraModel.Proto.Backend.Task.Status, ShouldEqual, pb.Status_STARTED)

			So(sch.Tasks(), ShouldHaveLength, 0)
			So(bs.Status, ShouldEqual, pb.Status_SCHEDULED)
			So(buildModel.Proto.Status, ShouldEqual, pb.Status_SCHEDULED)

			post_infra := &model.BuildInfra{Build: bk}
			So(datastore.Get(ctx, post_infra), ShouldBeNil)
			So(post_infra.Proto.Backend.Task, ShouldResembleProto, &pb.Task{
				Status: pb.Status_STARTED,
				Id: &pb.TaskID{
					Id:     "1",
					Target: "swarming",
				},
				UpdateId: 200,
			})
		})
	})
}

func TestUpdateBuildTask(t *testing.T) {
	t.Parallel()

	Convey("pubsub handler", t, func() {
		ctx := memory.UseWithAppID(context.Background(), "dev~app-id")
		ctx = cachingtest.WithGlobalCache(ctx, map[string]caching.BlobCache{
			"update-build-task-pubsub-msg-id": cachingtest.NewBlobCache(),
		})
		So(config.SetTestSettingsCfg(ctx, &pb.SettingsCfg{
			Backends: []*pb.BackendSetting{
				{
					Target:   "swarming://chromium-swarm",
					Hostname: "chromium-swarm.appspot.com",
					PubsubId: "chromium-swarm-backend",
				},
			},
		}), ShouldBeNil)

		t0 := testclock.TestRecentTimeUTC

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
							Id:     "one",
							Target: "swarming://chromium-swarm",
						},
						Status:   pb.Status_SCHEDULED,
						UpdateId: 1,
					},
				},
			},
		}
		So(datastore.Put(ctx, build, infra), ShouldBeNil)

		Convey("ok", func() {
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
			body := makeUpdateBuildTaskPubsubMsg(req, "msg_id_1")
			So(UpdateBuildTask(ctx, body), ShouldBeRPCOK)

			expectedBuildInfra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, &model.Build{ID: 1})}
			So(datastore.Get(ctx, expectedBuildInfra), ShouldBeNil)
			So(expectedBuildInfra.Proto.Backend.Task.Status, ShouldEqual, pb.Status_STARTED)
			So(expectedBuildInfra.Proto.Backend.Task.UpdateId, ShouldEqual, 2)
			So(expectedBuildInfra.Proto.Backend.Task.SummaryMarkdown, ShouldEqual, "imo, html is ugly to read")
		})
	})
}

func makeUpdateBuildTaskPubsubMsg(req *pb.BuildTaskUpdate, msgID string) io.Reader {
	data, err := proto.Marshal(req)
	if err != nil {
		return nil
	}
	msg := &pushRequest{
		Message: pubsub.PubsubMessage{
			Data:      base64.StdEncoding.EncodeToString(data),
			MessageId: msgID,
		},
		Subscription: "projects/app-id/subscriptions/chromium-swarm-backend",
	}
	jmsg, _ := json.Marshal(msg)
	return bytes.NewReader(jmsg)
}
