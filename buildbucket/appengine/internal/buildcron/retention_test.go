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

package buildcron

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/internal/buildid"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestDeleteOldBuilds(t *testing.T) {
	t.Parallel()

	ftt.Run("DeleteOldBuilds", t, func(t *ftt.Test) {
		// now needs to be further fresh enough from buildid.beginningOfTheWorld
		now := time.Date(2020, 01, 01, 0, 0, 0, 0, time.UTC)

		ctx, _ := testclock.UseTime(memory.Use(context.Background()), now)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		b := &model.Build{
			Proto: &pb.Build{
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_SUCCESS,
			},
		}
		setID := func(b *model.Build, ts time.Time) {
			b.ID = buildid.NewBuildIDs(ctx, ts, 1)[0]
			assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, &model.BuildSteps{
				ID:    1,
				Build: datastore.KeyForObj(ctx, b),
			}), should.BeNil)
		}

		t.Run("keeps builds", func(t *ftt.Test) {
			t.Run("as old as BuildStorageDuration", func(t *ftt.Test) {
				setID(b, now.Add(-model.BuildStorageDuration))
				assert.Loosely(t, DeleteOldBuilds(ctx), should.BeNil)
				assert.Loosely(t, datastore.Get(ctx, b), should.BeNil)
				count, err := datastore.Count(ctx, datastore.NewQuery(""))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.Match(int64(2)))
			})
			t.Run("younger than BuildStorageDuration", func(t *ftt.Test) {
				setID(b, now.Add(-model.BuildStorageDuration+time.Minute))
				assert.Loosely(t, DeleteOldBuilds(ctx), should.BeNil)
				count, err := datastore.Count(ctx, datastore.NewQuery(""))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.Match(int64(2)))
			})
		})

		t.Run("deletes builds older than BuildStorageDuration", func(t *ftt.Test) {
			setID(b, now.Add(-model.BuildStorageDuration-time.Minute))
			assert.Loosely(t, DeleteOldBuilds(ctx), should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, b), should.Equal(datastore.ErrNoSuchEntity))
			count, err := datastore.Count(ctx, datastore.NewQuery(""))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, count, should.Match(int64(0)))
		})

		t.Run("removes many builds", func(t *ftt.Test) {
			bs := make([]model.Build, 234)
			old := now.Add(-model.BuildStorageDuration - time.Minute)
			for i := range bs {
				bs[i] = *b
				setID(&bs[i], old)
			}
			assert.Loosely(t, DeleteOldBuilds(ctx), should.BeNil)
			count, err := datastore.Count(ctx, datastore.NewQuery(""))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, count, should.Match(int64(0)))
			assert.Loosely(t, datastore.Get(ctx, bs), should.ErrLike(
				"err[19]: datastore: no such entity\nerr[20:234] <omitted>"))
		})
	})
}
