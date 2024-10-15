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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/config_service/internal/common"
	"go.chromium.org/luci/config_service/testutil"
)

func TestValidateProjectsCfg(t *testing.T) {
	t.Parallel()

	ftt.Run("Validate projects.cfg", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}
		cs := config.MustServiceSet(testutil.AppID)
		path := common.ProjRegistryFilePath

		t.Run("valid", func(t *ftt.Test) {
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
			assert.Loosely(t, validateProjectsCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})

		t.Run("invalid proto", func(t *ftt.Test) {
			content := []byte(`bad config`)
			assert.Loosely(t, validateProjectsCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`in "projects.cfg"`))
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("invalid projects proto:"))
		})

		t.Run("missing maintenance contact", func(t *ftt.Test) {
			content := []byte(`teams {
				name: "Example Team"
				escalation_contact: "team-escalation@google.com"
			}`)
			assert.Loosely(t, validateProjectsCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(teams #0): maintenance_contact is required`))
		})
		t.Run("invalid maintenance contact", func(t *ftt.Test) {
			content := []byte(`teams {
				name: "Example Team"
				maintenance_contact: "bad email"
				escalation_contact: "team-escalation@google.com"
			}`)
			assert.Loosely(t, validateProjectsCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(teams #0 / maintenance_contact #0): invalid email address`))
		})

		t.Run("warn on missing escalation contact", func(t *ftt.Test) {
			content := []byte(`teams {
				name: "Example Team"
				maintenance_contact: "team-maintenance@example.com"
			}`)
			assert.Loosely(t, validateProjectsCfg(vctx, string(cs), path, content), should.BeNil)
			vErr := vctx.Finalize().(*validation.Error)
			assert.Loosely(t, vErr.WithSeverity(validation.Warning), should.ErrLike(`(teams #0): escalation_contact is recommended`))
		})
		t.Run("invalid escalation contact", func(t *ftt.Test) {
			content := []byte(`teams {
				name: "Example Team"
				maintenance_contact: "team-maintenance@example.com"
				escalation_contact: "bad email"
			}`)
			assert.Loosely(t, validateProjectsCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(teams #0 / escalation_contact #0): invalid email address`))
		})

		t.Run("not sorted teams", func(t *ftt.Test) {
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
			assert.Loosely(t, validateProjectsCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`teams are not sorted by id. First offending id: "Example Team A"`))
		})

		t.Run("invalid project ID", func(t *ftt.Test) {
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
			assert.Loosely(t, validateProjectsCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(projects #0 / id): invalid id:`))
		})

		t.Run("invalid gitiles location", func(t *ftt.Test) {
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
			assert.Loosely(t, validateProjectsCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(projects #0 / gitiles_location): repo: only https scheme is supported`))
		})

		t.Run("empty owned_by", func(t *ftt.Test) {
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
			assert.Loosely(t, validateProjectsCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(projects #0 / owned_by): not specified`))
		})
		t.Run("unknown owned_by", func(t *ftt.Test) {
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
			assert.Loosely(t, validateProjectsCfg(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(projects #0 / owned_by): unknown team "Example Team`))
		})
	})
}

func TestValidateProjectMetadata(t *testing.T) {
	t.Parallel()

	ftt.Run("Validate project.cfg", t, func(t *ftt.Test) {
		ctx := testutil.SetupContext()
		vctx := &validation.Context{Context: ctx}
		cs := config.MustServiceSet(testutil.AppID)
		path := common.ProjMetadataFilePath

		t.Run("valid", func(t *ftt.Test) {
			content := []byte(`
			name: "example-proj"
			access: "group:all"
			`)
			assert.Loosely(t, validateProjectMetadata(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.BeNil)
		})

		t.Run("invalid proto", func(t *ftt.Test) {
			content := []byte(`bad config`)
			assert.Loosely(t, validateProjectMetadata(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`in "project.cfg"`))
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("invalid project proto:"))
		})

		t.Run("missing name", func(t *ftt.Test) {
			content := []byte(`
			name: ""
			access: "group:all"
			`)
			assert.Loosely(t, validateProjectMetadata(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike("name is not specified"))
		})

		t.Run("invalid access group", func(t *ftt.Test) {
			content := []byte(`
				name: "example-proj"
				access: "group:goo!"
			`)
			assert.Loosely(t, validateProjectMetadata(vctx, string(cs), path, content), should.BeNil)
			assert.Loosely(t, vctx.Finalize(), should.ErrLike(`(access #0): invalid auth group`))
		})
	})
}
