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

package analysis

import (
	"context"
	"encoding/hex"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/analysis/internal/bqutil"
	"go.chromium.org/luci/analysis/internal/clustering"
	cpb "go.chromium.org/luci/analysis/internal/clustering/proto"
	"go.chromium.org/luci/analysis/pbutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// ClusteringHandler handles test result (re-)clustering events, to
// ensure analysis remains up-to-date.
type ClusteringHandler struct {
	cfClient ClusteredFailuresClient
}

// ClusteredFailuresClient exports clustered failures to BigQuery for
// further analysis.
type ClusteredFailuresClient interface {
	// Insert inserts the given rows into BigQuery.
	Insert(ctx context.Context, rows []*bqpb.ClusteredFailureRow) error
}

func NewClusteringHandler(cf ClusteredFailuresClient) *ClusteringHandler {
	return &ClusteringHandler{
		cfClient: cf,
	}
}

// HandleUpdatedClusters handles (re-)clustered test results. It is called
// after the spanner transaction effecting the (re-)clustering has committed.
// commitTime is the Spanner time the transaction committed.
//
// If this method fails, it will not be retried and data loss or inconsistency
// (in this method's BigQuery export) may occur. This could be improved in
// future with a two-stage apply process (journalling the BigQuery updates
// to be applied as part of the original transaction and retrying them at
// a later point if they do not succeed).
func (r *ClusteringHandler) HandleUpdatedClusters(ctx context.Context, updates *clustering.Update, commitTime time.Time) error {
	rowUpdates, err := prepareInserts(updates, commitTime)
	if err != nil {
		return errors.Annotate(err, "prepare rows to insert").Err()
	}
	if err := r.cfClient.Insert(ctx, rowUpdates); err != nil {
		return errors.Annotate(err, "inserting %d clustered failure rows", len(rowUpdates)).Err()
	}
	return nil
}

// prepareInserts prepares entries into the BigQuery clustered failures table
// in response to a (re-)clustering. For efficiency, only the updated rows are
// returned.
func prepareInserts(updates *clustering.Update, commitTime time.Time) ([]*bqpb.ClusteredFailureRow, error) {
	var result []*bqpb.ClusteredFailureRow
	for _, u := range updates.Updates {
		deleted := make(map[string]clustering.ClusterID)
		retained := make(map[string]clustering.ClusterID)
		new := make(map[string]clustering.ClusterID)

		previousInBugCluster := false
		for _, pc := range u.PreviousClusters {
			deleted[pc.Key()] = pc
			if pc.IsBugCluster() {
				previousInBugCluster = true
			}
		}
		newInBugCluster := false
		for _, nc := range u.NewClusters {
			key := nc.Key()
			if _, ok := deleted[key]; ok {
				delete(deleted, key)
				retained[key] = nc
			} else {
				new[key] = nc
			}
			if nc.IsBugCluster() {
				newInBugCluster = true
			}
		}
		// Create rows for deletions.
		for _, dc := range deleted {
			isIncluded := false
			isIncludedWithHighPriority := false
			row, err := entryFromUpdate(updates.Project, updates.ChunkID, dc, u.TestResult, isIncluded, isIncludedWithHighPriority, commitTime)
			if err != nil {
				return nil, err
			}
			result = append(result, row)
		}
		// Create rows for retained clusters for which inclusion was modified.
		for _, rc := range retained {
			isIncluded := true
			// A failure will appear with high priority in any bug clusters
			// it appears in, and if it appears in no bug clusters, it will
			// appear with high priority in any suggested clusters it appears
			// in.
			previousIncludedWithHighPriority := rc.IsBugCluster() || !previousInBugCluster
			newIncludedWithHighPriority := rc.IsBugCluster() || !newInBugCluster
			if previousIncludedWithHighPriority == newIncludedWithHighPriority {
				// The inclusion status of the test result in the cluster has not changed.
				// For efficiency, do not stream an update.
				continue
			}
			row, err := entryFromUpdate(updates.Project, updates.ChunkID, rc, u.TestResult, isIncluded, newIncludedWithHighPriority, commitTime)
			if err != nil {
				return nil, err
			}
			result = append(result, row)
		}
		// Create rows for new clusters.
		for _, nc := range new {
			isIncluded := true
			// A failure will appear with high priority in any bug clusters
			// it appears in, and if it appears in no bug clusters, it will
			// appear with high priority in any suggested clusters it appears
			// in.
			isIncludedWithHighPriority := nc.IsBugCluster() || !newInBugCluster
			row, err := entryFromUpdate(updates.Project, updates.ChunkID, nc, u.TestResult, isIncluded, isIncludedWithHighPriority, commitTime)
			if err != nil {
				return nil, err
			}
			result = append(result, row)
		}
	}
	return result, nil
}

func entryFromUpdate(project, chunkID string, cluster clustering.ClusterID, failure *cpb.Failure, included, includedWithHighPriority bool, commitTime time.Time) (*bqpb.ClusteredFailureRow, error) {
	// Copy the failure, to ensure the returned ClusteredFailure does not
	// alias any of the original failure's nested message protos.
	failure = proto.Clone(failure).(*cpb.Failure)

	exonerations := make([]*bqpb.ClusteredFailureRow_TestExoneration, 0, len(failure.Exonerations))
	for _, e := range failure.Exonerations {
		exonerations = append(exonerations, &bqpb.ClusteredFailureRow_TestExoneration{
			Reason: e.Reason,
		})
	}

	testIDStructured, err := bqutil.StructuredTestIdentifier(failure.TestId, failure.Variant)
	if err != nil {
		// This should not happen. It means we ingested a bad test identifier.
		return nil, errors.Annotate(err, "structured test identifier").Err()
	}
	entry := &bqpb.ClusteredFailureRow{
		ClusterAlgorithm: cluster.Algorithm,
		ClusterId:        cluster.ID,
		TestResultSystem: failure.TestResultId.System,
		TestResultId:     failure.TestResultId.Id,
		LastUpdated:      timestamppb.New(commitTime),
		Project:          project,

		PartitionTime: failure.PartitionTime,

		IsIncluded:                 included,
		IsIncludedWithHighPriority: includedWithHighPriority,

		ChunkId:    chunkID,
		ChunkIndex: failure.ChunkIndex,

		Realm:                failure.Realm,
		TestIdStructured:     testIDStructured,
		TestId:               failure.TestId,
		Variant:              variantPairs(failure.Variant),
		Tags:                 failure.Tags,
		VariantHash:          failure.VariantHash,
		FailureReason:        failure.FailureReason,
		BugTrackingComponent: failure.BugTrackingComponent,
		StartTime:            failure.StartTime,
		Duration:             failure.Duration.AsDuration().Seconds(),
		Exonerations:         exonerations,

		BuildStatus:                   ToBQBuildStatus(failure.BuildStatus),
		BuildCritical:                 failure.BuildCritical != nil && *failure.BuildCritical,
		IngestedInvocationId:          failure.IngestedInvocationId,
		IngestedInvocationResultIndex: failure.IngestedInvocationResultIndex,
		IngestedInvocationResultCount: failure.IngestedInvocationResultCount,
		IsIngestedInvocationBlocked:   failure.IsIngestedInvocationBlocked,
		TestRunId:                     failure.TestRunId,
		TestRunResultIndex:            failure.TestRunResultIndex,
		TestRunResultCount:            failure.TestRunResultCount,
		IsTestRunBlocked:              failure.IsTestRunBlocked,
		BuildGardenerRotations:        failure.BuildGardenerRotations,
		TestVariantBranch:             testVariantBranch(failure.TestVariantBranch),
	}
	if failure.Sources != nil {
		entry.Sources = failure.Sources
		ref := pbutil.SourceRefFromSources(failure.Sources)
		entry.SourceRef = ref
		entry.SourceRefHash = hex.EncodeToString(pbutil.SourceRefHash(ref))
	}

	if failure.PresubmitRun != nil {
		entry.PresubmitRunId = failure.PresubmitRun.PresubmitRunId
		entry.PresubmitRunOwner = failure.PresubmitRun.Owner
		entry.PresubmitRunMode = ToBQPresubmitRunMode(failure.PresubmitRun.Mode)
		entry.PresubmitRunStatus = ToBQPresubmitRunStatus(failure.PresubmitRun.Status)
	}
	return entry, nil
}

func testVariantBranch(tvb *cpb.TestVariantBranch) *bqpb.ClusteredFailureRow_TestVariantBranch {
	if tvb == nil {
		return nil
	}
	return &bqpb.ClusteredFailureRow_TestVariantBranch{
		FlakyVerdicts_24H:      tvb.FlakyVerdicts_24H,
		UnexpectedVerdicts_24H: tvb.UnexpectedVerdicts_24H,
		TotalVerdicts_24H:      tvb.TotalVerdicts_24H,
	}
}

func variantPairs(v *pb.Variant) []*pb.StringPair {
	var result []*pb.StringPair
	for k, v := range v.Def {
		result = append(result, &pb.StringPair{
			Key:   k,
			Value: v,
		})
	}
	return result
}
