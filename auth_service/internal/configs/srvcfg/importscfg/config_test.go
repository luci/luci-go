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

var importsCfg = &configspb.GroupImporterConfig{
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

func TestConfigContext(t *testing.T) {
	t.Parallel()

	ftt.Run("Getting without setting setting returns default", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		cfg, _, err := Get(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cfg, should.Match(&configspb.GroupImporterConfig{}))
	})

	ftt.Run("Testing config operations", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		metadata := &config.Meta{
			Path:     "permissions.cfg",
			Revision: "123abc",
		}
		assert.Loosely(t, SetInTest(ctx, importsCfg, metadata), should.BeNil)
		cfgFromGet, metadataFromGet, err := Get(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, cfgFromGet, should.Match(importsCfg))
		assert.Loosely(t, metadataFromGet, should.Match(metadata))
	})
}

func TestIsAuthorizedUploader(t *testing.T) {
	t.Parallel()

	ftt.Run("empty config", t, func(t *ftt.Test) {
		authorized := IsAuthorizedUploader(
			&configspb.GroupImporterConfig{}, "test-service-account@example.com", "example-tarball.tz")
		assert.Loosely(t, authorized, should.BeFalse)
	})

	ftt.Run("empty tarball name", t, func(t *ftt.Test) {
		authorized := IsAuthorizedUploader(
			importsCfg, "test-service-account@example.com", "")
		assert.Loosely(t, authorized, should.BeFalse)
	})

	ftt.Run("unknown tarball", t, func(t *ftt.Test) {
		authorized := IsAuthorizedUploader(
			importsCfg, "test-service-account@example.com", "test-tarball.tz")
		assert.Loosely(t, authorized, should.BeFalse)
	})

	ftt.Run("unauthorized uploader", t, func(t *ftt.Test) {
		authorized := IsAuthorizedUploader(
			importsCfg, "somebody@example.com", "example-tarball.tz")
		assert.Loosely(t, authorized, should.BeFalse)
	})

	ftt.Run("authorized uploader", t, func(t *ftt.Test) {
		authorized := IsAuthorizedUploader(
			importsCfg, "test-service-account@example.com", "example-tarball.tz")
		assert.Loosely(t, authorized, should.BeTrue)
	})
}
