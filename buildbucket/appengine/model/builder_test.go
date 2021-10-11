// Copyright 2021 The LUCI Authors.
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

package model

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	// TODO(crbug/1242998): Remove once safe get becomes datastore default.
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
)

func TestBuilderStat(t *testing.T) {
	t.Parallel()

	Convey("BuilderStat", t, func() {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		t := testclock.TestTimeUTC
		So(datastore.Put(ctx, &BuilderStat{
			ID:            "proj:bucket:builder1",
			LastScheduled: t,
		}), ShouldBeNil)

		Convey("update builder", func() {
			builds := []*Build{
				{
					ID: 1,
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "proj",
							Bucket:  "bucket",
							Builder: "builder1",
						},
					},
				},
				{
					ID: 2,
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "proj",
							Bucket:  "bucket",
							Builder: "builder2",
						},
					},
				},
				{
					ID: 3,
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "proj",
							Bucket:  "bucket",
							Builder: "builder1",
						},
					},
				},
			}
			now := t.Add(3600 * time.Second)
			err := UpdateBuilderStat(ctx, builds, now)
			So(err, ShouldBeNil)

			currentBuilders := []*BuilderStat{
				{ID: "proj:bucket:builder1"},
				{ID: "proj:bucket:builder2"},
			}
			err = datastore.Get(ctx, currentBuilders)
			So(err, ShouldBeNil)
			So(currentBuilders, ShouldResemble, []*BuilderStat{
				{
					ID:            "proj:bucket:builder1",
					LastScheduled: datastore.RoundTime(now),
				},
				{
					ID:            "proj:bucket:builder2",
					LastScheduled: datastore.RoundTime(now),
				},
			})
		})

		Convey("uninitialized build.proto.builder", func() {
			builds := []*Build{{ID: 1}}
			So(func() { UpdateBuilderStat(ctx, builds, t) }, ShouldPanic)
		})
	})
}
