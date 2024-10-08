// Copyright 2024 The LUCI Authors.
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

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestPushPendingBuildTask(t *testing.T) {
	t.Parallel()

	ftt.Run("PushPendingBuildTask", t, func(t *ftt.Test) {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, sch := tq.TestingContext(ctx, nil)
		now := testclock.TestRecentTimeLocal
		ctx, _ = testclock.UseTime(ctx, now)

		t.Run("builder not found", func(t *ftt.Test) {
			err := PushPendingBuildTask(ctx, 1, &pb.BuilderID{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
			})
			assert.Loosely(t, err, should.ErrLike("no such entity"))
			assert.Loosely(t, sch.Tasks(), should.BeEmpty)
		})

		t.Run("builderQueue does not exist", func(t *ftt.Test) {
			bldr := &model.Builder{
				ID:     "builder",
				Parent: model.BucketKey(ctx, "project", "bucket"),
				Config: &pb.BuilderConfig{
					MaxConcurrentBuilds: 2,
				},
			}
			assert.Loosely(t, datastore.Put(ctx, bldr), should.BeNil)
			err := PushPendingBuildTask(ctx, 1, &pb.BuilderID{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
			})
			assert.Loosely(t, err, should.BeNil)
			// create-backend-task-go
			assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
			bq := &model.BuilderQueue{
				ID: "project/bucket/builder",
			}
			assert.Loosely(t, datastore.Get(ctx, bq), should.BeNil)
			assert.Loosely(t, bq, should.Resemble(&model.BuilderQueue{
				ID: "project/bucket/builder",
				TriggeredBuilds: []int64{
					1,
				},
			}))
		})

		t.Run("max_concurrent_builds is 0", func(t *ftt.Test) {
			bldr := &model.Builder{
				ID:     "builder",
				Parent: model.BucketKey(ctx, "project", "bucket"),
				Config: &pb.BuilderConfig{
					MaxConcurrentBuilds: 0,
				},
			}
			assert.Loosely(t, datastore.Put(ctx, bldr), should.BeNil)
			err := PushPendingBuildTask(ctx, 1, &pb.BuilderID{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
			})
			assert.Loosely(t, err, should.BeNil)
			// create-backend-task-go
			assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
			bq := &model.BuilderQueue{
				ID: "project/bucket/builder",
			}
			assert.Loosely(t, datastore.Get(ctx, bq), should.ErrLike("no such entity"))
		})

		t.Run("pending_builds is empty", func(t *ftt.Test) {
			t.Run("triggered_builds is less than max_concurrent_builds", func(t *ftt.Test) {
				bldr := &model.Builder{
					ID:     "builder",
					Parent: model.BucketKey(ctx, "project", "bucket"),
					Config: &pb.BuilderConfig{
						MaxConcurrentBuilds: 2,
					},
				}
				bq := &model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1,
					},
				}
				assert.Loosely(t, datastore.Put(ctx, bldr, bq), should.BeNil)
				err := PushPendingBuildTask(ctx, 2, &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				})
				assert.Loosely(t, err, should.BeNil)
				// create-backend-task-go
				assert.Loosely(t, sch.Tasks(), should.HaveLength(1))
				bq = &model.BuilderQueue{
					ID: "project/bucket/builder",
				}
				assert.Loosely(t, datastore.Get(ctx, bq), should.BeNil)
				assert.Loosely(t, bq, should.Resemble(&model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1, 2,
					},
				}))
			})

			t.Run("triggered_builds is greater or equal to max_concurrent_builds", func(t *ftt.Test) {
				bldr := &model.Builder{
					ID:     "builder",
					Parent: model.BucketKey(ctx, "project", "bucket"),
					Config: &pb.BuilderConfig{
						MaxConcurrentBuilds: 1,
					},
				}
				bq := &model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1, 2,
					},
				}
				assert.Loosely(t, datastore.Put(ctx, bldr, bq), should.BeNil)
				err := PushPendingBuildTask(ctx, 3, &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				})
				assert.Loosely(t, err, should.BeNil)
				// No backend task triggered.
				assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				bq = &model.BuilderQueue{
					ID: "project/bucket/builder",
				}
				assert.Loosely(t, datastore.Get(ctx, bq), should.BeNil)
				assert.Loosely(t, bq, should.Resemble(&model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1, 2,
					},
					PendingBuilds: []int64{
						3,
					},
				}))
			})
		})

		t.Run("pending_builds is non-empty", func(t *ftt.Test) {
			t.Run("triggered_builds is less than max_concurrent_builds", func(t *ftt.Test) {
				bldr := &model.Builder{
					ID:     "builder",
					Parent: model.BucketKey(ctx, "project", "bucket"),
					Config: &pb.BuilderConfig{
						MaxConcurrentBuilds: 3,
					},
				}
				bq := &model.BuilderQueue{
					ID: "project/bucket/builder",
					PendingBuilds: []int64{
						1,
					},
					TriggeredBuilds: []int64{
						2, 3,
					},
				}
				assert.Loosely(t, datastore.Put(ctx, bldr, bq), should.BeNil)
				err := PushPendingBuildTask(ctx, 4, &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				})
				assert.Loosely(t, err, should.BeNil)
				// No backend task triggered.
				assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				bq = &model.BuilderQueue{
					ID: "project/bucket/builder",
				}
				assert.Loosely(t, datastore.Get(ctx, bq), should.BeNil)
				assert.Loosely(t, bq, should.Resemble(&model.BuilderQueue{
					ID: "project/bucket/builder",
					PendingBuilds: []int64{
						1, 4,
					},
					TriggeredBuilds: []int64{
						2, 3,
					},
				}))
			})

			t.Run("triggered_builds is greater or equal to max_concurrent_builds", func(t *ftt.Test) {
				bldr := &model.Builder{
					ID:     "builder",
					Parent: model.BucketKey(ctx, "project", "bucket"),
					Config: &pb.BuilderConfig{
						MaxConcurrentBuilds: 2,
					},
				}
				bq := &model.BuilderQueue{
					ID: "project/bucket/builder",
					PendingBuilds: []int64{
						3,
					},
					TriggeredBuilds: []int64{
						1, 2,
					},
				}
				assert.Loosely(t, datastore.Put(ctx, bldr, bq), should.BeNil)
				err := PushPendingBuildTask(ctx, 4, &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				})
				assert.Loosely(t, err, should.BeNil)
				// No backend task triggered.
				assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				bq = &model.BuilderQueue{
					ID: "project/bucket/builder",
				}
				assert.Loosely(t, datastore.Get(ctx, bq), should.BeNil)
				assert.Loosely(t, bq, should.Resemble(&model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1, 2,
					},
					PendingBuilds: []int64{
						3, 4,
					},
				}))
			})
		})
	})
}
