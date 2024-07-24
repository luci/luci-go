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

package projectconfig

import (
	"context"
	"testing"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	memcfg "go.chromium.org/luci/config/impl/memory"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/milo/internal/testutils"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
)

func TestACL(t *testing.T) {
	t.Parallel()

	ftt.Run("Test Environment", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		c = gologger.StdConfig.Use(c)
		c = testutils.SetUpTestGlobalCache(c)

		t.Run("Set up projects", func(t *ftt.Test) {
			c = cfgclient.Use(c, memcfg.New(aclConfgs))
			err := UpdateProjects(c)
			assert.Loosely(t, err, should.BeNil)

			t.Run("Anon wants to...", func(t *ftt.Test) {
				c = auth.WithState(c, &authtest.FakeState{
					Identity:       identity.AnonymousIdentity,
					IdentityGroups: []string{"all"},
				})
				t.Run("Read public project", func(t *ftt.Test) {
					ok, err := IsAllowed(c, "opensource")
					assert.Loosely(t, ok, should.Equal(true))
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("Read private project", func(t *ftt.Test) {
					ok, err := IsAllowed(c, "secret")
					assert.Loosely(t, ok, should.Equal(false))
					assert.Loosely(t, err, should.BeNil)
				})
			})

			t.Run("admin@google.com wants to...", func(t *ftt.Test) {
				c = auth.WithState(c, &authtest.FakeState{
					Identity:       "user:alicebob@google.com",
					IdentityGroups: []string{"administrators", "googlers", "all"},
				})
				t.Run("Read private project", func(t *ftt.Test) {
					ok, err := IsAllowed(c, "secret")
					assert.Loosely(t, ok, should.Equal(true))
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("Read un/misconfigured project", func(t *ftt.Test) {
					ok, err := IsAllowed(c, "misconfigured")
					assert.Loosely(t, ok, should.Equal(false))
					assert.Loosely(t, err, should.BeNil)
				})
			})

			t.Run("alicebob@google.com wants to...", func(t *ftt.Test) {
				c = auth.WithState(c, &authtest.FakeState{
					Identity:       "user:alicebob@google.com",
					IdentityGroups: []string{"googlers", "all"},
				})
				t.Run("Read private project", func(t *ftt.Test) {
					ok, err := IsAllowed(c, "secret")
					assert.Loosely(t, ok, should.Equal(true))
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("Read un/misconfigured project", func(t *ftt.Test) {
					ok, err := IsAllowed(c, "misconfigured")
					assert.Loosely(t, ok, should.Equal(false))
					assert.Loosely(t, err, should.BeNil)
				})
			})

			t.Run("eve@notgoogle.com wants to...", func(t *ftt.Test) {
				c = auth.WithState(c, &authtest.FakeState{
					Identity:       "user:eve@notgoogle.com",
					IdentityGroups: []string{"all"},
				})
				t.Run("Read public project", func(t *ftt.Test) {
					ok, err := IsAllowed(c, "opensource")
					assert.Loosely(t, ok, should.Equal(true))
					assert.Loosely(t, err, should.BeNil)
				})
				t.Run("Read private project", func(t *ftt.Test) {
					ok, err := IsAllowed(c, "secret")
					assert.Loosely(t, ok, should.Equal(false))
					assert.Loosely(t, err, should.BeNil)
				})
			})
		})
	})
}

var secretProjectCfg = `
name: "secret"
access: "group:googlers"
`

var publicProjectCfg = `
name: "opensource"
access: "group:all"
`

var aclConfgs = map[config.Set]memcfg.Files{
	"projects/secret": {
		"project.cfg": secretProjectCfg,
	},
	"projects/opensource": {
		"project.cfg": publicProjectCfg,
	},
	"project/misconfigured": {
		"probject.cfg": secretProjectCfg,
	},
}
