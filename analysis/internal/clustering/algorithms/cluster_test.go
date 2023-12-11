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

package algorithms

import (
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/failurereason"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/rulesalgorithm"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/testname"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/clustering/rules/cache"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCluster(t *testing.T) {
	Convey(`Cluster`, t, func() {
		Convey(`From scratch`, func() {
			s := fromScratchScenario(1)

			results := Cluster(s.config, s.ruleset, s.existing, s.failures)

			So(results.AlgorithmsVersion, ShouldEqual, s.expected.AlgorithmsVersion)
			So(results.RulesVersion, ShouldEqual, s.expected.RulesVersion)
			So(results.Algorithms, ShouldResemble, s.expected.Algorithms)
			So(diffClusters(results.Clusters, s.expected.Clusters), ShouldBeBlank)
		})
		Convey(`Incrementally`, func() {
			Convey(`From already up-to-date clustering`, func() {
				s := upToDateScenario(1)

				results := Cluster(s.config, s.ruleset, s.existing, s.failures)

				So(results.AlgorithmsVersion, ShouldEqual, s.expected.AlgorithmsVersion)
				So(results.RulesVersion, ShouldEqual, s.expected.RulesVersion)
				So(results.Algorithms, ShouldResemble, s.expected.Algorithms)
				So(diffClusters(results.Clusters, s.expected.Clusters), ShouldBeBlank)
			})

			Convey(`From older suggested clustering algorithm`, func() {
				s := fromOlderSuggestedClusteringScenario()

				results := Cluster(s.config, s.ruleset, s.existing, s.failures)

				So(results.AlgorithmsVersion, ShouldEqual, s.expected.AlgorithmsVersion)
				So(results.RulesVersion, ShouldEqual, s.expected.RulesVersion)
				So(results.Algorithms, ShouldResemble, s.expected.Algorithms)
				So(diffClusters(results.Clusters, s.expected.Clusters), ShouldBeBlank)
			})
			Convey(`Incrementally from older rule-based clustering`, func() {
				s := fromOlderRuleAlgorithmScenario()

				results := Cluster(s.config, s.ruleset, s.existing, s.failures)

				So(results.AlgorithmsVersion, ShouldEqual, s.expected.AlgorithmsVersion)
				So(results.RulesVersion, ShouldEqual, s.expected.RulesVersion)
				So(results.Algorithms, ShouldResemble, s.expected.Algorithms)
				So(diffClusters(results.Clusters, s.expected.Clusters), ShouldBeBlank)
			})
			Convey(`Incrementally from later clustering algorithms`, func() {
				s := fromLaterAlgorithmsScenario()

				results := Cluster(s.config, s.ruleset, s.existing, s.failures)

				So(results.AlgorithmsVersion, ShouldEqual, s.expected.AlgorithmsVersion)
				So(results.RulesVersion, ShouldEqual, s.expected.RulesVersion)
				So(results.Algorithms, ShouldResemble, s.expected.Algorithms)
				So(diffClusters(results.Clusters, s.expected.Clusters), ShouldBeBlank)
			})
			Convey(`Incrementally from older rules version`, func() {
				s := fromOlderRulesVersionScenario(1)

				results := Cluster(s.config, s.ruleset, s.existing, s.failures)

				So(results.AlgorithmsVersion, ShouldEqual, s.expected.AlgorithmsVersion)
				So(results.RulesVersion, ShouldEqual, s.expected.RulesVersion)
				So(results.Algorithms, ShouldResemble, s.expected.Algorithms)
				So(diffClusters(results.Clusters, s.expected.Clusters), ShouldBeBlank)
			})
			Convey(`Incrementally from newer rules version`, func() {
				s := fromNewerRulesVersionScenario()

				results := Cluster(s.config, s.ruleset, s.existing, s.failures)

				So(results.AlgorithmsVersion, ShouldEqual, s.expected.AlgorithmsVersion)
				So(results.RulesVersion, ShouldEqual, s.expected.RulesVersion)
				So(results.Algorithms, ShouldResemble, s.expected.Algorithms)
				So(diffClusters(results.Clusters, s.expected.Clusters), ShouldBeBlank)
			})
			Convey(`Incrementally from older config version`, func() {
				s := fromOlderConfigVersionScenario()

				results := Cluster(s.config, s.ruleset, s.existing, s.failures)

				So(results.AlgorithmsVersion, ShouldEqual, s.expected.AlgorithmsVersion)
				So(results.RulesVersion, ShouldEqual, s.expected.RulesVersion)
				So(results.Algorithms, ShouldResemble, s.expected.Algorithms)
				So(diffClusters(results.Clusters, s.expected.Clusters), ShouldBeBlank)
			})
			Convey(`Incrementally from newer config version`, func() {
				s := fromNewerConfigVersionScenario()

				results := Cluster(s.config, s.ruleset, s.existing, s.failures)

				So(results.AlgorithmsVersion, ShouldEqual, s.expected.AlgorithmsVersion)
				So(results.RulesVersion, ShouldEqual, s.expected.RulesVersion)
				So(results.Algorithms, ShouldResemble, s.expected.Algorithms)
				So(diffClusters(results.Clusters, s.expected.Clusters), ShouldBeBlank)
			})
		})
	})
}

func BenchmarkClusteringFromScratch(b *testing.B) {
	b.StopTimer()
	s := fromScratchScenario(1000)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_ = Cluster(s.config, s.ruleset, s.existing, s.failures)
	}
}

