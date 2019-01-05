// Copyright 2019 The LUCI Authors.
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

package projectscope

import (
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/policy"
)

// validateConfigBundle validates the structure of a config bundle fetched by
// fetchConfigs.
func validateConfigBundle(ctx *validation.Context, bundle policy.ConfigBundle) {
	ctx.SetFile(projectScopedServiceAccountsCfg)
	cfg, ok := bundle[projectScopedServiceAccountsCfg].(*admin.ProjectScopedServiceAccounts)
	if ok {
		validateProjectScopedServiceAccountsCfg(ctx, cfg)
	} else {
		ctx.Errorf("unexpectedly wrong proto type %T", cfg)
	}
}

// validateProjectScopedServiceAccountCfg checks deserialized service_accounts.cfg.
func validateProjectScopedServiceAccountsCfg(ctx *validation.Context, cfg *admin.ProjectScopedServiceAccounts) {
	association := map[string]string{} // service account -> rule name where its defined
	for _, scopedService := range cfg.Services {
		if _, found := association[scopedService.Service]; found {
			ctx.Errorf("two rules with identical name")
		}
		association[scopedService.Service] = scopedService.CloudProjectId
		validateScopedService(ctx, scopedService.Service, scopedService)
	}
}

// validateScopedService checks single ServiceConfig proto.
func validateScopedService(ctx *validation.Context, title string, r *admin.ServiceConfig) {
	ctx.Enter(title)
	defer ctx.Exit()

	validateServiceName(ctx, r.Service)
	validateProjectID(ctx, r.CloudProjectId)
	validateMaxGrantValidityDuration(ctx, r.MaxValidityDuration)
}

func validateServiceName(ctx *validation.Context, service string) {
	if service == "" {
		ctx.Errorf(`"service" is required`)
	}
}

func validateProjectID(ctx *validation.Context, projectID string) {
	if projectID == "" {
		ctx.Errorf(`"cloud_project_id" is required`)
	}
}

func validateMaxGrantValidityDuration(ctx *validation.Context, dur int64) {
	switch {
	case dur == 0:
		// valid
	case dur < 0:
		ctx.Errorf(`"max_grant_validity_duration" must be positive`)
	case dur > maxAllowedMaxGrantValidityDuration:
		ctx.Errorf(`"max_grant_validity_duration" must not exceed %d`, maxAllowedMaxGrantValidityDuration)
	}
}
