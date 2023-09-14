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
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/config_service/internal/clients"
	"go.chromium.org/luci/config_service/internal/model"
	"go.chromium.org/luci/config_service/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRetention(t *testing.T) {
	t.Parallel()

	Convey("DeleteStaleConfigs", t, func() {
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
		}
		oldCfg := &model.File{
			Path:       "config.cfg",
			Revision:   datastore.MakeKey(ctx, model.ConfigSetKind, "projects/foo", model.RevisionKind, "old"),
			CreateTime: now.Add(-configRetention - 1*time.Minute),
			GcsURI:     gs.MakePath("bucket", "file"),
		}

		mockGsClient.EXPECT().Delete(gomock.Any(), gomock.Eq("bucket"), gomock.Eq("file")).Return(nil)
		So(datastore.Put(ctx, cfgset, latestCfg, oldCfg), ShouldBeNil)

		So(DeleteStaleConfigs(ctx), ShouldBeNil)
		exists, err := datastore.Exists(ctx, cfgset, latestCfg, oldCfg)
		So(err, ShouldBeNil)
		So(exists.Get(0), ShouldBeTrue)
		So(exists.Get(1), ShouldBeTrue)
		So(exists.Get(2), ShouldBeFalse)
	})
}
