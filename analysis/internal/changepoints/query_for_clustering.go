// Copyright 2023 The LUCI Authors.
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

package changepoints

import (
	"context"
	"time"

	cpb "go.chromium.org/luci/analysis/internal/changepoints/proto"
	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	clusteringpb "go.chromium.org/luci/analysis/internal/clustering/proto"
	"go.chromium.org/luci/analysis/pbutil"
	"go.chromium.org/luci/common/errors"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"
)

// QueryStatsForClustering reads selected statistics for the
// test variant branch of nominated test verdicts. The statistics
// are those tracked in the clustered_failures table.
//
// The result slice will contain result items in 1:1 correspondance to
// the provided test verdicts (i.e. result[i] corresponds to tvs[i]).
// If no source information is available for some or all of the verdicts,
// the corresponding item in the response slice will be nil.
func QueryStatsForClustering(ctx context.Context, tvs []*rdbpb.TestVariant, project string, partitionTime time.Time, sourcesMap map[string]*rdbpb.Sources) ([]*clusteringpb.TestVariantBranch, error) {
	keys := make([]testvariantbranch.Key, 0, len(tvs))

	readIndexToResultIndex := make(map[int]int)
	for i, tv := range tvs {
		if tv.SourcesId == "" {
			continue
		}
		src := sourcesMap[tv.SourcesId]
		readIndexToResultIndex[len(keys)] = i
		keys = append(keys, testvariantbranch.Key{
			Project:     project,
			TestID:      tv.TestId,
			VariantHash: tv.VariantHash,
			RefHash:     testvariantbranch.RefHash(pbutil.SourceRefHash(pbutil.SourceRefFromSources(pbutil.SourcesFromResultDB(src)))),
		})
	}

	branches, err := testvariantbranch.Read(span.Single(ctx), keys)
	if err != nil {
		return nil, errors.Annotate(err, "read test variant branches").Err()
	}

	result := make([]*clusteringpb.TestVariantBranch, len(tvs))
	for i, branch := range branches {
		var resultItem *clusteringpb.TestVariantBranch
		if branch != nil {
			// If there are previous verdicts for test variant branch.
			stats := branch.MergedStatistics()
			resultItem = toSummary(stats, partitionTime)
		} else {
			// There are no previous known verdicts for the test variant branch,
			// so the statistics are zero.
			resultItem = &clusteringpb.TestVariantBranch{}
		}
		resultIndex := readIndexToResultIndex[i]
		result[resultIndex] = resultItem
	}
	return result, nil
}

func toSummary(stats *cpb.Statistics, partitionTime time.Time) *clusteringpb.TestVariantBranch {
	result := &clusteringpb.TestVariantBranch{}
	maxHour := int64(partitionTime.Truncate(time.Hour).Unix()) / 3600
	minHour := maxHour - 24
	for _, bucket := range stats.HourlyBuckets {
		if minHour < bucket.Hour && bucket.Hour <= maxHour {
			result.TotalVerdicts_24H += bucket.TotalVerdicts
			result.FlakyVerdicts_24H += bucket.FlakyVerdicts
			result.UnexpectedVerdicts_24H += bucket.UnexpectedVerdicts
		}
	}
	return result
}