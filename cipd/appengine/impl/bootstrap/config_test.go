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

package bootstrap

import (
	"context"
	"testing"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/impl/memory"
	gae "go.chromium.org/luci/gae/impl/memory"
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

			cfg, err := BootstrapConfig(ctx, "some/pkg")
			So(err, ShouldBeNil)
			So(cfg, ShouldBeNil)
		})

		Convey("Broken config", func() {
			mockConfig("broken")
			So(ImportConfig(ctx), ShouldErrLike, "validation errors")
		})

		Convey("Good config", func() {
			mockConfig(`
				bootstrap_config {
					prefix: "pkg/a/specific"
				}
				bootstrap_config {
					prefix: "pkg/a"
				}
			`)
			So(ImportConfig(ctx), ShouldBeNil)

			Convey("Scans in order", func() {
				cfg, err := BootstrapConfig(ctx, "pkg/a/specific/zzz")
				So(err, ShouldBeNil)
				So(cfg.Prefix, ShouldEqual, "pkg/a/specific")

				cfg, err = BootstrapConfig(ctx, "pkg/a/specific")
				So(err, ShouldBeNil)
				So(cfg.Prefix, ShouldEqual, "pkg/a/specific")

				cfg, err = BootstrapConfig(ctx, "pkg/a/another")
				So(err, ShouldBeNil)
				So(cfg.Prefix, ShouldEqual, "pkg/a")
			})

			Convey("No matching entry", func() {
				cfg, err := BootstrapConfig(ctx, "pkg/b/another")
				So(err, ShouldBeNil)
				So(cfg, ShouldBeNil)
			})
		})
	})
}
