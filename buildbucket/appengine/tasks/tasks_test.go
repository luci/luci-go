// Copyright 2020 The LUCI Authors.
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
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	taskdef "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestTasks(t *testing.T) {
	t.Parallel()

	Convey("tasks", t, func() {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		ctx, sch := tq.TestingContext(ctx, nil)

		Convey("CancelSwarmingTask", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					So(CancelSwarmingTask(ctx, nil), ShouldErrLike, "hostname is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("empty", func() {
					task := &taskdef.CancelSwarmingTaskGo{}
					So(CancelSwarmingTask(ctx, task), ShouldErrLike, "hostname is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("hostname", func() {
					task := &taskdef.CancelSwarmingTaskGo{
						TaskId: "id",
					}
					So(CancelSwarmingTask(ctx, task), ShouldErrLike, "hostname is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("task id", func() {
					task := &taskdef.CancelSwarmingTaskGo{
						Hostname: "example.com",
					}
					So(CancelSwarmingTask(ctx, task), ShouldErrLike, "task_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})
			})

			Convey("valid", func() {
				Convey("empty realm", func() {
					task := &taskdef.CancelSwarmingTaskGo{
						Hostname: "example.com",
						TaskId:   "id",
					}
					So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return CancelSwarmingTask(ctx, task)
					}, nil), ShouldBeNil)
					So(sch.Tasks(), ShouldHaveLength, 1)
				})

				Convey("non-empty realm", func() {
					task := &taskdef.CancelSwarmingTaskGo{
						Hostname: "example.com",
						TaskId:   "id",
						Realm:    "realm",
					}
					So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return CancelSwarmingTask(ctx, task)
					}, nil), ShouldBeNil)
					So(sch.Tasks(), ShouldHaveLength, 1)
				})
			})
		})

		Convey("ExportBigQuery", func() {
			Convey("invalid", func() {

				Convey("zero", func() {

					So(ExportBigQuery(ctx, 0), ShouldErrLike, "build_id is invalid")
					So(sch.Tasks(), ShouldBeEmpty)
				})
			})

			Convey("valid - to Go", func() {
				So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return ExportBigQuery(ctx, 1)
				}, nil), ShouldBeNil)
				So(sch.Tasks(), ShouldHaveLength, 1)
				So(sch.Tasks().Payloads()[0], ShouldResembleProto, &taskdef.ExportBigQueryGo{
					BuildId: 1,
				})
			})
		})

		Convey("NotifyPubSub", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					So(notifyPubSub(ctx, nil), ShouldErrLike, "build_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("empty", func() {
					task := &taskdef.NotifyPubSub{}
					So(notifyPubSub(ctx, task), ShouldErrLike, "build_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("zero", func() {
					task := &taskdef.NotifyPubSub{
						BuildId: 0,
					}
					So(notifyPubSub(ctx, task), ShouldErrLike, "build_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})
			})

			Convey("valid", func() {
				task := &taskdef.NotifyPubSub{
					BuildId:  1,
					Callback: true,
				}
				So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return notifyPubSub(ctx, task)
				}, nil), ShouldBeNil)
				So(sch.Tasks(), ShouldHaveLength, 1)
			})
		})

		Convey("CreateSwarmingBuildTask", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					So(CreateSwarmingBuildTask(ctx, nil), ShouldErrLike, "build_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("empty", func() {
					task := &taskdef.CreateSwarmingBuildTask{}
					So(CreateSwarmingBuildTask(ctx, task), ShouldErrLike, "build_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("zero", func() {
					task := &taskdef.CreateSwarmingBuildTask{
						BuildId: 0,
					}
					So(CreateSwarmingBuildTask(ctx, task), ShouldErrLike, "build_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})
			})

			Convey("valid", func() {
				task := &taskdef.CreateSwarmingBuildTask{
					BuildId: 1,
				}
				So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return CreateSwarmingBuildTask(ctx, task)
				}, nil), ShouldBeNil)
				So(sch.Tasks(), ShouldHaveLength, 1)
			})
		})

		Convey("SyncSwarmingBuildTask", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					So(SyncSwarmingBuildTask(ctx, nil, time.Second), ShouldErrLike, "build_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("empty", func() {
					task := &taskdef.SyncSwarmingBuildTask{}
					So(SyncSwarmingBuildTask(ctx, task, time.Second), ShouldErrLike, "build_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("zero generation", func() {
					task := &taskdef.SyncSwarmingBuildTask{
						BuildId:    123,
						Generation: 0,
					}
					So(SyncSwarmingBuildTask(ctx, task, time.Second), ShouldErrLike, "generation should be larger than 0")
					So(sch.Tasks(), ShouldBeEmpty)
				})
			})

			Convey("valid", func() {
				task := &taskdef.SyncSwarmingBuildTask{
					BuildId:    123,
					Generation: 1,
				}
				So(SyncSwarmingBuildTask(ctx, task, time.Second), ShouldBeNil)
				So(sch.Tasks(), ShouldHaveLength, 1)
			})
		})

		Convey("SyncWithBackend", func() {
			Convey("invalid", func() {
				Convey("empty backend", func() {
					So(SyncWithBackend(ctx, "", "project"), ShouldErrLike, "backend is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("empty project", func() {
					So(SyncWithBackend(ctx, "backend", ""), ShouldErrLike, "project is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})
			})

			Convey("valid", func() {
				So(SyncWithBackend(ctx, "backend", "project"), ShouldBeNil)
				So(sch.Tasks(), ShouldHaveLength, 1)
			})
		})

		Convey("CancelBackendTask", func() {
			Convey("invalid", func() {
				Convey("empty target", func() {
					So(CancelBackendTask(ctx, &taskdef.CancelBackendTask{
						Project: "project",
						TaskId:  "123",
					}), ShouldErrLike, "target is required")
				})

				Convey("empty task_id", func() {
					So(CancelBackendTask(ctx, &taskdef.CancelBackendTask{
						Project: "project",
						Target:  "123",
					}), ShouldErrLike, "task_id is required")
				})
				Convey("invalid project", func() {
					So(CancelBackendTask(ctx, &taskdef.CancelBackendTask{
						TaskId: "123",
						Target: "123",
					}), ShouldErrLike, "project is required")
				})
			})

			Convey("valid", func() {
				So(CancelBackendTask(ctx, &taskdef.CancelBackendTask{
					Target:  "abc",
					TaskId:  "123",
					Project: "project",
				}), ShouldBeNil)
				So(sch.Tasks(), ShouldHaveLength, 1)
			})
		})

		Convey("CheckBuildLiveness", func() {
			Convey("invalid", func() {
				Convey("build id", func() {
					So(CheckBuildLiveness(ctx, 0, 0, time.Duration(1)), ShouldErrLike, "build_id is invalid")
					So(sch.Tasks(), ShouldBeEmpty)
				})
			})

			Convey("valid", func() {
				So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return CheckBuildLiveness(ctx, 123, 60, time.Duration(1))
				}, nil), ShouldBeNil)
				So(sch.Tasks(), ShouldHaveLength, 1)
			})
		})

		Convey("CreatePushPendingTask", func() {
			Convey("invalid", func() {
				Convey("empty build_id", func() {
					So(CreatePushPendingBuildTask(ctx, &taskdef.PushPendingBuildTask{
						BuilderId: &pb.BuilderID{Builder: "builder", Bucket: "bucket", Project: "project"},
					}), ShouldErrLike, "build_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("empty builder_id", func() {
					So(CreatePushPendingBuildTask(ctx, &taskdef.PushPendingBuildTask{
						BuildId: 123,
					}), ShouldErrLike, "builder_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})
			})

			Convey("valid", func() {
				So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return CreatePushPendingBuildTask(ctx, &taskdef.PushPendingBuildTask{
						BuildId:   123,
						BuilderId: &pb.BuilderID{Builder: "builder", Bucket: "bucket", Project: "project"},
					})
				}, nil), ShouldBeNil)
				So(sch.Tasks(), ShouldHaveLength, 1)
			})
		})

		Convey("CreatePopPendingTask", func() {
			Convey("invalid", func() {
				So(CreatePopPendingBuildTask(ctx, &taskdef.PopPendingBuildTask{
					BuildId: 123,
				}), ShouldErrLike, "builder_id is required")
				So(sch.Tasks(), ShouldBeEmpty)
			})

			Convey("valid", func() {
				So(CreatePopPendingBuildTask(ctx, &taskdef.PopPendingBuildTask{
					BuildId:   123,
					BuilderId: &pb.BuilderID{Builder: "builder", Bucket: "bucket", Project: "project"},
				}), ShouldBeNil)
				So(sch.Tasks(), ShouldHaveLength, 1)
			})
		})
	})
}
