// Copyright 2022 The LUCI Authors.
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

package validation

import (
	"context"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config/validation"
)

func init() {
	validation.Rules.Add("services/${appid}", "ip_allowlist.cfg", validateAllowlist)
	validation.Rules.Add("services/${appid}", "imports.cfg", validateImportsCfg)
	validation.Rules.Add("services/${appid}", "oauth.cfg", validateOAuth)
	validation.Rules.Add("services/${appid}", "security.cfg", validateSecurityCfg)
	validation.Rules.Add("services/${appid}", "settings.cfg", validateSettingsCfg)
	validation.Rules.Add("services/${appid}", "permissions.cfg", validatePermissionsCfg)
}

// RegisterRealmsCfgValidation adds realms config file validation based on the
// environment (i.e. development vs production).
func RegisterRealmsCfgValidation(ctx context.Context) {
	path := GetRealmsCfgPath(ctx)
	logging.Infof(ctx, "registering %q validation...", path)

	validation.Rules.Add("services/${appid}", path, validateServiceRealmsCfg)
	validation.Rules.Add("regex:projects/.*", path, validateProjectRealmsCfg)
}
