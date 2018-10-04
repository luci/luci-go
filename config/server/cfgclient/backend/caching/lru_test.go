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
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/caching/lru"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/impl/memory"

	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/backend"
	"go.chromium.org/luci/config/server/cfgclient/backend/client"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLRUCache(t *testing.T) {
	t.Parallel()

	Convey(`A testing setup`, t, func() {
		c := context.Background()
		c, tc := testclock.UseTime(c, testclock.TestTimeLocal)

		mbase := map[config.Set]memory.Files{
			"projects/foo": {
				"file": "content",
			},
		}

		// Install our backend: memory backed by cache backed by force error.
		var be backend.B
		lcp := testconfig.Provider{
			Base: memory.New(mbase),
		}
		be = &client.Backend{
			Provider: &lcp,
		}

		c = backend.WithBackend(c, LRUBackend(be, lru.New(0), time.Hour))

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
				ShouldEqual, config.ErrNoConfig)

			// Re-add, still no config.
			mbase["projects/foo"] = memory.Files{
				"file": "content",
			}
			So(cfgclient.Get(c, cfgclient.AsService, "projects/foo", "file", cfgclient.String(&s), nil),
				ShouldEqual, config.ErrNoConfig)

			// "No config" expires, config is back.
			tc.Add(time.Hour)
			So(cfgclient.Get(c, cfgclient.AsService, "projects/foo", "file", cfgclient.String(&s), nil), ShouldBeNil)
			So(s, ShouldEqual, "content")
		})
	})
}
