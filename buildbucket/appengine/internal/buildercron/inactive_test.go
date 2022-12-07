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

	"go.chromium.org/luci/buildbucket/appengine/model"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResetExpiredLeases(t *testing.T) {
	t.Parallel()

	Convey("RemoveInactiveBuilderStats", t, func() {
		ctx := memory.Use(context.Background())
		now := testclock.TestTimeUTC
		ctx, _ = testclock.UseTime(ctx, now)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		Convey("works when the world is empty", func() {
			So(RemoveInactiveBuilderStats(ctx), ShouldBeNil)
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
			So(err, ShouldBeNil)
			return r.Any()
		}

		var (
			fresh = now.Add(-1 * time.Hour)
			old   = now.Add(-1 * model.BuilderExpirationDuration)
		)
		So(datastore.Put(ctx, builder), ShouldBeNil)
		So(statExist(), ShouldBeFalse)

		Convey("leaves Active Stats", func() {
			So(model.UpdateBuilderStat(ctx, builds, fresh), ShouldBeNil)
			So(RemoveInactiveBuilderStats(ctx), ShouldBeNil)
			So(statExist(), ShouldBeTrue)
		})

		Convey("removes BuilderStat for an inactive Builder", func() {
			So(model.UpdateBuilderStat(ctx, builds, old), ShouldBeNil)
			So(RemoveInactiveBuilderStats(ctx), ShouldBeNil)
			So(statExist(), ShouldBeFalse)
		})

		Convey("leaves young zombie BuilderStat", func() {
			So(model.UpdateBuilderStat(ctx, builds, fresh), ShouldBeNil)
			So(datastore.Delete(ctx, model.BuilderKey(ctx, "prj", "bkt", "bld")), ShouldBeNil)
			So(RemoveInactiveBuilderStats(ctx), ShouldBeNil)
			So(statExist(), ShouldBeTrue)
		})

		Convey("removes old zombie BuilderStat", func() {
			old := now.Add(-1 * model.BuilderStatZombieDuration)
			So(model.UpdateBuilderStat(ctx, builds, old), ShouldBeNil)
			So(datastore.Delete(ctx, model.BuilderKey(ctx, "prj", "bkt", "bld")), ShouldBeNil)
			So(RemoveInactiveBuilderStats(ctx), ShouldBeNil)
			So(statExist(), ShouldBeFalse)
		})
	})
}
