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
	"testing"

	"go.chromium.org/luci/common/data/text/pattern"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAddRules(t *testing.T) {
	t.Parallel()

	Convey("Can derive patterns from rules", t, func() {
		ctx := testutil.SetupContext()

		patterns, err := validation.Rules.ConfigPatterns(ctx)
		So(err, ShouldBeNil)
		So(patterns, ShouldResemble, []*validation.ConfigPattern{
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
		})
	})
}

func TestValidateACLsCfg(t *testing.T) {
	t.Parallel()

	Convey("Validate acl.cfg", t, func() {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}
		cs := config.MustServiceSet(testutil.AppID)
		path := common.ACLRegistryFilePath

		Convey("valid", func() {
			content := []byte(`
			project_access_group: "config-project-access"
			project_validation_group: "config-project-validation"
			project_reimport_group: "config-project-reimport"
			service_access_group: "config-service-access"
			service_validation_group: "config-service-validation"
			service_reimport_group: "config-service-reimport"
			`)
			So(validateACLsCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldBeNil)
		})

		Convey("invalid proto", func() {
			content := []byte(`bad config`)
			So(validateACLsCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `in "acl.cfg"`, "invalid AclCfg proto:")
		})

		for _, fieldName := range []string{
			"project_access_group",
			"project_validation_group",
			"project_reimport_group",
			"service_access_group",
			"service_validation_group",
			"service_reimport_group",
		} {
			Convey(fmt.Sprintf("invalid %s", fieldName), func() {
				content := []byte(fmt.Sprintf(`%s: "bad^group"`, fieldName))
				So(validateACLsCfg(vctx, string(cs), path, content), ShouldBeNil)
				So(vctx.Finalize(), ShouldErrLike, fmt.Sprintf(`invalid %s: "bad^group"`, fieldName))
			})
		}
	})
}

