// Copyright 2025 The LUCI Authors.
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

package projects

import (
	"context"
	"testing"

	configpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/server/auth/realms"
)

func TestProjects(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	prj := New(&configpb.ProjectsCfg{
		Projects: []*configpb.Project{
			{
				Id: "proj-a",
				IdentityConfig: &configpb.IdentityConfig{
					ServiceAccountEmail:        "proj-a-prod-acc",
					StagingServiceAccountEmail: "proj-a-staging-acc",
				},
				BillingConfig: &configpb.BillingConfig{
					BillingCloudProjectId: 123,
				},
			},
			{
				Id: "proj-b",
				IdentityConfig: &configpb.IdentityConfig{
					ServiceAccountEmail: "proj-b-prod-acc",
				},
			},
			{
				Id: "proj-c",
			},
			{
				Id: realms.InternalProject,
				IdentityConfig: &configpb.IdentityConfig{
					ServiceAccountEmail: "ignored",
				},
			},
		},
	}, &config.Meta{Revision: "rev"})

	assert.That(t, prj.Rev, should.Equal("rev"))

	t.Run("prod accounts", func(t *testing.T) {
		assert.That(t, prj.ProjectConfig(ctx, "proj-a", false), should.Match(&ProjectConfig{
			ProjectScopedAccount:  "proj-a-prod-acc",
			BillingCloudProjectID: 123,
		}))
		assert.That(t, prj.ProjectConfig(ctx, "proj-b", false), should.Match(&ProjectConfig{
			ProjectScopedAccount: "proj-b-prod-acc",
		}))
	})

	t.Run("staging accounts", func(t *testing.T) {
		assert.That(t, prj.ProjectConfig(ctx, "proj-a", true), should.Match(&ProjectConfig{
			ProjectScopedAccount:  "proj-a-staging-acc",
			BillingCloudProjectID: 123,
		}))
		assert.That(t, prj.ProjectConfig(ctx, "proj-b", true), should.Match(&ProjectConfig{
			ProjectScopedAccount: "proj-b-prod-acc", // no staging, uses the prod one
		}))
	})

	t.Run("no configs", func(t *testing.T) {
		assert.That(t, prj.ProjectConfig(ctx, realms.InternalProject, false), should.Match(&ProjectConfig{}))
		assert.That(t, prj.ProjectConfig(ctx, "proj-c", false), should.Match(&ProjectConfig{}))
		assert.That(t, prj.ProjectConfig(ctx, "unknown", false), should.Match(&ProjectConfig{}))
	})
}
