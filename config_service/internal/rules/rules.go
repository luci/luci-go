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

// Copyright 2018 The LUCI Authors.
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
	"context"
	"errors"
	"fmt"

	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/service/info"

	"go.chromium.org/luci/config_service/internal/common"
)

func init() {
	addRules(&validation.Rules)
}

func addRules(r *validation.RuleSet) {
	r.Vars.Register("appid", func(ctx context.Context) (string, error) {
		if appid := info.AppID(ctx); appid != "" {
			return appid, nil
		}
		return "", fmt.Errorf("can't resolve ${appid} from context")
	})
	r.Add("exact:services/${appid}", common.ACLRegistryFilePath, validateACLsCfg)
	r.Add("exact:services/${appid}", common.ProjRegistryFilePath, validateProjectsCfg)
	r.Add("exact:services/${appid}", common.ServiceRegistryFilePath, validateServicesCfg)
	r.Add("exact:services/${appid}", common.ImportConfigFilePath, validateImportCfg)
	r.Add("exact:services/${appid}", common.SchemaConfigFilePath, validateSchemaCfg)
	r.Add(`regex:projects/[^/]+`, common.ProjMetadataFilePath, validateProjectMetadata)
	r.Add(`regex:.+`, `regex:.+\.json`, validateJSON)
}

func validateACLsCfg(ctx *validation.Context, configSet, path string, content []byte) error {
	return errors.New("unimplemented")
}

func validateServicesCfg(ctx *validation.Context, configSet, path string, content []byte) error {
	return errors.New("unimplemented")
}

func validateImportCfg(ctx *validation.Context, configSet, path string, content []byte) error {
	return errors.New("unimplemented")
}

func validateSchemaCfg(ctx *validation.Context, configSet, path string, content []byte) error {
	return errors.New("unimplemented")
}

func validateJSON(ctx *validation.Context, configSet, path string, content []byte) error {
	return errors.New("unimplemented")
}
