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

package datastore

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/appengine/datastorecache"
	memConfig "go.chromium.org/luci/common/config/impl/memory"
	"go.chromium.org/luci/common/errors"
	configPB "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/luci_config/common/cfgtypes"
	"go.chromium.org/luci/luci_config/server/cfgclient"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend/caching"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend/client"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend/format"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend/testconfig"
	"go.chromium.org/luci/luci_config/server/cfgclient/textproto"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/gae/impl/memory"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

// testCache is a generic Cache testing layer.
type testCache interface {
	dsCacheBackend

	setCacheErr(err error)
	setProjectDNE(project string)
	addConfig(configSet cfgtypes.ConfigSet, path, content string) *backend.Item
	addProjectConfig(name cfgtypes.ProjectName, access string)
	addConfigSets(path string, configSets ...cfgtypes.ConfigSet) []string
}

func projectConfigWithAccess(name cfgtypes.ProjectName, access ...string) *configPB.ProjectCfg {
	return &configPB.ProjectCfg{
		Name:   proto.String(string(name)),
		Access: access,
	}
}

// fakeCache is a pure in-memory testCache implementation. It is very simple,
// storing only raw cache key/value pairs.
type fakeCache struct {
	d          map[string]datastorecache.Value
	serviceURL string
	err        error
}

func mkFakeCache(serviceURL string) *fakeCache {
	return &fakeCache{
		d:          make(map[string]datastorecache.Value),
		serviceURL: serviceURL,
	}
}

func (fc *fakeCache) Get(c context.Context, key []byte) (v datastorecache.Value, err error) {
	if err = fc.err; err != nil {
		return
	}

	var k caching.Key
	caching.Decode(key, &k)

	var ok bool
	if v, ok = fc.d[string(key)]; ok {
		return
	}

	err = errors.New("no cache entry")
	return
}

func (fc *fakeCache) set(key caching.Key, v *caching.Value) {
	encKey, err := caching.Encode(&key)
	if err != nil {
		panic(fmt.Errorf("failed to encode key: %s", err))
	}

	if v == nil {
		delete(fc.d, string(encKey))
		return
	}

	encValue, err := v.Encode()
	if err != nil {
		panic(fmt.Errorf("failed to encode cache value: %s", err))
	}

	fc.d[string(encKey)] = datastorecache.Value{
		Schema:      dsCacheSchema,
		Data:        encValue,
		Description: key.String(),
	}
}

func (fc *fakeCache) setCacheErr(err error) { fc.err = err }

func (fc *fakeCache) setProjectDNE(project string) {
	// Get for this project config will fail.
	fc.set(caching.Key{
		Schema:     caching.Schema,
		ServiceURL: fc.serviceURL,
		Op:         caching.OpGet,
		ConfigSet:  string(cfgtypes.ProjectConfigSet(cfgtypes.ProjectName(project))),
		Path:       cfgclient.ProjectConfigPath,
	}, nil)
}

func (fc *fakeCache) addConfigImpl(cs cfgtypes.ConfigSet, path, formatter, formatData, content string) *backend.Item {
	var (
		item *backend.Item
		cv   caching.Value
	)
	if content != "" {
		item = &backend.Item{
			Meta: backend.Meta{
				ConfigSet:   string(cs),
				Path:        path,
				ContentHash: "hash",
				ViewURL:     "fake_config.cfg",
			},
			Content:    content,
			FormatSpec: backend.FormatSpec{formatter, formatData},
		}
		cv.LoadItems(item)
	}

	fc.set(caching.Key{
		Schema:     caching.Schema,
		ServiceURL: fc.serviceURL,
		Authority:  backend.AsService,
		Op:         caching.OpGet,
		ConfigSet:  string(cs),
		Path:       path,
		Content:    true,
		Formatter:  formatter,
		FormatData: formatData,
	}, &cv)

	return item
}

func (fc *fakeCache) addConfig(cs cfgtypes.ConfigSet, path, content string) *backend.Item {
	return fc.addConfigImpl(cs, path, "", "", content)
}

// addProjectConfig caches a "project.cfg" file for the specified project with
// the specified access string.
func (fc *fakeCache) addProjectConfig(name cfgtypes.ProjectName, access string) {
	// We're loading the resolved version of this cache item.
	pcfg := projectConfigWithAccess(name, access)
	pcfgName := proto.MessageName(pcfg)

	f := textproto.Formatter{}
	formattedData, err := f.FormatItem(proto.MarshalTextString(pcfg), pcfgName)
	if err != nil {
		panic(err)
	}

	fc.addConfigImpl(cfgtypes.ProjectConfigSet(name), cfgclient.ProjectConfigPath,
		textproto.BinaryFormat, pcfgName, formattedData)
}

