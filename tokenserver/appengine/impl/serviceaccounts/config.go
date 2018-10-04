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
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/identityset"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/policy"
)

// serviceAccountsCfg is name of the config file with the policy.
//
// Also used as a name for the imported configs in the datastore, so change it
// very carefully.
const serviceAccountsCfg = "service_accounts.cfg"

const (
	// defaultMaxGrantValidityDuration is value for max_grant_validity_duration if
	// it isn't specified in the config.
	defaultMaxGrantValidityDuration = 48 * 3600

	// maxAllowedMaxGrantValidityDuration is maximal allowed value for
	// max_grant_validity_duration in service_accounts.cfg.
	maxAllowedMaxGrantValidityDuration = 7 * 24 * 3600
)

var (
	// errGenericDenied is returned to unrecognized callers to indicate they don't
	// have access.
	errGenericDenied = status.Errorf(codes.PermissionDenied, "unknown service account or not enough permissions to use it")

	// errGenericInternal is returned on internal errors if we still don't know
	// whether the caller is trusted to see more details or not.
	errGenericInternal = status.Errorf(codes.Internal, "internal error when querying rules, see logs")
)

// Rules is queryable representation of service_accounts.cfg rules.
type Rules struct {
	revision      string           // config revision this policy is imported from
	rulesPerAcc   map[string]*Rule // service account email -> rule for it
	rulesPerGroup map[string]*Rule // group with accounts -> rule for it
	groups        []string         // list of keys in 'rulesPerGroup'
}

// Rule is queriable in-memory representation of ServiceAccountRule.
//
// It should be treated like read-only object. It is shared by many concurrent
// requests.
type Rule struct {
	Rule           *admin.ServiceAccountRule // original proto with the rule
	Revision       string                    // revision of the file with the rule
	AllowedScopes  stringset.Set             // parsed 'allowed_scope'
	EndUsers       *identityset.Set          // parsed 'end_user'
	Proxies        *identityset.Set          // parsed 'proxy'
	TrustedProxies *identityset.Set          // parsed 'trusted_proxy'
	AllProxies     *identityset.Set          // union of 'proxy' and 'trusted_proxy'
}

