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

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	memcfg "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func TestConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("Test Environment", t, func(t *ftt.Test) {
		c := gaetesting.TestingContext()
		datastore.GetTestable(c).Consistent(true)

		t.Run("Tests about global configs", func(t *ftt.Test) {
			t.Run("Read a config before anything is set", func(t *ftt.Test) {
				c = cfgclient.Use(c, memcfg.New(mockedConfigs))
				_, err := UpdateServiceConfig(c)
				assert.Loosely(t, err.Error(), should.Match("could not load settings.cfg from luci-config: no such config"))
				settings := GetSettings(c)
				assert.Loosely(t, settings.Buildbucket, should.BeNil)
			})

			t.Run("Read a config", func(t *ftt.Test) {
				mockedConfigs["services/${appid}"] = memcfg.Files{
					"settings.cfg": settingsCfg,
				}
				c = cfgclient.Use(c, memcfg.New(mockedConfigs))
				rSettings, err := UpdateServiceConfig(c)
				assert.Loosely(t, err, should.BeNil)
				settings := GetSettings(c)
				assert.Loosely(t, rSettings, should.Match(settings))
				assert.Loosely(t, settings.Buildbucket.Name, should.Equal("dev"))
				assert.Loosely(t, settings.Buildbucket.Host, should.Equal("cr-buildbucket-dev.appspot.com"))
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