func (fc *fakeCache) addConfigSets(path string, configSets ...cfgtypes.ConfigSet) []string {
	items := make([]*backend.Item, len(configSets))
	contents := make([]string, len(configSets))
	for i, cs := range configSets {
		contents[i] = string(cs)
		items[i] = &backend.Item{
			Meta: backend.Meta{
				ConfigSet:   string(cs),
				Path:        path,
				ContentHash: "hash",
				ViewURL:     "fake_config.cfg",
			},
			Content: contents[i],
		}
	}

	for _, t := range []backend.GetAllTarget{backend.GetAllProject, backend.GetAllRef} {
		var cv caching.Value
		cv.LoadItems(items...)

		fc.set(caching.Key{
			Schema:       caching.Schema,
			ServiceURL:   fc.serviceURL,
			Authority:    backend.AsService,
			Op:           caching.OpGetAll,
			Content:      true,
			Path:         path,
			GetAllTarget: t,
		}, &cv)
	}
	return contents
}

// fullStackCache is a testCache implementation built on top of an in-memory
// base backend.B with the datastore Cache layer on top of it.
type fullStackCache struct {
	cache *datastorecache.Cache
	err   error

	data    map[string]memConfig.ConfigSet
	backend backend.B
	junkIdx int
}

func (fsc *fullStackCache) Get(c context.Context, key []byte) (datastorecache.Value, error) {
	if err := fsc.err; err != nil {
		return datastorecache.Value{}, err
	}
	return fsc.cache.Get(c, key)
}

func (fsc *fullStackCache) setCacheErr(err error) { fsc.err = err }

func (fsc *fullStackCache) setProjectDNE(project string) {
	key := "projects/" + project
	for k := range fsc.data {
		if k == key || strings.HasPrefix(k, key+"/") {
			delete(fsc.data, k)
		}
	}
}

func (fsc *fullStackCache) addConfig(cs cfgtypes.ConfigSet, path, content string) *backend.Item {
	cset := fsc.data[string(cs)]
	if cset == nil {
		cset = memConfig.ConfigSet{}
		fsc.data[string(cs)] = cset
	}
	if content == "" {
		delete(cset, path)
		return nil
	}
	cset[path] = content

	// Pull the config right back out of the base service.
	item, err := fsc.backend.Get(context.Background(), string(cs), path, backend.Params{
		Authority: backend.AsService,
	})
	if err != nil {
		panic(err)
	}
	return item
}

// addProjectConfig caches a "project.cfg" file for the specified project with
// the specified access string.
func (fsc *fullStackCache) addProjectConfig(name cfgtypes.ProjectName, access string) {
	fsc.addConfig(cfgtypes.ProjectConfigSet(name), cfgclient.ProjectConfigPath,
		proto.MarshalTextString(projectConfigWithAccess(name, access)))
}

func (fsc *fullStackCache) addConfigSets(path string, configSets ...cfgtypes.ConfigSet) []string {
	// Sort the config sets list, then put it back.
	cstr := make([]string, len(configSets))
	for i, cs := range configSets {
		cstr[i] = string(cs)
	}
	sort.Strings(cstr)
	for i, cs := range cstr {
		configSets[i] = cfgtypes.ConfigSet(cs)
	}

	items := make([]*backend.Item, len(configSets))
	for i, cs := range configSets {
		items[i] = fsc.addConfig(cs, path, string(cs))
	}
	return cstr
}

// stripMeta strips cache-specific identifying information from a set of Metas.
func stripMeta(metas []*cfgclient.Meta) []*cfgclient.Meta {
	for _, meta := range metas {
		meta.ContentHash = ""
		meta.Revision = ""
		meta.ViewURL = ""
	}
	return metas
}

