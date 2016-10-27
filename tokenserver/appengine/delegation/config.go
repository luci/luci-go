// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package delegation

import (
	"fmt"
	"strings"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	ds "github.com/luci/gae/service/datastore"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/data/caching/lazyslot"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/server/auth/identity"

	"github.com/luci/luci-go/tokenserver/api/admin/v1"
	"github.com/luci/luci-go/tokenserver/appengine/utils/identityset"
)

// Requestor is magical token that may be used in the config and requests as
// a substitute for caller's ID.
//
// See config.proto for more info.
const Requestor = "REQUESTOR"

// DelegationConfig is a singleton entity that stores imported delegation.cfg.
type DelegationConfig struct {
	_id int64 `gae:"$id,1"`

	// Revision this config was imported from.
	Revision string `gae:",noindex"`

	// Config is serialized DelegationPermissions proto message.
	Config []byte `gae:",noindex"`

	// ParsedConfig is deserialized message stored in Config.
	ParsedConfig *admin.DelegationPermissions `gae:"-"`

	// rules is preprocessed config rules.
	//
	// Used by 'FindMatchingRule', built in 'Initialize'.
	rules []*delegationRule `gae:"-"`

	// requestors is a union of all 'Requestor' fields in all rules.
	//
	// Used by 'IsAuthorizedRequestor', built in 'Initialize'.
	requestors *identityset.Set `gae:"-"`
}

// RulesQuery contains parameters to match against the delegation rules.
//
// Used by 'FindMatchingRule'.
type RulesQuery struct {
	Requestor identity.Identity // who is requesting the token
	Delegatee identity.Identity // what identity will be delegated/impersonated
	Audience  *identityset.Set  // the requested audience set
	Services  *identityset.Set  // the requested target services set
}

// delegationRule is preprocessed admin.DelegationRule message.
//
// This object is used by 'FindMatchingRule'.
type delegationRule struct {
	rule *admin.DelegationRule // the original unaltered rule proto

	requestors *identityset.Set // matched to RulesQuery.Requestor
	delegatees *identityset.Set // matched to RulesQuery.Delegatee
	audience   *identityset.Set // matched to RulesQuery.Audience
	services   *identityset.Set // matched to RulesQuery.Services

	addRequestorAsDelegatee bool // if true, add RulesQuery.Requestor to 'delegatees' set
	addRequestorToAudience  bool // if true, add RulesQuery.Requestor to 'audience' set
}

// procCacheExpiration is how long to keep DelegationConfig in process memory.
const procCacheExpiration = time.Minute

// FetchDelegationConfig loads DelegationConfig entity from the datastore.
//
// Returns empty entity if there is no config stored yet. Doesn't attempt to
// deserialize 'Config' protobuf field.
func FetchDelegationConfig(c context.Context) (*DelegationConfig, error) {
	cfg := &DelegationConfig{}
	switch err := ds.Get(c, cfg); {
	case err == ds.ErrNoSuchEntity:
		return cfg, nil
	case err != nil:
		return nil, errors.WrapTransient(err)
	}
	return cfg, nil
}

// DelegationConfigLoader constructs a function that lazy-loads delegation
// config and keeps it cached in memory, refreshing the cached copy each minute.
//
// Used as MintDelegationTokenRPC.ConfigLoader implementation in prod.
func DelegationConfigLoader() func(context.Context) (*DelegationConfig, error) {
	slot := lazyslot.Slot{
		Fetcher: func(c context.Context, prev lazyslot.Value) (lazyslot.Value, error) {
			newCfg, err := FetchDelegationConfig(c)
			if err != nil {
				return lazyslot.Value{}, err
			}

			// Reuse existing unpacked validated config if the revision didn't change.
			prevCfg, _ := prev.Value.(*DelegationConfig)
			if prevCfg != nil && prevCfg.Revision == newCfg.Revision {
				return lazyslot.Value{
					Value:      prevCfg,
					Expiration: clock.Now(c).Add(procCacheExpiration),
				}, nil
			}

			// An error here can happen if previously validated config is no longer
			// valid (e.g. if the service code is updated and new code doesn't like
			// the stored config anymore).
			//
			// If this check fails, the service is effectively offline until config is
			// updated. Presumably, it is better than silently using no longer valid
			// config.
			logging.Infof(c, "Using delegation config at ref %s", newCfg.Revision)
			if err := newCfg.Initialize(); err != nil {
				logging.Errorf(c, "Existing delegation config is invalid - %s", err)
				return lazyslot.Value{}, err
			}

			return lazyslot.Value{
				Value:      newCfg,
				Expiration: clock.Now(c).Add(procCacheExpiration),
			}, nil
		},
	}

	return func(c context.Context) (*DelegationConfig, error) {
		val, err := slot.Get(c)
		if err != nil {
			return nil, err
		}
		return val.Value.(*DelegationConfig), nil
	}
}

