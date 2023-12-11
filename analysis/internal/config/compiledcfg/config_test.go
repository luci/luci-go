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

package compiledcfg

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/analysis/internal/config"
	configpb "go.chromium.org/luci/analysis/proto/config"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCompiledConfig(t *testing.T) {
	Convey(`With In-Process Cache`, t, func() {
		ctx := context.Background()
		ctx = memory.Use(ctx)
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx, tc := testclock.UseTime(ctx, testclock.TestTimeUTC)

		create := func(uniqifier int) {
			cfg := generateProjectConfig(uniqifier)
			projectsCfg := map[string]*configpb.ProjectConfig{
				"myproject": cfg,
			}
			err := config.SetTestProjectConfig(ctx, projectsCfg)
			So(err, ShouldBeNil)
		}
		clear := func() {
			projectsCfg := map[string]*configpb.ProjectConfig{}
			err := config.SetTestProjectConfig(ctx, projectsCfg)
			So(err, ShouldBeNil)
		}
		verify := func(minimumVersion time.Time, uniqifier int) {
			cfg, err := Project(ctx, "myproject", minimumVersion)
			So(err, ShouldBeNil)

			expectedCfg := generateProjectConfig(uniqifier)
			So(cfg.Config, ShouldResembleProto, expectedCfg)
			So(cfg.LastUpdated, ShouldEqual, expectedCfg.LastUpdated.AsTime())
			So(len(cfg.TestNameRules), ShouldEqual, 1)

			testName := fmt.Sprintf(`ninja://test_name/%v`, uniqifier)
			rule := cfg.TestNameRules[0]
			like, ok := rule(testName)
			So(ok, ShouldBeTrue)
			So(like, ShouldEqual, testName+"%")

			So(cfg.ReasonMaskPatterns[0].String(), ShouldEqual, `(?:^\[Fixture failure\] )[a-zA-Z0-9_]+(?:[:])`)
		}
		verifyNotExists := func(minimumVersion time.Time) {
			cfg, err := Project(ctx, "myproject", minimumVersion)
			So(err, ShouldBeNil)
			So(cfg.Config, ShouldResembleProto, &configpb.ProjectConfig{LastUpdated: timestamppb.New(config.StartingEpoch)})
		}
		Convey(`Does not exist`, func() {
			verifyNotExists(config.StartingEpoch)

			Convey(`Then exists`, func() {
				create(1)

				// Verify the old cache item is retained.
				verifyNotExists(config.StartingEpoch)

				Convey(`Evict by cache expiry`, func() {
					// Let the cache expire (note this expires the cache
					// in the config package, not this package).
					tc.Add(2 * config.ProjectCacheExpiry)

					verify(config.StartingEpoch, 1)
					verify(configVersion(1), 1)
				})
				Convey(`Manually evict`, func() {
					// Force the cache to be cleared by requesting
					// a more recent version of config.
					verify(configVersion(1), 1)

					verify(config.StartingEpoch, 1)
				})
			})
			Convey(`Then not exists`, func() {
				clear()
				verifyNotExists(config.StartingEpoch)
			})
		})
		Convey(`Exists`, func() {
			create(1)
			verify(config.StartingEpoch, 1)
			verify(configVersion(1), 1)

			Convey(`Then modify`, func() {
				create(2)

				// Verify the old entry is retained.
				verify(config.StartingEpoch, 1)
				verify(configVersion(1), 1)

				Convey(`Evict by cache expiry`, func() {
					// Let the cache expire (note this expires the cache
					// in the config package, not this package).
					tc.Add(2 * config.ProjectCacheExpiry)

					verify(config.StartingEpoch, 2)
					verify(configVersion(1), 2)
					verify(configVersion(2), 2)
				})
				Convey(`Manually evict`, func() {
					// Force the cache to be cleared by requesting
					// a more recent version of config.
					verify(configVersion(2), 2)

					verify(configVersion(1), 2)
					verify(config.StartingEpoch, 2)
				})
			})
			Convey(`Then retain`, func() {
				// Let the cache expire (note this expires the cache
				// in the config package, not this package).
				tc.Add(2 * config.ProjectCacheExpiry)

				verify(config.StartingEpoch, 1)
				verify(configVersion(1), 1)
			})
			Convey(`Then delete`, func() {
				clear()

				// Let the cache expire (note this expires the cache
				// in the config package, not this package).
				tc.Add(2 * config.ProjectCacheExpiry)

				verifyNotExists(config.StartingEpoch)
			})
		})
	})
}

func generateProjectConfig(uniqifier int) *configpb.ProjectConfig {
	cfg := &configpb.Clustering{
		TestNameRules: []*configpb.TestNameClusteringRule{
			{
				Name:         "Google Test (Value-parameterized)",
				Pattern:      fmt.Sprintf(`^ninja://test_name/%v$`, uniqifier),
				LikeTemplate: fmt.Sprintf(`ninja://test_name/%v%%`, uniqifier),
			},
		},
		ReasonMaskPatterns: []string{`(?:^\[Fixture failure\] )[a-zA-Z0-9_]+(?:[:])`},
	}
	version := configVersion(uniqifier)
	projectCfg := &configpb.ProjectConfig{
		Clustering:  cfg,
		LastUpdated: timestamppb.New(version),
	}
	return projectCfg
}

func configVersion(uniqifier int) time.Time {
	return time.Date(2020, 1, 2, 3, 4, 5, uniqifier, time.UTC)
}
