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

package gaeconfig

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	configPB "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	memConfig "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/backend"
	"go.chromium.org/luci/config/server/cfgclient/backend/client"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/settings"

	"go.chromium.org/gae/filter/count"
	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func projectConfigWithAccess(project string, access ...string) *configPB.ProjectCfg {
	return &configPB.ProjectCfg{
		Name:   project,
		Access: access,
	}
}

// stripMeta strips cache-specific identifying information from a set of Metas.
func stripMeta(metas []*config.Meta) []*config.Meta {
	for _, meta := range metas {
		meta.ContentHash = ""
		meta.Revision = ""
		meta.ViewURL = ""
	}
	return metas
}

func TestDatastoreCacheIntegration(t *testing.T) {
	t.Parallel()

	Convey(`A testing environment with fake cache and config service`, t, func() {
		c, clk := testclock.UseTime(context.Background(), ds.RoundTime(testclock.TestTimeUTC))

		c = memory.Use(c)
		c = caching.WithEmptyProcessCache(c)

		// Install fake auth state.
		var authState authtest.FakeState
		c = auth.WithState(c, &authState)
		authState.Identity = "user:person@example.com"
		authState.IdentityGroups = []string{"all", "users"}

		// Use a memory-backed Config service instance.
		baseMap := map[config.Set]memConfig.Files{
			"projects/open": map[string]string{
				"project.cfg": proto.MarshalTextString(projectConfigWithAccess("foo", "group:all")),
				"test.cfg":    "Test Config Content",
			},
			"projects/foo": map[string]string{
				"project.cfg": proto.MarshalTextString(projectConfigWithAccess("foo", "group:restricted")),
			},
			"projects/noconfig": map[string]string{},
			"projects/invalidconfig": map[string]string{
				"project.cfg": "!!! not a valid config !!!",
			},
			"projects/foo/refs/heads/master": map[string]string{
				"ref.cfg": "foo",
			},
			"projects/foo/refs/branches/bar": map[string]string{
				"ref.cfg": "foo/bar",
			},
			"projects/bar/refs/heads/master": map[string]string{
				"ref.cfg": "bar",
			},
		}
		base := memConfig.New(baseMap)
		be := &client.Backend{
			Provider: &testconfig.Provider{
				Base: base,
			},
		}

		// Install our settings into a memory-backed Settings storage.
		memSettings := settings.MemoryStorage{}
		c = settings.Use(c, settings.New(&memSettings))
		s := Settings{
			CacheExpirationSec: 10,
			DatastoreCacheMode: DSCacheEnabled,
		}
		putSettings := func() {
			if err := settings.GetSettings(c).Set(c, settingsKey, &s, "test harness", "initial settings"); err != nil {
				panic(err)
			}
		}
		putSettings()

		// Install our middleware chain into our Router.
		baseMW := router.NewMiddlewareChain(func(ctx *router.Context, next router.Handler) {
			ctx.Context = c
			next(ctx)
		})
		rtr := router.New()
		server := httptest.NewServer(rtr)
		defer server.Close()

		// Install our Manager cron handlers.
		installCacheCronHandlerImpl(rtr, baseMW, be)

		runCron := func() int {
			ds.GetTestable(c).CatchupIndexes()
			resp, err := http.Get(fmt.Sprintf("%s/admin/config/cache/manager", server.URL))
			if err != nil {
				panic(fmt.Errorf("failed to GET: %s", err))
			}
			return resp.StatusCode
		}
		So(runCron(), ShouldEqual, http.StatusOK)

		// Called when all parameters are in place to install our config service
		// layers into "c".
		installConfig := func(c context.Context) context.Context { return useImpl(c, be) }

		loadProjectConfigs := func(c context.Context, a cfgclient.Authority) (projs []string, meta []*config.Meta, err error) {
			err = cfgclient.Projects(c, a, "project.cfg", cfgclient.StringSlice(&projs), &meta)
			return
		}

		Convey(`Cache modes`, func() {
			// Load all of our project configs and metadata. Load directly from our
			// non-datastore Backend, since this data is not part of the test.
			allProjs, _, err := loadProjectConfigs(backend.WithBackend(c, be), cfgclient.AsService)
			if err != nil {
				panic(err)
			}

			Convey(`Disabled, skips datastore cache.`, func() {
				s.DatastoreCacheMode = DSCacheDisabled
				putSettings()

				c = installConfig(c)
				c, cnt := count.FilterRDS(c)

				projs, _, err := loadProjectConfigs(c, cfgclient.AsService)
				So(err, ShouldBeNil)
				So(projs, ShouldResemble, allProjs)

				// No datastore operations should have been performed.
				So(cnt.GetMulti.Total(), ShouldEqual, 0)
				So(cnt.PutMulti.Total(), ShouldEqual, 0)
			})

			Convey(`Enabled`, func() {
				s.DatastoreCacheMode = DSCacheEnabled
				putSettings()

				c = installConfig(c)
				c, cnt := count.FilterRDS(c)
				projs, _, err := loadProjectConfigs(c, cfgclient.AsService)
				So(err, ShouldBeNil)
				So(projs, ShouldResemble, allProjs)

				So(cnt.GetMulti.Total(), ShouldEqual, 1)
				So(cnt.PutMulti.Total(), ShouldEqual, 1)

				// Reload from cache. Should not touch datastore b/c local caches are
				// retaining the results.
				projs, _, err = loadProjectConfigs(c, cfgclient.AsService)
				So(err, ShouldBeNil)
				So(projs, ShouldResemble, allProjs)

				So(cnt.GetMulti.Total(), ShouldEqual, 1)
				So(cnt.PutMulti.Total(), ShouldEqual, 1)

				// Expire the cache. Now that the entries are expired, we will
				// continue to load the configuration from the datastore.
				clk.Add(time.Hour)

				projs, _, err = loadProjectConfigs(c, cfgclient.AsService)
				So(err, ShouldBeNil)

				So(cnt.GetMulti.Total(), ShouldEqual, 2)
				So(cnt.PutMulti.Total(), ShouldEqual, 1)
			})
		})

		Convey(`A user, denied access to a project, gains access once project config refreshes.`, func() {
			c = installConfig(c)

			_, metas, err := loadProjectConfigs(c, cfgclient.AsUser)
			So(err, ShouldBeNil)
			So(stripMeta(metas), ShouldResemble, []*config.Meta{
				{ConfigSet: "projects/open", Path: "project.cfg"},
			})

			// Update "foo" project ACLs to include the "users" group.
			baseMap["projects/foo"]["project.cfg"] = proto.MarshalTextString(projectConfigWithAccess("foo", "group:users"))

			// Still denied access (cached).
			c = installConfig(c)
			_, metas, err = loadProjectConfigs(c, cfgclient.AsUser)
			So(err, ShouldBeNil)
			So(stripMeta(metas), ShouldResemble, []*config.Meta{
				{ConfigSet: "projects/open", Path: "project.cfg"},
			})

			// Expire cache.
			clk.Add(time.Hour)
			_, _, err = loadProjectConfigs(c, cfgclient.AsUser)
			So(err, ShouldBeNil)

			// Update our cache entries.
			So(runCron(), ShouldEqual, http.StatusOK)

			// Our datastore is refreshed, but our in-memory cache still holds the
			// value from the previous lookup. Let that expire.
			clk.Add(10 * time.Second)
			c = installConfig(c)
			_, metas, err = loadProjectConfigs(c, cfgclient.AsUser)
			So(err, ShouldBeNil)
			So(stripMeta(metas), ShouldResemble, []*config.Meta{
				{ConfigSet: "projects/foo", Path: "project.cfg"},
				{ConfigSet: "projects/open", Path: "project.cfg"},
			})
		})

		Convey(`GetConfig iterative updates`, func() {
			c = installConfig(c)

			get := func(c context.Context) (v string, meta config.Meta, err error) {
				err = cfgclient.Get(c, cfgclient.AsService, "projects/open", "test.cfg", cfgclient.String(&v), &meta)
				return
			}

			baseContentHash := func(configSet config.Set, config string) string {
				cfg, err := base.GetConfig(c, configSet, config, true)
				if err != nil {
					panic(err)
				}
				return cfg.ContentHash
			}

			_, meta, err := get(c)
			So(err, ShouldBeNil)
			So(meta.ContentHash, ShouldEqual, baseContentHash("projects/open", "test.cfg"))

			Convey(`When the content changes, the hash is updated.`, func() {
				// Change content, hash should not have changed.
				baseMap["projects/open"]["test.cfg"] = "New Content!"
				_, meta2, err := get(c)
				So(err, ShouldBeNil)
				So(meta2.ContentHash, ShouldEqual, meta.ContentHash)

				// Expire the cache entry and refresh.
				clk.Add(time.Hour)
				So(runCron(), ShouldEqual, http.StatusOK)

				// Re-get the config. The content hash should have changed.
				_, meta, err = get(c)
				So(err, ShouldBeNil)
				So(meta.ContentHash, ShouldEqual, baseContentHash("projects/open", "test.cfg"))
				So(meta.ContentHash, ShouldNotEqual, meta2.ContentHash)
			})

			Convey(`When the content is deleted, returns ErrNoConfig.`, func() {
				// Change content, hash should not have changed.
				delete(baseMap["projects/open"], "test.cfg")
				_, meta2, err := get(c)
				So(err, ShouldBeNil)
				So(meta2.ContentHash, ShouldEqual, meta.ContentHash)

				// Expire the cache entry and refresh.
				clk.Add(time.Hour)
				So(runCron(), ShouldEqual, http.StatusOK)

				_, _, err = get(c)
				So(err, ShouldEqual, config.ErrNoConfig)
			})
		})

		// Load project config, return a slice of hashes.
		type getAllFn func(context.Context, cfgclient.Authority, string, cfgclient.MultiResolver, *[]*config.Meta) error
		getHashes := func(c context.Context, gaFn getAllFn, path string) ([]string, error) {
			var (
				content []string
				metas   []*config.Meta
			)
			err := gaFn(c, cfgclient.AsService, path, cfgclient.StringSlice(&content), &metas)
			if err != nil {
				return nil, err
			}

			hashes := make([]string, len(metas))
			for i, meta := range metas {
				hashes[i] = meta.ContentHash
			}
			return hashes, nil
		}

		// Return project config hashes from backing memory config store.
		type baseConfigsFn func(context.Context, string, bool) ([]config.Config, error)
		baseContentHashes := func(fn baseConfigsFn, path string) []string {
			cfgs, err := fn(c, path, true)
			if err != nil {
				panic(err)
			}
			hashes := make([]string, len(cfgs))
			for i := range cfgs {
				hashes[i] = cfgs[i].ContentHash
			}
			return hashes
		}

		Convey(`Projects for iterative updates`, func() {
			c = installConfig(c)

			hashes, err := getHashes(c, cfgclient.Projects, "project.cfg")
			So(err, ShouldBeNil)
			So(hashes, ShouldResemble, baseContentHashes(base.GetProjectConfigs, "project.cfg"))

			Convey(`When the project list doesn't change, is not re-fetched.`, func() {
				// Expire the cache entry and refresh.
				clk.Add(time.Hour)
				So(runCron(), ShouldEqual, http.StatusOK)

				hashes2, err := getHashes(c, cfgclient.Projects, "project.cfg")
				So(err, ShouldBeNil)
				So(hashes2, ShouldResemble, hashes)
			})

			Convey(`When the project changes, is re-fetched.`, func() {
				delete(baseMap["projects/invalidconfig"], "project.cfg")

				// Expire the cache entry and refresh.
				clk.Add(time.Hour)
				So(runCron(), ShouldEqual, http.StatusOK)

				hashes2, err := getHashes(c, cfgclient.Projects, "project.cfg")
				So(err, ShouldBeNil)
				So(hashes2, ShouldResemble, baseContentHashes(base.GetProjectConfigs, "project.cfg"))
				So(hashes2, ShouldNotResemble, hashes)
			})
		})

		Convey(`Ref for iterative updates`, func() {
			c = installConfig(c)

			hashes, err := getHashes(c, cfgclient.Refs, "ref.cfg")
			So(err, ShouldBeNil)
			So(hashes, ShouldResemble, baseContentHashes(base.GetRefConfigs, "ref.cfg"))

			Convey(`When the ref list doesn't change, is not re-fetched.`, func() {
				// Expire the cache entry and refresh.
				clk.Add(time.Hour)
				So(runCron(), ShouldEqual, http.StatusOK)

				hashes2, err := getHashes(c, cfgclient.Refs, "ref.cfg")
				So(err, ShouldBeNil)
				So(hashes2, ShouldResemble, hashes)
			})

			Convey(`When the ref changes, is re-fetched.`, func() {
				delete(baseMap["projects/foo/refs/branches/bar"], "ref.cfg")

				// Expire the cache entry and refresh.
				clk.Add(time.Hour)
				So(runCron(), ShouldEqual, http.StatusOK)

				hashes2, err := getHashes(c, cfgclient.Refs, "ref.cfg")
				So(err, ShouldBeNil)
				So(hashes2, ShouldResemble, baseContentHashes(base.GetRefConfigs, "ref.cfg"))
				So(hashes2, ShouldNotResemble, hashes)
			})
		})
	})
}
