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
	"context"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/config"
	configset "go.chromium.org/luci/config"
	"go.chromium.org/luci/config/cfgclient"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectidentity"
)

// SetupConfigValidation registers the tokenserver custom projects.cfg validator.
func SetupConfigValidation(rules *validation.RuleSet) {
	rules.Add("services/${config_service_appid}", projectsCfg, func(ctx *validation.Context, configSet, path string, content []byte) error {
		ctx.SetFile(projectsCfg)
		cfg := &config.ProjectsCfg{}
		if err := prototext.Unmarshal(content, cfg); err != nil {
			ctx.Errorf("not a valid ProjectsCfg proto message - %s", err)
		} else {
			validateProjectsCfg(ctx, cfg)
		}
		return nil
	})
}

// importIdentities analyzes projects.cfg to import or update project scoped service accounts.
func importIdentities(c context.Context, cfg *config.ProjectsCfg) error {
	storage := projectidentity.ProjectIdentities(c)

	// TODO (fmatenaar): Make this transactional and provide some guarantees around cleanup
	// but do this after we have a stronger story for warning about config changes which are
	// about to remove an identity config from a project since this can cause an outage.
	for _, project := range cfg.Projects {
		identity := &projectidentity.ProjectIdentity{
			Project: project.Id,
		}
		if project.IdentityConfig != nil && project.IdentityConfig.ServiceAccountEmail != "" {
			identity.Email = project.IdentityConfig.ServiceAccountEmail
			logging.Infof(c, "updating project scoped account: %v", identity)
			if _, err := storage.Update(c, identity); err != nil {
				logging.Errorf(c, "failed to update project scoped account: %v", identity)
				return err
			}
		} else {
			logging.Warningf(c, "removing project scoped account: %v", identity)
			if err := storage.Delete(c, identity); err != nil {
				logging.Errorf(c, "failed to remove project scoped account: %v", identity)
			}
		}
	}
	return nil
}

// fetchConfigs loads proto messages with rules from the config.
func fetchConfigs(c context.Context) (*config.ProjectsCfg, string, error) {
	cfg := &config.ProjectsCfg{}
	var meta configset.Meta
	if err := cfgclient.Get(c, "services/${config_service_appid}", projectsCfg, cfgclient.ProtoText(cfg), &meta); err != nil {
		return nil, "", err
	}
	return cfg, meta.Revision, nil
}

// ImportConfigs fetches projects.cfg and updates datastore copy of it.
//
// Called from cron.
func ImportConfigs(c context.Context) (string, error) {
	cfg, rev, err := fetchConfigs(c)
	if err != nil {
		return "", errors.Fmt("failed to fetch project configs: %w", err)
	}
	if err := importIdentities(c, cfg); err != nil {
		return "", errors.Fmt("failed to import project configs: %w", err)
	}
	return rev, nil
}
