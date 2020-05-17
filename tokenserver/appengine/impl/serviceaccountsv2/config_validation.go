// Copyright 2020 The LUCI Authors.
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

package serviceaccountsv2

import (
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/policy"
)

// validateConfigBundle validates the structure of a config bundle fetched by
// fetchConfigs.
func validateConfigBundle(ctx *validation.Context, bundle policy.ConfigBundle) {
	ctx.SetFile(configFileName)
	cfg, ok := bundle[configFileName].(*admin.ServiceAccountsProjectMapping)
	if ok {
		validateMappingCfg(ctx, cfg)
	} else {
		ctx.Errorf("unexpectedly wrong proto type %T", cfg)
	}
}

// validateMappingCfg checks deserialized project_owned_accounts.cfg.
func validateMappingCfg(ctx *validation.Context, cfg *admin.ServiceAccountsProjectMapping) {
	seenProjects := stringset.New(0)
	seenAccounts := stringset.New(0)

	for i, m := range cfg.Mapping {
		ctx.Enter("mapping #%d", i+1)

		// A mapping{...} entry with no projects makes no sense.
		if len(m.Project) == 0 {
			ctx.Errorf("at least one project must be given")
		}
		for _, project := range m.Project {
			if _, err := identity.MakeIdentity("project:" + project); err != nil {
				ctx.Errorf("bad project %q: %s", project, err)
			} else if !seenProjects.Add(project) {
				ctx.Errorf("project %q appears in more that one mapping", project)
			}
		}

		// A mapping{...} entry with an empty list of accounts is OK, it may be some
		// yet-to-be-populated entry with a "// TODO" or something.
		for _, account := range m.ServiceAccount {
			if _, err := identity.MakeIdentity("user:" + account); err != nil {
				ctx.Errorf("bad service_account %q: %s", account, err)
			} else if !seenAccounts.Add(account) {
				ctx.Errorf("service_account %q appears in more that one mapping", account)
			}
		}

		ctx.Exit()
	}
}
