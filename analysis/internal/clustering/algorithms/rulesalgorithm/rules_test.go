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

package rulesalgorithm

import (
	"sort"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/clustering/rules/cache"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestAlgorithm(t *testing.T) {
	ftt.Run(`Name`, t, func(t *ftt.Test) {
		// Algorithm name should be valid.
		assert.Loosely(t, clustering.AlgorithmRe.MatchString(AlgorithmName), should.BeTrue)
	})
	ftt.Run(`Cluster from scratch`, t, func(t *ftt.Test) {
		a := &Algorithm{}
		existingRulesVersion := rules.StartingEpoch
		ids := make(map[string]struct{})
		t.Run(`Empty Rules`, func(t *ftt.Test) {
			ruleset := &cache.Ruleset{}
			a.Cluster(ruleset, existingRulesVersion, ids, &clustering.Failure{
				Reason: &pb.FailureReason{PrimaryErrorMessage: "Null pointer exception at ip 0x45637271"},
			})
			assert.Loosely(t, ids, should.BeEmpty)
		})
		t.Run(`With Rules`, func(t *ftt.Test) {
			rule1, err := cache.NewCachedRule(
				rules.NewRule(100).
					WithRuleDefinition(`test = "ninja://test_name_one/"`).
					Build())
			assert.Loosely(t, err, should.BeNil)
			rule2, err := cache.NewCachedRule(
				rules.NewRule(101).
					WithRuleDefinition(`reason LIKE "failed to connect to %.%.%.%"`).
					Build())
			assert.Loosely(t, err, should.BeNil)

			rulesVersion := rules.Version{
				Predicates: time.Now(),
			}
			lastUpdated := time.Now()
			rules := []*cache.CachedRule{rule1, rule2}
			ruleset := cache.NewRuleset("myproject", rules, rulesVersion, lastUpdated)

			t.Run(`Without failure reason`, func(t *ftt.Test) {
				t.Run(`Matching`, func(t *ftt.Test) {
					a.Cluster(ruleset, existingRulesVersion, ids, &clustering.Failure{
						TestID: "ninja://test_name_one/",
					})
					assert.Loosely(t, ids, should.Resemble(map[string]struct{}{
						rule1.Rule.RuleID: {},
					}))
				})
				t.Run(`Non-matcing`, func(t *ftt.Test) {
					a.Cluster(ruleset, existingRulesVersion, ids, &clustering.Failure{
						TestID: "ninja://test_name_two/",
					})
					assert.Loosely(t, ids, should.BeEmpty)
				})
			})
			t.Run(`Matches one`, func(t *ftt.Test) {
				a.Cluster(ruleset, existingRulesVersion, ids, &clustering.Failure{
					TestID: "ninja://test_name_three/",
					Reason: &pb.FailureReason{
						PrimaryErrorMessage: "failed to connect to 192.168.0.1",
					},
				})
				assert.Loosely(t, ids, should.Resemble(map[string]struct{}{
					rule2.Rule.RuleID: {},
				}))
			})
			t.Run(`Matches multiple`, func(t *ftt.Test) {
				a.Cluster(ruleset, existingRulesVersion, ids, &clustering.Failure{
					TestID: "ninja://test_name_one/",
					Reason: &pb.FailureReason{
						PrimaryErrorMessage: "failed to connect to 192.168.0.1",
					},
				})
				expectedIDs := []string{rule1.Rule.RuleID, rule2.Rule.RuleID}
				sort.Strings(expectedIDs)
				assert.Loosely(t, ids, should.Resemble(map[string]struct{}{
					rule1.Rule.RuleID: {},
					rule2.Rule.RuleID: {},
				}))
			})
		})
	})
	ftt.Run(`Cluster incrementally`, t, func(t *ftt.Test) {
		a := &Algorithm{}
		originalRulesVersion := time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC)
		testFailure := &clustering.Failure{
			TestID: "ninja://test_name_one/",
			Reason: &pb.FailureReason{
				PrimaryErrorMessage: "failed to connect to 192.168.0.1",
			},
		}

		// The ruleset we are incrementally clustering with has a new rule
		// (rule 3) and no longer has rule 2. We silently set the definition
		// of rule1 to FALSE without changing its last updated time (this
		// should never happen in reality) to check it is never evaluated.
		rule1, err := cache.NewCachedRule(
			rules.NewRule(100).WithRuleDefinition(`FALSE`).
				WithPredicateLastUpdateTime(originalRulesVersion).Build())
		assert.Loosely(t, err, should.BeNil)
		rule3, err := cache.NewCachedRule(
			rules.NewRule(102).
				WithRuleDefinition(`reason LIKE "failed to connect to %"`).
				WithPredicateLastUpdateTime(originalRulesVersion.Add(time.Hour)).Build())
		assert.Loosely(t, err, should.BeNil)

		rs := []*cache.CachedRule{rule1, rule3}
		newRulesVersion := rules.Version{
			Predicates: originalRulesVersion.Add(time.Hour),
		}
		lastUpdated := time.Now()
		secondRuleset := cache.NewRuleset("myproject", rs, newRulesVersion, lastUpdated)

		ids := map[string]struct{}{
			rule1.Rule.RuleID: {},
			"rule2-id":        {},
		}

		// Test incrementally clustering leads to the correct outcome,
		// matching rule 3 and unmatching rule 2.
		a.Cluster(secondRuleset, originalRulesVersion, ids, testFailure)
		assert.Loosely(t, ids, should.Resemble(map[string]struct{}{
			rule1.Rule.RuleID: {},
			rule3.Rule.RuleID: {},
		}))
	})
}