func testDatastoreCacheImpl(c context.Context, be backend.B, cache testCache) {
	// Install fake auth state.
	var authState authtest.FakeState
	c = auth.WithState(c, &authState)
	authState.Identity = "user:person@example.com"
	authState.IdentityGroups = []string{"users"}

	dsc := Config{
		RefreshInterval: 1 * time.Hour,
		FailOpen:        false,
		cache:           cache,
	}
	c = backend.WithBackend(c, dsc.Backend(be))

	testErr := errors.New("test error")

	Convey(`Test Get`, func() {
		var v string

		Convey(`Config missing`, func() {
			cache.addConfig("projects/test", "foo", "")

			So(cfgclient.Get(c, cfgclient.AsService, "projects/test", "foo", cfgclient.String(&v), nil),
				ShouldEqual, cfgclient.ErrNoConfig)
		})

		Convey(`Config is present`, func() {
			cache.addConfig("projects/test", "foo", "bar")
			cache.addProjectConfig("test", "group:privileged")

			Convey(`As service`, func() {
				So(cfgclient.Get(c, cfgclient.AsService, "projects/test", "foo", cfgclient.String(&v), nil), ShouldBeNil)
				So(v, ShouldEqual, "bar")
			})

			Convey(`As user, when not a project group member, fails with ErrNoConfig`, func() {
				So(cfgclient.Get(c, cfgclient.AsUser, "projects/test", "foo", cfgclient.String(&v), nil),
					ShouldEqual, cfgclient.ErrNoConfig)
			})

			Convey(`As user, when a project group member, succeeds.`, func() {
				authState.IdentityGroups = append(authState.IdentityGroups, "privileged")
				So(cfgclient.Get(c, cfgclient.AsUser, "projects/test", "foo", cfgclient.String(&v), nil), ShouldBeNil)
				So(v, ShouldEqual, "bar")
			})

			Convey(`As anonymous, fails with ErrNoConfig`, func() {
				So(cfgclient.Get(c, cfgclient.AsAnonymous, "projects/test", "foo", cfgclient.String(&v), nil),
					ShouldEqual, cfgclient.ErrNoConfig)
			})
		})
	})

	Convey(`Test Projects`, func() {
		var v []string
		var meta []*cfgclient.Meta

		Convey(`When cache returns an error`, func() {
			cache.setCacheErr(testErr)

			So(cfgclient.Projects(c, cfgclient.AsService, "test.cfg", cfgclient.StringSlice(&v), nil),
				ShouldUnwrapTo, testErr)
			So(cfgclient.Projects(c, cfgclient.AsUser, "test.cfg", cfgclient.StringSlice(&v), nil),
				ShouldUnwrapTo, testErr)
			So(cfgclient.Projects(c, cfgclient.AsAnonymous, "test.cfg", cfgclient.StringSlice(&v), nil),
				ShouldUnwrapTo, testErr)
		})

		Convey(`With project configs installed`, func() {
			allConfigs := cache.addConfigSets("test.cfg",
				"projects/bar",
				"projects/baz",
				"projects/foo")

			Convey(`As service, retrieves all configs.`, func() {
				So(cfgclient.Projects(c, cfgclient.AsService, "test.cfg", cfgclient.StringSlice(&v), &meta), ShouldBeNil)
				So(v, ShouldResemble, allConfigs)
				So(stripMeta(meta), ShouldResemble, []*cfgclient.Meta{
					{ConfigSet: "projects/bar", Path: "test.cfg"},
					{ConfigSet: "projects/baz", Path: "test.cfg"},
					{ConfigSet: "projects/foo", Path: "test.cfg"},
				})
			})

			Convey(`As user`, func() {
				Convey(`Not a member of any projects, receives empty slice.`, func() {
					cache.addProjectConfig("foo", "group:someone")

					So(cfgclient.Projects(c, cfgclient.AsUser, "test.cfg", cfgclient.StringSlice(&v), &meta), ShouldBeNil)
					So(v, ShouldResemble, []string(nil))
					So(stripMeta(meta), ShouldResemble, []*cfgclient.Meta{})
				})

				Convey(`Member of "foo", gets only "foo".`, func() {
					cache.addProjectConfig("foo", "group:users")

					So(cfgclient.Projects(c, cfgclient.AsUser, "test.cfg", cfgclient.StringSlice(&v), &meta), ShouldBeNil)
					So(v, ShouldResemble, allConfigs[2:3])
					So(stripMeta(meta), ShouldResemble, []*cfgclient.Meta{
						{ConfigSet: "projects/foo", Path: "test.cfg"},
					})
				})

				Convey(`Member of all projects, gets all projects.`, func() {
					cache.addProjectConfig("foo", "group:users")
					cache.addProjectConfig("bar", "group:users")
					cache.addProjectConfig("baz", "group:users")

					So(cfgclient.Projects(c, cfgclient.AsUser, "test.cfg", cfgclient.StringSlice(&v), &meta), ShouldBeNil)
					So(v, ShouldResemble, allConfigs)
					So(stripMeta(meta), ShouldResemble, []*cfgclient.Meta{
						{ConfigSet: "projects/bar", Path: "test.cfg"},
						{ConfigSet: "projects/baz", Path: "test.cfg"},
						{ConfigSet: "projects/foo", Path: "test.cfg"},
					})
				})
			})
		})
	})

	Convey(`Test Refs`, func() {
		var v []string
		var meta []*cfgclient.Meta

		Convey(`When cache returns an error`, func() {
			cache.setCacheErr(testErr)

			So(cfgclient.Refs(c, cfgclient.AsService, "test.cfg", cfgclient.StringSlice(&v), nil),
				ShouldUnwrapTo, testErr)
			So(cfgclient.Refs(c, cfgclient.AsUser, "test.cfg", cfgclient.StringSlice(&v), nil),
				ShouldUnwrapTo, testErr)
			So(cfgclient.Refs(c, cfgclient.AsAnonymous, "test.cfg", cfgclient.StringSlice(&v), nil),
				ShouldUnwrapTo, testErr)
		})

		Convey(`With ref configs installed`, func() {
			allConfigs := cache.addConfigSets("test.cfg",
				"projects/bar/refs/branches/mybranch",
				"projects/bar/refs/heads/master",
				"projects/foo/refs/branches/mybranch",
				"projects/foo/refs/heads/master")

			Convey(`As service, retrieves all configs.`, func() {
				So(cfgclient.Refs(c, cfgclient.AsService, "test.cfg", cfgclient.StringSlice(&v), &meta), ShouldBeNil)
				So(v, ShouldResemble, allConfigs)
				So(stripMeta(meta), ShouldResemble, []*cfgclient.Meta{
					{ConfigSet: "projects/bar/refs/branches/mybranch", Path: "test.cfg"},
					{ConfigSet: "projects/bar/refs/heads/master", Path: "test.cfg"},
					{ConfigSet: "projects/foo/refs/branches/mybranch", Path: "test.cfg"},
					{ConfigSet: "projects/foo/refs/heads/master", Path: "test.cfg"},
				})
			})

			Convey(`As user`, func() {
				Convey(`Not a member of any projects, receives empty slice.`, func() {
					cache.addProjectConfig("foo", "group:someone")

					So(cfgclient.Refs(c, cfgclient.AsUser, "test.cfg", cfgclient.StringSlice(&v), &meta), ShouldBeNil)
					So(v, ShouldResemble, []string(nil))
					So(stripMeta(meta), ShouldResemble, []*cfgclient.Meta{})
				})

				Convey(`Member of "foo", gets only "foo".`, func() {
					cache.addProjectConfig("foo", "group:users")

					So(cfgclient.Refs(c, cfgclient.AsUser, "test.cfg", cfgclient.StringSlice(&v), &meta), ShouldBeNil)
					So(v, ShouldResemble, allConfigs[2:4])
					So(stripMeta(meta), ShouldResemble, []*cfgclient.Meta{
						{ConfigSet: "projects/foo/refs/branches/mybranch", Path: "test.cfg"},
						{ConfigSet: "projects/foo/refs/heads/master", Path: "test.cfg"},
					})
				})

				Convey(`Member of all projects, gets all projects.`, func() {
					cache.addProjectConfig("foo", "group:users")
					cache.addProjectConfig("bar", "group:users")

					So(cfgclient.Refs(c, cfgclient.AsUser, "test.cfg", cfgclient.StringSlice(&v), &meta), ShouldBeNil)
					So(v, ShouldResemble, allConfigs)
					So(stripMeta(meta), ShouldResemble, []*cfgclient.Meta{
						{ConfigSet: "projects/bar/refs/branches/mybranch", Path: "test.cfg"},
						{ConfigSet: "projects/bar/refs/heads/master", Path: "test.cfg"},
						{ConfigSet: "projects/foo/refs/branches/mybranch", Path: "test.cfg"},
						{ConfigSet: "projects/foo/refs/heads/master", Path: "test.cfg"},
					})
				})
			})
		})
	})
}

