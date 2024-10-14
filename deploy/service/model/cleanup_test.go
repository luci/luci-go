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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestCleanupOldEntities(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		now := testclock.TestRecentTimeUTC.Round(time.Millisecond)
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = memory.Use(ctx)

		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		t.Run("Cleanup works", func(t *ftt.Test) {
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
			assert.Loosely(t, datastore.Put(ctx, oldActuation, newActuation, oldEntry, newEntry), should.BeNil)

			assert.Loosely(t, CleanupOldEntities(ctx), should.BeNil)

			assert.Loosely(t, datastore.Get(ctx, oldActuation), should.Equal(datastore.ErrNoSuchEntity))
			assert.Loosely(t, datastore.Get(ctx, newActuation), should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, oldEntry), should.Equal(datastore.ErrNoSuchEntity))
			assert.Loosely(t, datastore.Get(ctx, newEntry), should.BeNil)
		})
	})
}
