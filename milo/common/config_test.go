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

	"github.com/luci/gae/impl/memory"
	memcfg "github.com/luci/luci-go/common/config/impl/memory"
	"github.com/luci/luci-go/common/logging/gologger"
	"github.com/luci/luci-go/luci_config/server/cfgclient/backend/testconfig"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	Convey("Test Environment", t, func() {
		c := memory.UseWithAppID(context.Background(), "dev~luci-milo")
		c = gologger.StdConfig.Use(c)

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
			err = UpdateProjectConfigs(c)
			So(err, ShouldBeNil)

			Convey("Check Project config updated", func() {
				p, err := GetProject(c, "foo")
				So(err, ShouldBeNil)
				So(p.ID, ShouldEqual, "foo")
			})

			Convey("Check Console config updated", func() {
				cs, err := GetConsole(c, "foo", "default")
				So(err, ShouldBeNil)
				So(cs.Name, ShouldEqual, "default")
				So(cs.RepoURL, ShouldEqual, "https://chromium.googlesource.com/foo/bar")
			})
		})
	})
}

var fooCfg = `
ID: "foo"
Consoles: {
	Name: "default"
	RepoURL: "https://chromium.googlesource.com/foo/bar"
	Branch: "master"
	Builders: {
		Name: "buildbucket/luci.foo.something/bar"
		Category: "main|something"
		ShortName: "s"
	}
	Builders: {
		Name: "buildbucket/luci.foo.other/baz"
		Category: "main|other"
		ShortName: "o"
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
