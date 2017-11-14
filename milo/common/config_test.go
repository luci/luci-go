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

	"go.chromium.org/luci/appengine/gaetesting"
	memcfg "go.chromium.org/luci/common/config/impl/memory"
	"go.chromium.org/luci/luci_config/server/cfgclient/backend/testconfig"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	Convey("Test Environment", t, func() {
		c := gaetesting.TestingContextWithAppID("dev~luci-milo")

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
				So(cs.Def.RepoUrl, ShouldEqual, "https://chromium.googlesource.com/foo/bar")
				So(cs.Def.Header, ShouldBeNil)
			})

			Convey("Check Console config updated with header", func() {
				cs, err := GetConsole(c, "foo", "default_header")
				So(err, ShouldBeNil)
				So(cs.ID, ShouldEqual, "default_header")
				So(cs.Def.RepoUrl, ShouldEqual, "https://chromium.googlesource.com/foo/bar")
				So(cs.Def.Header.Id, ShouldEqual, "main_header")
				So(cs.Def.Header.TreeStatusHost, ShouldEqual, "blarg.example.com")
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
