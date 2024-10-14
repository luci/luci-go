// Copyright 2023 The LUCI Authors.
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

package retention

import (
	"testing"
	"time"

	"github.com/golang/mock/gomock"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/gcloud/gs"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/testutil"
)

func TestRetention(t *testing.T) {
	t.Parallel()

	ftt.Run("DeleteStaleConfigs", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		ctl := gomock.NewController(t)
		mockGsClient := clients.NewMockGsClient(ctl)
		ctx = clients.WithGsClient(ctx, mockGsClient)

		cfgset := &model.ConfigSet{
			ID:             config.MustProjectSet("foo"),
			LatestRevision: model.RevisionInfo{ID: "latest"},
		}
		now := clock.Get(ctx).(testclock.TestClock).Now()
		latestCfg := &model.File{
			Path:       "config.cfg",
			Revision:   datastore.MakeKey(ctx, model.ConfigSetKind, "projects/foo", model.RevisionKind, "latest"),
			CreateTime: now,
			GcsURI:     gs.MakePath("bucket", "latestCfg_sha256"),
		}
		oldCfg1 := &model.File{
			Path:       "config.cfg",
			Revision:   datastore.MakeKey(ctx, model.ConfigSetKind, "projects/foo", model.RevisionKind, "old1"),
			CreateTime: now.Add(-configRetention - 1*time.Minute),
			GcsURI:     gs.MakePath("bucket", "oldCfg1_sha256"),
		}
		oldCfg2 := &model.File{
			Path:       "config.cfg",
			Revision:   datastore.MakeKey(ctx, model.ConfigSetKind, "projects/foo", model.RevisionKind, "old2"),
			CreateTime: now.Add(-configRetention - 2*time.Minute),
			GcsURI:     gs.MakePath("bucket", "oldCfg2_sha256"),
		}

		anotherCfgset := &model.ConfigSet{
			ID:             config.MustProjectSet("bar"),
			LatestRevision: model.RevisionInfo{ID: "latest"},
		}
		barCfg := &model.File{
			Path:       "config.cfg",
			Revision:   datastore.MakeKey(ctx, model.ConfigSetKind, "projects/bar", model.RevisionKind, "latest"),
			CreateTime: now,
			GcsURI:     gs.MakePath("bucket", "oldCfg2_sha256"),
		}

		mockGsClient.EXPECT().Delete(gomock.Any(), gomock.Eq("bucket"), gomock.Eq("oldCfg1_sha256")).Return(nil)
		// Don't expect oldCfg2's Gcs file to be deleted as it's referred by another in use File entity.
		mockGsClient.EXPECT().Delete(gomock.Any(), gomock.Eq("bucket"), gomock.Eq("oldCfg2_sha256")).Times(0)
		assert.Loosely(t, datastore.Put(ctx, cfgset, latestCfg, oldCfg1, oldCfg2, anotherCfgset, barCfg), should.BeNil)

		assert.Loosely(t, DeleteStaleConfigs(ctx), should.BeNil)
		exists, err := datastore.Exists(ctx, cfgset, latestCfg, oldCfg1, oldCfg2, anotherCfgset, barCfg)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, exists.Get(0), should.BeTrue)  // cfgset
		assert.Loosely(t, exists.Get(1), should.BeTrue)  // latestCfg
		assert.Loosely(t, exists.Get(2), should.BeFalse) // oldCfg1
		assert.Loosely(t, exists.Get(3), should.BeFalse) // oldCfg2
		assert.Loosely(t, exists.Get(4), should.BeTrue)  // anotherCfgset
		assert.Loosely(t, exists.Get(5), should.BeTrue)  // barCfg
	})
}
