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

package config

import (
	"testing"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	memcfg "go.chromium.org/luci/config/impl/memory"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	Convey("Test Environment", t, func() {
		c := gaetesting.TestingContext()
		datastore.GetTestable(c).Consistent(true)

		Convey("Tests about global configs", func() {
			Convey("Read a config before anything is set", func() {
				c = cfgclient.Use(c, memcfg.New(mockedConfigs))
				_, err := UpdateServiceConfig(c)
				So(err.Error(), ShouldResemble, "could not load settings.cfg from luci-config: no such config")
				settings := GetSettings(c)
				So(settings.Buildbucket, ShouldBeNil)
			})

			Convey("Read a config", func() {
				mockedConfigs["services/${appid}"] = memcfg.Files{
					"settings.cfg": settingsCfg,
				}
				c = cfgclient.Use(c, memcfg.New(mockedConfigs))
				rSettings, err := UpdateServiceConfig(c)
				So(err, ShouldBeNil)
				settings := GetSettings(c)
				So(rSettings, ShouldResembleProto, settings)
				So(settings.Buildbucket.Name, ShouldEqual, "dev")
				So(settings.Buildbucket.Host, ShouldEqual, "cr-buildbucket-dev.appspot.com")
			})
		})
	})
}

var settingsCfg = `
buildbucket: {
	name: "dev"
	host: "cr-buildbucket-dev.appspot.com"
}
`

var mockedConfigs = map[config.Set]memcfg.Files{}