func BenchmarkClusteringFromOlderRules(b *testing.B) {
	b.StopTimer()
	s := fromOlderRulesVersionScenario(1000)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_ = Cluster(s.config, s.ruleset, s.existing, s.failures)
	}
}

func BenchmarkClusteringUpToDate(b *testing.B) {
	b.StopTimer()
	s := upToDateScenario(1000)
	b.StartTimer()
	for i := 0; i < b.N; i++ {
		_ = Cluster(s.config, s.ruleset, s.existing, s.failures)
	}
}

type scenario struct {
	failures []*clustering.Failure
	rules    []*cache.CachedRule
	ruleset  *cache.Ruleset
	config   *compiledcfg.ProjectConfig
	existing clustering.ClusterResults
	expected clustering.ClusterResults
}

func upToDateScenario(size int) *scenario {
	rulesVersion := rules.Version{
		Predicates: time.Date(2020, time.January, 1, 1, 0, 0, 0, time.UTC),
	}

	rule1, err := cache.NewCachedRule(
		rules.NewRule(100).
			WithRuleDefinition(`test = "ninja://test_name/2"`).
			WithPredicateLastUpdateTime(rulesVersion.Predicates.Add(-1 * time.Hour)).
			Build())
	if err != nil {
		panic(err)
	}

	rule2, err := cache.NewCachedRule(
		rules.NewRule(101).
			WithRuleDefinition(`reason LIKE "failed to connect to %.%.%.%"`).
			WithPredicateLastUpdateTime(rulesVersion.Predicates).Build())
	if err != nil {
		panic(err)
	}

	lastUpdated := time.Now()
	rules := []*cache.CachedRule{rule1, rule2}
	ruleset := cache.NewRuleset("myproject", rules, rulesVersion, lastUpdated)

	cfgpb := &configpb.ProjectConfig{
		Clustering:  TestClusteringConfig(),
		LastUpdated: timestamppb.New(time.Date(2020, time.February, 1, 1, 0, 0, 0, time.UTC)),
	}
	cfg, err := compiledcfg.NewConfig(cfgpb)
	if err != nil {
		// Should not occur, test data should be valid.
		panic(err)
	}

	failures := []*clustering.Failure{
		{
			TestID: "ninja://test_name/1",
		},
	}
	for i := 0; i < size; i++ {
		failures = append(failures,
			&clustering.Failure{
				TestID: "ninja://test_name/2",
				Reason: &pb.FailureReason{
					PrimaryErrorMessage: "failed to connect to 192.168.0.1",
				},
			})
	}

	// This is an up-to-date clustering of the test results.
	existing := clustering.ClusterResults{
		AlgorithmsVersion: AlgorithmsVersion,
		ConfigVersion:     cfg.LastUpdated,
		RulesVersion:      rulesVersion.Predicates,
		Algorithms: map[string]struct{}{
			failurereason.AlgorithmName:  {},
			rulesalgorithm.AlgorithmName: {},
			testname.AlgorithmName:       {},
		},
		Clusters: [][]clustering.ClusterID{
			{
				testNameClusterID(cfg, failures[0]),
			},
		},
	}
	for i := 0; i < size; i++ {
		clusters := []clustering.ClusterID{
			failureReasonClusterID(cfg, failures[1]),
			testNameClusterID(cfg, failures[1]),
			ruleClusterID(rule1.Rule.RuleID),
			ruleClusterID(rule2.Rule.RuleID),
		}
		clustering.SortClusters(clusters)
		existing.Clusters = append(existing.Clusters, clusters)
	}

	// Same as existing, a deep copy as the other methods
	// may modify this scenario and we don't want to run into
	// unexpected aliasing issues.
	expected := clustering.ClusterResults{
		AlgorithmsVersion: AlgorithmsVersion,
		ConfigVersion:     cfg.LastUpdated,
		RulesVersion:      rulesVersion.Predicates,
		Algorithms: map[string]struct{}{
			failurereason.AlgorithmName:  {},
			rulesalgorithm.AlgorithmName: {},
			testname.AlgorithmName:       {},
		},
		Clusters: [][]clustering.ClusterID{
			{
				testNameClusterID(cfg, failures[0]),
			},
		},
	}
	for i := 0; i < size; i++ {
		clusters := []clustering.ClusterID{
			failureReasonClusterID(cfg, failures[1]),
			testNameClusterID(cfg, failures[1]),
			ruleClusterID(rule1.Rule.RuleID),
			ruleClusterID(rule2.Rule.RuleID),
		}
		clustering.SortClusters(clusters)
		expected.Clusters = append(expected.Clusters, clusters)
	}

	return &scenario{
		failures: failures,
		rules:    rules,
		ruleset:  ruleset,
		config:   cfg,
		expected: expected,
		existing: existing,
	}
}

