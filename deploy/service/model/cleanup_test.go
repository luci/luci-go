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

package model

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCleanupOldEntities(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		now := testclock.TestRecentTimeUTC.Round(time.Millisecond)
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = memory.Use(ctx)

		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		Convey("Cleanup works", func() {
			oldActuation := &Actuation{
				ID:      "old-actuation",
				Created: clock.Now(ctx).Add(-retentionPeriod - time.Hour).UTC(),
			}
			newActuation := &Actuation{
				ID:      "new-actuation",
				Created: clock.Now(ctx).Add(-retentionPeriod + time.Hour).UTC(),
			}
			oldEntry := &AssetHistory{
				ID:      1,
				Parent:  datastore.NewKey(ctx, "Asset", "old-asset", 0, nil),
				Created: clock.Now(ctx).Add(-retentionPeriod - time.Hour).UTC(),
			}
			newEntry := &AssetHistory{
				ID:      1,
				Parent:  datastore.NewKey(ctx, "Asset", "new-asset", 0, nil),
				Created: clock.Now(ctx).Add(-retentionPeriod + time.Hour).UTC(),
			}
			So(datastore.Put(ctx, oldActuation, newActuation, oldEntry, newEntry), ShouldBeNil)

			So(CleanupOldEntities(ctx), ShouldBeNil)

			So(datastore.Get(ctx, oldActuation), ShouldEqual, datastore.ErrNoSuchEntity)
			So(datastore.Get(ctx, newActuation), ShouldBeNil)
			So(datastore.Get(ctx, oldEntry), ShouldEqual, datastore.ErrNoSuchEntity)
			So(datastore.Get(ctx, newEntry), ShouldBeNil)
		})
	})
}
