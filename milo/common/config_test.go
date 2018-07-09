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

package common

import (
	"testing"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/config"
	memcfg "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"
	"go.chromium.org/luci/config/validation"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	Convey("Test Environment", t, func() {
		c := gaetesting.TestingContextWithAppID("dev~luci-milo")
		datastore.GetTestable(c).Consistent(true)

		Convey("Validation tests", func() {
			ctx := &validation.Context{
				Context: c,
			}
			configSet := "projects/foobar"
			path := "luci-milo.cfg"
			Convey("Load a bad config", func() {
				content := []byte(badCfg)
				validateProjectCfg(ctx, configSet, path, content)
				So(ctx.Finalize().Error(), ShouldResemble, "in <unspecified file>: line 4: unknown field name \"\" in config.Header")
			})
			Convey("Load another bad config", func() {
				content := []byte(badCfg2)
				validateProjectCfg(ctx, configSet, path, content)
				err := ctx.Finalize()
				ve, ok := err.(*validation.Error)
				So(ok, ShouldEqual, true)
				So(len(ve.Errors), ShouldEqual, 14)
				So(ve.Errors[0].Error(), ShouldContainSubstring, "duplicate header id")
				So(ve.Errors[1].Error(), ShouldContainSubstring, "missing id")
				So(ve.Errors[2].Error(), ShouldContainSubstring, "missing manifest name")
				So(ve.Errors[3].Error(), ShouldContainSubstring, "missing repo url")
				So(ve.Errors[4].Error(), ShouldContainSubstring, "missing ref")
				So(ve.Errors[5].Error(), ShouldContainSubstring, "header non-existant not defined")
			})
			Convey("Load a good config", func() {
				content := []byte(fooCfg)
				validateProjectCfg(ctx, configSet, path, content)
				So(ctx.Finalize(), ShouldBeNil)
			})
		})

		Convey("Tests about global configs", func() {
			Convey("Read a config before anything is set", func() {
				c = testconfig.WithCommonClient(c, memcfg.New(mockedConfigs))
				_, err := UpdateServiceConfig(c)
				So(err.Error(), ShouldResemble, "could not load settings.cfg from luci-config: no such config")
				settings := GetSettings(c)
				So(settings.Buildbot.InternalReader, ShouldEqual, "")
			})
			Convey("Read a config", func() {
				mockedConfigs["services/luci-milo"] = memcfg.Files{
					"settings.cfg": settingsCfg,
				}
				c = testconfig.WithCommonClient(c, memcfg.New(mockedConfigs))
				rSettings, err := UpdateServiceConfig(c)
				So(err, ShouldBeNil)
				settings := GetSettings(c)
				So(rSettings, ShouldResembleProto, settings)
				So(settings.Buildbot.InternalReader, ShouldEqual, "googlers")
			})
		})

		Convey("Send update", func() {
			c = testconfig.WithCommonClient(c, memcfg.New(mockedConfigs))
			_, err := UpdateServiceConfig(c)
			So(err, ShouldBeNil)
			// Send update here
			So(UpdateConsoles(c), ShouldBeNil)

			Convey("Check Console config updated", func() {
				cs, err := GetConsole(c, "foo", "default")
				So(err, ShouldBeNil)
				So(cs.ID, ShouldEqual, "default")
				So(cs.Ordinal, ShouldEqual, 0)
				So(cs.Def.Header, ShouldBeNil)
			})

			Convey("Check Console config updated with header", func() {
				cs, err := GetConsole(c, "foo", "default_header")
				So(err, ShouldBeNil)
				So(cs.ID, ShouldEqual, "default_header")
				So(cs.Ordinal, ShouldEqual, 1)
				So(cs.Def.Header.Id, ShouldEqual, "main_header")
				So(cs.Def.Header.TreeStatusHost, ShouldEqual, "blarg.example.com")
			})

			Convey("Check second update reorders", func() {
				mockedConfigsUpdate["services/luci-milo"] = memcfg.Files{
					"settings.cfg": settingsCfg,
				}
				c = testconfig.WithCommonClient(c, memcfg.New(mockedConfigsUpdate))
				_, err = UpdateServiceConfig(c)
				So(err, ShouldBeNil)
				// Send update here
				So(UpdateConsoles(c), ShouldBeNil)

				Convey("Check Console config removed", func() {
					cs, err := GetConsole(c, "foo", "default")
					So(err, ShouldNotBeNil)
					So(cs, ShouldEqual, nil)
				})

				Convey("Check builder group configs in correct order", func() {
					cs, err := GetConsole(c, "foo", "default_header")
					So(err, ShouldBeNil)
					So(cs.ID, ShouldEqual, "default_header")
					So(cs.Ordinal, ShouldEqual, 0)
					So(cs.Def.Header.Id, ShouldEqual, "main_header")
					So(cs.Def.Header.TreeStatusHost, ShouldEqual, "blarg.example.com")
					cs, err = GetConsole(c, "foo", "console.bar")
					So(err, ShouldBeNil)
					So(cs.ID, ShouldEqual, "console.bar")
					So(cs.Ordinal, ShouldEqual, 1)
					So(cs.Builders, ShouldResemble, []string{"buildbucket/luci.foo.something/bar"})

					cs, err = GetConsole(c, "foo", "console.baz")
					So(err, ShouldBeNil)
					So(cs.ID, ShouldEqual, "console.baz")
					So(cs.Ordinal, ShouldEqual, 2)
					So(cs.Builders, ShouldResemble, []string{"buildbucket/luci.foo.other/baz"})
				})

				Convey("Check getting project builder groups in correct order", func() {
					cs, err := GetProjectConsoles(c, "foo")
					So(err, ShouldBeNil)

					ids := make([]string, 0, len(cs))
					for _, c := range cs {
						ids = append(ids, c.ID)
					}
					So(ids, ShouldResemble, []string{"default_header", "console.bar", "console.baz"})
				})
			})
		})
	})
}

