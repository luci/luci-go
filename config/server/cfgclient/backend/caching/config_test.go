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
	"encoding/hex"
	"testing"
	"time"

	"go.chromium.org/luci/common/config/impl/memory"
	"go.chromium.org/luci/config/common/cfgtypes"
	"go.chromium.org/luci/config/server/cfgclient"
	"go.chromium.org/luci/config/server/cfgclient/backend"
	"go.chromium.org/luci/config/server/cfgclient/backend/client"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

type testCache struct {
	data     map[string][]byte
	callback func(hit bool)
}

func (tc *testCache) Store(c context.Context, key string, expire time.Duration, value []byte) {
	if tc.data == nil {
		tc.data = map[string][]byte{}
	}
	tc.data[key] = value
}

func (tc *testCache) Retrieve(c context.Context, key string) []byte {
	d, ok := tc.data[key]
	if tc.callback != nil {
		tc.callback(ok)
	}
	return d
}

func (tc *testCache) invalidate() {
	tc.data = nil
}

func (tc *testCache) nothingCached() bool {
	return len(tc.data) == 0
}

type testingBackend struct {
	backend.B

	getContentCalls   int
	getNoContentCalls int

	err error
}

func (b *testingBackend) Get(c context.Context, configSet, path string, p backend.Params) (*backend.Item, error) {
	if p.Content {
		b.getContentCalls++
	} else {
		b.getNoContentCalls++
	}

	if b.err != nil {
		return nil, b.err
	}
	return b.B.Get(c, configSet, path, p)
}

func (b *testingBackend) GetAll(c context.Context, t backend.GetAllTarget, path string, p backend.Params) ([]*backend.Item, error) {
	if p.Content {
		b.getContentCalls++
	} else {
		b.getNoContentCalls++
	}

	if b.err != nil {
		return nil, b.err
	}
	return b.B.GetAll(c, t, path, p)
}

func (b *testingBackend) reset() {
	b.getContentCalls = 0
	b.getNoContentCalls = 0
}

