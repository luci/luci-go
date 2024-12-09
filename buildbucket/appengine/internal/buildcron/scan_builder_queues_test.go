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

package buildcron

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/buildid"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func newBuildsInBuilder(ctx context.Context, bldr string, n int, st pb.Status, t time.Time) []*model.Build {
	ids := buildid.NewBuildIDs(ctx, t, n)
	bs := make([]*model.Build, n)
	for i, id := range ids {
		bs[i] = &model.Build{
			ID: id,
			Proto: &pb.Build{
				Id: id,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: bldr,
				},
				Status: st,
			},
		}
	}
	return bs
}

func newEntities(ctx context.Context, bldrNames []string, t time.Time) ([]*model.BuilderQueue, []*model.Builder, []*model.Build, []*model.Build) {
	bldrQs := make([]*model.BuilderQueue, len(bldrNames))
	bldrs := make([]*model.Builder, len(bldrNames))
	var pendingBuilds []*model.Build
	var triggeredBuilds []*model.Build
	for i, name := range bldrNames {
		bldrQs[i] = &model.BuilderQueue{
			ID: "project/bucket/" + name,
		}
		bldrs[i] = &model.Builder{
			ID:     name,
			Parent: model.BucketKey(ctx, "project", "bucket"),
		}
		newPending := newBuildsInBuilder(ctx, name, i+1, pb.Status_SCHEDULED, now)
		newTriggered := newBuildsInBuilder(ctx, name, i+1, pb.Status_STARTED, now)
		pendingBuilds = append(pendingBuilds, newPending...)
		triggeredBuilds = append(triggeredBuilds, newTriggered...)

		for _, b := range newPending {
			bldrQs[i].PendingBuilds = append(bldrQs[i].PendingBuilds, b.ID)
		}
		for _, b := range newTriggered {
			bldrQs[i].TriggeredBuilds = append(bldrQs[i].TriggeredBuilds, b.ID)
		}
	}
	return bldrQs, bldrs, pendingBuilds, triggeredBuilds
}

func TestScanBuilderQueues(t *testing.T) {
	t.Parallel()

	ftt.Run("ScanBuilderQueues", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, sch := tq.TestingContext(ctx, nil)
		now := testclock.TestRecentTimeLocal
		ctx, _ = testclock.UseTime(ctx, now)

		t.Run("nothing_to_update", func(t *ftt.Test) {
			bldrNames := []string{"builder1", "builder2"}
			bldrQs, bldrs, pendingBuilds, triggeredBuilds := newEntities(ctx, bldrNames, now)
			assert.Loosely(t, datastore.Put(ctx, bldrQs, bldrs, pendingBuilds, triggeredBuilds), should.BeNil)
			assert.Loosely(t, ScanBuilderQueues(ctx), should.BeNil)
			assert.Loosely(t, sch.Tasks(), should.HaveLength(0))
		})

		t.Run("nothing_to_update-recently_ended", func(t *ftt.Test) {
			bldrNames := []string{"builder1", "builder2"}
			bldrQs, bldrs, pendingBuilds, triggeredBuilds := newEntities(ctx, bldrNames, now)
			triggeredBuilds[0].Proto.Status = pb.Status_FAILURE
			triggeredBuilds[0].Proto.EndTime = timestamppb.New(now)
			assert.Loosely(t, datastore.Put(ctx, bldrQs, bldrs, pendingBuilds, triggeredBuilds), should.BeNil)
			assert.Loosely(t, ScanBuilderQueues(ctx), should.BeNil)
			assert.Loosely(t, sch.Tasks(), should.HaveLength(0))
		})

		t.Run("send_pop_pending_build_tasks", func(t *ftt.Test) {
			bldrNames := []string{"builder1", "builder2"}
			bldrQs, bldrs, pendingBuilds, triggeredBuilds := newEntities(ctx, bldrNames, now)
			pendingBuilds[0].Proto.Status = pb.Status_CANCELED
			pendingBuilds[0].Proto.EndTime = timestamppb.New(now.Add(-10 * time.Minute))
			triggeredBuilds[0].Proto.Status = pb.Status_FAILURE
			triggeredBuilds[0].Proto.EndTime = timestamppb.New(now.Add(-10 * time.Minute))
			assert.Loosely(t, datastore.Put(ctx, bldrQs, bldrs, pendingBuilds, triggeredBuilds), should.BeNil)
			assert.Loosely(t, ScanBuilderQueues(ctx), should.BeNil)
			assert.Loosely(t, sch.Tasks(), should.HaveLength(2))
		})
	})
}
