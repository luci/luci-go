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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	taskdef "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestTasks(t *testing.T) {
	t.Parallel()

	ftt.Run("tasks", t, func(t *ftt.Test) {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		ctx, sch := tq.TestingContext(ctx, nil)

		t.Run("CancelSwarmingTask", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					assert.Loosely(t, CancelSwarmingTask(ctx, nil), should.ErrLike("hostname is required"))
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})

				t.Run("empty", func(t *ftt.Test) {
					task := &taskdef.CancelSwarmingTaskGo{}
					assert.Loosely(t, CancelSwarmingTask(ctx, task), should.ErrLike("hostname is required"))
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})

				t.Run("hostname", func(t *ftt.Test) {
					task := &taskdef.CancelSwarmingTaskGo{
						TaskId: "id",
					}
					assert.Loosely(t, CancelSwarmingTask(ctx, task), should.ErrLike("hostname is required"))
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})

				t.Run("task id", func(t *ftt.Test) {
					task := &taskdef.CancelSwarmingTaskGo{
						Hostname: "example.com",
					}
					assert.Loosely(t, CancelSwarmingTask(ctx, task), should.ErrLike("task_id is required"))
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				t.Run("empty realm", func(t *ftt.Test) {
					task := &taskdef.CancelSwarmingTaskGo{
						Hostname: "example.com",
						TaskId:   "id",
					}
					assert.Loosely(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return CancelSwarmingTask(ctx, task)
					}, nil), should.BeNil)
					assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
				})

				t.Run("non-empty realm", func(t *ftt.Test) {
					task := &taskdef.CancelSwarmingTaskGo{
						Hostname: "example.com",
						TaskId:   "id",
						Realm:    "realm",
					}
					assert.Loosely(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return CancelSwarmingTask(ctx, task)
					}, nil), should.BeNil)
					assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
				})
			})
		})

		t.Run("ExportBigQuery", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("zero", func(t *ftt.Test) {
					assert.Loosely(t, ExportBigQuery(ctx, 0), should.ErrLike("build_id is invalid"))
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})
			})

			t.Run("valid - to Go", func(t *ftt.Test) {
				assert.Loosely(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return ExportBigQuery(ctx, 1)
				}, nil), should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
				assert.Loosely(t, sch.Tasks().Payloads()[0], should.Match(&taskdef.ExportBigQueryGo{
					BuildId: 1,
				}))
			})
		})

		t.Run("CreateSwarmingBuildTask", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					assert.Loosely(t, CreateSwarmingBuildTask(ctx, nil), should.ErrLike("build_id is required"))
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})

				t.Run("empty", func(t *ftt.Test) {
					task := &taskdef.CreateSwarmingBuildTask{}
					assert.Loosely(t, CreateSwarmingBuildTask(ctx, task), should.ErrLike("build_id is required"))
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})

				t.Run("zero", func(t *ftt.Test) {
					task := &taskdef.CreateSwarmingBuildTask{
						BuildId: 0,
					}
					assert.Loosely(t, CreateSwarmingBuildTask(ctx, task), should.ErrLike("build_id is required"))
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				task := &taskdef.CreateSwarmingBuildTask{
					BuildId: 1,
				}
				assert.Loosely(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return CreateSwarmingBuildTask(ctx, task)
				}, nil), should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
			})
		})

		t.Run("SyncSwarmingBuildTask", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("nil", func(t *ftt.Test) {
					assert.Loosely(t, SyncSwarmingBuildTask(ctx, nil, time.Second), should.ErrLike("build_id is required"))
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})

				t.Run("empty", func(t *ftt.Test) {
					task := &taskdef.SyncSwarmingBuildTask{}
					assert.Loosely(t, SyncSwarmingBuildTask(ctx, task, time.Second), should.ErrLike("build_id is required"))
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})

				t.Run("zero generation", func(t *ftt.Test) {
					task := &taskdef.SyncSwarmingBuildTask{
						BuildId:    123,
						Generation: 0,
					}
					assert.Loosely(t, SyncSwarmingBuildTask(ctx, task, time.Second), should.ErrLike("generation should be larger than 0"))
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				task := &taskdef.SyncSwarmingBuildTask{
					BuildId:    123,
					Generation: 1,
				}
				assert.Loosely(t, SyncSwarmingBuildTask(ctx, task, time.Second), should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
			})
		})

		t.Run("SyncWithBackend", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("empty backend", func(t *ftt.Test) {
					assert.Loosely(t, SyncWithBackend(ctx, "", "project"), should.ErrLike("backend is required"))
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})

				t.Run("empty project", func(t *ftt.Test) {
					assert.Loosely(t, SyncWithBackend(ctx, "backend", ""), should.ErrLike("project is required"))
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				assert.Loosely(t, SyncWithBackend(ctx, "backend", "project"), should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
			})
		})

		t.Run("CancelBackendTask", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("empty target", func(t *ftt.Test) {
					assert.Loosely(t, CancelBackendTask(ctx, &taskdef.CancelBackendTask{
						Project: "project",
						TaskId:  "123",
					}), should.ErrLike("target is required"))
				})

				t.Run("empty task_id", func(t *ftt.Test) {
					assert.Loosely(t, CancelBackendTask(ctx, &taskdef.CancelBackendTask{
						Project: "project",
						Target:  "123",
					}), should.ErrLike("task_id is required"))
				})
				t.Run("invalid project", func(t *ftt.Test) {
					assert.Loosely(t, CancelBackendTask(ctx, &taskdef.CancelBackendTask{
						TaskId: "123",
						Target: "123",
					}), should.ErrLike("project is required"))
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				assert.Loosely(t, CancelBackendTask(ctx, &taskdef.CancelBackendTask{
					Target:  "abc",
					TaskId:  "123",
					Project: "project",
				}), should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
			})
		})

		t.Run("CheckBuildLiveness", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("build id", func(t *ftt.Test) {
					assert.Loosely(t, CheckBuildLiveness(ctx, 0, 0, time.Duration(1)), should.ErrLike("build_id is invalid"))
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				assert.Loosely(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return CheckBuildLiveness(ctx, 123, 60, time.Duration(1))
				}, nil), should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
			})
		})

		t.Run("CreatePushPendingTask", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				t.Run("empty build_id", func(t *ftt.Test) {
					assert.Loosely(t, CreatePushPendingBuildTask(ctx, &taskdef.PushPendingBuildTask{
						BuilderId: &pb.BuilderID{Builder: "builder", Bucket: "bucket", Project: "project"},
					}), should.ErrLike("build_id is required"))
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})

				t.Run("empty builder_id", func(t *ftt.Test) {
					assert.Loosely(t, CreatePushPendingBuildTask(ctx, &taskdef.PushPendingBuildTask{
						BuildId: 123,
					}), should.ErrLike("builder_id is required"))
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})
			})

			t.Run("valid", func(t *ftt.Test) {
				assert.Loosely(t, datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return CreatePushPendingBuildTask(ctx, &taskdef.PushPendingBuildTask{
						BuildId:   123,
						BuilderId: &pb.BuilderID{Builder: "builder", Bucket: "bucket", Project: "project"},
					})
				}, nil), should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
			})
		})

		t.Run("CreatePopPendingTask", func(t *ftt.Test) {
			t.Run("invalid", func(t *ftt.Test) {
				assert.Loosely(t, CreatePopPendingBuildTask(ctx, &taskdef.PopPendingBuildTask{
					BuildId: 123,
				}, ""), should.ErrLike("builder_id is required"))
				assert.Loosely(t, sch.Tasks(), should.BeEmpty)
			})

			t.Run("valid", func(t *ftt.Test) {
				assert.Loosely(t, CreatePopPendingBuildTask(ctx, &taskdef.PopPendingBuildTask{
					BuildId:   123,
					BuilderId: &pb.BuilderID{Builder: "builder", Bucket: "bucket", Project: "project"},
				}, ""), should.BeNil)
				assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
			})
		})
	})
}
