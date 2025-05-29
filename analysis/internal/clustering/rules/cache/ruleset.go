// Copyright 2022 The LUCI Authors.
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

package cache

import (
	"context"
	"sort"
	"time"

	"go.opentelemetry.io/otel/attribute"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/clustering/rules/lang"
	"go.chromium.org/luci/analysis/internal/tracing"
)

// CachedRule represents a "compiled" version of a failure
// association rule.
// It should be treated as immutable, and is therefore safe to
// share across multiple threads.
type CachedRule struct {
	// The failure association rule.
	Rule rules.Entry
	// The parsed and compiled failure association rule.
	Expr *lang.Expr
}

// NewCachedRule initialises a new CachedRule from the given failure
// association rule.
func NewCachedRule(rule *rules.Entry) (*CachedRule, error) {
	expr, err := lang.Parse(rule.RuleDefinition)
	if err != nil {
		return nil, err
	}
	return &CachedRule{
		Rule: *rule,
		Expr: expr,
	}, nil
}

// Ruleset represents a version of the set of failure
// association rules in use by a LUCI Project.
// It should be treated as immutable, and therefore safe to share
// across multiple threads.
type Ruleset struct {
	// The LUCI Project.
	Project string
	// ActiveRulesSorted is the set of active failure association rules
	// (should be used by LUCI Analysis for matching), sorted in descending
	// PredicateLastUpdated time order.
	ActiveRulesSorted []*CachedRule
	// ActiveRulesByID stores active failure association
	// rules by their Rule ID.
	ActiveRulesByID map[string]*CachedRule
	// Version versions the contents of the Ruleset. These timestamps only
	// change if a rule is modified.
	Version rules.Version
	// LastRefresh contains the monotonic clock reading when the last ruleset
	// refresh was initiated. The refresh is guaranteed to contain all rules
	// changes made prior to this timestamp.
	LastRefresh time.Time
}

// ActiveRulesWithPredicateUpdatedSince returns the set of rules that are
// active and whose predicates have been updated since (but not including)
// the given time.
// Rules which have been made inactive since the given time will NOT be
// returned. To check if a previous rule has been made inactive, consider
// using IsRuleActive instead.
// The returned slice must not be mutated.
func (r *Ruleset) ActiveRulesWithPredicateUpdatedSince(t time.Time) []*CachedRule {
	// Use the property that ActiveRules is sorted by descending
	// LastUpdated time.
	for i, rule := range r.ActiveRulesSorted {
		if !rule.Rule.PredicateLastUpdateTime.After(t) {
			// This is the first rule that has not been updated since time t.
			// Return all rules up to (but not including) this rule.
			return r.ActiveRulesSorted[:i]
		}
	}
	return r.ActiveRulesSorted
}

// Returns whether the given ruleID is an active rule.
func (r *Ruleset) IsRuleActive(ruleID string) bool {
	_, ok := r.ActiveRulesByID[ruleID]
	return ok
}

// newEmptyRuleset initialises a new empty ruleset.
// This initial ruleset is invalid and must be refreshed before use.
func newEmptyRuleset(project string) *Ruleset {
	return &Ruleset{
		Project:           project,
		ActiveRulesSorted: nil,
		ActiveRulesByID:   make(map[string]*CachedRule),
		// The zero predicate last updated time is not valid and will be
		// rejected by clustering state validation if we ever try to save
		// it to Spanner as a chunk's RulesVersion.
		Version:     rules.Version{},
		LastRefresh: time.Time{},
	}
}

// NewRuleset creates a new ruleset with the given project,
// active rules, rules last updated and last refresh time.
func NewRuleset(project string, activeRules []*CachedRule, version rules.Version, lastRefresh time.Time) *Ruleset {
	return &Ruleset{
		Project:           project,
		ActiveRulesSorted: sortByDescendingPredicateLastUpdated(activeRules),
		ActiveRulesByID:   rulesByID(activeRules),
		Version:           version,
		LastRefresh:       lastRefresh,
	}
}

