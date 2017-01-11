// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package memcache

import (
	"math/rand"
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock/testclock"
	memconfig "github.com/luci/luci-go/common/config/impl/memory"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/luci_config/server/cfgclient"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/caching"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/client"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/testconfig"

	"github.com/luci/gae/filter/featureBreaker"
	"github.com/luci/gae/impl/memory"
	mc "github.com/luci/gae/service/memcache"

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

		getMCStats := func(c context.Context) *mc.Statistics {
			st, err := mc.Stats(c)
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

		var be backend.B
		be = &client.Backend{
			Provider: &testconfig.Provider{
				Base: memconfig.New(configDB),
			},
		}
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
					Schema:    caching.Schema,
					Op:        caching.OpGet,
					ConfigSet: "projects/foo",
					Path:      "test.cfg",
					Content:   true,
					Authority: backend.AsService,
				}
				So(mc.Set(c, mc.NewItem(c, memcacheKey(&cacheKey)).SetValue([]byte("!!! trash !!!"))), ShouldBeNil)
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
