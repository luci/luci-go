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
	"strings"

	"go.chromium.org/luci/common/auth/identity"
	"go.chromium.org/luci/common/config/validation"
	"go.chromium.org/luci/common/data/stringset"

	"go.chromium.org/luci/tokenserver/api/admin/v1"
	"go.chromium.org/luci/tokenserver/appengine/impl/utils/policy"
)

// validateConfigs validates the structure of configs fetched by fetchConfigs.
func validateConfigs(ctx *validation.Context, bundle policy.ConfigBundle) {
	ctx.SetFile(serviceAccountsCfg)
	cfg, ok := bundle[serviceAccountsCfg].(*admin.ServiceAccountsPermissions)
	if !ok {
		ctx.Error("unexpectedly wrong proto type %T", cfg)
		return
	}

	if cfg.Defaults != nil {
		validateDefaults(ctx, "defaults", cfg.Defaults)
	}

	names := stringset.New(0)
	accounts := map[string]string{} // service account -> rule name where its defined
	groups := map[string]string{}   // group with accounts -> rule name where its defined
	for i, rule := range cfg.Rules {
		// Rule name must be unique. Missing name will be handled by 'validateRule'.
		if rule.Name != "" {
			if names.Has(rule.Name) {
				ctx.Error("two rules with identical name %q", rule.Name)
			} else {
				names.Add(rule.Name)
			}
		}

		// There should be no overlap between service account sets covered by each
		// rule. Unfortunately we can't reliably dive into groups, since they may
		// change after the config validation step. So compare only top level group
		// names, Rules.Rule() method relies on this.
		for _, account := range rule.ServiceAccount {
			if name, ok := accounts[account]; ok {
				ctx.Error("service account %q is mentioned by more than one rule (%q and %q)", account, name, rule.Name)
			} else {
				accounts[account] = rule.Name
			}
		}
		for _, group := range rule.ServiceAccountGroup {
			if name, ok := groups[group]; ok {
				ctx.Error("service account group %q is mentioned by more than one rule (%q and %q)", group, name, rule.Name)
			} else {
				groups[group] = rule.Name
			}
		}

		validateRule(ctx, fmt.Sprintf("rule #%d: %q", i+1, rule.Name), rule)
	}
}

// validateDefaults checks ServiceAccountRuleDefaults proto.
func validateDefaults(ctx *validation.Context, title string, d *admin.ServiceAccountRuleDefaults) {
	ctx.Enter(title)
	defer ctx.Exit()
	validateScopes(ctx, "allowed_scope", d.AllowedScope)
	validateMaxGrantValidityDuration(ctx, d.MaxGrantValidityDuration)
}

// validateRule checks single ServiceAccountRule proto.
func validateRule(ctx *validation.Context, title string, r *admin.ServiceAccountRule) {
	ctx.Enter(title)
	defer ctx.Exit()

	if r.Name == "" {
		ctx.Error(`"name" is required`)
	}

	// Note: we allow any of the sets to be empty. The rule will just not match
	// anything in this case, this is fine.
	validateEmails(ctx, "service_account", r.ServiceAccount)
	validateGroups(ctx, "service_account_group", r.ServiceAccountGroup)
	validateScopes(ctx, "allowed_scope", r.AllowedScope)
	validateIDSet(ctx, "end_user", r.EndUser)
	validateIDSet(ctx, "proxy", r.Proxy)
	validateIDSet(ctx, "trusted_proxy", r.TrustedProxy)
	validateMaxGrantValidityDuration(ctx, r.MaxGrantValidityDuration)
}

func validateEmails(ctx *validation.Context, field string, emails []string) {
	ctx.Enter("%q", field)
	defer ctx.Exit()
	for _, email := range emails {
		// We reuse 'user:' identity validator, user identities are emails too.
		if _, err := identity.MakeIdentity("user:" + email); err != nil {
			ctx.Error("bad email %q - %s", email, err)
		}
	}
}

func validateGroups(ctx *validation.Context, field string, groups []string) {
	ctx.Enter("%q", field)
	defer ctx.Exit()
	for _, gr := range groups {
		if gr == "" {
			ctx.Error("the group name must not be empty")
		}
	}
}

func validateScopes(ctx *validation.Context, field string, scopes []string) {
	ctx.Enter("%q", field)
	defer ctx.Exit()
	for _, scope := range scopes {
		if !strings.HasPrefix(scope, "https://www.googleapis.com/") {
			ctx.Error("bad scope %q", scope)
		}
	}
}

func validateIDSet(ctx *validation.Context, field string, ids []string) {
	ctx.Enter("%q", field)
	defer ctx.Exit()
	for _, entry := range ids {
		if strings.HasPrefix(entry, "group:") {
			if entry[len("group:"):] == "" {
				ctx.Error("bad group entry - no group name")
			}
		} else if _, err := identity.MakeIdentity(entry); err != nil {
			ctx.Error("bad identity %q - %s", entry, err)
		}
	}
}

func validateMaxGrantValidityDuration(ctx *validation.Context, dur int64) {
	switch {
	case dur == 0:
		// valid
	case dur < 0:
		ctx.Error(`"max_grant_validity_duration" must be positive`)
	case dur > maxAllowedMaxGrantValidityDuration:
		ctx.Error(`"max_grant_validity_duration" must not exceed %d`, maxAllowedMaxGrantValidityDuration)
	}
}
