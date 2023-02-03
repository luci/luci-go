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

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/impl/memory"
	gae "go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	Convey("With mocks", t, func() {
		configs := map[config.Set]memory.Files{
			"services/${appid}": map[string]string{},
		}
		mockConfig := func(body string) {
			configs["services/${appid}"][cachedCfg.Path] = body
		}

		ctx := gae.Use(context.Background())
		ctx = cfgclient.Use(ctx, memory.New(configs))
		ctx = caching.WithEmptyProcessCache(ctx)

		Convey("No config", func() {
			So(ImportConfig(ctx), ShouldBeNil)

			cfg, err := monitoringConfig(ctx)
			So(err, ShouldBeNil)
			So(cfg, ShouldBeNil)
		})

		Convey("Broken config", func() {
			mockConfig("broken")
			So(ImportConfig(ctx), ShouldErrLike, "validation errors")
		})

		Convey("Good config", func() {
			mockConfig(`
				client_monitoring_config {
					ip_whitelist: "ignored"
					label: "ignored-label"
				}
				client_monitoring_config {
					ip_whitelist: "bots"
					label: "bots-label"
				}
			`)
			So(ImportConfig(ctx), ShouldBeNil)

			Convey("Has matching entry", func() {
				e, err := monitoringConfig(auth.WithState(ctx, &authtest.FakeState{
					PeerIPAllowlist: []string{"bots"},
				}))
				So(err, ShouldBeNil)
				So(e.Label, ShouldEqual, "bots-label")
			})

			Convey("No matching entry", func() {
				e, err := monitoringConfig(auth.WithState(ctx, &authtest.FakeState{
					PeerIPAllowlist: []string{"something-else"},
				}))
				So(err, ShouldBeNil)
				So(e, ShouldBeNil)
			})
		})
	})
}
