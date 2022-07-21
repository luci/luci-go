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
					task := &taskdef.CancelSwarmingTask{}
					So(CancelSwarmingTask(ctx, task), ShouldErrLike, "hostname is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("hostname", func() {
					task := &taskdef.CancelSwarmingTask{
						TaskId: "id",
					}
					So(CancelSwarmingTask(ctx, task), ShouldErrLike, "hostname is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("task id", func() {
					task := &taskdef.CancelSwarmingTask{
						Hostname: "example.com",
					}
					So(CancelSwarmingTask(ctx, task), ShouldErrLike, "task_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})
			})

			Convey("valid", func() {
				Convey("empty realm", func() {
					task := &taskdef.CancelSwarmingTask{
						Hostname: "example.com",
						TaskId:   "id",
					}
					So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return CancelSwarmingTask(ctx, task)
					}, nil), ShouldBeNil)
					So(sch.Tasks(), ShouldHaveLength, 1)
				})

				Convey("non-empty realm", func() {
					task := &taskdef.CancelSwarmingTask{
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

		Convey("CreateSwarmingTask", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					So(CreateSwarmingTask(ctx, nil), ShouldErrLike, "build_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("empty", func() {
					task := &taskdef.CreateSwarmingTask{}
					So(CreateSwarmingTask(ctx, task), ShouldErrLike, "build_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("zero", func() {
					task := &taskdef.CreateSwarmingTask{
						BuildId: 0,
					}
					So(CreateSwarmingTask(ctx, task), ShouldErrLike, "build_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})
			})

			Convey("valid", func() {
				task := &taskdef.CreateSwarmingTask{
					BuildId: 1,
				}
				So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return CreateSwarmingTask(ctx, task)
				}, nil), ShouldBeNil)
				So(sch.Tasks(), ShouldHaveLength, 1)
			})
		})

		Convey("ExportBigQuery", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					So(ExportBigQuery(ctx, nil), ShouldErrLike, "build_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("empty", func() {
					task := &taskdef.ExportBigQuery{}
					So(ExportBigQuery(ctx, task), ShouldErrLike, "build_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("zero", func() {
					task := &taskdef.ExportBigQuery{
						BuildId: 0,
					}
					So(ExportBigQuery(ctx, task), ShouldErrLike, "build_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})
			})

			Convey("valid", func() {
				task := &taskdef.ExportBigQuery{
					BuildId: 1,
				}
				So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return ExportBigQuery(ctx, task)
				}, nil), ShouldBeNil)
				So(sch.Tasks(), ShouldHaveLength, 1)
			})
		})

		Convey("FinalizeResultDB", func() {
			Convey("invalid", func() {
				Convey("nil", func() {
					So(FinalizeResultDB(ctx, nil), ShouldErrLike, "build_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("empty", func() {
					task := &taskdef.FinalizeResultDB{}
					So(FinalizeResultDB(ctx, task), ShouldErrLike, "build_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("zero", func() {
					task := &taskdef.FinalizeResultDB{
						BuildId: 0,
					}
					So(FinalizeResultDB(ctx, task), ShouldErrLike, "build_id is required")
					So(sch.Tasks(), ShouldBeEmpty)
				})
			})

			Convey("valid", func() {
				task := &taskdef.FinalizeResultDB{
					BuildId: 1,
				}
				So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return FinalizeResultDB(ctx, task)
				}, nil), ShouldBeNil)
				So(sch.Tasks(), ShouldHaveLength, 1)
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
	})
}
