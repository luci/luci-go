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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/analysis/internal/config"
	configpb "go.chromium.org/luci/analysis/proto/config"
)

func TestCompiledConfig(t *testing.T) {
	ftt.Run(`With In-Process Cache`, t, func(t *ftt.Test) {
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
			assert.Loosely(t, err, should.BeNil)
		}
		clear := func() {
			projectsCfg := map[string]*configpb.ProjectConfig{}
			err := config.SetTestProjectConfig(ctx, projectsCfg)
			assert.Loosely(t, err, should.BeNil)
		}
		verify := func(minimumVersion time.Time, uniqifier int) {
			cfg, err := Project(ctx, "myproject", minimumVersion)
			assert.Loosely(t, err, should.BeNil)

			expectedCfg := generateProjectConfig(uniqifier)
			assert.Loosely(t, cfg.Config, should.Resemble(expectedCfg))
			assert.That(t, cfg.LastUpdated, should.Match(expectedCfg.LastUpdated.AsTime()))
			assert.Loosely(t, len(cfg.TestNameRules), should.Equal(1))

			testName := fmt.Sprintf(`ninja://test_name/%v`, uniqifier)
			rule := cfg.TestNameRules[0]
			like, ok := rule(testName)
			assert.Loosely(t, ok, should.BeTrue)
			assert.Loosely(t, like, should.Equal(testName+"%"))

			assert.Loosely(t, cfg.ReasonMaskPatterns[0].String(), should.Equal(`(?:^\[Fixture failure\] )[a-zA-Z0-9_]+(?:[:])`))
		}
		verifyNotExists := func(minimumVersion time.Time) {
			cfg, err := Project(ctx, "myproject", minimumVersion)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, cfg.Config, should.Resemble(&configpb.ProjectConfig{LastUpdated: timestamppb.New(config.StartingEpoch)}))
		}
		t.Run(`Does not exist`, func(t *ftt.Test) {
			verifyNotExists(config.StartingEpoch)

			t.Run(`Then exists`, func(t *ftt.Test) {
				create(1)

				// Verify the old cache item is retained.
				verifyNotExists(config.StartingEpoch)

				t.Run(`Evict by cache expiry`, func(t *ftt.Test) {
					// Let the cache expire (note this expires the cache
					// in the config package, not this package).
					tc.Add(2 * config.ProjectCacheExpiry)

					verify(config.StartingEpoch, 1)
					verify(configVersion(1), 1)
				})
				t.Run(`Manually evict`, func(t *ftt.Test) {
					// Force the cache to be cleared by requesting
					// a more recent version of config.
					verify(configVersion(1), 1)

					verify(config.StartingEpoch, 1)
				})
			})
			t.Run(`Then not exists`, func(t *ftt.Test) {
				clear()
				verifyNotExists(config.StartingEpoch)
			})
		})
		t.Run(`Exists`, func(t *ftt.Test) {
			create(1)
			verify(config.StartingEpoch, 1)
			verify(configVersion(1), 1)

			t.Run(`Then modify`, func(t *ftt.Test) {
				create(2)

				// Verify the old entry is retained.
				verify(config.StartingEpoch, 1)
				verify(configVersion(1), 1)

				t.Run(`Evict by cache expiry`, func(t *ftt.Test) {
					// Let the cache expire (note this expires the cache
					// in the config package, not this package).
					tc.Add(2 * config.ProjectCacheExpiry)

					verify(config.StartingEpoch, 2)
					verify(configVersion(1), 2)
					verify(configVersion(2), 2)
				})
				t.Run(`Manually evict`, func(t *ftt.Test) {
					// Force the cache to be cleared by requesting
					// a more recent version of config.
					verify(configVersion(2), 2)

					verify(configVersion(1), 2)
					verify(config.StartingEpoch, 2)
				})
			})
			t.Run(`Then retain`, func(t *ftt.Test) {
				// Let the cache expire (note this expires the cache
				// in the config package, not this package).
				tc.Add(2 * config.ProjectCacheExpiry)

				verify(config.StartingEpoch, 1)
				verify(configVersion(1), 1)
			})
			t.Run(`Then delete`, func(t *ftt.Test) {
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
