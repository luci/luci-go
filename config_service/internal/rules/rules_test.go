// Copyright 2023 The LUCI Authors.
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

package rules

import (
	"fmt"
	"reflect"
	"regexp"
	"testing"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/luci/common/data/text/pattern"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/testutil"
)

func init() {
	regexpT := reflect.TypeFor[*regexp.Regexp]()
	registry.RegisterCmpOption(cmp.AllowUnexported(pattern.Regexp(regexp.MustCompile("hi"))))
	registry.RegisterCmpOption(cmp.FilterPath(func(p cmp.Path) bool {
		return p.Last().Type() == regexpT
	}, cmp.Transformer("regexp.Regexp", func(r *regexp.Regexp) string {
		return r.String()
	})))
}

func TestAddRules(t *testing.T) {
	t.Parallel()

	ftt.Run("Can derive patterns from rules", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()

		patterns, err := validation.Rules.ConfigPatterns(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, patterns, should.Match([]*validation.ConfigPattern{
			{
				ConfigSet: pattern.MustParse("exact:services/" + testutil.AppID),
				Path:      pattern.MustParse(common.ACLRegistryFilePath),
			},
			{
				ConfigSet: pattern.MustParse("exact:services/" + testutil.AppID),
				Path:      pattern.MustParse(common.ProjRegistryFilePath),
			},
			{
				ConfigSet: pattern.MustParse("exact:services/" + testutil.AppID),
				Path:      pattern.MustParse(common.ServiceRegistryFilePath),
			},
			{
				ConfigSet: pattern.MustParse("exact:services/" + testutil.AppID),
				Path:      pattern.MustParse(common.ImportConfigFilePath),
			},
			{
				ConfigSet: pattern.MustParse("exact:services/" + testutil.AppID),
				Path:      pattern.MustParse(common.SchemaConfigFilePath),
			},
			{
				ConfigSet: pattern.MustParse(`regex:projects/[^/]+`),
				Path:      pattern.MustParse(common.ProjMetadataFilePath),
			},
			{
				ConfigSet: pattern.MustParse(`regex:.+`),
				Path:      pattern.MustParse(`regex:.+\.json`),
			},
		}))
	})
}

func TestValidateACLsCfg(t *testing.T) {
	t.Parallel()

	ftt.Run("Validate acl.cfg", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}
		cs := config.MustServiceSet(testutil.AppID)
		path := common.ACLRegistryFilePath

		t.Run("valid", func(t *ftt.Test) {
			content := []byte(`
			project_access_group: "config-project-access"
			project_validation_group: "config-project-validation"
			project_reimport_group: "config-project-reimport"
			service_access_group: "config-service-access"
			service_validation_group: "config-service-validation"
			service_reimport_group: "config-service-reimport"
			`)
			assert.Loosely(t, validateACLsCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})

		t.Run("invalid proto", func(t *ftt.Test) {
			content := []byte(`bad config`)
			assert.Loosely(t, validateACLsCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`in "acl.cfg"`))
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("invalid AclCfg proto:"))
		})

		for _, fieldName := range []string{
			"project_access_group",
			"project_validation_group",
			"project_reimport_group",
			"service_access_group",
			"service_validation_group",
			"service_reimport_group",
		} {
			t.Run(fmt.Sprintf("invalid %s", fieldName), func(t *ftt.Test) {
				content := []byte(fmt.Sprintf(`%s: "bad^group"`, fieldName))
				assert.Loosely(t, validateACLsCfg(vctx, string(cs), path, content), should.BeNil)
				assert.Loosely(t, vctx.Finalize(), should.ErrLike(fmt.Sprintf(`invalid %s: "bad^group"`, fieldName)))
			})
		}
	})
}

