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
	"errors"
	"testing"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/appengine/gaetesting"
	memcfg "go.chromium.org/luci/common/config/impl/memory"
	"go.chromium.org/luci/config/server/cfgclient/backend/testconfig"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	Convey("Test Environment", t, func() {
		c := gaetesting.TestingContextWithAppID("dev~luci-milo")
		datastore.GetTestable(c).Consistent(true)

		Convey("Tests about global configs", func() {
			Convey("Read a config before anything is set", func() {
				c = testconfig.WithCommonClient(c, memcfg.New(mockedConfigs))
				_, err := UpdateServiceConfig(c)
				So(err, ShouldResemble, errors.New("could not load settings.cfg from luci-config: no such config"))
				settings := GetSettings(c)
				So(settings.Buildbot.InternalReader, ShouldEqual, "")
			})
			Convey("Read a config", func() {
				mockedConfigs["services/luci-milo"] = memcfg.ConfigSet{
					"settings.cfg": settingsCfg,
				}
				c = testconfig.WithCommonClient(c, memcfg.New(mockedConfigs))
				rSettings, err := UpdateServiceConfig(c)
				So(err, ShouldBeNil)
				settings := GetSettings(c)
				So(rSettings, ShouldResemble, settings)
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
				mockedConfigsUpdate["services/luci-milo"] = memcfg.ConfigSet{
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
	ref: "master"
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
	ref: "master"
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

var fooCfg2 = `
headers: {
	id: "main_header"
	tree_status_host: "blarg.example.com"
}
consoles: {
	id: "default_header"
	repo_url: "https://chromium.googlesource.com/foo/bar"
	ref: "master"
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
	ref: "master"
	builders: {
		name: "buildbucket/luci.foo.something/bar"
		category: "main|something"
		short_name: "s"
	}
}
consoles: {
	id: "console.baz"
	repo_url: "https://chromium.googlesource.com/foo/bar"
	ref: "master"
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

var mockedConfigs = map[string]memcfg.ConfigSet{
	"projects/foo": {
		"luci-milo.cfg": fooCfg,
	},
}

var mockedConfigsUpdate = map[string]memcfg.ConfigSet{
	"projects/foo": {
		"luci-milo.cfg": fooCfg2,
	},
}