// RulesQuery describes circumstances of using some service account.
//
// Passed to 'Check'.
type RulesQuery struct {
	ServiceAccount string            // email of an account being used
	Rule           *Rule             // the matching rule, if already known
	Proxy          identity.Identity // who's calling the Token Server
	EndUser        identity.Identity // who initiates the usage of an account
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
			Name:     serviceAccountsCfg,   // used as part of datastore keys
			Fetch:    fetchConfigs,         // see below
			Validate: validateConfigBundle, // see config_validation.go
			Prepare:  prepareRules,         // see below
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
	rules.Add("services/${appid}", serviceAccountsCfg, func(ctx *validation.Context, configSet, path string, content []byte) error {
		cfg := &admin.ServiceAccountsPermissions{}
		if err := proto.UnmarshalText(string(content), cfg); err != nil {
			ctx.Errorf("not a valid ServiceAccountsPermissions proto message - %s", err)
		} else {
			validateServiceAccountsCfg(ctx, cfg)
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
	cfg := &admin.ServiceAccountsPermissions{}
	if err := f.FetchTextProto(c, serviceAccountsCfg, cfg); err != nil {
		return nil, err
	}
	return policy.ConfigBundle{serviceAccountsCfg: cfg}, nil
}

// prepareRules converts validated configs into *Rules.
//
// Returns them as policy.Queryable object to satisfy policy.Policy API.
func prepareRules(c context.Context, cfg policy.ConfigBundle, revision string) (policy.Queryable, error) {
	parsed, ok := cfg[serviceAccountsCfg].(*admin.ServiceAccountsPermissions)
	if !ok {
		return nil, fmt.Errorf("wrong type of %s - %T", serviceAccountsCfg, cfg[serviceAccountsCfg])
	}

	// Grab defaults from the config or construct on the fly.
	defaults := admin.ServiceAccountRuleDefaults{}
	if parsed.Defaults != nil {
		defaults = *parsed.Defaults
	}
	if defaults.MaxGrantValidityDuration == 0 {
		defaults.MaxGrantValidityDuration = defaultMaxGrantValidityDuration
	}

	// Note: per policy.Policy API the config here was already validated when it
	// was imported, but we double check core assumptions anyway. This check may
	// fail if new code (with some new validation rules) uses old configs stored
	// in the datastore (which were validated by old code). In practice this most
	// certainly never happens.
	ctx := &validation.Context{Context: c}
	validateDefaults(ctx, "defaults", &defaults)
	if err := ctx.Finalize(); err != nil {
		return nil, err
	}

	rulesPerAcc := map[string]*Rule{}
	rulesPerGroup := map[string]*Rule{}
	for _, ruleProto := range parsed.Rules {
		r, err := makeRule(c, ruleProto, &defaults, revision)
		if err != nil {
			return nil, err
		}
		for _, account := range ruleProto.ServiceAccount {
			if rulesPerAcc[account] != nil {
				return nil, fmt.Errorf("two rules for service account %q", account)
			}
			rulesPerAcc[account] = r
		}
		for _, group := range ruleProto.ServiceAccountGroup {
			if rulesPerGroup[group] != nil {
				return nil, fmt.Errorf("two rules for service account group %q", group)
			}
			rulesPerGroup[group] = r
		}
	}

	groups := make([]string, 0, len(rulesPerGroup))
	for g := range rulesPerGroup {
		groups = append(groups, g)
	}

	return &Rules{
		revision:      revision,
		rulesPerAcc:   rulesPerAcc,
		rulesPerGroup: rulesPerGroup,
		groups:        groups,
	}, nil
}

// ConfigRevision is part of policy.Queryable interface.
func (r *Rules) ConfigRevision() string {
	return r.revision
}

// MatchingRules returns all rules (zero or more, sorted by name) that
// apply to the given service account.
//
// Returns an error if the group membership check fails.
//
// Relatively heavy operation, try to reuse the result.
func (r *Rules) MatchingRules(c context.Context, serviceAccount string) ([]*Rule, error) {
	// TODO(vadimsh): This will not scale when the number of rules becomes large.
	// At some point we'll have to give up the perfect consistency and start
	// caching the results.

	// Set of rules that match the service account. If the service account matches
	// some rule via multiple different groups, this is NOT an error, since
	// there's no ambiguity in this case.
	matchingRules := make(map[*Rule]struct{}, 1)

	// Query all groups mentioned in the config to figure out which ones the
	// account belongs to and then pick the corresponding rules.
	if len(r.groups) != 0 {
		ident := identity.Identity("user:" + serviceAccount)
		groups, err := auth.GetState(c).DB().CheckMembership(c, ident, r.groups)
		if err != nil {
			return nil, errors.Annotate(err, "failed to check membership of %s", serviceAccount).Err()
		}
		for _, g := range groups {
			matchingRules[r.rulesPerGroup[g]] = struct{}{}
		}
	}

	// Try rules with explicitly mentioned accounts too.
	if rule := r.rulesPerAcc[serviceAccount]; rule != nil {
		matchingRules[rule] = struct{}{}
	}

	// Convert to an array and sort for deterministic error messages.
	out := make([]*Rule, 0, len(matchingRules))
	for rule := range matchingRules {
		out = append(out, rule)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rule.Name < out[j].Rule.Name })
	return out, nil
}

// Rule returns a rule matching the service account or a grpc error.
//
// It uses the given 'proxy' exclusively to decide whether it is okay to put
// detailed error message in the response. Unknown proxies get only vague
// generic reply. It does NOT check whether proxy is allowed to use the rule,
// this should be done by the caller.
//
// If 'proxy' is an empty string, the error message contains all possible
// details (this is used only from admin RPCs).
//
// Always logs detailed errors.
func (r *Rules) Rule(c context.Context, serviceAccount string, proxy identity.Identity) (*Rule, error) {
	rules, err := r.MatchingRules(c, serviceAccount)
	if err != nil {
		logging.WithError(err).Errorf(c, "Failed to query rules for account %q using config rev %s", serviceAccount, r.revision)
		return nil, errGenericInternal
	}

	switch len(rules) {
	case 0:
		logging.Errorf(c, "No rule for service account %q in the config rev %s", serviceAccount, r.revision)
		if proxy == "" {
			return nil, status.Errorf(codes.PermissionDenied, "the service account is not specified in the rules (rev %s)", r.revision)
		}
		return nil, errGenericDenied
	case 1:
		logging.Infof(c, "Found the matching rule %q in the config rev %s", rules[0].Rule.Name, r.revision)
		return rules[0], nil
	}

	// Here we have two or more rules matching the account. This is forbidden.
	// Construct an error message that contains only rule names visible to the
	// caller ('proxy'), if any. Log all rules to the server log.
	allRules := make([]string, 0, len(rules))
	visibleRules := make([]string, 0, len(rules))
	for _, rule := range rules {
		visible := proxy == ""
		if !visible {
			var err error
			if visible, err = rule.AllProxies.IsMember(c, proxy); err != nil {
				logging.WithError(err).Errorf(c, "Failed to check membership of proxy %q in rule %q", proxy, rule.Rule.Name)
				return nil, errGenericInternal
			}
		}
		name := fmt.Sprintf("%q", rule.Rule.Name)
		allRules = append(allRules, name)
		if visible {
			visibleRules = append(visibleRules, name)
		}
	}

	// Always log all details to the server log.
	logging.Errorf(c, "Service account %q matches %d rules in the config rev %s: %s",
		serviceAccount, len(rules), r.revision, strings.Join(allRules, ", "))

	// Totally unknown proxies see no details.
	if len(visibleRules) == 0 {
		return nil, errGenericDenied
	}

	// Show names of visible rules in the error message.
	msg := fmt.Sprintf("service account %q matches %d rules in the config rev %s: %s",
		serviceAccount, len(rules), r.revision, strings.Join(visibleRules, ", "))
	if len(visibleRules) != len(rules) {
		msg += fmt.Sprintf(" and %d more", len(rules)-len(visibleRules))
	}
	return nil, status.Errorf(codes.InvalidArgument, msg)
}

// Check checks that rules allow the requested usage.
//
// Returns the corresponding rule on success, or gRPC error on failure.
// The returned rule can be consulted further to check additional restrictions,
// such as allowed OAuth scopes or validity duration.
//
// Note that ambiguities in rules are forbidden: an account must match at most
// one rule. If it matches multiple rules, PermissionDenied error will be
// returned, indicating the account is misconfigured and must not be used until
// the ambiguity is fixed.
//
// Supposed to be called as part of some RPC handler. It logs errors internally,
// so no need to log them outside.
func (r *Rules) Check(c context.Context, query *RulesQuery) (*Rule, error) {
	rule := query.Rule
	if rule == nil {
		if query.Proxy == "" {
			panic("query.Proxy must be already populated here")
		}
		var err error
		if rule, err = r.Rule(c, query.ServiceAccount, query.Proxy); err != nil {
			return nil, err
		}
	}

	// Trusted proxies are allowed to skip the end user check. We trust them to do
	// it themselves.
	switch isTrustedProxy, err := rule.TrustedProxies.IsMember(c, query.Proxy); {
	case err != nil:
		logging.WithError(err).Errorf(c, "Failed to check membership of caller %q", query.Proxy)
		return nil, status.Errorf(codes.Internal, "membership check failed")
	case isTrustedProxy:
		return rule, nil
	}

	switch isProxy, err := rule.Proxies.IsMember(c, query.Proxy); {
	case err != nil:
		logging.WithError(err).Errorf(c, "Failed to check membership of caller %q", query.Proxy)
		return nil, status.Errorf(codes.Internal, "membership check failed")
	case !isProxy:
		// If the 'Proxy' is not in 'Proxies' and 'TrustedProxies' lists, we assume
		// it's a total stranger and keep error messages not very detailed, to avoid
		// leaking internal configuration.
		logging.Errorf(c, "Caller %q is not authorized to use account %q", query.Proxy, query.ServiceAccount)
		return nil, errGenericDenied
	}

	// Here the proxy is in 'Proxies' list, but not in 'TrustedProxies' list.
	// Check that the end user is authorized by the rule.
	switch known, err := rule.EndUsers.IsMember(c, query.EndUser); {
	case err != nil:
		logging.WithError(err).Errorf(c, "Failed to check membership of end user %q", query.EndUser)
		return nil, status.Errorf(codes.Internal, "membership check failed")
	case !known:
		logging.Errorf(c, "End user %q is not authorized to use account %q", query.EndUser, query.ServiceAccount)
		return nil, status.Errorf(
			codes.PermissionDenied, "per rule %q the user %q is not authorized to use the service account %q",
			rule.Rule.Name, query.EndUser, query.ServiceAccount)
	}

	return rule, nil
}

// makeRule converts ServiceAccountRule into queriable Rule.
//
// Mutates 'ruleProto' in-place filling in defaults.
func makeRule(c context.Context, ruleProto *admin.ServiceAccountRule, defaults *admin.ServiceAccountRuleDefaults, rev string) (*Rule, error) {
	ctx := &validation.Context{Context: c}
	validateRule(ctx, ruleProto.Name, ruleProto)
	if err := ctx.Finalize(); err != nil {
		return nil, err
	}

	allowedScopes := stringset.New(len(ruleProto.AllowedScope) + len(defaults.AllowedScope))
	for _, scope := range ruleProto.AllowedScope {
		allowedScopes.Add(scope)
	}
	for _, scope := range defaults.AllowedScope {
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

	trustedProxies, err := identityset.FromStrings(ruleProto.TrustedProxy, nil)
	if err != nil {
		return nil, fmt.Errorf("bad 'trusted_proxy' set - %s", err)
	}

	if ruleProto.MaxGrantValidityDuration == 0 {
		ruleProto.MaxGrantValidityDuration = defaults.MaxGrantValidityDuration
	}

	return &Rule{
		Rule:           ruleProto,
		Revision:       rev,
		AllowedScopes:  allowedScopes,
		EndUsers:       endUsers,
		Proxies:        proxies,
		TrustedProxies: trustedProxies,
		AllProxies:     identityset.Union(proxies, trustedProxies),
	}, nil
}

// CheckScopes returns no errors if all passed scopes are allowed.
func (r *Rule) CheckScopes(scopes []string) error {
	var notAllowed []string
	for _, scope := range scopes {
		if !r.AllowedScopes.Has(scope) {
			notAllowed = append(notAllowed, scope)
		}
	}
	if len(notAllowed) != 0 {
		return fmt.Errorf("following scopes are not allowed by the rule %q - %q", r.Rule.Name, notAllowed)
	}
	return nil
}
