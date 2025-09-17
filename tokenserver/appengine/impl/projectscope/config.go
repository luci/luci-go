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

const projectsCfg = "projects.cfg"

// SetupConfigValidation registers the tokenserver custom projects.cfg validator.
func SetupConfigValidation(rules *validation.RuleSet, useStagingEmail bool) {
	rules.Add("services/${config_service_appid}", projectsCfg, func(ctx *validation.Context, configSet, path string, content []byte) error {
		ctx.SetFile(projectsCfg)
		cfg := &config.ProjectsCfg{}
		if err := prototext.Unmarshal(content, cfg); err != nil {
			ctx.Errorf("not a valid ProjectsCfg proto message - %s", err)
		} else {
			validateProjectsCfg(ctx, cfg, useStagingEmail)
		}
		return nil
	})
}

// importIdentities analyzes projects.cfg to import or update project scoped service accounts.
func importIdentities(ctx context.Context, cfg *config.ProjectsCfg, useStagingEmail bool) error {
	storage := projectidentity.ProjectIdentities(ctx)

	// TODO: Remove entries for projects no longer listed in cfg.Projects.
	var merr errors.MultiError
	for _, project := range cfg.Projects {
		var err error
		if email := projectIdentityEmail(project.IdentityConfig, useStagingEmail); email != "" {
			err = storage.Update(ctx, &projectidentity.ProjectIdentity{
				Project: project.Id,
				Email:   email,
			})
		} else {
			err = storage.Delete(ctx, project.Id)
		}
		if err != nil {
			logging.Errorf(ctx, "Updating project scoped identity for %q: %v", project.Id, err)
			merr = append(merr, err)
		}
	}
	return merr.AsError()
}

// projectIdentityEmail returns an email representing a project or "" if
// unconfigured.
func projectIdentityEmail(cfg *config.IdentityConfig, useStagingEmail bool) string {
	prod := cfg.GetServiceAccountEmail()
	staging := cfg.GetStagingServiceAccountEmail()
	if staging == "" {
		staging = prod
	}
	if useStagingEmail {
		return staging
	}
	return prod
}

// fetchConfigs loads proto messages with rules from the config.
func fetchConfigs(ctx context.Context) (*config.ProjectsCfg, string, error) {
	cfg := &config.ProjectsCfg{}
	var meta configset.Meta
	if err := cfgclient.Get(ctx, "services/${config_service_appid}", projectsCfg, cfgclient.ProtoText(cfg), &meta); err != nil {
		return nil, "", err
	}
	return cfg, meta.Revision, nil
}

// ImportConfigs fetches projects.cfg and updates datastore copy of it.
//
// Called from cron.
func ImportConfigs(ctx context.Context, useStagingEmail bool) (string, error) {
	cfg, rev, err := fetchConfigs(ctx)
	if err != nil {
		return "", errors.Fmt("failed to fetch project configs: %w", err)
	}
	if err := importIdentities(ctx, cfg, useStagingEmail); err != nil {
		return "", errors.Fmt("failed to import project configs: %w", err)
	}
	return rev, nil
}
