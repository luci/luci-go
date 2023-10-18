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
	"sort"
	"strings"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/proto/config"
	"go.chromium.org/luci/config/validation"
)

const (
	projectsCfg = "projects.cfg"
)

func validateHasIdentityConfig(ctx *validation.Context, project *config.Project) (*config.IdentityConfig, bool) {
	if project.IdentityConfig != nil && project.IdentityConfig.ServiceAccountEmail != "" {
		return project.IdentityConfig, true
	}
	return nil, false
}

func validateProjectsCfg(ctx *validation.Context, cfg *config.ProjectsCfg) {
	validateSingleIdentityProjectAssignment(ctx, cfg)
}

func validateSingleIdentityProjectAssignment(ctx *validation.Context, cfg *config.ProjectsCfg) {
	ctx.Enter("identity configuration")
	defer ctx.Exit()

	// Service account email => list of teams that use it.
	idents := map[string]stringset.Set{}

	for _, project := range cfg.Projects {
		ctx.Enter("Validate project %s IdentityConfig", project.Id)
		// Check whether valid identity config is present, otherwise skip the project
		// TODO(fmatenaar): Enforce valid identity config once migration towards
		//  project identities is completed by all customers.
		if identcfg, valid := validateHasIdentityConfig(ctx, project); valid {
			validateCanIssueTokenForIdentity(ctx, identcfg.ServiceAccountEmail)
			if set := idents[identcfg.ServiceAccountEmail]; set == nil {
				idents[identcfg.ServiceAccountEmail] = stringset.NewFromSlice(project.OwnedBy)
			} else {
				set.Add(project.OwnedBy)
			}
		}
		ctx.Exit()
	}

	// Error when projects share identities with different project owners.
	var shared []string
	for ident, projs := range idents {
		if len(projs) > 1 {
			shared = append(shared, ident)
		}
	}
	sort.Strings(shared)
	for _, ident := range shared {
		ctx.Errorf(
			"project-scoped account %s is used by multiple teams: %s",
			ident, strings.Join(idents[ident].ToSortedSlice(), ", "))
	}
}

func validateCanIssueTokenForIdentity(ctx *validation.Context, identity string) {
	// TODO(fmatenaar): Issue a token for the identity with minimum validity to check tokenserver access.
	/*
		Opinion:
		This actually will require some careful approach:
			1. We probably do not want to mint tokens for all projects on all validation calls (this is a lot of RPCs). Only for accounts we haven't seen before. So we may need some cache.
			2. If the account had invalid ACLs, and they were fixed, there's currently no way to retrigger the validation. LUCI Config uses contents of the config as sole input. If config body didn't change, it would think the config is still broken. The workaround is either a whitespace change in the config, or manually "Reimport config" button in https://config.luci.app UI.

		Opinion:
		this is one of my use-cases for validation context warning instead of Error.

			config may eventually be correct if some other system's state change,
			=> accept config, but tell user about problems.
	*/
}