func fromOlderSuggestedClusteringScenario() *scenario {
	s := upToDateScenario(1)
	s.existing.AlgorithmsVersion--
	delete(s.existing.Algorithms, failurereason.AlgorithmName)
	s.existing.Algorithms["failurereason-v1"] = struct{}{}
	s.existing.Clusters[1][0] = clustering.ClusterID{
		Algorithm: "failurereason-v1",
		ID:        "old-failure-reason-cluster-id",
	}
	clustering.SortClusters(s.existing.Clusters[1])
	return s
}

func fromOlderRuleAlgorithmScenario() *scenario {
	s := upToDateScenario(1)
	s.existing.AlgorithmsVersion--
	delete(s.existing.Algorithms, rulesalgorithm.AlgorithmName)
	s.existing.Algorithms["rules-v0"] = struct{}{}
	s.existing.Clusters[1] = []clustering.ClusterID{
		failureReasonClusterID(s.config, s.failures[1]),
		testNameClusterID(s.config, s.failures[1]),
		{Algorithm: "rules-v0", ID: s.rules[0].Rule.RuleID},
		{Algorithm: "rules-v0", ID: "rule-no-longer-matched-with-v1"},
	}
	clustering.SortClusters(s.existing.Clusters[1])
	return s
}

func fromLaterAlgorithmsScenario() *scenario {
	s := upToDateScenario(1)
	s.existing.AlgorithmsVersion = AlgorithmsVersion + 1
	s.existing.Algorithms = map[string]struct{}{
		"futurealgorithm-v1": {},
	}
	s.existing.Clusters = [][]clustering.ClusterID{
		{
			{Algorithm: "futurealgorithm-v1", ID: "aa"},
		},
		{
			{Algorithm: "futurealgorithm-v1", ID: "bb"},
		},
	}
	// As the algorithms version is later, the clustering
	// should be left completely untouched.
	s.expected = s.existing
	return s
}

func fromOlderRulesVersionScenario(size int) *scenario {
	s := upToDateScenario(size)
	s.existing.RulesVersion = s.existing.RulesVersion.Add(-1 * time.Hour)
	for i := 1; i <= size; i++ {
		s.existing.Clusters[i] = []clustering.ClusterID{
			failureReasonClusterID(s.config, s.failures[i]),
			testNameClusterID(s.config, s.failures[i]),
			ruleClusterID(s.rules[0].Rule.RuleID),
			ruleClusterID("now-deleted-rule-id"),
		}
		clustering.SortClusters(s.existing.Clusters[i])
	}
	return s
}

