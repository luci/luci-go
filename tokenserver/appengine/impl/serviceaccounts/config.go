// Copyright 2017 The LUCI Authors.
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
	"fmt"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/tokenserver/api/admin/v1"
	"github.com/luci/luci-go/tokenserver/appengine/impl/utils/policy"
)

// serviceAccountsCfg is name of the config file with the policy.
//
// Also used as a name for the imported configs in the datastore, so change it
// very carefully.
const serviceAccountsCfg = "service_accounts.cfg"

// Rules is queryable representation of service_accounts.cfg rules.
type Rules struct {
	revision string                 // config revision this policy is imported from
	rules    map[string]*parsedRule // service account email -> rule for it
}

// parsedRule is queriable in-memory representation of ServiceAccountRule.
type parsedRule struct {
	// TODO(vadimsh): Implement.
}

// RulesCache is a stateful object with parsed service_accounts.cfg rules.
//
// It uses policy.Policy internally to manage datastore-cached copy of imported
// service accounts configs.
//
// Use NewRulesCache() to create a new instance. Each instance owns its own
// in-memory cache, but uses same shared datastore cache.
//
// There's also a process global instance of RulesCache (GlobalRulesCache var)
// which is used by the main process. Unit tests don't use it though to avoid
// relying on shared state.
type RulesCache struct {
	policy policy.Policy // holds cached *Rules
}

// GlobalRulesCache is the process-wide rules cache.
var GlobalRulesCache = NewRulesCache()

// NewRulesCache properly initializes RulesCache instance.
func NewRulesCache() *RulesCache {
	return &RulesCache{
		policy: policy.Policy{
			Name:     serviceAccountsCfg, // used as part of datastore keys
			Fetch:    fetchConfigs,       // see below
			Validate: validateConfigs,    // see config_validation.go
			Prepare:  prepareRules,       // see below
		},
	}
}

// ImportConfigs refetches service_accounts.cfg and updates the datastore copy.
//
// Called from cron.
func (rc *RulesCache) ImportConfigs(c context.Context) (rev string, err error) {
	return rc.policy.ImportConfigs(c)
}

// Rules returns in-memory copy of service accounts rules, ready for querying.
func (rc *RulesCache) Rules(c context.Context) (*Rules, error) {
	q, err := rc.policy.Queryable(c)
	if err != nil {
		return nil, err
	}
	return q.(*Rules), nil
}

// fetchConfigs loads proto messages with rules from the config.
func fetchConfigs(c context.Context, f policy.ConfigFetcher) (policy.ConfigBundle, error) {
	cfg := &admin.ServiceAccountsPermissions{}
	if err := f.FetchTextProto(c, serviceAccountsCfg, cfg); err != nil {
		return nil, err
	}
	return policy.ConfigBundle{serviceAccountsCfg: cfg}, nil
}

// prepareRules converts validated configs into *Rules.
//
// Returns them as policy.Queryable object to satisfy policy.Policy API.
func prepareRules(cfg policy.ConfigBundle, revision string) (policy.Queryable, error) {
	parsed, ok := cfg[serviceAccountsCfg].(*admin.ServiceAccountsPermissions)
	if !ok {
		return nil, fmt.Errorf("wrong type of %s - %T", serviceAccountsCfg, cfg[serviceAccountsCfg])
	}
	// TODO(vadimsh): Convert parsed.Rules into map[string]*parsedRule.
	_ = parsed
	return &Rules{
		revision: revision,
	}, nil
}

// ConfigRevision is part of policy.Queryable interface.
func (r *Rules) ConfigRevision() string {
	return r.revision
}

// TODO(vadimsh): Implement rest of Rules.
