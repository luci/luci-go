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

package importscfg

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
	importsCfg := &configspb.GroupImporterConfig{
		Tarball: []*configspb.GroupImporterConfig_TarballEntry{
			{
				Url: "some-url",
				OauthScopes: []string{
					"example-oauth",
				},
				Domain: "example.com",
				Systems: []string{
					"groups",
				},
				Groups: []string{
					"committers",
					"testers",
				},
			},
		},
		Plainlist: []*configspb.GroupImporterConfig_PlainlistEntry{
			{
				Url: "plainlistexampleurl",
				OauthScopes: []string{
					"example-oauth-scope",
				},
				Domain: "example.com",
				Group:  "plain-group/example",
			},
		},
		TarballUpload: []*configspb.GroupImporterConfig_TarballUploadEntry{
			{
				Name: "example-tarball.tz",
				AuthorizedUploader: []string{
					"test-service-account@example.com",
				},
				Domain: "example.com",
				Systems: []string{
					"groups",
				},
				Groups: []string{
					"committers",
				},
			},
		},
	}

	ftt.Run("Getting without setting fails", t, func(t *ftt.Test) {
		_, err := Get(ctx)
		assert.Loosely(t, err, should.NotBeNil)

		_, _, err = GetWithMetadata(ctx)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("Testing basic config operations", t, func(t *ftt.Test) {
		assert.Loosely(t, SetConfig(ctx, importsCfg), should.BeNil)
		cfgFromGet, err := Get(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cfgFromGet, should.Resemble(importsCfg))
	})

	ftt.Run("Testing config operations with metadata", t, func(t *ftt.Test) {
		metadata := &config.Meta{
			Path:     "permissions.cfg",
			Revision: "123abc",
		}
		assert.Loosely(t, SetConfigWithMetadata(ctx, importsCfg, metadata), should.BeNil)
		cfgFromGet, metadataFromGet, err := GetWithMetadata(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cfgFromGet, should.Resemble(importsCfg))
		assert.Loosely(t, metadataFromGet, should.Resemble(metadata))
	})
}
