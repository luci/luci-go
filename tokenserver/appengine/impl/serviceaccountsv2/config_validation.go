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
	// TODO(vadimsh): Implement.
}
