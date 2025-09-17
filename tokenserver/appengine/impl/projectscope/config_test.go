// Copyright 2019 The LUCI Authors.
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

package projectscope

import (
	"context"
	"testing"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	configset "go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/impl/memory"

	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectidentity"
)

const fakeConfig = `
	projects {
		id: "id1"
		gitiles_location {
			repo: "https://some/repo"
			ref: "refs/heads/main"
		}
		identity_config {
			service_account_email: "foo@bar.com"
		}
	}
	projects {
		id: "id2"
		gitiles_location {
			repo: "https://some/other/repo"
			ref: "refs/heads/main"
		}
		identity_config {
			service_account_email: "bar@bar.com"
			staging_service_account_email: "staging-bar@bar.com"
		}
	}
`

func TestRules(t *testing.T) {
	t.Parallel()

	assertExpected := func(ctx context.Context, t *testing.T, expected map[string]string) {
		storage := projectidentity.ProjectIdentities(ctx)
		got := make(map[string]string, len(expected))
		for project := range expected {
			identity, err := storage.LookupByProject(ctx, project)
			assert.NoErr(t, err)
			got[project] = identity.Email
		}
		assert.That(t, got, should.Match(expected))
	}

	t.Run("ImportConfig prod", func(t *testing.T) {
		ctx := prepareCfg(gaetesting.TestingContext(), fakeConfig)
		_, err := ImportConfigs(ctx, false)
		assert.NoErr(t, err)
		assertExpected(ctx, t, map[string]string{
			"id1": "foo@bar.com",
			"id2": "bar@bar.com",
		})
	})

	t.Run("ImportConfig staging", func(t *testing.T) {
		ctx := prepareCfg(gaetesting.TestingContext(), fakeConfig)
		_, err := ImportConfigs(ctx, true)
		assert.NoErr(t, err)
		assertExpected(ctx, t, map[string]string{
			"id1": "foo@bar.com",
			"id2": "staging-bar@bar.com",
		})
	})
}

// prepareCfg injects config.Backend implementation with a bunch of
// config files.
func prepareCfg(c context.Context, configFile string) context.Context {
	return cfgclient.Use(c, memory.New(map[configset.Set]memory.Files{
		"services/${config_service_appid}": {
			"projects.cfg": configFile,
		},
	}))
}
