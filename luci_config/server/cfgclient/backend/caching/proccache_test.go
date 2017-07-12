// Copyright 2015 The LUCI Authors.
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