func TestConfig(t *testing.T) {
	t.Parallel()

	Convey(`A cache backed by a memory Config`, t, func() {
		c := context.Background()

		// Very simple cache.
		var cache map[string][]byte
		flushCache := func() {
			cache = make(map[string][]byte)
		}
		flushCache()

		mbase := map[string]memory.ConfigSet{
			"services/foo": {
				"file": "body",
			},
			"projects/proj1": {
				"file": "project1 file",
			},
			"projects/goesaway": {
				"file": "goesaway file",
			},
			"projects/goesaway/refs/heads/master": {
				"file": "goesaway master ref",
			},
			"projects/goesaway/refs/heads/other": {
				"file": "goesaway other ref",
			},
		}
		mconfig := memory.New(mbase)

		// Install our backend: memory backed by cache backed by force error.
		//
		// Cache => Testing => In-Memory
		var be backend.B
		be = &client.Backend{
			Provider: &testconfig.Provider{
				Base: mconfig,
			},
		}
		tb := testingBackend{B: be}
		be = &tb

		metaFor := func(configSet, path string) *cfgclient.Meta {
			cfg, err := mconfig.GetConfig(c, configSet, path, false)
			if err != nil {
				panic(err)
			}
			return &cfgclient.Meta{
				ConfigSet:   cfgtypes.ConfigSet(cfg.ConfigSet),
				Path:        cfg.Path,
				ContentHash: cfg.ContentHash,
				Revision:    cfg.Revision,
				ViewURL:     cfg.ViewURL,
			}
		}

		var expired bool
		be = &Backend{
			B: be,
			CacheGet: func(c context.Context, k Key, l Loader) (*Value, error) {
				cacheKey := hex.EncodeToString(k.ParamHash())

				var v *Value
				if d, ok := cache[cacheKey]; ok {
					dv, err := DecodeValue(d)
					if err != nil {
						return nil, err
					}
					if !expired {
						return dv, nil
					}

					v = dv
				}

				v, err := l(c, k, v)
				if err != nil {
					return nil, err
				}
				d, err := v.Encode()
				if err != nil {
					panic(err)
				}
				cache[cacheKey] = d
				return v, nil
			},
		}

		c = backend.WithBackend(c, be)

		// Advance underlying config, expectation.
		advance := func() {
			mbase["services/foo"]["file"] = "body2"
			mbase["services/foo"]["late"] = "late config"
			mbase["projects/showsup"] = memory.ConfigSet{
				"file": "shows up",
			}
			delete(mbase, "projects/goesaway")
			delete(mbase, "projects/goesaway/refs/heads/master")
			delete(mbase, "projects/goesaway/refs/heads/other")
		}

		Convey(`Get`, func() {
			Convey(`Get works, caches, invalidates.`, func() {
				var s string
				So(cfgclient.Get(c, cfgclient.AsService, "services/foo", "file", cfgclient.String(&s), nil), ShouldBeNil)
				So(tb.getNoContentCalls, ShouldEqual, 0)
				So(tb.getContentCalls, ShouldEqual, 1)
				So(s, ShouldEqual, "body")

				// The value should now be cached.
				s = ""
				So(cfgclient.Get(c, cfgclient.AsService, "services/foo", "file", cfgclient.String(&s), nil), ShouldBeNil)
				So(tb.getNoContentCalls, ShouldEqual, 0)
				So(tb.getContentCalls, ShouldEqual, 1) // (Unchanged)
				So(s, ShouldEqual, "body")

				// Expire content. Should do one Get w/out content, see no change, and
				// be done.
				expired = true
				So(cfgclient.Get(c, cfgclient.AsService, "services/foo", "file", cfgclient.String(&s), nil), ShouldBeNil)
				So(tb.getNoContentCalls, ShouldEqual, 1)
				So(tb.getContentCalls, ShouldEqual, 1) // (Unchanged)
				So(s, ShouldEqual, "body")

				// Backing config changes, but not expired.
				advance()
				expired = false

				So(cfgclient.Get(c, cfgclient.AsService, "services/foo", "file", cfgclient.String(&s), nil), ShouldBeNil)
				So(tb.getNoContentCalls, ShouldEqual, 1) // (Unchanged)
				So(tb.getContentCalls, ShouldEqual, 1)   // (Unchanged)
				So(s, ShouldEqual, "body")               // Real one is "body2", but we load from cache.

				// Expire local config, does full reload on hash difference.
				expired = true
				So(cfgclient.Get(c, cfgclient.AsService, "services/foo", "file", cfgclient.String(&s), nil), ShouldBeNil)
				So(tb.getNoContentCalls, ShouldEqual, 2)
				So(tb.getContentCalls, ShouldEqual, 2)
				So(s, ShouldEqual, "body2")
			})

			Convey(`Get w/ missing entry caches the miss.`, func() {
				// Get missing entry.
				var s string
				So(cfgclient.Get(c, cfgclient.AsService, "services/foo", "late", cfgclient.String(&s), nil), ShouldEqual, cfgclient.ErrNoConfig)
				So(tb.getNoContentCalls, ShouldEqual, 0)
				So(tb.getContentCalls, ShouldEqual, 1)

				// Entry is still gone (cached).
				So(cfgclient.Get(c, cfgclient.AsService, "services/foo", "late", cfgclient.String(&s), nil), ShouldEqual, cfgclient.ErrNoConfig)
				So(tb.getNoContentCalls, ShouldEqual, 0) // (Unchanged)
				So(tb.getContentCalls, ShouldEqual, 1)   // (Unchanged)

				// Entry comes into existence, but still cached as gone.
				advance()

				So(cfgclient.Get(c, cfgclient.AsService, "services/foo", "late", cfgclient.String(&s), nil), ShouldEqual, cfgclient.ErrNoConfig)
				So(tb.getNoContentCalls, ShouldEqual, 0) // (Unchanged)
				So(tb.getContentCalls, ShouldEqual, 1)   // (Unchanged)

				// Cache expires, entry content is loaded.
				expired = true
				So(cfgclient.Get(c, cfgclient.AsService, "services/foo", "late", cfgclient.String(&s), nil), ShouldBeNil)
				So(tb.getNoContentCalls, ShouldEqual, 0) // (Unchanged)
				So(tb.getContentCalls, ShouldEqual, 2)
				So(s, ShouldEqual, "late config")

				// Entry disappears, re-caches as missing through no content load.
				expired = true
				delete(mbase["services/foo"], "late")

				So(cfgclient.Get(c, cfgclient.AsService, "services/foo", "late", cfgclient.String(&s), nil), ShouldEqual, cfgclient.ErrNoConfig)
				So(tb.getNoContentCalls, ShouldEqual, 1)
				So(tb.getContentCalls, ShouldEqual, 2) // (Unchanged)
			})
		})

		Convey(`GetAll`, func() {
			Convey(`Successfully loads, caches, refreshes projects.`, func() {
				origMetas := []*cfgclient.Meta{
					metaFor("projects/goesaway", "file"),
					metaFor("projects/proj1", "file"),
				}

				// Load all successfully.
				var s []string
				var meta []*cfgclient.Meta
				So(cfgclient.Projects(c, cfgclient.AsService, "file", cfgclient.StringSlice(&s), &meta), ShouldBeNil)
				So(s, ShouldResemble, []string{"goesaway file", "project1 file"})
				So(meta, ShouldResemble, origMetas)
				So(tb.getNoContentCalls, ShouldEqual, 0)
				So(tb.getContentCalls, ShouldEqual, 1)

				// Expire the cache, reloads, same entries, no content only.
				expired = true
				So(cfgclient.Projects(c, cfgclient.AsService, "file", cfgclient.StringSlice(&s), &meta), ShouldBeNil)
				So(s, ShouldResemble, []string{"goesaway file", "project1 file"})
				So(meta, ShouldResemble, origMetas)
				So(tb.getNoContentCalls, ShouldEqual, 1)
				So(tb.getContentCalls, ShouldEqual, 1) // (Unchanged)

				// Advance, "projects/goesaway" goes away, still loads all successfully
				// (cache).
				expired = false
				advance()

				So(cfgclient.Projects(c, cfgclient.AsService, "file", cfgclient.StringSlice(&s), &meta), ShouldBeNil)
				So(s, ShouldResemble, []string{"goesaway file", "project1 file"})
				So(meta, ShouldResemble, origMetas)
				So(tb.getNoContentCalls, ShouldEqual, 1) // (Unchanged)
				So(tb.getContentCalls, ShouldEqual, 1)   // (Unchanged)

				// Expire the cache, reloads, notices missing entry (count same), reloads.
				expired = true

				So(cfgclient.Projects(c, cfgclient.AsService, "file", cfgclient.StringSlice(&s), &meta), ShouldBeNil)
				So(s, ShouldResemble, []string{"project1 file", "shows up"})
				So(meta, ShouldResemble, []*cfgclient.Meta{
					metaFor("projects/proj1", "file"),
					metaFor("projects/showsup", "file"),
				})
				So(tb.getNoContentCalls, ShouldEqual, 2)
				So(tb.getContentCalls, ShouldEqual, 2)

				// Expire the cache, reloads, notices missing entry (count differs),
				// reloads.
				delete(mbase, "projects/showsup")
				expired = true

				So(cfgclient.Projects(c, cfgclient.AsService, "file", cfgclient.StringSlice(&s), &meta), ShouldBeNil)
				So(s, ShouldResemble, []string{"project1 file"})
				So(meta, ShouldResemble, []*cfgclient.Meta{
					metaFor("projects/proj1", "file"),
				})
				So(tb.getNoContentCalls, ShouldEqual, 3)
				So(tb.getContentCalls, ShouldEqual, 3)
			})

			Convey(`Works with refs too.`, func() {
				origMetas := []*cfgclient.Meta{
					metaFor("projects/goesaway/refs/heads/master", "file"),
					metaFor("projects/goesaway/refs/heads/other", "file"),
				}

				// Load all successfully.
				var s []string
				var meta []*cfgclient.Meta

				So(cfgclient.Refs(c, cfgclient.AsService, "file", cfgclient.StringSlice(&s), &meta), ShouldBeNil)
				So(s, ShouldResemble, []string{"goesaway master ref", "goesaway other ref"})
				So(meta, ShouldResemble, origMetas)
				So(tb.getNoContentCalls, ShouldEqual, 0)
				So(tb.getContentCalls, ShouldEqual, 1) // (Unchanged)

				// Delete project, entries still cached.
				advance()

				So(cfgclient.Refs(c, cfgclient.AsService, "file", cfgclient.StringSlice(&s), &meta), ShouldBeNil)
				So(s, ShouldResemble, []string{"goesaway master ref", "goesaway other ref"})
				So(meta, ShouldResemble, origMetas)
				So(tb.getNoContentCalls, ShouldEqual, 0)
				So(tb.getContentCalls, ShouldEqual, 1) // (Unchanged)

				// Expire the cache, reloads, same entries, no content only.
				expired = true
				So(cfgclient.Refs(c, cfgclient.AsService, "file", cfgclient.StringSlice(&s), &meta), ShouldBeNil)
				So(s, ShouldResemble, []string(nil))
				So(tb.getNoContentCalls, ShouldEqual, 1)
				So(tb.getContentCalls, ShouldEqual, 2) // (Unchanged)
			})

			Convey(`Handles no entries.`, func() {
				var s []string
				So(cfgclient.Refs(c, cfgclient.AsService, "none", cfgclient.StringSlice(&s), nil), ShouldBeNil)
				So(s, ShouldResemble, []string(nil))
				So(tb.getNoContentCalls, ShouldEqual, 0)
				So(tb.getContentCalls, ShouldEqual, 1)
			})
		})
	})
}
