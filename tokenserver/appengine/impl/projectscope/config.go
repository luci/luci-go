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
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/config/appengine/gaeconfig"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/proto/config"
	configset "go.chromium.org/luci/config"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/tokenserver/appengine/impl/utils/policy"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/projectidentity"
)

// Rules is used to indicate the imported revision.
type Rules struct {
	revision string
}

// ConfigRevision is part of policy.Queryable interface.
func (r *Rules) ConfigRevision() string {
	return r.revision
}

// RulesCache is a stateful object with parsed projects.cfg rules.
//
// It uses policy.Policy internally to manage datastore-cached copy of imported
// project -> service accounts mapping.
//
// Use NewRulesCache() to create a new instance. Each instance owns its own
// in-memory cache, but uses same shared datastore cache.
//
// There's also a process global instance of RulesCache (GlobalRulesCache var)
// which is used by the main process. Unit tests don't use it though to avoid
// relying on shared state.
type RulesCache struct {
	policy policy.Policy
}

// GlobalRulesCache is the process-wide rules cache.
var GlobalRulesCache = NewRulesCache()

// NewRulesCache properly initializes IdentityCache instance.
func NewRulesCache() *RulesCache {
	return &RulesCache{
		policy: policy.Policy{
			Name:     projectsCfg,          // used as part of datastore keys
			Fetch:    fetchConfigs,         // see below
			Validate: validateConfigBundle, // see config_validation.go
			Prepare:  importIdentities,     // see below
		},
	}
}

// ImportConfigs refetches delegation.cfg and updates datastore copy of it.
//
// Called from cron.
func (rc *RulesCache) ImportConfigs(c context.Context) (rev string, err error) {
	return rc.policy.ImportConfigs(c)
}

// SetupConfigValidation registers the tokenserver custom projects.cfg validator.
func (rc *RulesCache) SetupConfigValidation(rules *validation.RuleSet) {
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

func importIdentities(c context.Context, cfg policy.ConfigBundle, revision string) (policy.Queryable, error) {
	projects, ok := cfg[projectsCfg].(*config.ProjectsCfg)
	if !ok {
		return nil, fmt.Errorf("wrong type of projects.cfg - %T", cfg[projectsCfg])
	}

	storage := projectidentity.ProjectIdentities(c)
	for _, project := range projects.Projects {
		if project.IdentityConfig != nil && project.IdentityConfig.ServiceAccountEmail != "" {
			identity := &projectidentity.ProjectIdentity{
				Project: project.Id,
				Email:   project.IdentityConfig.ServiceAccountEmail,
			}
			storage.Update(c, identity)
		}
	}

	return &Rules{
		revision: revision,
	}, nil
}

// fetchConfigs loads proto messages with rules from the config.
func fetchConfigs(c context.Context, f policy.ConfigFetcher) (policy.ConfigBundle, error) {
	// TODO(fmatenaar): Refactor config validation to offer rendering here.
	// Because projects.cfg is stored part of luci config service and
	// rendering is not supported outside of validation, we need
	// to manually resolve the config service name to fetch
	// project.cfg proto
	configServiceAppID, err := gaeconfig.GetConfigServiceAppID(c)
	if err != nil {
		return nil, err
	}
	configSet := fmt.Sprintf("services/%s-dev", configServiceAppID)

	logging.Infof(c, "ConfigFetcher: %v", f)
	cfg := &config.ProjectsCfg{}
	if err := f.FetchTextProtoFromService(c, configset.Set(configSet), projectsCfg, cfg); err != nil {
		return nil, err
	}
	return policy.ConfigBundle{projectsCfg: cfg}, nil
}