func TestValidateServicesCfg(t *testing.T) {
	t.Parallel()

	Convey("Validate services.cfg", t, func() {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}
		cs := config.MustServiceSet(testutil.AppID)
		path := common.ServiceRegistryFilePath

		Convey("passed", func() {
			content := []byte(`services {
				id: "luci-config-dev"
				owners: "luci-config-dev@google.com"
				hostname: "luci-config-dev.example.com"
				access: "group:googlers"
				access: "user:user-a@example.com"
				access: "user-b@example.com"
			}`)
			So(validateServicesCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldBeNil)
		})

		Convey("invalid proto", func() {
			content := []byte(`bad  config`)
			So(validateServicesCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `in "services.cfg"`, "invalid services proto:")
		})

		Convey("empty id", func() {
			content := []byte(`services {
				id: ""
			}`)
			So(validateServicesCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(services #0 / id): not specified`)
		})

		Convey("invalid id", func() {
			content := []byte(`services {
				id: "foo/bar"
			}`)
			So(validateServicesCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(services #0 / id): invalid id:`)
		})

		Convey("duplicate id", func() {
			content := []byte(`services {
				id: "foo"
			}
			services {
				id: "foo"
			}`)
			So(validateServicesCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(services #1 / id): duplicate: "foo"`)
		})

		Convey("bad owner email address", func() {
			content := []byte(`services {
					id: "foo"
					owners: "bad email"
				}`)
			So(validateServicesCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(services #0 / owners #0): invalid email address:`)
		})

		Convey("invalid metadata url", func() {
			content := []byte(`services {
				id: "foo"
				metadata_url: "https://example.com\\metadata"
			}`)
			So(validateServicesCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(services #0 / metadata_url): invalid url:`)
		})

		Convey("invalid service hostname", func() {
			content := []byte(`services {
				id: "foo"
				hostname: "https://foo.example.com\\\\api/"
			}`)
			So(validateServicesCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(services #0 / hostname): hostname does not match regex`)
		})

		Convey("invalid access group", func() {
			content := []byte(`services {
				id: "foo"
				access: "group:goo!"
			}`)
			So(validateServicesCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(services #0 / access #0): invalid auth group: "goo!"`)
		})

		Convey("not sorted", func() {
			content := []byte(`services {
				id: "foo"
			}
			services {
				id: "bar"
			}`)
			So(validateServicesCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `services are not sorted by id. First offending id: "bar"`)
		})
	})
}

func TestValidateImportCfg(t *testing.T) {
	t.Parallel()

	Convey("Validate import.cfg", t, func() {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}
		cs := config.MustServiceSet(testutil.AppID)
		path := common.ImportConfigFilePath

		Convey("invalid", func() {
			content := []byte(`bad  config`)
			So(validateImportCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `in "import.cfg"`, "invalid import proto:")
		})

		Convey("valid", func() {
			content := []byte(`gitiles {
				project_config_default_ref: "refs/heads/infra/config"
				ref_config_default_path: "infra/config"
				fetch_log_deadline: 60
			}`)
			So(validateImportCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldBeNil)
		})
	})
}

func TestValidateSchemaCfg(t *testing.T) {
	t.Parallel()

	Convey("Validate schemas.cfg", t, func() {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}
		cs := config.MustServiceSet(testutil.AppID)
		path := common.SchemaConfigFilePath

		Convey("passed", func() {
			content := []byte(`schemas {
				name: "projects:foo.cfg"
				url: "https://example.com/foo.proto"
			}
			schemas {
				name: "services/abc:bar.cfg"
				url: "https://example.com/bar.proto"
			}`)
			So(validateSchemaCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldBeNil)
		})

		Convey("invalid proto", func() {
			content := []byte(`bad config`)
			So(validateSchemaCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `in "schemas.cfg"`, "invalid schema proto")
		})

		Convey("missing name", func() {
			content := []byte(`schemas {
				name: ""
				url: "https://example.com/foo.proto"
			}`)
			So(validateSchemaCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, "(schemas #0 / name): not specified")
		})

		Convey("missing colon", func() {
			content := []byte(`schemas {
				name: "projects"
				url: "https://example.com/foo.proto"
			}`)
			So(validateSchemaCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(schemas #0 / name): must contain ":"`)
		})

		Convey("duplicate names", func() {
			content := []byte(`schemas {
				name: "projects:foo.cfg"
				url: "https://example.com/foo.proto"
			}
			schemas {
				name: "projects:foo.cfg"
				url: "https://example.com/bar.proto"
			}`)
			So(validateSchemaCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(schemas #1 / name): duplicate: "projects:foo.cfg"`)
		})

		Convey("invalid left hand side of colon", func() {
			content := []byte(`schemas {
				name: "projects/abc:foo.cfg"
				url: "https://example.com/foo.proto"
			}`)
			So(validateSchemaCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(schemas #0 / name): left side of ":" must be a service config set or "projects"`)
		})

		Convey("missing path", func() {
			content := []byte(`schemas {
				name: "services/foo:"
				url: "https://example.com/foo.proto"
			}`)
			So(validateSchemaCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(schemas #0 / name / right side of ":" (path)): not specified`)
		})

		Convey("absolute", func() {
			content := []byte(`schemas {
				name: "services/foo:/etc/foo.cfg"
				url: "https://example.com/foo.proto"
			}`)
			So(validateSchemaCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(schemas #0 / name / right side of ":" (path)): must not be absolute: "/etc/foo.cfg"`)
		})

		Convey("contain .", func() {
			content := []byte(`schemas {
				name: "services/foo:./foo.cfg"
				url: "https://example.com/foo.proto"
			}`)
			So(validateSchemaCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(schemas #0 / name / right side of ":" (path)): must not contain "." or ".." components: "./foo.cfg"`)
		})

		Convey("contain ..", func() {
			content := []byte(`schemas {
				name: "services/foo:../foo.cfg"
				url: "https://example.com/foo.proto"
			}`)
			So(validateSchemaCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(schemas #0 / name / right side of ":" (path)): must not contain "." or ".." components: "../foo.cfg"`)
		})

		Convey("invalid url ", func() {
			content := []byte(`schemas {
				name: "projects:foo.cfg"
				url: "https://example.com\\foo.proto"
			}`)
			So(validateSchemaCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(schemas #0 / url): invalid url:`)
		})
	})
}

func TestValidateJSON(t *testing.T) {
	t.Parallel()

	Convey("Validate JSON", t, func() {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}
		cs := config.MustProjectSet("foo")
		path := "bar.json"

		Convey("invalid", func() {
			content := []byte(`{not a json object - "config"}`)
			So(validateJSON(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `in "bar.json"`, "invalid JSON:")
		})

		Convey("valid", func() {
			content := []byte(`{"abc": "xyz"}`)
			So(validateJSON(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldBeNil)
		})
	})
}
