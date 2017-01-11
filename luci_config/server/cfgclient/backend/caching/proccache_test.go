// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package caching

import (
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/config/impl/memory"
	"github.com/luci/luci-go/common/data/caching/proccache"
	"github.com/luci/luci-go/luci_config/server/cfgclient"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/client"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/testconfig"

	"golang.org/x/net/context"

	//. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestProcCache(t *testing.T) {
	t.Parallel()

	Convey(`A testing setup`, t, func() {
		c := context.Background()
		c, tc := testclock.UseTime(c, testclock.TestTimeLocal)

		mbase := map[string]memory.ConfigSet{
			"projects/foo": {
				"file": "content",
			},
		}

		var pc proccache.Cache
		c = proccache.Use(c, &pc)

		// Install our backend: memory backed by cache backed by force error.
		var be backend.B
		lcp := testconfig.Provider{
			Base: memory.New(mbase),
		}
		be = &client.Backend{
			Provider: &lcp,
		}
		be = ProcCache(be, time.Hour)
		c = backend.WithBackend(c, be)

		Convey(`Will store and cache a value.`, func() {
			var s string
			So(cfgclient.Get(c, cfgclient.AsService, "projects/foo", "file", cfgclient.String(&s), nil), ShouldBeNil)
			So(s, ShouldEqual, "content")

			delete(mbase, "projects/foo")

			// Cached.
			So(cfgclient.Get(c, cfgclient.AsService, "projects/foo", "file", cfgclient.String(&s), nil), ShouldBeNil)
			So(s, ShouldEqual, "content")

			// Expires, records lack of config.
			tc.Add(time.Hour)

			So(cfgclient.Get(c, cfgclient.AsService, "projects/foo", "file", cfgclient.String(&s), nil),
				ShouldEqual, cfgclient.ErrNoConfig)

			// Re-add, still no config.
			mbase["projects/foo"] = memory.ConfigSet{
				"file": "content",
			}
			So(cfgclient.Get(c, cfgclient.AsService, "projects/foo", "file", cfgclient.String(&s), nil),
				ShouldEqual, cfgclient.ErrNoConfig)

			// "No config" expires, config is back.
			tc.Add(time.Hour)
			So(cfgclient.Get(c, cfgclient.AsService, "projects/foo", "file", cfgclient.String(&s), nil), ShouldBeNil)
			So(s, ShouldEqual, "content")
		})
	})
}
