// Copyright 2024 The LUCI Authors.
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

package cfg

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	cfgmem "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/impl/memory"
)

func TestProvider(t *testing.T) {
	t.Parallel()

	var testTime = time.Date(2024, time.January, 1, 2, 3, 4, 0, time.UTC)

	ftt.Run("With test context", t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx, tc := testclock.UseTime(ctx, testTime)
		ctx = memory.Use(ctx)

		updateConfig := func(files cfgmem.Files) error {
			return UpdateConfigs(cfgclient.Use(ctx, cfgmem.New(map[config.Set]cfgmem.Files{
				"services/${appid}": files,
			})), nil, nil)
		}

		configSet1 := cfgmem.Files{
			"bots.cfg": `
				trusted_dimensions: "pool"
				trusted_dimensions: "boo"`,
		}
		configSet2 := cfgmem.Files{
			"bots.cfg": `
				trusted_dimensions: "pool"
				trusted_dimensions: "boo"`,
			"unrelated.cfg": "doesnt-matter",
		}
		configSet3 := cfgmem.Files{
			"bots.cfg": `
				trusted_dimensions: "pool"
				trusted_dimensions: "bah"`,
		}

		t.Run("Empty config", func(t *ftt.Test) {
			p, err := NewProvider(ctx)
			assert.Loosely(t, err, should.BeNil)

			cfg, err := p.Latest(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, cfg.VersionInfo.Revision, should.Equal(emptyRev))
			assert.That(t, cfg.Refreshed, should.Equal(testTime))
			assert.That(t, p.Cached(ctx), should.Equal(cfg))

			// Few seconds later returns the exact same config.
			tc.Add(4 * time.Second)
			new1, err := p.Latest(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, new1, should.Equal(cfg))

			// Even later returns the same config semantically, just with updated
			// refresh time.
			tc.Add(10 * time.Second)
			new2, err := p.Latest(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, new2, should.NotEqual(cfg))
			assert.That(t, new2.VersionInfo.Revision, should.Equal(emptyRev))
			assert.That(t, new2.Refreshed, should.Equal(testTime.Add(14*time.Second)))

			// It is also cached now.
			assert.That(t, p.Cached(ctx), should.Equal(new2))
		})

		t.Run("Config changes", func(t *ftt.Test) {
			// Start with the default empty config.
			p, err := NewProvider(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, p.Cached(ctx).VersionInfo.Revision, should.Equal(emptyRev))

			// A new config appears in the datastore.
			assert.Loosely(t, updateConfig(configSet1), should.BeNil)

			// Latest returns it now.
			cfg, err := p.Latest(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, cfg.VersionInfo.Revision, should.Equal("37053cd507f1def1ed78dc2b3103d62629fa1f44"))
			assert.That(t, cfg.VersionInfo.Fetched, should.Equal(testTime))
			assert.That(t, p.Cached(ctx), should.Equal(cfg))

			// A new config revision appears, but it is semantically the same.
			tc.Add(1 * time.Second)
			assert.Loosely(t, updateConfig(configSet2), should.BeNil)

			// Latest ignores it at first.
			cfg, err = p.Latest(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, cfg.VersionInfo.Revision, should.Equal("37053cd507f1def1ed78dc2b3103d62629fa1f44"))
			assert.That(t, cfg.VersionInfo.Fetched, should.Equal(testTime))
			assert.That(t, p.Cached(ctx), should.Equal(cfg))

			// But at some later time updates the non-semantic info.
			tc.Add(10 * time.Second)
			cfg, err = p.Latest(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, cfg.VersionInfo.Revision, should.Equal("b016ee8be51456ec214affaa705fb45c30b29fb5"))
			assert.That(t, cfg.VersionInfo.Fetched, should.Equal(testTime)) // no change! the actual config didn't change
			assert.That(t, p.Cached(ctx), should.Equal(cfg))

			// A real config change happens.
			tc.Add(1 * time.Second)
			assert.Loosely(t, updateConfig(configSet3), should.BeNil)

			// It is picked up right away.
			cfg, err = p.Latest(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, cfg.VersionInfo.Revision, should.Equal("2b3ef3c422c800c9eef3ad58815912209b777945"))
			assert.That(t, cfg.VersionInfo.Fetched, should.Equal(testTime.Add(12*time.Second)))
			assert.That(t, p.Cached(ctx), should.Equal(cfg))
		})

		t.Run("Config rollback", func(t *ftt.Test) {
			// Loaded the initial version of the config.
			assert.Loosely(t, updateConfig(configSet1), should.BeNil)
			p, err := NewProvider(ctx)
			assert.Loosely(t, err, should.BeNil)
			cfg := p.Cached(ctx)
			assert.That(t, cfg.VersionInfo.Revision, should.Equal("37053cd507f1def1ed78dc2b3103d62629fa1f44"))
			assert.That(t, cfg.VersionInfo.Fetched, should.Equal(testTime))

			// A new config lands and soon gets reverted.
			tc.Add(1 * time.Second)
			assert.Loosely(t, updateConfig(configSet3), should.BeNil)
			tc.Add(1 * time.Second)
			assert.Loosely(t, updateConfig(configSet1), should.BeNil)

			// This gets loads as a new config (with updated Fetched).
			cfg, err = p.Latest(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, cfg.VersionInfo.Revision, should.Equal("37053cd507f1def1ed78dc2b3103d62629fa1f44"))
			assert.That(t, cfg.VersionInfo.Fetched, should.Equal(testTime.Add(2*time.Second)))
		})
	})
}

