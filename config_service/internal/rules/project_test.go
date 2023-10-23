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
	"testing"

	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateProjectsCfg(t *testing.T) {
	t.Parallel()

	Convey("Validate projects.cfg", t, func() {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}
		cs := config.MustServiceSet(testutil.AppID)
		path := common.ProjRegistryFilePath

		Convey("valid", func() {
			content := []byte(`teams {
				name: "Example Team"
				maintenance_contact: "team-maintenance@example.com"
				escalation_contact: "team-escalation@google.com"
			}
			projects {
				id: "example-proj"
				owned_by: "Example Team"
				gitiles_location {
					repo: "https://example.googlesource.com/example"
					ref:  "refs/heads/main"
					path: "infra/config/generated"
				}
				identity_config {
					service_account_email: "example-sa@example.com"
				}
			}`)
			So(validateProjectsCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldBeNil)
		})

		Convey("invalid proto", func() {
			content := []byte(`bad config`)
			So(validateProjectsCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `in "projects.cfg"`, "invalid projects proto:")
		})

		Convey("missing maintenance contact", func() {
			content := []byte(`teams {
				name: "Example Team"
				escalation_contact: "team-escalation@google.com"
			}`)
			So(validateProjectsCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(teams #0): maintenance_contact is required`)
		})
		Convey("invalid maintenance contact", func() {
			content := []byte(`teams {
				name: "Example Team"
				maintenance_contact: "bad email"
				escalation_contact: "team-escalation@google.com"
			}`)
			So(validateProjectsCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(teams #0 / maintenance_contact #0): invalid email address`)
		})

		Convey("warn on missing escalation contact", func() {
			content := []byte(`teams {
				name: "Example Team"
				maintenance_contact: "team-maintenance@example.com"
			}`)
			So(validateProjectsCfg(vctx, string(cs), path, content), ShouldBeNil)
			vErr := vctx.Finalize().(*validation.Error)
			So(vErr.WithSeverity(validation.Warning), ShouldErrLike, `(teams #0): escalation_contact is recommended`)
		})
		Convey("invalid escalation contact", func() {
			content := []byte(`teams {
				name: "Example Team"
				maintenance_contact: "team-maintenance@example.com"
				escalation_contact: "bad email"
			}`)
			So(validateProjectsCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(teams #0 / escalation_contact #0): invalid email address`)
		})

		Convey("not sorted teams", func() {
			content := []byte(`teams {
				name: "Example Team B"
				maintenance_contact: "team-b-maintenance@example.com"
				escalation_contact: "team-b-escalation@google.com"
			}
			teams {
				name: "Example Team A"
				maintenance_contact: "team-a-maintenance@example.com"
				escalation_contact: "team-a-escalation@google.com"
			}`)
			So(validateProjectsCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `teams are not sorted by id. First offending id: "Example Team A"`)
		})

		Convey("invalid project ID", func() {
			content := []byte(`teams {
				name: "Example Team"
				maintenance_contact: "team-maintenance@example.com"
				escalation_contact: "team-escalation@google.com"
			}
			projects {
				id: "bad/project"
				owned_by: "Example Team"
				gitiles_location {
					repo: "https://example.googlesource.com/example"
					ref:  "refs/heads/main"
					path: "infra/config/generated"
				}
				identity_config {
					service_account_email: "example-sa@example.com"
				}
			}`)
			So(validateProjectsCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(projects #0 / id): invalid id:`)
		})

		Convey("invalid gitiles location", func() {
			content := []byte(`teams {
				name: "Example Team"
				maintenance_contact: "team-maintenance@example.com"
				escalation_contact: "team-escalation@google.com"
			}
			projects {
				id: "example-proj"
				owned_by: "Example Team"
				gitiles_location {
					repo: "http://example.googlesource.com/another-example"
					ref:  "refs/heads/main"
					path: "infra/config/generated"
				}
				identity_config {
					service_account_email: "example-sa@example.com"
				}
			}`)
			So(validateProjectsCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(projects #0 / gitiles_location): repo: only https scheme is supported`)
		})

		Convey("empty owned_by", func() {
			content := []byte(`projects {
				id: "example-proj"
				owned_by: ""
				gitiles_location {
					repo: "https://example.googlesource.com/another-example"
					ref:  "refs/heads/main"
					path: "infra/config/generated"
				}
				identity_config {
					service_account_email: "example-sa@example.com"
				}
			}`)
			So(validateProjectsCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(projects #0 / owned_by): not specified`)
		})
		Convey("unknown owned_by", func() {
			content := []byte(`projects {
				id: "example-proj"
				owned_by: "Example Team"
				gitiles_location {
					repo: "https://example.googlesource.com/another-example"
					ref:  "refs/heads/main"
					path: "infra/config/generated"
				}
				identity_config {
					service_account_email: "example-sa@example.com"
				}
			}`)
			So(validateProjectsCfg(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(projects #0 / owned_by): unknown team "Example Team`)
		})
	})
}

func TestValidateProjectMetadata(t *testing.T) {
	t.Parallel()

	Convey("Validate project.cfg", t, func() {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}
		cs := config.MustServiceSet(testutil.AppID)
		path := common.ProjMetadataFilePath

		Convey("valid", func() {
			content := []byte(`
			name: "example-proj"
			access: "group:all"
			`)
			So(validateProjectMetadata(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldBeNil)
		})

		Convey("invalid proto", func() {
			content := []byte(`bad config`)
			So(validateProjectMetadata(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `in "project.cfg"`, "invalid project proto:")
		})

		Convey("missing name", func() {
			content := []byte(`
			name: ""
			access: "group:all"
			`)
			So(validateProjectMetadata(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, "name is not specified")
		})

		Convey("invalid access group", func() {
			content := []byte(`
				name: "example-proj"
				access: "group:goo!"
			`)
			So(validateProjectMetadata(vctx, string(cs), path, content), ShouldBeNil)
			So(vctx.Finalize(), ShouldErrLike, `(access #0): invalid auth group`)
		})
	})
}
