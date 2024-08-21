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
		ctx := memory.Use(context.Background())
		_, err := Get(ctx)
		assert.Loosely(t, err, should.NotBeNil)

		_, _, err = GetWithMetadata(ctx)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("Testing basic config operations", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		assert.Loosely(t, SetConfig(ctx, importsCfg), should.BeNil)
		cfgFromGet, err := Get(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cfgFromGet, should.Match(importsCfg))
	})

	ftt.Run("Testing config operations with metadata", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		metadata := &config.Meta{
			Path:     "permissions.cfg",
			Revision: "123abc",
		}
		assert.Loosely(t, SetConfigWithMetadata(ctx, importsCfg, metadata), should.BeNil)
		cfgFromGet, metadataFromGet, err := GetWithMetadata(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cfgFromGet, should.Match(importsCfg))
		assert.Loosely(t, metadataFromGet, should.Match(metadata))
	})

	ftt.Run("IsAuthorizedUploader works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		t.Run("fails without setting", func(t *ftt.Test) {
			authorized, err := IsAuthorizedUploader(
				ctx, "test-service-account@example.com", "example-tarball.tz")
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, authorized, should.BeFalse)
		})

		t.Run("with config set", func(t *ftt.Test) {
			assert.Loosely(t, SetConfig(ctx, importsCfg), should.BeNil)

			t.Run("empty tarball name", func(t *ftt.Test) {
				authorized, err := IsAuthorizedUploader(
					ctx, "test-service-account@example.com", "")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, authorized, should.BeFalse)
			})

			t.Run("unknown tarball", func(t *ftt.Test) {
				authorized, err := IsAuthorizedUploader(
					ctx, "test-service-account@example.com", "test-tarball.tz")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, authorized, should.BeFalse)
			})

			t.Run("unauthorized uploader", func(t *ftt.Test) {
				authorized, err := IsAuthorizedUploader(
					ctx, "somebody@example.com", "example-tarball.tz")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, authorized, should.BeFalse)
			})

			t.Run("authorized uploader", func(t *ftt.Test) {
				authorized, err := IsAuthorizedUploader(
					ctx, "test-service-account@example.com", "example-tarball.tz")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, authorized, should.BeTrue)
			})
		})
	})
}
