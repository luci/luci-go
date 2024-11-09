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

package updater

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	"cloud.google.com/go/bigquery"

	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/failurereason"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/rulesalgorithm"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/testname"
	"go.chromium.org/luci/analysis/internal/config/compiledcfg"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func emptyMetricValues() map[metrics.ID]metrics.TimewiseCounts {
	result := make(map[metrics.ID]metrics.TimewiseCounts)
	for _, m := range metrics.ComputedMetrics {
		result[m.ID] = metrics.TimewiseCounts{}
	}
	return result
}

func makeTestNameCluster(t testing.TB, config *compiledcfg.ProjectConfig, uniqifier int) *analysis.Cluster {
	t.Helper()
	testID := fmt.Sprintf("testname-%v", uniqifier)
	return &analysis.Cluster{
		ClusterID: testIDClusterID(t, config, testID),
		MetricValues: map[metrics.ID]metrics.TimewiseCounts{
			metrics.Failures.ID: {
				OneDay:   metrics.Counts{Residual: 9},
				ThreeDay: metrics.Counts{Residual: 29},
				SevenDay: metrics.Counts{Residual: 69},
			},
		},
		TopTestIDs: []analysis.TopCount{{Value: testID, Count: 1}},
	}
}

func makeReasonCluster(t testing.TB, config *compiledcfg.ProjectConfig, uniqifier int) *analysis.Cluster {
	// Because the failure reason clustering algorithm removes numbers
	// when clustering failure reasons, it is better not to use the
	// uniqifier directly in the reason, to avoid cluster ID collisions.
	var foo strings.Builder
	for i := 0; i < uniqifier; i++ {
		foo.WriteString("foo")
	}
	reason := fmt.Sprintf("want %s, got bar", foo.String())

	return &analysis.Cluster{
		ClusterID: reasonClusterID(t, config, reason),
		MetricValues: map[metrics.ID]metrics.TimewiseCounts{
			metrics.Failures.ID: {
				OneDay:   metrics.Counts{Residual: 9},
				ThreeDay: metrics.Counts{Residual: 29},
				SevenDay: metrics.Counts{Residual: 69},
			},
		},
		TopTestIDs: []analysis.TopCount{
			{Value: fmt.Sprintf("testname-a-%v", uniqifier), Count: 1},
			{Value: fmt.Sprintf("testname-b-%v", uniqifier), Count: 1},
		},
		ExampleFailureReason: bigquery.NullString{Valid: true, StringVal: reason},
	}
}

func makeBugCluster(ruleID string) *analysis.Cluster {
	return &analysis.Cluster{
		ClusterID: bugClusterID(ruleID),
		MetricValues: map[metrics.ID]metrics.TimewiseCounts{
			metrics.Failures.ID: {
				OneDay:   metrics.Counts{Residual: 9},
				ThreeDay: metrics.Counts{Residual: 29},
				SevenDay: metrics.Counts{Residual: 69},
			},
		},
		TopTestIDs: []analysis.TopCount{{Value: "testname-0", Count: 1}},
	}
}

func testIDClusterID(t testing.TB, config *compiledcfg.ProjectConfig, testID string) clustering.ClusterID {
	t.Helper()
	testAlg, err := algorithms.SuggestingAlgorithm(testname.AlgorithmName)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())

	return clustering.ClusterID{
		Algorithm: testname.AlgorithmName,
		ID: hex.EncodeToString(testAlg.Cluster(config, &clustering.Failure{
			TestID: testID,
		})),
	}
}

func reasonClusterID(t testing.TB, config *compiledcfg.ProjectConfig, reason string) clustering.ClusterID {
	t.Helper()
	reasonAlg, err := algorithms.SuggestingAlgorithm(failurereason.AlgorithmName)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())

	return clustering.ClusterID{
		Algorithm: failurereason.AlgorithmName,
		ID: hex.EncodeToString(reasonAlg.Cluster(config, &clustering.Failure{
			Reason: &pb.FailureReason{PrimaryErrorMessage: reason},
		})),
	}
}

func bugClusterID(ruleID string) clustering.ClusterID {
	return clustering.ClusterID{
		Algorithm: rulesalgorithm.AlgorithmName,
		ID:        ruleID,
	}
}

type fakeAnalysisClient struct {
	clusters []*analysis.Cluster
}

func (f *fakeAnalysisClient) RebuildAnalysis(ctx context.Context) error {
	return nil
}

func (f *fakeAnalysisClient) PurgeStaleRows(ctx context.Context) error {
	return nil
}

func (f *fakeAnalysisClient) ReadImpactfulClusters(ctx context.Context, opts analysis.ImpactfulClusterReadOptions) ([]*analysis.Cluster, error) {
	var results []*analysis.Cluster
	for _, c := range f.clusters {
		include := opts.AlwaysIncludeBugClusters && c.ClusterID.IsBugCluster()
		for _, t := range opts.Thresholds {
			counts, ok := c.MetricValues[metrics.ID(t.MetricId)]
			if !ok {
				continue
			}
			include = include || meetsMetricThreshold(counts, t.Threshold)
		}
		if include {
			results = append(results, c)
		}
	}
	return results, nil
}

func meetsMetricThreshold(values metrics.TimewiseCounts, threshold *configpb.MetricThreshold) bool {
	return meetsThreshold(values.OneDay.Residual, threshold.OneDay) ||
		meetsThreshold(values.ThreeDay.Residual, threshold.ThreeDay) ||
		meetsThreshold(values.SevenDay.Residual, threshold.SevenDay)
}

func meetsThreshold(value int64, threshold *int64) bool {
	// threshold == nil is treated as an unsatisfiable threshold.
	return threshold != nil && value >= *threshold
}
