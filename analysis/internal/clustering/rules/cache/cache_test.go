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
	"sort"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/analysis/internal/bugs"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/testutil"
)

var cache = caching.RegisterLRUCache[string, *Ruleset](50)

func TestRulesCache(t *testing.T) {
	ftt.Run(`With Spanner Test Database`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = caching.WithEmptyProcessCache(ctx)

		rc := NewRulesCache(cache)
		err := rules.SetForTesting(ctx, t, nil)
		assert.Loosely(t, err, should.BeNil)

		test := func(minimumPredicatesVerison time.Time, expectedRules []*rules.Entry, expectedVersion rules.Version) {
			// Tests the content of the cache is as expected.
			ruleset, err := rc.Ruleset(ctx, "myproject", minimumPredicatesVerison)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ruleset.Version, should.Resemble(expectedVersion))

			activeRules := 0
			for _, e := range expectedRules {
				if e.IsActive {
					activeRules++
				}
			}
			assert.Loosely(t, len(ruleset.ActiveRulesSorted), should.Equal(activeRules))
			assert.Loosely(t, len(ruleset.ActiveRulesByID), should.Equal(activeRules))

			sortedExpectedRules := sortRulesByPredicateLastUpdated(expectedRules)

			actualRuleIndex := 0
			for _, e := range sortedExpectedRules {
				if e.IsActive {
					a := ruleset.ActiveRulesSorted[actualRuleIndex]
					assert.Loosely(t, a.Rule, should.Resemble(*e))
					// Technically (*lang.Expr).String() may not get us
					// back the original rule if RuleDefinition didn't use
					// normalised formatting. But for this test, we use
					// normalised formatting, so that is not an issue.
					assert.Loosely(t, a.Expr, should.NotBeNil)
					assert.Loosely(t, a.Expr.String(), should.Equal(e.RuleDefinition))
					actualRuleIndex++

					a2, ok := ruleset.ActiveRulesByID[a.Rule.RuleID]
					assert.Loosely(t, ok, should.BeTrue)
					assert.Loosely(t, a2.Rule, should.Resemble(*e))
				}
			}
			assert.Loosely(t, len(ruleset.ActiveRulesWithPredicateUpdatedSince(rules.StartingEpoch)), should.Equal(activeRules))
			assert.Loosely(t, len(ruleset.ActiveRulesWithPredicateUpdatedSince(time.Date(2100, time.January, 1, 1, 0, 0, 0, time.UTC))), should.BeZero)
		}

		t.Run(`Initially Empty`, func(t *ftt.Test) {
			err := rules.SetForTesting(ctx, t, nil)
			assert.Loosely(t, err, should.BeNil)
			test(rules.StartingEpoch, nil, rules.StartingVersion)

			t.Run(`Then Empty`, func(t *ftt.Test) {
				// Test cache.
				test(rules.StartingEpoch, nil, rules.StartingVersion)

				tc.Add(refreshInterval)

				test(rules.StartingEpoch, nil, rules.StartingVersion)
				test(rules.StartingEpoch, nil, rules.StartingVersion)
			})
			t.Run(`Then Non-Empty`, func(t *ftt.Test) {
				// Spanner commit timestamps are in microsecond
				// (not nanosecond) granularity, and some Spanner timestamp
				// operators truncates to microseconds. For this
				// reason, we use microsecond resolution timestamps
				// when testing.
				reference := time.Date(2020, 1, 2, 3, 4, 5, 6000, time.UTC)

				rs := []*rules.Entry{
					rules.NewRule(100).
						WithLastUpdateTime(reference.Add(-1 * time.Hour)).
						WithPredicateLastUpdateTime(reference.Add(-2 * time.Hour)).
						Build(),
					rules.NewRule(101).WithActive(false).
						WithLastUpdateTime(reference.Add(1 * time.Hour)).
						WithPredicateLastUpdateTime(reference).
						Build(),
				}
				err := rules.SetForTesting(ctx, t, rs)
				assert.Loosely(t, err, should.BeNil)

				expectedRulesVersion := rules.Version{
					Total:      reference.Add(1 * time.Hour),
					Predicates: reference,
				}

				t.Run(`By Strong Read`, func(t *ftt.Test) {
					test(StrongRead, rs, expectedRulesVersion)
					test(StrongRead, rs, expectedRulesVersion)
				})
				t.Run(`By Requesting Version`, func(t *ftt.Test) {
					test(expectedRulesVersion.Predicates, rs, expectedRulesVersion)
				})
				t.Run(`By Cache Expiry`, func(t *ftt.Test) {
					// Test cache is working and still returning the old value.
					tc.Add(refreshInterval / 2)
					test(rules.StartingEpoch, nil, rules.StartingVersion)

					tc.Add(refreshInterval)

					test(rules.StartingEpoch, rs, expectedRulesVersion)
					test(rules.StartingEpoch, rs, expectedRulesVersion)
				})
			})
		})
		t.Run(`Initially Non-Empty`, func(t *ftt.Test) {
			reference := time.Date(2021, 1, 2, 3, 4, 5, 6000, time.UTC)

			ruleOne := rules.NewRule(100).
				WithLastUpdateTime(reference.Add(-2 * time.Hour)).
				WithPredicateLastUpdateTime(reference.Add(-3 * time.Hour))
			ruleTwo := rules.NewRule(101).
				WithLastUpdateTime(reference.Add(-2 * time.Hour)).
				WithPredicateLastUpdateTime(reference.Add(-3 * time.Hour))
			ruleThree := rules.NewRule(102).WithActive(false).
				WithLastUpdateTime(reference).
				WithPredicateLastUpdateTime(reference.Add(-1 * time.Hour))

			rs := []*rules.Entry{
				ruleOne.Build(),
				ruleTwo.Build(),
				ruleThree.Build(),
			}
			err := rules.SetForTesting(ctx, t, rs)
			assert.Loosely(t, err, should.BeNil)

			expectedRulesVersion := rules.Version{
				Total:      reference,
				Predicates: reference.Add(-1 * time.Hour),
			}
			test(rules.StartingEpoch, rs, expectedRulesVersion)

			t.Run(`Then Empty`, func(t *ftt.Test) {
				// Mark all rules inactive.
				newRules := []*rules.Entry{
					ruleOne.WithActive(false).
						WithLastUpdateTime(reference.Add(4 * time.Hour)).
						WithPredicateLastUpdateTime(reference.Add(3 * time.Hour)).
						Build(),
					ruleTwo.WithActive(false).
						WithLastUpdateTime(reference.Add(2 * time.Hour)).
						WithPredicateLastUpdateTime(reference.Add(1 * time.Hour)).
						Build(),
					ruleThree.WithActive(false).
						WithLastUpdateTime(reference.Add(2 * time.Hour)).
						WithPredicateLastUpdateTime(reference.Add(1 * time.Hour)).
						Build(),
				}
				err := rules.SetForTesting(ctx, t, newRules)
				assert.Loosely(t, err, should.BeNil)

				oldRulesVersion := expectedRulesVersion
				expectedRulesVersion := rules.Version{
					Total:      reference.Add(4 * time.Hour),
					Predicates: reference.Add(3 * time.Hour),
				}

				t.Run(`By Strong Read`, func(t *ftt.Test) {
					test(StrongRead, newRules, expectedRulesVersion)
					test(StrongRead, newRules, expectedRulesVersion)
				})
				t.Run(`By Requesting Version`, func(t *ftt.Test) {
					test(expectedRulesVersion.Predicates, newRules, expectedRulesVersion)
				})
				t.Run(`By Cache Expiry`, func(t *ftt.Test) {
					// Test cache is working and still returning the old value.
					tc.Add(refreshInterval / 2)
					test(rules.StartingEpoch, rs, oldRulesVersion)

					tc.Add(refreshInterval)

					test(rules.StartingEpoch, newRules, expectedRulesVersion)
					test(rules.StartingEpoch, newRules, expectedRulesVersion)
				})
			})
			t.Run(`Then Non-Empty`, func(t *ftt.Test) {
				newRules := []*rules.Entry{
					// Mark an existing rule inactive.
					ruleOne.WithActive(false).
						WithLastUpdateTime(reference.Add(time.Hour)).
						WithPredicateLastUpdateTime(reference.Add(time.Hour)).
						Build(),
					// Make a non-predicate change on an active rule.
					ruleTwo.
						WithBug(bugs.BugID{System: "monorail", ID: "project/123"}).
						WithLastUpdateTime(reference.Add(time.Hour)).
						Build(),
					// Make an existing rule active.
					ruleThree.WithActive(true).
						WithLastUpdateTime(reference.Add(time.Hour)).
						WithPredicateLastUpdateTime(reference.Add(time.Hour)).
						Build(),
					// Add a new active rule.
					rules.NewRule(103).
						WithPredicateLastUpdateTime(reference.Add(time.Hour)).
						WithLastUpdateTime(reference.Add(time.Hour)).
						Build(),
					// Add a new inactive rule.
					rules.NewRule(104).WithActive(false).
						WithPredicateLastUpdateTime(reference.Add(2 * time.Hour)).
						WithLastUpdateTime(reference.Add(3 * time.Hour)).
						Build(),
				}
				err := rules.SetForTesting(ctx, t, newRules)
				assert.Loosely(t, err, should.BeNil)

				oldRulesVersion := expectedRulesVersion
				expectedRulesVersion := rules.Version{
					Total:      reference.Add(3 * time.Hour),
					Predicates: reference.Add(2 * time.Hour),
				}

				t.Run(`By Strong Read`, func(t *ftt.Test) {
					test(StrongRead, newRules, expectedRulesVersion)
					test(StrongRead, newRules, expectedRulesVersion)
				})
				t.Run(`By Forced Eviction`, func(t *ftt.Test) {
					test(expectedRulesVersion.Predicates, newRules, expectedRulesVersion)
				})
				t.Run(`By Cache Expiry`, func(t *ftt.Test) {
					// Test cache is working and still returning the old value.
					tc.Add(refreshInterval / 2)
					test(rules.StartingEpoch, rs, oldRulesVersion)

					tc.Add(refreshInterval)

					test(rules.StartingEpoch, newRules, expectedRulesVersion)
					test(rules.StartingEpoch, newRules, expectedRulesVersion)
				})
			})
		})
	})
}

func sortRulesByPredicateLastUpdated(rs []*rules.Entry) []*rules.Entry {
	result := make([]*rules.Entry, len(rs))
	copy(result, rs)
	sort.Slice(result, func(i, j int) bool {
		if result[i].PredicateLastUpdateTime.Equal(result[j].PredicateLastUpdateTime) {
			return result[i].RuleID < result[j].RuleID
		}
		return result[i].PredicateLastUpdateTime.After(result[j].PredicateLastUpdateTime)
	})
	return result
}
