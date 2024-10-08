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

package buildercron

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

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestResetExpiredLeases(t *testing.T) {
	t.Parallel()

	ftt.Run("RemoveInactiveBuilderStats", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		now := testclock.TestTimeUTC
		ctx, _ = testclock.UseTime(ctx, now)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		t.Run("works when the world is empty", func(t *ftt.Test) {
			assert.Loosely(t, RemoveInactiveBuilderStats(ctx), should.BeNil)
		})

		builds := []*model.Build{
			{
				ID: 1,
				Proto: &pb.Build{
					Builder: &pb.BuilderID{
						Project: "prj",
						Bucket:  "bkt",
						Builder: "bld",
					},
					Status: pb.Status_SCHEDULED,
				},
			},
		}
		builder := &model.Builder{
			ID:         "bld",
			Parent:     model.BucketKey(ctx, "prj", "bkt"),
			ConfigHash: "deadbeef",
		}
		statExist := func() bool {
			r, err := datastore.Exists(ctx, model.BuilderStatKey(ctx, "prj", "bkt", "bld"))
			assert.Loosely(t, err, should.BeNil)
			return r.Any()
		}

		var (
			fresh = now.Add(-1 * time.Hour)
			old   = now.Add(-1 * model.BuilderExpirationDuration)
		)
		assert.Loosely(t, datastore.Put(ctx, builder), should.BeNil)
		assert.Loosely(t, statExist(), should.BeFalse)

		t.Run("leaves Active Stats", func(t *ftt.Test) {
			assert.Loosely(t, model.UpdateBuilderStat(ctx, builds, fresh), should.BeNil)
			assert.Loosely(t, RemoveInactiveBuilderStats(ctx), should.BeNil)
			assert.Loosely(t, statExist(), should.BeTrue)
		})

		t.Run("removes BuilderStat for an inactive Builder", func(t *ftt.Test) {
			assert.Loosely(t, model.UpdateBuilderStat(ctx, builds, old), should.BeNil)
			assert.Loosely(t, RemoveInactiveBuilderStats(ctx), should.BeNil)
			assert.Loosely(t, statExist(), should.BeFalse)
		})

		t.Run("leaves young zombie BuilderStat", func(t *ftt.Test) {
			assert.Loosely(t, model.UpdateBuilderStat(ctx, builds, fresh), should.BeNil)
			assert.Loosely(t, datastore.Delete(ctx, model.BuilderKey(ctx, "prj", "bkt", "bld")), should.BeNil)
			assert.Loosely(t, RemoveInactiveBuilderStats(ctx), should.BeNil)
			assert.Loosely(t, statExist(), should.BeTrue)
		})

		t.Run("removes old zombie BuilderStat", func(t *ftt.Test) {
			old := now.Add(-1 * model.BuilderStatZombieDuration)
			assert.Loosely(t, model.UpdateBuilderStat(ctx, builds, old), should.BeNil)
			assert.Loosely(t, datastore.Delete(ctx, model.BuilderKey(ctx, "prj", "bkt", "bld")), should.BeNil)
			assert.Loosely(t, RemoveInactiveBuilderStats(ctx), should.BeNil)
			assert.Loosely(t, statExist(), should.BeFalse)
		})
	})
}
