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

package serviceaccounts

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/policy"
)

const configFileName = "project_owned_accounts.cfg"

// Mapping is a queryable representation of project_owned_accounts.cfg.
type Mapping struct {
	revision         string                          // config revision this policy is imported from
	pairs            map[projectAccountPair]struct{} // allowed (project, account) pairs
	useProjectScoped stringset.Set                   // LUCI projects opted-in into using project-scoped accounts for minting tokens
}

type projectAccountPair struct {
	project string // e.g. "chromium"
	account string // e.g. "ci-builder@..."
}

// UseProjectScopedAccount returns true if the token server should use
// project-scoped accounts when minting tokens in context of the given LUCI
// project.
func (m *Mapping) UseProjectScopedAccount(project string) bool {
	return m.useProjectScoped.Has(project)
}

// CanProjectUseAccount returns true if the given project is allowed to mint
// tokens of the given service account.
//
// The project name is extracted from a realm name and it can be "@internal"
// for internal realms.
func (m *Mapping) CanProjectUseAccount(project, account string) bool {
	_, yes := m.pairs[projectAccountPair{project, account}]
	return yes
}

// ConfigRevision is part of policy.Queryable interface.
func (m *Mapping) ConfigRevision() string {
	return m.revision
}

// MappingCache is a stateful object with parsed project_owned_accounts.cfg.
//
// It uses policy.Policy internally to manage datastore-cached copy of imported
// service accounts configs.
//
// Use NewMappingCache() to create a new instance. Each instance owns its own
// in-memory cache, but uses the same shared datastore cache.
//
// There's also a process global instance of MappingCache (GlobalMappingCache
// var) which is used by the main process. Unit tests don't use it though to
// avoid relying on shared state.
type MappingCache struct {
	policy policy.Policy // holds cached *Mapping
}

// GlobalMappingCache is the process-wide mapping cache.
var GlobalMappingCache = NewMappingCache()

// NewMappingCache properly initializes MappingCache instance.
func NewMappingCache() *MappingCache {
	return &MappingCache{
		policy: policy.Policy{
			Name:     configFileName,       // used as part of datastore keys
			Fetch:    fetchConfigs,         // see below
			Validate: validateConfigBundle, // see config_validation.go
			Prepare:  prepareMapping,       // see below
		},
	}
}

// ImportConfigs refetches project_owned_accounts.cfg and updates the datastore.
//
// Called from cron.
func (mc *MappingCache) ImportConfigs(ctx context.Context) (rev string, err error) {
	return mc.policy.ImportConfigs(ctx)
}

// SetupConfigValidation registers the config validation rules.
func (mc *MappingCache) SetupConfigValidation(rules *validation.RuleSet) {
	rules.Add("services/${appid}", configFileName, func(ctx *validation.Context, configSet, path string, content []byte) error {
		cfg := &admin.ServiceAccountsProjectMapping{}
		if err := prototext.Unmarshal(content, cfg); err != nil {
			ctx.Errorf("not a valid ServiceAccountsProjectMapping proto message - %s", err)
		} else {
			validateMappingCfg(ctx, cfg)
		}
		return nil
	})
}

// Mapping returns in-memory copy of the mapping, ready for querying.
func (mc *MappingCache) Mapping(ctx context.Context) (*Mapping, error) {
	q, err := mc.policy.Queryable(ctx)
	if err != nil {
		return nil, err
	}
	return q.(*Mapping), nil
}

// fetchConfigs loads proto messages with the mapping from the config.
func fetchConfigs(ctx context.Context, f policy.ConfigFetcher) (policy.ConfigBundle, error) {
	cfg := &admin.ServiceAccountsProjectMapping{}
	if err := f.FetchTextProto(ctx, configFileName, cfg); err != nil {
		return nil, err
	}
	return policy.ConfigBundle{configFileName: cfg}, nil
}

// prepareMapping converts validated configs into *Mapping.
//
// Returns it as a policy.Queryable object to satisfy policy.Policy API.
func prepareMapping(ctx context.Context, cfg policy.ConfigBundle, revision string) (policy.Queryable, error) {
	parsed, ok := cfg[configFileName].(*admin.ServiceAccountsProjectMapping)
	if !ok {
		return nil, fmt.Errorf("wrong type of %s - %T", configFileName, cfg[configFileName])
	}

	pairs := map[projectAccountPair]struct{}{}
	for _, m := range parsed.Mapping {
		for _, project := range m.Project {
			for _, account := range m.ServiceAccount {
				pairs[projectAccountPair{project, account}] = struct{}{}
			}
		}
	}

	return &Mapping{
		revision:         revision,
		pairs:            pairs,
		useProjectScoped: stringset.NewFromSlice(parsed.UseProjectScopedAccount...),
	}, nil
}
