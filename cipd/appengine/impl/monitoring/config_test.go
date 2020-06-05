// Copyright 2019 The LUCI Authors.
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

package monitoring

import (
	"context"
	"testing"

	gae "go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	ctx := gae.Use(context.Background())

	Convey("importConfig", t, func() {
		Convey("invalid", func() {
			ctx := cfgclient.Use(ctx, memory.New(map[config.Set]memory.Files{
				"services/${appid}": map[string]string{
					cfgFile: "invalid",
				},
			}))
			So(ImportConfig(ctx), ShouldErrLike, "failed to import")
		})

		Convey("empty", func() {
			ctx := cfgclient.Use(ctx, memory.New(map[config.Set]memory.Files{
				"services/${appid}": map[string]string{
					cfgFile: `
					client_monitoring_config <
						ip_whitelist: "whitelist-1"
						label: "label-1"
					>
					client_monitoring_config <
						ip_whitelist: "whitelist-2"
						label: "label-2"
					>
					`,
				},
			}))
			So(ImportConfig(ctx), ShouldBeNil)

			wl := &clientMonitoringWhitelist{
				ID: wlID,
			}
			So(datastore.Get(ctx, wl), ShouldBeNil)
			So(wl.Entries, ShouldResemble, []clientMonitoringConfig{
				{
					IPWhitelist: "whitelist-1",
					Label:       "label-1",
				},
				{
					IPWhitelist: "whitelist-2",
					Label:       "label-2",
				},
			})
		})
	})

	Convey("monitoringConfig", t, func() {
		So(datastore.Put(ctx, &clientMonitoringWhitelist{
			ID: wlID,
			Entries: []clientMonitoringConfig{
				{
					IPWhitelist: "bots",
					Label:       "bots",
				},
			},
		}), ShouldBeNil)

		Convey("not configured", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{})
			cfg, err := monitoringConfig(ctx)
			So(err, ShouldBeNil)
			So(cfg, ShouldBeNil)
		})

		Convey("configured", func() {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				PeerIPWhitelists: []string{"bots"},
			})
			cfg, err := monitoringConfig(ctx)
			So(err, ShouldBeNil)
			So(cfg, ShouldResemble, &clientMonitoringConfig{
				IPWhitelist: "bots",
				Label:       "bots",
			})
		})
	})
}
