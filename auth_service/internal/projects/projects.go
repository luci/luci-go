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

// Package projects contains helpers for working with LUCI project configs.
package projects

import (
	"context"

	"go.chromium.org/luci/common/logging"
	configpb "go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config"
	"go.chromium.org/luci/server/auth/realms"
)

// Project is a set of all LUCI projects configured in projects.cfg.
type Projects struct {
	// Rev is a revision of projects.cfg used to load projects data.
	Rev string
	// projects is a map "project name => its projects.cfg config entry".
	projects map[string]*configpb.Project
}

// ProjectConfig is a single project config extracted from projects.cfg.
//
// See auth/proto/realms.proto or proto/config/service_config.proto for detailed
// meaning of fields.
type ProjectConfig struct {
	// ProjectScopedAccount is service account LUCI uses on behalf of a project.
	ProjectScopedAccount string
	// BillingCloudProjectID is used to bill GCP calls to.
	BillingCloudProjectID uint64
}

// IsEmpty is true if this is an empty config.
func (pc *ProjectConfig) IsEmpty() bool {
	return pc.ProjectScopedAccount == "" && pc.BillingCloudProjectID == 0
}

// New converts a fetched projects.cfg into a queryable form.
func New(cfg *configpb.ProjectsCfg, meta *config.Meta) *Projects {
	projects := make(map[string]*configpb.Project, len(cfg.Projects))
	for _, p := range cfg.Projects {
		projects[p.Id] = p
	}
	return &Projects{
		Rev:      meta.Revision,
		projects: projects,
	}
}

// ProjectConfig returns a config for the given project, if any.
//
// If there's no project config or its relevant fields are not populated,
// returns a default empty config.
func (p *Projects) ProjectConfig(ctx context.Context, projectID string, useStagingEmail bool) *ProjectConfig {
	// "@internal" is not a real project, return default config.
	if projectID == realms.InternalProject {
		return &ProjectConfig{}
	}

	cfg := p.projects[projectID]
	if cfg == nil {
		logging.Warningf(ctx, "Looking up projects.cfg config of unknown project %q", projectID)
		return &ProjectConfig{}
	}

	return &ProjectConfig{
		ProjectScopedAccount:  pickScopedAccount(cfg.IdentityConfig, useStagingEmail),
		BillingCloudProjectID: cfg.BillingConfig.GetBillingCloudProjectId(),
	}
}

// pickScopedAccount chooses the correct project account for the environment.
func pickScopedAccount(idc *configpb.IdentityConfig, useStagingEmail bool) string {
	prod := idc.GetServiceAccountEmail()
	staging := idc.GetStagingServiceAccountEmail()
	if staging == "" {
		staging = prod
	}
	if useStagingEmail {
		return staging
	}
	return prod
}