// Initialize parses the loaded config, initializing the guts of the object.
func (cfg *DelegationConfig) Initialize() error {
	parsed := &admin.DelegationPermissions{}
	if err := proto.Unmarshal(cfg.Config, parsed); err != nil {
		return err
	}

	rules := make([]*delegationRule, len(parsed.Rules))
	requestors := make([]*identityset.Set, len(parsed.Rules))

	for i, msg := range parsed.Rules {
		rule, err := makeDelegationRule(msg)
		if err != nil {
			return err
		}
		rules[i] = rule
		requestors[i] = rule.requestors
	}

	cfg.ParsedConfig = parsed
	cfg.rules = rules
	cfg.requestors = identityset.Union(requestors...)

	return nil
}

// makeDelegationRule preprocesses admin.DelegationRule proto.
//
// It also checks that the rule is passing validation.
func makeDelegationRule(rule *admin.DelegationRule) (*delegationRule, error) {
	if merr := ValidateRule(rule); len(merr) != 0 {
		return nil, merr
	}

	// The main validation step has been done above. Here we just assert that
	// everything looks sane (it should). See corresponding chunks of
	// 'ValidateRule' code.
	requestors, err := identityset.FromStrings(rule.Requestor, nil)
	if err != nil {
		panic(err)
	}
	delegatees, err := identityset.FromStrings(rule.AllowedToImpersonate, skipRequestor)
	if err != nil {
		panic(err)
	}
	audience, err := identityset.FromStrings(rule.AllowedAudience, skipRequestor)
	if err != nil {
		panic(err)
	}
	services, err := identityset.FromStrings(rule.TargetService, nil)
	if err != nil {
		panic(err)
	}

	return &delegationRule{
		rule:                    rule,
		requestors:              requestors,
		delegatees:              delegatees,
		audience:                audience,
		services:                services,
		addRequestorAsDelegatee: sliceHasString(rule.AllowedToImpersonate, Requestor),
		addRequestorToAudience:  sliceHasString(rule.AllowedAudience, Requestor),
	}, nil
}

func skipRequestor(s string) bool {
	return s == Requestor
}

func sliceHasString(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// IsAuthorizedRequestor returns true if the caller belongs to 'requestor' set
// of at least one rule.
func (cfg *DelegationConfig) IsAuthorizedRequestor(c context.Context, id identity.Identity) (bool, error) {
	return cfg.requestors.IsMember(c, id)
}

// FindMatchingRule finds one and only one rule matching the query.
//
// If multiple rules match or none rules match, an error is returned.
func (cfg *DelegationConfig) FindMatchingRule(c context.Context, q *RulesQuery) (*admin.DelegationRule, error) {
	var matches []*admin.DelegationRule
	for _, rule := range cfg.rules {
		switch yes, err := rule.matchesQuery(c, q); {
		case err != nil:
			return nil, err // usually transient
		case yes:
			matches = append(matches, rule.rule)
		}
	}

	if len(matches) == 0 {
		return nil, fmt.Errorf("no matching delegation rules in the config")
	}

	if len(matches) > 1 {
		names := make([]string, len(matches))
		for i, m := range matches {
			names[i] = fmt.Sprintf("%q", m.Name)
		}
		return nil, fmt.Errorf(
			"ambiguous request, multiple delegation rules match (%s)",
			strings.Join(names, ", "))
	}

	return matches[0], nil
}

// matchesQuery returns true if this rule matches the query.
//
// See doc in config.proto, DelegationRule for exact description of when this
// happens. Basically, all sets in rule must be supersets of corresponding sets
// in RulesQuery.
//
// May return transient errors.
func (rule *delegationRule) matchesQuery(c context.Context, q *RulesQuery) (bool, error) {
	// Rule's 'requestor' set contains the requestor?
	switch found, err := rule.requestors.IsMember(c, q.Requestor); {
	case err != nil:
		return false, err
	case !found:
		return false, nil
	}

	// Rule's 'delegatee' set contains the identity being delegated/impersonated?
	allowedDelegatees := rule.delegatees
	if rule.addRequestorAsDelegatee {
		allowedDelegatees = identityset.Extend(allowedDelegatees, q.Requestor)
	}
	switch found, err := allowedDelegatees.IsMember(c, q.Delegatee); {
	case err != nil:
		return false, err
	case !found:
		return false, nil
	}

	// Rule's 'audience' is superset of requested audience?
	allowedAudience := rule.audience
	if rule.addRequestorToAudience {
		allowedAudience = identityset.Extend(allowedAudience, q.Requestor)
	}
	if !allowedAudience.IsSuperset(q.Audience) {
		return false, nil
	}

	// Rule's allowed targets is superset of requested targets?
	if !rule.services.IsSuperset(q.Services) {
		return false, nil
	}

	return true, nil
}