func fromNewerRulesVersionScenario() *scenario {
	s := upToDateScenario(1)
	s.existing.RulesVersion = s.existing.RulesVersion.Add(1 * time.Hour)
	s.existing.Clusters[1] = []clustering.ClusterID{
		failureReasonClusterID(s.config, s.failures[1]),
		testNameClusterID(s.config, s.failures[1]),
		ruleClusterID(s.rules[0].Rule.RuleID),
		ruleClusterID("later-added-rule-id"),
	}
	clustering.SortClusters(s.existing.Clusters[1])

	s.expected.RulesVersion = s.expected.RulesVersion.Add(1 * time.Hour)
	// Should keep existing rule clusters, as they are newer.
	s.expected.Clusters = s.existing.Clusters
	return s
}

func fromOlderConfigVersionScenario() *scenario {
	s := upToDateScenario(1)
	oldConfigVersion := s.existing.ConfigVersion.Add(-1 * time.Hour)
	s.existing.ConfigVersion = oldConfigVersion

	for _, cs := range s.existing.Clusters {
		for j := range cs {
			if cs[j].Algorithm == testname.AlgorithmName {
				cs[j].ID = hex.EncodeToString([]byte("old-test-cluster"))
			}
		}
		clustering.SortClusters(cs)
	}
	return s
}

func fromNewerConfigVersionScenario() *scenario {
	s := upToDateScenario(1)
	newConfigVersion := s.existing.ConfigVersion.Add(1 * time.Hour)
	s.existing.ConfigVersion = newConfigVersion

	for _, cs := range s.existing.Clusters {
		for j := range cs {
			if cs[j].Algorithm == testname.AlgorithmName {
				cs[j].ID = hex.EncodeToString([]byte("new-test-cluster"))
			}
		}
		clustering.SortClusters(cs)
	}

	s.expected.ConfigVersion = newConfigVersion
	// Should keep existing clusters, as they are newer.
	s.expected.Clusters = s.existing.Clusters
	return s
}

func fromScratchScenario(size int) *scenario {
	s := upToDateScenario(size)
	s.existing = NewEmptyClusterResults(len(s.failures))
	return s
}

func testNameClusterID(config *compiledcfg.ProjectConfig, failure *clustering.Failure) clustering.ClusterID {
	alg := &testname.Algorithm{}
	return clustering.ClusterID{
		Algorithm: testname.AlgorithmName,
		ID:        hex.EncodeToString(alg.Cluster(config, failure)),
	}
}

func failureReasonClusterID(config *compiledcfg.ProjectConfig, failure *clustering.Failure) clustering.ClusterID {
	alg := &failurereason.Algorithm{}
	return clustering.ClusterID{
		Algorithm: failurereason.AlgorithmName,
		ID:        hex.EncodeToString(alg.Cluster(config, failure)),
	}
}

func ruleClusterID(ruleID string) clustering.ClusterID {
	return clustering.ClusterID{
		Algorithm: rulesalgorithm.AlgorithmName,
		ID:        ruleID,
	}
}

// diffClusters checks actual and expected clusters are equivalent (after
// accounting for ordering differences). If not, a message explaining
// the differences is returned.
func diffClusters(actual [][]clustering.ClusterID, expected [][]clustering.ClusterID) string {
	if len(actual) != len(expected) {
		return fmt.Sprintf("got clusters for %v test results; want %v", len(actual), len(expected))
	}
	for i, actualClusters := range actual {
		expectedClusters := expected[i]
		expectedClusterSet := make(map[string]struct{})
		for _, e := range expectedClusters {
			expectedClusterSet[e.Key()] = struct{}{}
		}

		actualClusterSet := make(map[string]struct{})
		for _, e := range actualClusters {
			actualClusterSet[e.Key()] = struct{}{}
		}
		for j, a := range actualClusters {
			if _, ok := expectedClusterSet[a.Key()]; ok {
				delete(expectedClusterSet, a.Key())
			} else {
				return fmt.Sprintf("actual clusters for test result %v includes cluster %v at position %v, which is not expected", i, a.Key(), j)
			}
		}
		if len(expectedClusterSet) > 0 {
			var missingClusters []string
			for c := range expectedClusterSet {
				missingClusters = append(missingClusters, c)
			}
			return fmt.Sprintf("actual clusters for test result %v is missing cluster(s): %s", i, strings.Join(missingClusters, ", "))
		}
	}
	return ""
}
