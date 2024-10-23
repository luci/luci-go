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

package config

import (
	"context"
	"os"
	"testing"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"

	configpb "go.chromium.org/luci/resultdb/proto/config"
)

func TestProjectConfigValidator(t *testing.T) {
	t.Parallel()

	validate := func(cfg *configpb.ProjectConfig) error {
		c := validation.Context{Context: context.Background()}
		validateProjectConfig(&c, cfg)
		return c.Finalize()
	}

	ftt.Run("config template is valid", t, func(t *ftt.Test) {
		content, err := os.ReadFile(
			"../../configs/projects/chromeos/luci-resultdb-dev-template.cfg",
		)
		assert.Loosely(t, err, should.BeNil)
		cfg := &configpb.ProjectConfig{}
		assert.Loosely(t, prototext.Unmarshal(content, cfg), should.BeNil)
		assert.Loosely(t, validate(cfg), should.BeNil)
	})

	ftt.Run("valid config is valid", t, func(t *ftt.Test) {
		cfg := CreatePlaceholderProjectConfig()
		assert.Loosely(t, validate(cfg), should.BeNil)
	})

	ftt.Run("GCS allow list", t, func(t *ftt.Test) {
		cfg := CreatePlaceholderProjectConfig()
		assert.Loosely(t, cfg.GcsAllowList, should.NotBeNil)
		assert.Loosely(t, len(cfg.GcsAllowList), should.Equal(1))
		assert.Loosely(t, len(cfg.GcsAllowList[0].Buckets), should.Equal(1))
		gcsAllowList := cfg.GcsAllowList[0]

		t.Run("users", func(t *ftt.Test) {
			t.Run("must be specified", func(t *ftt.Test) {
				gcsAllowList.Users = []string{}
				assert.Loosely(t, validate(cfg), should.NotBeNil)
			})
			t.Run("must be non-empty", func(t *ftt.Test) {
				gcsAllowList.Users = []string{""}
				assert.Loosely(t, validate(cfg), should.NotBeNil)
			})
			t.Run("invalid", func(t *ftt.Test) {
				gcsAllowList.Users = []string{"a:b"}
				assert.Loosely(t, validate(cfg), should.NotBeNil)
			})
			t.Run("valid", func(t *ftt.Test) {
				gcsAllowList.Users = []string{"user:test@test.com"}
				assert.Loosely(t, validate(cfg), should.BeNil)
			})
			t.Run("multiple", func(t *ftt.Test) {
				gcsAllowList.Users = []string{"user:test@test.com", "user:test2@test.com"}
				assert.Loosely(t, validate(cfg), should.BeNil)
			})
		})

		t.Run("GCS buckets", func(t *ftt.Test) {
			t.Run("bucket", func(t *ftt.Test) {
				t.Run("must be specified", func(t *ftt.Test) {
					gcsAllowList.Buckets[0] = ""
					assert.Loosely(t, validate(cfg), should.ErrLike("empty bucket is not allowed"))
				})
				t.Run("invalid", func(t *ftt.Test) {
					gcsAllowList.Buckets[0] = "b"
					assert.Loosely(t, validate(cfg), should.ErrLike(`invalid bucket: "b"`))
				})
				t.Run("valid", func(t *ftt.Test) {
					gcsAllowList.Buckets[0] = "bucket"
					assert.Loosely(t, validate(cfg), should.BeNil)
				})
			})
		})
	})
}

func TestServiceConfigValidator(t *testing.T) {
	t.Parallel()

	validate := func(cfg *configpb.Config) error {
		c := validation.Context{Context: context.Background()}
		validateServiceConfig(&c, cfg)
		return c.Finalize()
	}

	ftt.Run("config template is valid", t, func(t *ftt.Test) {
		content, err := os.ReadFile(
			"../../configs/service/template.cfg",
		)
		assert.Loosely(t, err, should.BeNil)
		cfg := &configpb.Config{}
		assert.Loosely(t, prototext.Unmarshal(content, cfg), should.BeNil)
		assert.Loosely(t, validate(cfg), should.BeNil)
	})

	ftt.Run("valid config is valid", t, func(t *ftt.Test) {
		cfg := CreatePlaceHolderServiceConfig()
		assert.Loosely(t, validate(cfg), should.BeNil)
	})

	ftt.Run("bq artifact export config", t, func(t *ftt.Test) {
		cfg := CreatePlaceHolderServiceConfig()
		t.Run("is nil", func(t *ftt.Test) {
			cfg.BqArtifactExportConfig = nil
			assert.Loosely(t, validate(cfg), should.NotBeNil)
		})

		t.Run("percentage smaller than 0", func(t *ftt.Test) {
			cfg.BqArtifactExportConfig = &configpb.BqArtifactExportConfig{
				ExportPercent: -1,
			}
			assert.Loosely(t, validate(cfg), should.NotBeNil)
		})

		t.Run("percentage bigger than 100", func(t *ftt.Test) {
			cfg.BqArtifactExportConfig = &configpb.BqArtifactExportConfig{
				ExportPercent: 101,
			}
			assert.Loosely(t, validate(cfg), should.NotBeNil)
		})
	})
}