// refresh updates the ruleset. To ensure existing users of the rulset
// do not observe changes while they are using it, a new copy is returned.
func (r *Ruleset) refresh(ctx context.Context) (ruleset *Ruleset, err error) {
	// Under our design assumption of 10,000 active rules per project,
	// pulling and compiling all rules could take a meaningful amount
	// of time (@ 1KB per rule, = ~10MB).
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/clustering/rules/cache.Refresh",
		attribute.String("project", r.Project),
	)
	defer func() { tracing.End(s, err) }()

	// Use clock reading before refresh. The refresh is guaranteed
	// to contain all rule changes committed to Spanner prior to
	// this timestamp.
	lastRefresh := clock.Now(ctx)

	txn, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	var activeRules []*CachedRule
	if r.Version == (rules.Version{}) {
		// On the first refresh, query all active rules.
		ruleRows, err := rules.ReadActive(txn, r.Project)
		if err != nil {
			return nil, err
		}
		activeRules, err = cachedRulesFromFullRead(ruleRows)
		if err != nil {
			return nil, err
		}
	} else {
		// On subsequent refreshes, query just the differences.
		delta, err := rules.ReadDelta(txn, r.Project, r.Version.Total)
		if err != nil {
			return nil, err
		}
		activeRules, err = cachedRulesFromDelta(r.ActiveRulesSorted, delta)
		if err != nil {
			return nil, err
		}
	}

	// Get the version of set of rules read by ReadActive/ReadDelta.
	// Must occur in the same spanner transaction as ReadActive/ReadDelta.
	// If the project has no rules, this returns rules.StartingEpoch.
	rulesVersion, err := rules.ReadVersion(txn, r.Project)
	if err != nil {
		return nil, err
	}

	return NewRuleset(r.Project, activeRules, rulesVersion, lastRefresh), nil
}

// cachedRulesFromFullRead obtains a set of cached rules from the given set of
// active failure association rules.
func cachedRulesFromFullRead(activeRules []*rules.Entry) ([]*CachedRule, error) {
	var result []*CachedRule
	for _, r := range activeRules {
		cr, err := NewCachedRule(r)
		if err != nil {
			return nil, errors.Fmt("rule %s is invalid: %w", r.RuleID, err)
		}
		result = append(result, cr)
	}
	return result, nil
}

// cachedRulesFromDelta applies deltas to an existing list of rules,
// to obtain an updated set of rules.
func cachedRulesFromDelta(existing []*CachedRule, delta []*rules.Entry) ([]*CachedRule, error) {
	ruleByID := make(map[string]*CachedRule)
	for _, r := range existing {
		ruleByID[r.Rule.RuleID] = r
	}
	for _, d := range delta {
		if d.IsActive {
			cr, err := NewCachedRule(d)
			if err != nil {
				return nil, errors.Fmt("rule %s is invalid: %w", d.RuleID, err)
			}
			ruleByID[d.RuleID] = cr
		} else {
			// Delete the rule, if it exists.
			delete(ruleByID, d.RuleID)
		}
	}
	var results []*CachedRule
	for _, r := range ruleByID {
		results = append(results, r)
	}
	return results, nil
}

// sortByDescendingPredicateLastUpdated sorts the given rules in descending
// predicate last-updated time order. If two rules have the same
// PredicateLastUpdated time, they are sorted in RuleID order.
func sortByDescendingPredicateLastUpdated(rules []*CachedRule) []*CachedRule {
	result := make([]*CachedRule, len(rules))
	copy(result, rules)
	sort.Slice(result, func(i, j int) bool {
		if result[i].Rule.PredicateLastUpdateTime.Equal(result[j].Rule.PredicateLastUpdateTime) {
			return result[i].Rule.RuleID < result[j].Rule.RuleID
		}
		return result[i].Rule.PredicateLastUpdateTime.After(result[j].Rule.PredicateLastUpdateTime)
	})
	return result
}

// rulesByID creates a mapping from rule ID to rules for the given list
// of failure association rules.
func rulesByID(rules []*CachedRule) map[string]*CachedRule {
	result := make(map[string]*CachedRule)
	for _, r := range rules {
		result[r.Rule.RuleID] = r
	}
	return result
}
