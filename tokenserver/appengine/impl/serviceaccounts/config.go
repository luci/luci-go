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

	"github.com/luci/luci-go/common/config/validation"
	"github.com/luci/luci-go/common/data/stringset"

	"github.com/luci/luci-go/tokenserver/api/admin/v1"
	"github.com/luci/luci-go/tokenserver/appengine/impl/utils/identityset"
	"github.com/luci/luci-go/tokenserver/appengine/impl/utils/policy"
)

// serviceAccountsCfg is name of the config file with the policy.
//
// Also used as a name for the imported configs in the datastore, so change it
// very carefully.
const serviceAccountsCfg = "service_accounts.cfg"

const (
	// defaultMaxGrantValidityDuration is value for max_grant_validity_duration if
	// it isn't specified in the config.
	defaultMaxGrantValidityDuration = 24 * 3600

	// maxAllowedMaxGrantValidityDuration is maximal allowed value for
	// max_grant_validity_duration in service_accounts.cfg.
	maxAllowedMaxGrantValidityDuration = 7 * 24 * 3600
)

// Rules is queryable representation of service_accounts.cfg rules.
type Rules struct {
	revision string           // config revision this policy is imported from
	rules    map[string]*Rule // service account email -> rule for it
}

// Rule is queriable in-memory representation of ServiceAccountRule.
//
// It should be treated like read-only object. It is shared by many concurrent
// requests.
type Rule struct {
	Rule          *admin.ServiceAccountRule // original proto with the rule
	AllowedScopes stringset.Set             // parsed 'allowed_scope'
	EndUsers      *identityset.Set          // parsed 'end_user'
	Proxies       *identityset.Set          // parsed 'proxy'
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

	// Note: per policy.Policy API the config here was already validated when it
	// was imported, but we double check core assumptions anyway. This check may
	// fail if new code (with some new validation rules) uses old configs stored
	// in the datastore (which were validated by old code). In practice this most
	// certainly never happens.
	rules := map[string]*Rule{}
	for _, ruleProto := range parsed.Rules {
		r, err := makeRule(ruleProto)
		if err != nil {
			return nil, err
		}
		for _, account := range ruleProto.ServiceAccount {
			if rules[account] != nil {
				return nil, fmt.Errorf("two rules for service account %q", account)
			}
			rules[account] = r
		}
	}

	return &Rules{
		revision: revision,
		rules:    rules,
	}, nil
}

// ConfigRevision is part of policy.Queryable interface.
func (r *Rules) ConfigRevision() string {
	return r.revision
}

// Rule returns a rule governing the access to the given service account.
//
// Returns nil if such service account is not specified in the config.
func (r *Rules) Rule(serviceAccount string) *Rule {
	return r.rules[serviceAccount]
}

// makeRule converts ServiceAccountRule into queriable Rule.
//
// Mutates 'ruleProto' in-place filling in defaults.
func makeRule(ruleProto *admin.ServiceAccountRule) (*Rule, error) {
	v := validation.Context{}
	validateRule(ruleProto.Name, ruleProto, &v)
	if err := v.Finalize(); err != nil {
		return nil, err
	}

	allowedScopes := stringset.New(len(ruleProto.AllowedScope))
	for _, scope := range ruleProto.AllowedScope {
		allowedScopes.Add(scope)
	}

	endUsers, err := identityset.FromStrings(ruleProto.EndUser, nil)
	if err != nil {
		return nil, fmt.Errorf("bad 'end_user' set - %s", err)
	}

	proxies, err := identityset.FromStrings(ruleProto.Proxy, nil)
	if err != nil {
		return nil, fmt.Errorf("bad 'proxy' set - %s", err)
	}

	if ruleProto.MaxGrantValidityDuration == 0 {
		ruleProto.MaxGrantValidityDuration = defaultMaxGrantValidityDuration
	}

	return &Rule{
		Rule:          ruleProto,
		AllowedScopes: allowedScopes,
		EndUsers:      endUsers,
		Proxies:       proxies,
	}, nil
}
