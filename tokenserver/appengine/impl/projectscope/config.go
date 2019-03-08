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
	"fmt"

	"go.chromium.org/luci/config/appengine/gaeconfig"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/config"
	configset "go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/tokenserver/appengine/impl/utils/policy"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectidentity"
)

// ProjectsConfigImporter parses and imports IdentityConfig objects from projects.cfg.
type ProjectsConfigImporter struct {
}

// GlobalProjectsConfigImporter should be used to trigger projects.cfg imports.
var GlobalProjectsConfigImporter = NewRulesCache()

// NewRulesCache properly initializes IdentityCache instance.
func NewRulesCache() *ProjectsConfigImporter {
	return &ProjectsConfigImporter{}
}

// SetupConfigValidation registers the tokenserver custom projects.cfg validator.
func (rc *ProjectsConfigImporter) SetupConfigValidation(rules *validation.RuleSet) {
	rules.Add("services/${config_service_appid}", projectsCfg, func(ctx *validation.Context, configSet, path string, content []byte) error {
		ctx.SetFile(projectsCfg)
		cfg := &config.ProjectsCfg{}
		if err := proto.UnmarshalText(string(content), cfg); err != nil {
			ctx.Errorf("not a valid ProjectsCfg proto message - %s", err)
		} else {
			validateProjectsCfg(ctx, cfg)
		}
		return nil
	})
}

// importIdentities analyzes projects.cfg to import or update project scoped service accounts.
func (rc *ProjectsConfigImporter) importIdentities(c context.Context, cfg policy.ConfigBundle, revision string) error {
	projects, ok := cfg[projectsCfg].(*config.ProjectsCfg)
	if !ok {
		return fmt.Errorf("wrong type of projects.cfg - %T", cfg[projectsCfg])
	}

	storage := projectidentity.ProjectIdentities(c)
	for _, project := range projects.Projects {
		if project.IdentityConfig != nil && project.IdentityConfig.ServiceAccountEmail != "" {
			identity := &projectidentity.ProjectIdentity{
				Project: project.Id,
				Email:   project.IdentityConfig.ServiceAccountEmail,
			}
			logging.Infof(c, "updating project scoped account: %v", identity)
			storage.Update(c, identity)
		}
	}
	return nil
}

// fetchConfigs loads proto messages with rules from the config.
func (rc *ProjectsConfigImporter) fetchConfigs(c context.Context, f policy.ConfigFetcher) (policy.ConfigBundle, error) {
	// TODO(fmatenaar): Refactor config validation to offer rendering here.
	// Because projects.cfg is stored part of luci config service and
	// rendering is not supported outside of validation, we need
	// to manually resolve the config service name to fetch
	// project.cfg proto
	configServiceAppID, err := gaeconfig.GetConfigServiceAppID(c)
	if err != nil {
		return nil, err
	}
	configSet := fmt.Sprintf("services/%s", configServiceAppID)

	cfg := &config.ProjectsCfg{}
	if err := f.FetchTextProtoFromService(c, configset.Set(configSet), projectsCfg, cfg); err != nil {
		return nil, err
	}
	return policy.ConfigBundle{projectsCfg: cfg}, nil
}

// ImportConfigs refetches projects.cfg and updates datastore copy of it.
//
// Called from cron.
func (rc *ProjectsConfigImporter) ImportConfigs(c context.Context) (rev string, err error) {
	fetcher := policy.NewLuciConfigFetcher()
	bundle, err := rc.fetchConfigs(c, fetcher)
	if err == nil && len(bundle) == 0 {
		err = errors.New("no configs fetched by the callback")
	}
	if err != nil {
		return "", errors.Annotate(err, "failed to fetch policy configs").Err()
	}
	rev = fetcher.Revision(c)
	if err := rc.importIdentities(c, bundle, rev); err != nil {
		return "", errors.Annotate(err, "failed to import policy configs").Err()
	}
	return rev, nil
}
