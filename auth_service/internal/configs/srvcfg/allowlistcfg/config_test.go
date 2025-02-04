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

package allowlistcfg

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
	allowlistCfg := &configspb.IPAllowlistConfig{
		IpAllowlists: []*configspb.IPAllowlistConfig_IPAllowlist{
			{
				Name: "test-allowlist-1",
				Subnets: []string{
					"127.0.0.1/32",
					"0.0.0.0/0",
				},
			},
			{
				Name: "test-allowlist-2",
				Includes: []string{
					"test-allowlist-1",
				},
			},
		},
	}

	ftt.Run("Getting without setting fails", t, func(t *ftt.Test) {
		_, err := Get(ctx)
		assert.Loosely(t, err, should.NotBeNil)

		_, err = GetMetadata(ctx)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("Testing basic Config operations", t, func(t *ftt.Test) {
		assert.Loosely(t, SetConfig(ctx, allowlistCfg), should.BeNil)
		cfgFromGet, err := Get(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cfgFromGet, should.Match(allowlistCfg))
	})

	ftt.Run("Testing config operations with metadata", t, func(t *ftt.Test) {
		metadata := &config.Meta{
			Path:     "ip_allowlist.cfg",
			Revision: "123abc",
			ViewURL:  "https://example.com/config/revision/123abc",
		}
		assert.Loosely(t, SetConfigWithMetadata(ctx, allowlistCfg, metadata), should.BeNil)
		metadataFromGet, err := GetMetadata(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, metadataFromGet, should.Match(metadata))
	})
}
