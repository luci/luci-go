// Copyright 2016 The LUCI Authors.
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

package memcache

import (
	"math/rand"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	memconfig "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/backend"
	"go.chromium.org/luci/config/server/cfgclient/backend/caching"
	"go.chromium.org/luci/config/server/cfgclient/backend/client"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"

	"go.chromium.org/gae/filter/featureBreaker"
	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/memcache"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMemcache(t *testing.T) {
	t.Parallel()

	Convey("Test cache", t, func() {
		c := context.Background()
		c = memory.Use(c)

		c, clk := testclock.UseTime(c, testclock.TestTimeUTC)
		_ = clk

		c, mcFB := featureBreaker.FilterMC(c, nil)

		getMCStats := func(c context.Context) *memcache.Statistics {
			st, err := memcache.Stats(c)
			if err != nil {
				panic(err)
			}
			return st
		}

		configDB := map[string]memconfig.ConfigSet{
			"projects/foo": {
				"test.cfg": "foo",
			},
		}

		var be backend.B = &client.Backend{
			Provider: &testconfig.Provider{
				Base: memconfig.New(configDB),
			},
		}
		mcURL := be.ServiceURL(c)
		be = Backend(be, time.Minute)
		c = backend.WithBackend(c, be)

		Convey(`Pulling items from memcache`, func() {
			var content string

			Convey(`Should be able to fetch a config as service`, func() {
				So(cfgclient.Get(c, cfgclient.AsService, "projects/foo", "test.cfg", cfgclient.String(&content), nil), ShouldBeNil)
				So(content, ShouldEqual, "foo")
				So(getMCStats(c).Misses, ShouldEqual, 1)
				So(getMCStats(c).Hits, ShouldEqual, 0)
				So(getMCStats(c).Items, ShouldEqual, 1)

				// A second fetch hits memcache.
				So(cfgclient.Get(c, cfgclient.AsService, "projects/foo", "test.cfg", cfgclient.String(&content), nil), ShouldBeNil)
				So(content, ShouldEqual, "foo")
				So(getMCStats(c).Misses, ShouldEqual, 1)
				So(getMCStats(c).Hits, ShouldEqual, 1)
				So(getMCStats(c).Items, ShouldEqual, 1)

				// Memcache expires, full reload.
				clk.Add(time.Minute)

				So(cfgclient.Get(c, cfgclient.AsService, "projects/foo", "test.cfg", cfgclient.String(&content), nil), ShouldBeNil)
				So(content, ShouldEqual, "foo")
				So(getMCStats(c).Misses, ShouldEqual, 2)
				So(getMCStats(c).Hits, ShouldEqual, 1)
				So(getMCStats(c).Items, ShouldEqual, 1)
			})

			Convey(`When memcache is broken`, func() {
				testErr := errors.New("test error")
				mcFB.BreakFeatures(testErr, "GetMulti")

				So(cfgclient.Get(c, cfgclient.AsService, "projects/foo", "test.cfg", cfgclient.String(&content), nil), ShouldBeNil)
				So(content, ShouldEqual, "foo")
				So(getMCStats(c).Misses, ShouldEqual, 0)
				So(getMCStats(c).Hits, ShouldEqual, 0)
				So(getMCStats(c).Items, ShouldEqual, 0)
			})

			Convey(`When a cached entry is corrupted`, func() {
				cacheKey := caching.Key{
					Schema:     caching.Schema,
					ServiceURL: mcURL.String(),
					Op:         caching.OpGet,
					ConfigSet:  "projects/foo",
					Path:       "test.cfg",
					Content:    true,
					Authority:  backend.AsService,
				}
				So(memcache.Set(c, memcache.NewItem(c, memcacheKey(&cacheKey)).SetValue([]byte("!!! trash !!!"))), ShouldBeNil)
				So(cfgclient.Get(c, cfgclient.AsService, "projects/foo", "test.cfg", cfgclient.String(&content), nil), ShouldBeNil)
				So(content, ShouldEqual, "foo")
				So(getMCStats(c).Misses, ShouldEqual, 0)
				So(getMCStats(c).Hits, ShouldEqual, 1)
				So(getMCStats(c).Items, ShouldEqual, 1)
			})

			Convey(`Will skip the cache if an item is too large.`, func() {
				// We need to use a pseudo-random string b/c we compress it.
				buf := make([]byte, maxMemCacheSize*2)
				rng := rand.New(rand.NewSource(0))
				_, _ = rng.Read(buf)

				orig := string(buf)
				configDB["projects/foo"]["test.cfg"] = orig

				So(cfgclient.Get(c, cfgclient.AsService, "projects/foo", "test.cfg", cfgclient.String(&content), nil), ShouldBeNil)
				So(content, ShouldEqual, orig)
				So(getMCStats(c).Misses, ShouldEqual, 1)
				So(getMCStats(c).Hits, ShouldEqual, 0)
				So(getMCStats(c).Items, ShouldEqual, 0)
			})
		})
	})
}
