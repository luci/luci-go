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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/cachingtest"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
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
			req := &pb.BuildTaskUpdate{
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
			req := &pb.BuildTaskUpdate{
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
			req := &pb.BuildTaskUpdate{
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
			req := &pb.BuildTaskUpdate{
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
			req := &pb.BuildTaskUpdate{
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
			So(validateBuildTaskUpdate(ctx, req), ShouldBeNil)
		})
		Convey("is missing task", func() {
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
			}
			So(validateBuildTaskUpdate(ctx, req), ShouldBeError)
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
			So(validateBuildTaskUpdate(ctx, req), ShouldBeError)
		})
		Convey("is missing task ID", func() {
			req := &pb.BuildTaskUpdate{
				BuildId: "1",
				Task: &pb.Task{
					Status: pb.Status_STARTED,
				},
			}
			So(validateBuildTaskUpdate(ctx, req), ShouldBeError)
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
				},
			}
			So(validateBuildTaskUpdate(ctx, req), ShouldBeError)
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
				},
			}
			So(validateBuildTaskUpdate(ctx, req), ShouldBeError)
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
				},
			}
			So(validateBuildTaskUpdate(ctx, req), ShouldBeError)
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
				},
			}
			So(validateBuildTaskUpdate(ctx, req), ShouldBeError)
		})
	})
}

func TestUpdateTaskEntity(t *testing.T) {
	t.Parallel()
	Convey("UpdateTaskEntity", t, func() {
		ctx := memory.Use(context.Background())

		t0 := testclock.TestRecentTimeUTC

		taskProto := &pb.Task{
			Status: pb.Status_STARTED,
			Id: &pb.TaskID{
				Id:     "1",
				Target: "swarming",
			},
		}
		req := &pb.BuildTaskUpdate{
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
			ID:         1,
			Proto:      buildProto,
			CreateTime: t0,
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

func TestUpdateBuildTask(t *testing.T) {
	t.Parallel()

	Convey("pubsub handler", t, func() {
		ctx := memory.Use(context.Background())
		ctx = cachingtest.WithGlobalCache(ctx, map[string]caching.BlobCache{
			"update-build-task-pubsub-msg-id": cachingtest.NewBlobCache(),
		})
		So(config.SetTestSettingsCfg(ctx, &pb.SettingsCfg{
			Backends: []*pb.BackendSetting{
				{
					Target:       "swarming://chromium-swarm",
					Hostname:     "chromium-swarm.appspot.com",
					Subscription: "projects/myproject/subscriptions/mysubscription",
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
						Status: pb.Status_SCHEDULED,
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
				},
			}
			body := makeUpdateBuildTaskPubsubMsg(req, "msg_id_1")
			So(UpdateBuildTask(ctx, body), ShouldBeRPCOK)

			expectedBuild := &model.BuildInfra{Build: datastore.KeyForObj(ctx, &model.Build{ID: 1})}
			So(datastore.Get(ctx, expectedBuild), ShouldBeNil)
			So(expectedBuild.Proto.Backend.Task.Status, ShouldEqual, pb.Status_STARTED)
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
		Subscription: "projects/myproject/subscriptions/mysubscription",
	}
	jmsg, _ := json.Marshal(msg)
	return bytes.NewReader(jmsg)
}