var fooCfg = `
headers: {
	id: "main_header"
	tree_status_host: "blarg.example.com"
}
consoles: {
	id: "default"
	repo_url: "https://chromium.googlesource.com/foo/bar"
	refs: "refs/heads/master"
	manifest_name: "REVISION"
	builders: {
		name: "buildbucket/luci.foo.something/bar"
		category: "main|something"
		short_name: "s"
	}
	builders: {
		name: "buildbucket/luci.foo.other/baz"
		category: "main|other"
		short_name: "o"
	}
}
consoles: {
	id: "default_header"
	repo_url: "https://chromium.googlesource.com/foo/bar"
	refs: "refs/heads/master"
	manifest_name: "REVISION"
	builders: {
		name: "buildbucket/luci.foo.something/bar"
		category: "main|something"
		short_name: "s"
	}
	builders: {
		name: "buildbucket/luci.foo.other/baz"
		category: "main|other"
		short_name: "o"
	}
	header_id: "main_header"
}
`

var badCfg = `
headers: {
	id: "main_header",
	tree_status_host: "blarg.example.com"
`

var badCfg2 = `
headers: {
	id: "main_header",
	tree_status_host: "blarg.example.com"
}
headers: {
	id: "main_header",
	tree_status_host: "blarg.example.com"
}
consoles {
	header_id: "non-existant"
}
consoles {
	id: "foo"
}
consoles {
	id: "foo"
}
logo_url: "badurl"
`

var fooCfg2 = `
headers: {
	id: "main_header"
	tree_status_host: "blarg.example.com"
}
consoles: {
	id: "default_header"
	repo_url: "https://chromium.googlesource.com/foo/bar"
	refs: "refs/heads/master"
	builders: {
		name: "buildbucket/luci.foo.something/bar"
		category: "main|something"
		short_name: "s"
	}
	builders: {
		name: "buildbucket/luci.foo.other/baz"
		category: "main|other"
		short_name: "o"
	}
	header_id: "main_header"
}
consoles: {
	id: "console.bar"
	repo_url: "https://chromium.googlesource.com/foo/bar"
	refs: "refs/heads/master"
	builders: {
		name: "buildbucket/luci.foo.something/bar"
		category: "main|something"
		short_name: "s"
	}
}
consoles: {
	id: "console.baz"
	repo_url: "https://chromium.googlesource.com/foo/bar"
	refs: "refs/heads/master"
	builders: {
		name: "buildbucket/luci.foo.other/baz"
		category: "main|other"
		short_name: "o"
	}
}
`

var settingsCfg = `
buildbot: {
	internal_reader: "googlers"
}
`

var mockedConfigs = map[config.Set]memcfg.Files{
	"projects/foo": {
		"luci-milo.cfg": fooCfg,
	},
}

var mockedConfigsUpdate = map[config.Set]memcfg.Files{
	"projects/foo": {
		"luci-milo.cfg": fooCfg2,
	},
}
