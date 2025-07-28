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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	configset "go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

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
		}
	}
`

func TestRules(t *testing.T) {
	t.Parallel()

	ctx := auth.WithState(gaetesting.TestingContext(), &authtest.FakeState{
		Identity: "user:unused@example.com",
	})
	storage := projectidentity.ProjectIdentities(ctx)

	ftt.Run("Loads", t, func(t *ftt.Test) {
		ctx = prepareCfg(ctx, fakeConfig)
		_, err := ImportConfigs(ctx)
		assert.Loosely(t, err, should.BeNil)

		expected := map[string]string{
			"id1": "foo@bar.com",
			"id2": "bar@bar.com",
		}

		for project, email := range expected {
			identity, err := storage.LookupByProject(ctx, project)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, identity, should.Resemble(&projectidentity.ProjectIdentity{
				Project: project,
				Email:   email,
			}))
		}
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
