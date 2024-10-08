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

func TestPopPendingBuildTask(t *testing.T) {
	t.Parallel()

	ftt.Run("PopPendingBuildTask", t, func(t *ftt.Test) {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, sch := tq.TestingContext(ctx, nil)
		now := testclock.TestRecentTimeLocal
		ctx, _ = testclock.UseTime(ctx, now)

		t.Run("builder not found", func(t *ftt.Test) {
			err := PopPendingBuildTask(ctx, 1, &pb.BuilderID{
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
			err := PopPendingBuildTask(ctx, 1, &pb.BuilderID{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
			})
			assert.Loosely(t, err, should.BeNil)
			// create-backend-task-go
			assert.Loosely(t, sch.Tasks(), should.HaveLength(0))
			bq := &model.BuilderQueue{
				ID: "project/bucket/builder",
			}
			assert.Loosely(t, datastore.Get(ctx, bq), should.ErrLike("no such entity"))
		})

		// A nil buildID means that PopPendingBuildTask
		// was triggered from the builder config ingestion.
		t.Run("buildID is nil", func(t *ftt.Test) {
			t.Run("max_concurrent_builds was increased", func(t *ftt.Test) {
				bldr := &model.Builder{
					ID:     "builder",
					Parent: model.BucketKey(ctx, "project", "bucket"),
					Config: &pb.BuilderConfig{
						MaxConcurrentBuilds: 3,
					},
				}
				assert.Loosely(t, datastore.Put(ctx, bldr), should.BeNil)
				bq := &model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1,
					},
					PendingBuilds: []int64{
						2, 3, 4,
					},
				}
				assert.Loosely(t, datastore.Put(ctx, bldr, bq), should.BeNil)
				err := PopPendingBuildTask(ctx, 0, &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				})
				assert.Loosely(t, err, should.BeNil)
				// create-backend-task-go
				assert.Loosely(t, sch.Tasks(), should.HaveLength(2))
				bq = &model.BuilderQueue{
					ID: "project/bucket/builder",
				}
				assert.Loosely(t, datastore.Get(ctx, bq), should.BeNil)
				assert.Loosely(t, bq, should.Resemble(&model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1, 2, 3,
					},
					PendingBuilds: []int64{
						4,
					},
				}))
			})

			t.Run("max_concurrent_builds was reset", func(t *ftt.Test) {
				bldr := &model.Builder{
					ID:     "builder",
					Parent: model.BucketKey(ctx, "project", "bucket"),
					Config: &pb.BuilderConfig{
						MaxConcurrentBuilds: 0,
					},
				}
				assert.Loosely(t, datastore.Put(ctx, bldr), should.BeNil)
				bq := &model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1, 2,
					},
					PendingBuilds: []int64{
						3, 4,
					},
				}
				assert.Loosely(t, datastore.Put(ctx, bldr, bq), should.BeNil)
				err := PopPendingBuildTask(ctx, 0, &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				})
				assert.Loosely(t, err, should.BeNil)
				// create-backend-task-go
				assert.Loosely(t, sch.Tasks(), should.HaveLength(2))
				bq = &model.BuilderQueue{
					ID: "project/bucket/builder",
				}
				assert.Loosely(t, datastore.Get(ctx, bq), should.ErrLike("no such entity"))
			})
		})

		t.Run("build is not nil", func(t *ftt.Test) {
			t.Run("max_concurrent_builds was decreased", func(t *ftt.Test) {
				bldr := &model.Builder{
					ID:     "builder",
					Parent: model.BucketKey(ctx, "project", "bucket"),
					Config: &pb.BuilderConfig{
						MaxConcurrentBuilds: 2,
					},
				}
				assert.Loosely(t, datastore.Put(ctx, bldr), should.BeNil)
				bq := &model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1, 2, 3,
					},
					PendingBuilds: []int64{
						4,
					},
				}
				assert.Loosely(t, datastore.Put(ctx, bldr, bq), should.BeNil)
				err := PopPendingBuildTask(ctx, 1, &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				})
				assert.Loosely(t, err, should.BeNil)
				// create-backend-task-go
				assert.Loosely(t, sch.Tasks(), should.HaveLength(0))
				bq = &model.BuilderQueue{
					ID: "project/bucket/builder",
				}
				assert.Loosely(t, datastore.Get(ctx, bq), should.BeNil)
				assert.Loosely(t, bq, should.Resemble(&model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						3, 2,
					},
					PendingBuilds: []int64{
						4,
					},
				}))
			})

			t.Run("build was not tracked", func(t *ftt.Test) {
				bldr := &model.Builder{
					ID:     "builder",
					Parent: model.BucketKey(ctx, "project", "bucket"),
					Config: &pb.BuilderConfig{
						MaxConcurrentBuilds: 2,
					},
				}
				assert.Loosely(t, datastore.Put(ctx, bldr), should.BeNil)
				bq := &model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1, 2,
					},
					PendingBuilds: []int64{
						3, 4,
					},
				}
				assert.Loosely(t, datastore.Put(ctx, bldr, bq), should.BeNil)
				err := PopPendingBuildTask(ctx, 10, &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				})
				assert.Loosely(t, err, should.BeNil)
				// create-backend-task-go
				assert.Loosely(t, sch.Tasks(), should.HaveLength(0))
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

			t.Run("build was tracked - happy path", func(t *ftt.Test) {
				bldr := &model.Builder{
					ID:     "builder",
					Parent: model.BucketKey(ctx, "project", "bucket"),
					Config: &pb.BuilderConfig{
						MaxConcurrentBuilds: 2,
					},
				}
				assert.Loosely(t, datastore.Put(ctx, bldr), should.BeNil)
				bq := &model.BuilderQueue{
					ID: "project/bucket/builder",
					TriggeredBuilds: []int64{
						1, 2,
					},
					PendingBuilds: []int64{
						3, 4,
					},
				}
				assert.Loosely(t, datastore.Put(ctx, bldr, bq), should.BeNil)
				err := PopPendingBuildTask(ctx, 1, &pb.BuilderID{
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
						2, 3,
					},
					PendingBuilds: []int64{
						4,
					},
				}))
			})
		})

	})

}
