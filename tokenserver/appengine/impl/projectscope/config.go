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

package projectscope

import (
	"context"
	"fmt"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/config/validation"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/policy"
)

// projectScopedServiceAccountsCfg is name of the config file with the policy.
//
// Also used as a name for the imported configs in the datastore, so change it
// very carefully.
const projectScopedServiceAccountsCfg = "project_scoped_service_accounts.cfg"

const (
	// defaultMaxGrantValidityDuration is value for max_grant_validity_duration if
	// it isn't specified in the config.
	defaultMaxGrantValidityDuration = 3600

	// maxAllowedMaxGrantValidityDuration is maximal allowed value for
	// max_grant_validity_duration in service_accounts.cfg.
	maxAllowedMaxGrantValidityDuration = 3600
)

var (
	// errGenericDenied is returned to unrecognized callers to indicate they don't
	// have access.
	errGenericDenied = status.Errorf(codes.PermissionDenied, "unknown service account or not enough permissions to use it")

	// errGenericInternal is returned on internal errors if we still don't know
	// whether the caller is trusted to see more details or not.
	errGenericInternal = status.Errorf(codes.Internal, "internal error when querying rules, see logs")
)

// Rules is queryable representation of project_scoped_service_accounts.cfg rules.
type Rules struct {
	revision        string           // config revision this policy is imported from
	rulesPerService map[string]*Rule // service -> rule for it
}

// Rule is queryable in-memory representation of ServiceConfig.
//
// It should be treated like read-only object. It is shared by many concurrent
// requests.
type Rule struct {
	CloudProjectID string
	Rule           *admin.ServiceConfig // original proto with the rule
	Revision       string               // revision of the file with the rule
}

// RulesQuery describes circumstances of using some service account.
//
// Passed to 'Check'.
type RulesQuery struct {
	Service string // name of the service
	Rule    *Rule  // the matching rule, if already known
}

// RulesCache is a stateful object with parsed project_scoped_service_accounts.cfg rules.
//
// It uses policy.Policy internally to manage datastore-cached copy of imported
// project scoped service accounts configs.
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
			Name:     projectScopedServiceAccountsCfg, // used as part of datastore keys
			Fetch:    fetchConfigs,                    // see below
			Validate: validateConfigBundle,            // see config_validation.go
			Prepare:  prepareRules,                    // see below
		},
	}
}

// ImportConfigs refetches service_accounts.cfg and updates the datastore copy.
//
// Called from cron.
func (rc *RulesCache) ImportConfigs(c context.Context) (rev string, err error) {
	return rc.policy.ImportConfigs(c)
}

// SetupConfigValidation registers the config validation rules.
func (rc *RulesCache) SetupConfigValidation(rules *validation.RuleSet) {
	rules.Add("services/${appid}", projectScopedServiceAccountsCfg, func(ctx *validation.Context, configSet, path string, content []byte) error {
		cfg := &admin.ProjectScopedServiceAccounts{}
		if err := proto.UnmarshalText(string(content), cfg); err != nil {
			ctx.Errorf("not a valid ServiceAccountsPermissions proto message - %s", err)
		} else {
			validateProjectScopedServiceAccountsCfg(ctx, cfg)
		}
		return nil
	})
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
	cfg := &admin.ProjectScopedServiceAccounts{}
	if err := f.FetchTextProto(c, projectScopedServiceAccountsCfg, cfg); err != nil {
		return nil, err
	}
	return policy.ConfigBundle{projectScopedServiceAccountsCfg: cfg}, nil
}

// prepareRules converts validated configs into *Rules.
//
// Returns them as policy.Queryable object to satisfy policy.Policy API.
func prepareRules(c context.Context, cfg policy.ConfigBundle, revision string) (policy.Queryable, error) {
	parsed, ok := cfg[projectScopedServiceAccountsCfg].(*admin.ProjectScopedServiceAccounts)
	if !ok {
		return nil, fmt.Errorf("wrong type of %s - %T", projectScopedServiceAccountsCfg, cfg[projectScopedServiceAccountsCfg])
	}

	rulesPerService := map[string]*Rule{}
	for _, association := range parsed.Services {
		rule := &Rule{
			CloudProjectID: association.CloudProjectId,
			Rule:           association,
			Revision:       revision,
		}
		rulesPerService[association.Service] = rule
	}

	return &Rules{
		revision:        revision,
		rulesPerService: rulesPerService,
	}, nil
}

// ConfigRevision is part of policy.Queryable interface.
func (r *Rules) ConfigRevision() string {
	return r.revision
}

// MatchingRules returns all rules (zero or more, sorted by name) that
// apply to the given service.
//
// Returns an error if the service is not found.
func (r *Rules) MatchingRules(c context.Context, service string) ([]*Rule, error) {
	rules, found := r.rulesPerService[service]
	if !found {
		return nil, errors.Annotate(fmt.Errorf("service not found"), "failed to find service rules for %s", service).Err()
	}
	return []*Rule{rules}, nil
}

// makeRule converts ServiceAccountRule into queriable Rule.
//
// Mutates 'ruleProto' in-place filling in defaults.
func makeRule(c context.Context, ruleProto *admin.ServiceConfig, rev string) (*Rule, error) {
	ctx := &validation.Context{Context: c}
	validateScopedService(ctx, ruleProto.Service, ruleProto)
	if err := ctx.Finalize(); err != nil {
		return nil, err
	}

	return &Rule{
		Rule:     ruleProto,
		Revision: rev,
	}, nil
}