func TestProviderConcurrencyStressTest(t *testing.T) {
	t.Parallel()

	// Note: not mocking time in this test.
	ctx := memory.Use(context.Background())

	updateConfig := func(counter int) {
		files := cfgmem.Files{
			"settings.cfg": fmt.Sprintf("max_bot_sleep_time: %d", counter),
		}
		err := UpdateConfigs(cfgclient.Use(ctx, cfgmem.New(map[config.Set]cfgmem.Files{
			"services/${appid}": files,
		})), nil, nil)
		assert.Loosely(t, err, should.BeNil)
	}

	loadedCounter := func(cfg *Config) int {
		return int(cfg.settings.MaxBotSleepTime)
	}

	randomSleep := func(period time.Duration) {
		time.Sleep(time.Duration(rand.Int63n(int64(period))))
	}

	updateConfig(1)

	provider, err := NewProvider(ctx)
	assert.Loosely(t, err, should.BeNil)

	cfg := provider.Cached(ctx)
	assert.That(t, loadedCounter(cfg), should.Equal(1))
	initialFetched := cfg.VersionInfo.Fetched
	initialRefreshed := cfg.Refreshed

	// Hit VersionInfo refresh code path often and with more concurrency
	// (effective remove refresh age randomization).
	provider.versionInfoRefreshMinAge = time.Millisecond
	provider.versionInfoRefreshMaxAge = time.Millisecond + time.Nanosecond

	// Hammer Latest() concurrently from many goroutines, while updating the
	// config in parallel a bunch of times. Each individual goroutine should
	// never see the config going back in time. And they all should eventually
	// see the latest config.

	const goroutineCount = 200
	const maxCounterValue = 100

	var wg sync.WaitGroup
	wg.Add(goroutineCount)
	defer wg.Wait()

	for idx := 0; idx < goroutineCount; idx++ {
		idx := idx

		go func() {
			defer wg.Done()

			lastSeenCounter := 1
			lastFetched := initialFetched
			lastRefreshed := initialRefreshed

			rounds := 0
			updates := 0

			for lastSeenCounter != maxCounterValue {
				rounds++

				cfg, err := provider.Latest(ctx)
				assert.Loosely(t, err, should.BeNil)

				// The config content should only go forward in time.
				latestCounter := loadedCounter(cfg)
				if latestCounter != lastSeenCounter {
					updates++
				}
				if latestCounter < lastSeenCounter {
					t.Errorf("Going back in time: seen counter %d after seeing %d", latestCounter, lastSeenCounter)
				}
				lastSeenCounter = latestCounter

				// Fetched should only go forward in time.
				if cfg.VersionInfo.Fetched.Before(lastFetched) {
					t.Errorf("Going back in time: seen fetched %s after seeing %s", cfg.VersionInfo.Fetched, lastFetched)
				}
				lastFetched = cfg.VersionInfo.Fetched

				// Refreshed should only go forward in time.
				if cfg.Refreshed.Before(lastRefreshed) {
					t.Errorf("Going back in time: seen refreshed %s after seeing %s", cfg.Refreshed, lastRefreshed)
				}
				lastRefreshed = cfg.Refreshed

				randomSleep(3 * time.Millisecond)
			}

			t.Logf("Goroutine #%d did %d rounds and saw %d updates", idx, rounds, updates)
		}()
	}

	for i := 2; i <= maxCounterValue; i++ {
		updateConfig(i)
		randomSleep(10 * time.Millisecond)
	}
}
