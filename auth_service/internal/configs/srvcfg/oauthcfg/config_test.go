// Copyright 2022 The LUCI Authors.
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

package oauthcfg

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/gae/impl/memory"

	"go.chromium.org/luci/auth_service/api/configspb"
)

func TestConfigContext(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	oauthCfg := &configspb.OAuthConfig{
		PrimaryClientId:     "424242.test.example.com",
		PrimaryClientSecret: "SuPeRsecRetSTring",
		ClientIds: []string{
			"123456789.test.example.com",
			"987654321.test.example.com",
		},
		TokenServerUrl: "https://test-token-server.example.com",
	}

	ftt.Run("Getting without setting returns default", t, func(t *ftt.Test) {
		cfg, _, err := Get(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cfg, should.Match(&configspb.OAuthConfig{}))
	})

	ftt.Run("Testing config operations", t, func(t *ftt.Test) {
		metadata := &config.Meta{
			Path:     "oauth.cfg",
			Revision: "123abc",
			ViewURL:  "https://example.com/config/revision/123abc",
		}
		assert.Loosely(t, SetInTest(ctx, oauthCfg, metadata), should.BeNil)
		gotCfg, metadataFromGet, err := Get(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, gotCfg, should.Match(oauthCfg))
		assert.Loosely(t, metadataFromGet, should.Match(metadata))
	})
}
