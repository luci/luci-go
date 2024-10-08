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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/buildbucket/appengine/internal/buildid"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestDeleteOldBuilds(t *testing.T) {
	t.Parallel()

	Convey("DeleteOldBuilds", t, func() {
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
			So(datastore.Put(ctx, b), ShouldBeNil)
			So(datastore.Put(ctx, &model.BuildSteps{
				ID:    1,
				Build: datastore.KeyForObj(ctx, b),
			}), ShouldBeNil)
		}

		Convey("keeps builds", func() {
			Convey("as old as BuildStorageDuration", func() {
				setID(b, now.Add(-model.BuildStorageDuration))
				So(DeleteOldBuilds(ctx), ShouldBeNil)
				So(datastore.Get(ctx, b), ShouldBeNil)
				count, err := datastore.Count(ctx, datastore.NewQuery(""))
				So(err, ShouldBeNil)
				So(count, ShouldResemble, int64(2))
			})
			Convey("younger than BuildStorageDuration", func() {
				setID(b, now.Add(-model.BuildStorageDuration+time.Minute))
				So(DeleteOldBuilds(ctx), ShouldBeNil)
				count, err := datastore.Count(ctx, datastore.NewQuery(""))
				So(err, ShouldBeNil)
				So(count, ShouldResemble, int64(2))
			})
		})

		Convey("deletes builds older than BuildStorageDuration", func() {
			setID(b, now.Add(-model.BuildStorageDuration-time.Minute))
			So(DeleteOldBuilds(ctx), ShouldBeNil)
			So(datastore.Get(ctx, b), ShouldEqual, datastore.ErrNoSuchEntity)
			count, err := datastore.Count(ctx, datastore.NewQuery(""))
			So(err, ShouldBeNil)
			So(count, ShouldResemble, int64(0))
		})

		Convey("removes many builds", func() {
			bs := make([]model.Build, 234)
			old := now.Add(-model.BuildStorageDuration - time.Minute)
			for i := range bs {
				bs[i] = *b
				setID(&bs[i], old)
			}
			So(DeleteOldBuilds(ctx), ShouldBeNil)
			count, err := datastore.Count(ctx, datastore.NewQuery(""))
			So(err, ShouldBeNil)
			So(count, ShouldResemble, int64(0))
			So(datastore.Get(ctx, bs), ShouldErrLike,
				"datastore: no such entity (and 233 other errors)")
		})
	})
}