func TestDatastoreCache(t *testing.T) {
	t.Parallel()

	Convey(`Testing with in-memory stub cache`, t, func() {
		c := context.Background()

		var be backend.B = &client.Backend{
			Provider: &testconfig.Provider{
				Base: memConfig.New(nil),
			},
		}
		mcURL := be.ServiceURL(c)
		fc := mkFakeCache(mcURL.String())

		be = &format.Backend{B: be}

		Convey(`Standard datastore tests`, func() {
			testDatastoreCacheImpl(c, be, fc)
		})

		Convey(`A testing setup built around the fake cache`, func() {
			dsc := Config{
				RefreshInterval: 1 * time.Hour,
				FailOpen:        false,
				cache:           fc,
			}
			c = backend.WithBackend(c, dsc.Backend(be))

			Convey(`Errors with different schema.`, func() {
				fc.addConfig("foo", "bar", "value")
				for k, v := range fc.d {
					v.Schema = "unknown"
					fc.d[k] = v
				}

				var v string
				So(cfgclient.Get(c, cfgclient.AsService, "foo", "bar", cfgclient.String(&v), nil),
					ShouldErrLike, `response schema ("unknown") doesn't match current`)
			})
		})
	})
}

func TestDatastoreCacheFullStack(t *testing.T) {
	t.Parallel()

	Convey(`Testing full-stack datastore cache`, t, func() {
		c := memory.Use(context.Background())

		data := map[string]memConfig.ConfigSet{}

		var be backend.B
		be = &client.Backend{
			Provider: &testconfig.Provider{
				Base: memConfig.New(data),
			},
		}

		be = &format.Backend{B: be}

		fsc := fullStackCache{
			cache:   &Cache,
			data:    data,
			backend: be,
		}
		testDatastoreCacheImpl(c, be, &fsc)
	})
}
