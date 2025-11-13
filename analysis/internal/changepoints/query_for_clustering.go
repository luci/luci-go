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

	"go.chromium.org/luci/common/errors"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	clusteringpb "go.chromium.org/luci/analysis/internal/clustering/proto"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// QueryStatsForClustering reads selected statistics for the
// test variant branch of nominated test verdicts. The statistics
// are those tracked in the clustered_failures table.
//
// The result slice will contain result items in 1:1 correspondance to
// the provided test verdicts (i.e. result[i] corresponds to tvs[i]).
// If no source information is available for some or all of the verdicts,
// the corresponding item in the response slice will be nil.
func QueryStatsForClustering(ctx context.Context, tvs []*rdbpb.TestVariant, project string, partitionTime time.Time, sourcesMap map[string]*pb.Sources) ([]*clusteringpb.TestVariantBranch, error) {
	keys := make([]testvariantbranch.Key, 0, len(tvs))

	readIndexToResultIndex := make(map[int]int)
	for i, tv := range tvs {
		if tv.SourcesId == "" {
			continue
		}
		src := sourcesMap[tv.SourcesId]
		if src.BaseSources == nil {
			// There is no SourceRef for this result.
			continue
		}
		readIndexToResultIndex[len(keys)] = i
		keys = append(keys, testvariantbranch.Key{
			Project:     project,
			TestID:      tv.TestId,
			VariantHash: tv.VariantHash,
			RefHash:     testvariantbranch.RefHash(pbutil.SourceRefHash(pbutil.SourceRefFromSources(src))),
		})
	}

	result := make([]*clusteringpb.TestVariantBranch, len(tvs))
	f := func(i int, e *testvariantbranch.Entry) error {
		var resultItem *clusteringpb.TestVariantBranch
		if e != nil {
			// If there are previous verdicts for test variant branch.
			stats := e.HourlyStatistics()
			resultItem = toSummary(stats, partitionTime)
		} else {
			// There are no previous known verdicts for the test variant branch,
			// so the statistics are zero.
			resultItem = &clusteringpb.TestVariantBranch{}
		}
		resultIndex := readIndexToResultIndex[i]
		result[resultIndex] = resultItem
		return nil
	}
	// We might read up to 10,000 test variant branches in this method
	// (as at writing, this is the maximum number of verdicts that can
	// be ingested in one task).
	// As each deserialized test variant branch can take 10 or more kilobytes
	// of memory, reading all of them may ordinarily require a hundred or more MB
	// of memory.
	// By using ReadF instead of Read, we allow each deserialized test variant
	// to be garbage collected as it is used, and only hold onto the summaries,
	// which requires far less memory.
	err := testvariantbranch.ReadF(span.Single(ctx), keys, f)
	if err != nil {
		return nil, errors.Fmt("read test variant branches: %w", err)
	}
	return result, nil
}

func toSummary(statsByHour map[int64]testvariantbranch.HourlyStats, partitionTime time.Time) *clusteringpb.TestVariantBranch {
	result := &clusteringpb.TestVariantBranch{}
	maxHour := int64(partitionTime.Truncate(time.Hour).Unix()) / 3600
	minHour := maxHour - 24
	for hour, bucket := range statsByHour {
		if minHour < hour && hour <= maxHour {
			result.TotalVerdicts_24H += bucket.TotalSourceVerdicts
			result.FlakyVerdicts_24H += bucket.FlakySourceVerdicts
			result.UnexpectedVerdicts_24H += bucket.UnexpectedSourceVerdicts
		}
	}
	return result
}