func TestValidateServicesCfg(t *testing.T) {
	t.Parallel()

	ftt.Run("Validate services.cfg", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}
		cs := config.MustServiceSet(testutil.AppID)
		path := common.ServiceRegistryFilePath

		t.Run("passed", func(t *ftt.Test) {
			content := []byte(`services {
				id: "luci-config-dev"
				owners: "luci-config-dev@google.com"
				hostname: "luci-config-dev.example.com"
				access: "group:googlers"
				access: "user:user-a@example.com"
				access: "user-b@example.com"
			}`)
			assert.Loosely(t, validateServicesCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})

		t.Run("invalid proto", func(t *ftt.Test) {
			content := []byte(`bad  config`)
			assert.Loosely(t, validateServicesCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`in "services.cfg"`))
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("invalid services proto:"))
		})

		t.Run("empty id", func(t *ftt.Test) {
			content := []byte(`services {
				id: ""
			}`)
			assert.Loosely(t, validateServicesCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(services #0 / id): not specified`))
		})

		t.Run("invalid id", func(t *ftt.Test) {
			content := []byte(`services {
				id: "foo/bar"
			}`)
			assert.Loosely(t, validateServicesCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(services #0 / id): invalid id:`))
		})

		t.Run("duplicate id", func(t *ftt.Test) {
			content := []byte(`services {
				id: "foo"
			}
			services {
				id: "foo"
			}`)
			assert.Loosely(t, validateServicesCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(services #1 / id): duplicate: "foo"`))
		})

		t.Run("bad owner email address", func(t *ftt.Test) {
			content := []byte(`services {
					id: "foo"
					owners: "bad email"
				}`)
			assert.Loosely(t, validateServicesCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(services #0 / owners #0): invalid email address:`))
		})

		t.Run("invalid metadata url", func(t *ftt.Test) {
			content := []byte(`services {
				id: "foo"
				metadata_url: "https://example.com\\metadata"
			}`)
			assert.Loosely(t, validateServicesCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(services #0 / metadata_url): invalid url:`))
		})

		t.Run("invalid service hostname", func(t *ftt.Test) {
			content := []byte(`services {
				id: "foo"
				hostname: "https://foo.example.com\\\\api/"
			}`)
			assert.Loosely(t, validateServicesCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(services #0 / hostname): hostname does not match regex`))
		})

		t.Run("invalid access group", func(t *ftt.Test) {
			content := []byte(`services {
				id: "foo"
				access: "group:goo!"
			}`)
			assert.Loosely(t, validateServicesCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(services #0 / access #0): invalid auth group: "goo!"`))
		})
	})
}

func TestValidateImportCfg(t *testing.T) {
	t.Parallel()

	ftt.Run("Validate import.cfg", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}
		cs := config.MustServiceSet(testutil.AppID)
		path := common.ImportConfigFilePath

		t.Run("invalid", func(t *ftt.Test) {
			content := []byte(`bad  config`)
			assert.Loosely(t, validateImportCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`in "import.cfg"`))
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("invalid import proto:"))
		})

		t.Run("valid", func(t *ftt.Test) {
			content := []byte(`gitiles {
				project_config_default_ref: "refs/heads/infra/config"
				ref_config_default_path: "infra/config"
				fetch_log_deadline: 60
			}`)
			assert.Loosely(t, validateImportCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})
	})
}

func TestValidateSchemaCfg(t *testing.T) {
	t.Parallel()

	ftt.Run("Validate schemas.cfg", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}
		cs := config.MustServiceSet(testutil.AppID)
		path := common.SchemaConfigFilePath

		t.Run("passed", func(t *ftt.Test) {
			content := []byte(`schemas {
				name: "projects:foo.cfg"
				url: "https://example.com/foo.proto"
			}
			schemas {
				name: "services/abc:bar.cfg"
				url: "https://example.com/bar.proto"
			}`)
			assert.Loosely(t, validateSchemaCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})

		t.Run("invalid proto", func(t *ftt.Test) {
			content := []byte(`bad config`)
			assert.Loosely(t, validateSchemaCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`in "schemas.cfg"`))
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("invalid schema proto"))
		})

		t.Run("missing name", func(t *ftt.Test) {
			content := []byte(`schemas {
				name: ""
				url: "https://example.com/foo.proto"
			}`)
			assert.Loosely(t, validateSchemaCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("(schemas #0 / name): not specified"))
		})

		t.Run("missing colon", func(t *ftt.Test) {
			content := []byte(`schemas {
				name: "projects"
				url: "https://example.com/foo.proto"
			}`)
			assert.Loosely(t, validateSchemaCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(schemas #0 / name): must contain ":"`))
		})

		t.Run("duplicate names", func(t *ftt.Test) {
			content := []byte(`schemas {
				name: "projects:foo.cfg"
				url: "https://example.com/foo.proto"
			}
			schemas {
				name: "projects:foo.cfg"
				url: "https://example.com/bar.proto"
			}`)
			assert.Loosely(t, validateSchemaCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(schemas #1 / name): duplicate: "projects:foo.cfg"`))
		})

		t.Run("invalid left hand side of colon", func(t *ftt.Test) {
			content := []byte(`schemas {
				name: "projects/abc:foo.cfg"
				url: "https://example.com/foo.proto"
			}`)
			assert.Loosely(t, validateSchemaCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(schemas #0 / name): left side of ":" must be a service config set or "projects"`))
		})

		t.Run("missing path", func(t *ftt.Test) {
			content := []byte(`schemas {
				name: "services/foo:"
				url: "https://example.com/foo.proto"
			}`)
			assert.Loosely(t, validateSchemaCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(schemas #0 / name / right side of ":" (path)): not specified`))
		})

		t.Run("absolute", func(t *ftt.Test) {
			content := []byte(`schemas {
				name: "services/foo:/etc/foo.cfg"
				url: "https://example.com/foo.proto"
			}`)
			assert.Loosely(t, validateSchemaCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(schemas #0 / name / right side of ":" (path)): must not be absolute: "/etc/foo.cfg"`))
		})

		t.Run("contain .", func(t *ftt.Test) {
			content := []byte(`schemas {
				name: "services/foo:./foo.cfg"
				url: "https://example.com/foo.proto"
			}`)
			assert.Loosely(t, validateSchemaCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(schemas #0 / name / right side of ":" (path)): must not contain "." or ".." components: "./foo.cfg"`))
		})

		t.Run("contain ..", func(t *ftt.Test) {
			content := []byte(`schemas {
				name: "services/foo:../foo.cfg"
				url: "https://example.com/foo.proto"
			}`)
			assert.Loosely(t, validateSchemaCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(schemas #0 / name / right side of ":" (path)): must not contain "." or ".." components: "../foo.cfg"`))
		})

		t.Run("invalid url ", func(t *ftt.Test) {
			content := []byte(`schemas {
				name: "projects:foo.cfg"
				url: "https://example.com\\foo.proto"
			}`)
			assert.Loosely(t, validateSchemaCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(schemas #0 / url): invalid url:`))
		})
	})
}

func TestValidateJSON(t *testing.T) {
	t.Parallel()

	ftt.Run("Validate JSON", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}
		cs := config.MustProjectSet("foo")
		path := "bar.json"

		t.Run("invalid", func(t *ftt.Test) {
			content := []byte(`{not a json object - "config"}`)
			assert.Loosely(t, validateJSON(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`in "bar.json"`))
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("invalid JSON:"))
		})

		t.Run("valid", func(t *ftt.Test) {
			content := []byte(`{"abc": "xyz"}`)
			assert.Loosely(t, validateJSON(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})
	})
}
