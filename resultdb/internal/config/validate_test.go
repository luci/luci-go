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
	"fmt"
	"os"
	"strings"
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

	ftt.Run("schemes", t, func(t *ftt.Test) {
		cfg := CreatePlaceHolderServiceConfig()
		cfg.Schemes = []*configpb.Scheme{
			{
				Id:                "junit",
				HumanReadableName: "JUnit",
				Coarse: &configpb.Scheme_Level{
					HumanReadableName: "Package",
				},
				Fine: &configpb.Scheme_Level{
					HumanReadableName: "Class",
				},
				Case: &configpb.Scheme_Level{
					HumanReadableName: "Method",
				},
			},
		}
		t.Run("Valid", func(t *ftt.Test) {
			assert.Loosely(t, validate(cfg), should.BeNil)
		})
		t.Run("Collection too Large", func(t *ftt.Test) {
			t.Run("By size", func(t *ftt.Test) {
				cfg.Schemes = make([]*configpb.Scheme, 0, 1001)
				for i := 0; i < 1000; i++ {
					// Each scheme is >100 bytes, and there are 1000, so
					// the total size will be over 100 KB.
					cfg.Schemes = append(cfg.Schemes, &configpb.Scheme{
						Id:                fmt.Sprintf("scheme%d", i),
						HumanReadableName: strings.Repeat("A", 100),
						Case: &configpb.Scheme_Level{
							HumanReadableName: fmt.Sprintf("Case %d", i),
						},
					})
				}
				assert.Loosely(t, validate(cfg), should.ErrLike("(schemes): too large; total size of configured schemes must not exceed 100 KB"))
			})
			t.Run("By elements", func(t *ftt.Test) {
				cfg.Schemes = make([]*configpb.Scheme, 0, 1001)
				for i := 0; i < 1001; i++ {
					cfg.Schemes = append(cfg.Schemes, &configpb.Scheme{
						Id:                fmt.Sprintf("scheme%d", i),
						HumanReadableName: fmt.Sprintf("Scheme %d", i),
						Case: &configpb.Scheme_Level{
							HumanReadableName: fmt.Sprintf("Case %d", i),
						},
					})
				}
				assert.Loosely(t, validate(cfg), should.ErrLike("(schemes): too large; may not exceed 1000 configured schemes"))
			})
		})

		scheme := cfg.Schemes[0]
		path := "schemes / [0]"
		t.Run("Id", func(t *ftt.Test) {
			path := path + " / id"
			t.Run("Empty", func(t *ftt.Test) {
				scheme.Id = ""
				assert.Loosely(t, validate(cfg), should.ErrLike(`(`+path+`): unspecified`))
			})
			t.Run("Invalid", func(t *ftt.Test) {
				scheme.Id = "some_thing"
				assert.Loosely(t, validate(cfg), should.ErrLike(`(`+path+`): does not match pattern "^[a-z][a-z0-9]{0,19}$"`))
			})
			t.Run("Reserved", func(t *ftt.Test) {
				scheme.Id = "legacy"
				assert.Loosely(t, validate(cfg), should.ErrLike(`(`+path+`): "legacy" is a reserved built-in scheme and cannot be configured`))
			})
			t.Run("Duplicate IDs", func(t *ftt.Test) {
				// Create another scheme with the same ID.
				cfg.Schemes = append(cfg.Schemes, &configpb.Scheme{
					Id:                "junit",
					HumanReadableName: "JUnit",
					Coarse: &configpb.Scheme_Level{
						HumanReadableName: "Package",
					},
					Fine: &configpb.Scheme_Level{
						HumanReadableName: "Class",
					},
					Case: &configpb.Scheme_Level{
						HumanReadableName: "Method",
					},
				})
				assert.Loosely(t, validate(cfg), should.ErrLike(`(schemes / [1] / id): scheme with ID "junit" appears in collection more than once`))
			})
		})
		t.Run("Human Readable Name", func(t *ftt.Test) {
			path := path + " / human_readable_name"
			t.Run("Empty", func(t *ftt.Test) {
				scheme.HumanReadableName = ""
				assert.Loosely(t, validate(cfg), should.ErrLike(`(`+path+`): unspecified`))
			})
			t.Run("Invalid", func(t *ftt.Test) {
				scheme.HumanReadableName = "\n"
				assert.Loosely(t, validate(cfg), should.ErrLike(`(`+path+`): does not match pattern "^[[:print:]]{1,100}$"`))
			})
		})
		t.Run("Coarse", func(t *ftt.Test) {
			scheme.Coarse = &configpb.Scheme_Level{
				HumanReadableName: "Package",
				ValidationRegexp:  "^[a-z.0-9]+$",
			}
			path := path + " / coarse"

			t.Run("Valid", func(t *ftt.Test) {
				assert.Loosely(t, validate(cfg), should.BeNil)
			})
			t.Run("Unset", func(t *ftt.Test) {
				// The coarse level may be unset, this means it should not be used for tests using that scheme.
				scheme.Coarse = nil
				assert.Loosely(t, validate(cfg), should.BeNil)
			})
			t.Run("Human Readable Name", func(t *ftt.Test) {
				path := path + " / human_readable_name"
				t.Run("Empty", func(t *ftt.Test) {
					scheme.Coarse.HumanReadableName = ""
					assert.Loosely(t, validate(cfg), should.ErrLike(`(`+path+`): unspecified`))
				})
				t.Run("Invalid", func(t *ftt.Test) {
					scheme.Coarse.HumanReadableName = "\n"
					assert.Loosely(t, validate(cfg), should.ErrLike(`(`+path+`): does not match pattern "^[[:print:]]{1,100}$"`))
				})
			})
			t.Run("Validation Regexp", func(t *ftt.Test) {
				path := path + " / validation_regexp"
				t.Run("Empty", func(t *ftt.Test) {
					// Empty validation regexp is valid, it means no additional validation should be applied.
					scheme.Coarse.ValidationRegexp = ""
					assert.Loosely(t, validate(cfg), should.BeNil)
				})
				t.Run("Invalid (no starting ^)", func(t *ftt.Test) {
					scheme.Coarse.ValidationRegexp = "a$"
					assert.Loosely(t, validate(cfg), should.ErrLike(`(`+path+`): pattern must start and end with ^ and $`))
				})
				t.Run("Invalid (no ending $)", func(t *ftt.Test) {
					scheme.Coarse.ValidationRegexp = "^a"
					assert.Loosely(t, validate(cfg), should.ErrLike(`(`+path+`): pattern must start and end with ^ and $`))
				})

				t.Run("Invalid (does not compile)", func(t *ftt.Test) {
					scheme.Coarse.ValidationRegexp = "^[$"
					assert.Loosely(t, validate(cfg), should.ErrLike(`(`+path+`): could not compile pattern: error parsing regexp: missing closing ]: `))
				})
			})
		})
		t.Run("Fine", func(t *ftt.Test) {
			scheme.Fine = &configpb.Scheme_Level{
				HumanReadableName: "Class",
				ValidationRegexp:  "^[a-zA-Z_0-9]+$",
			}
			path := path + " / fine"

			t.Run("Valid", func(t *ftt.Test) {
				assert.Loosely(t, validate(cfg), should.BeNil)
			})
			t.Run("Unset", func(t *ftt.Test) {
				t.Run("With coarse unset", func(t *ftt.Test) {
					scheme.Coarse = nil
					scheme.Fine = nil
					assert.Loosely(t, validate(cfg), should.BeNil)
				})
				t.Run("With coarse set", func(t *ftt.Test) {
					scheme.Coarse = &configpb.Scheme_Level{
						HumanReadableName: "Package",
					}
					scheme.Fine = nil
					assert.Loosely(t, validate(cfg), should.ErrLike(`invalid combination of levels, got coarse set and fine unset; if only one level is to be used, configure the fine level instead of the coarse level`))
				})
			})
			t.Run("Invalid", func(t *ftt.Test) {
				// Do not need to test all invalid cases, uses a common validation
				// routine as the coarse level, and that level is tested above.
				scheme.Fine.HumanReadableName = ""
				assert.Loosely(t, validate(cfg), should.ErrLike(`(`+path+` / human_readable_name): unspecified`))
			})
		})
		t.Run("Case", func(t *ftt.Test) {
			scheme.Case = &configpb.Scheme_Level{
				HumanReadableName: "Method",
				ValidationRegexp:  "^[a-zA-Z_0-9]+$",
			}
			path := path + " / case"

			t.Run("Valid", func(t *ftt.Test) {
				assert.Loosely(t, validate(cfg), should.BeNil)
			})
			t.Run("Unset", func(t *ftt.Test) {
				// It is an error to not configure the test case level.
				scheme.Case = nil
				assert.Loosely(t, validate(cfg), should.ErrLike(`(`+path+`): unspecified`))
			})
			t.Run("Invalid", func(t *ftt.Test) {
				// Do not need to test all invalid cases, uses a common validation
				// routine as the coarse level, and that level is tested above.
				scheme.Case.HumanReadableName = ""
				assert.Loosely(t, validate(cfg), should.ErrLike(`(`+path+` / human_readable_name): unspecified`))
			})
		})
	})
}
