// Copyright 2025 The LUCI Authors.
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

// Package verdictingester defines the top-level task queue which ingests
// test verdicts from ResultDB and pushes it into LUCI Analysis's analysis
// and BigQuery export pipelines.
package verdictingester

import (
	"context"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/analysis/internal/changepoints"
	"go.chromium.org/luci/analysis/internal/clustering/ingestion"
	clusteringpb "go.chromium.org/luci/analysis/internal/clustering/proto"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/tracing"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// ClusteringExporter is an ingestion stage that exports test results for clustering.
// It implements IngestionSink.
type ClusteringExporter struct {
	clustering *ingestion.Ingester
}

// Name returns a unique name for the ingestion stage.
func (ClusteringExporter) Name() string {
	return "export-clustering"
}

// Ingest exports the provided test results for clustering.
func (e *ClusteringExporter) Ingest(ctx context.Context, input Inputs) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/verdictingester.ClusteringExporter.Ingest")
	defer func() { tracing.End(s, err) }()

	// Exit early if no verdict needs to be ingested.
	if len(input.Verdicts) == 0 {
		return nil
	}
	ing, err := extractIngestionContext(input.Payload, input.Invocation)
	if err != nil {
		return err
	}
	payload := input.Payload
	if payload.Build == nil {
		// TODO: support clustering for invocation without a build.
		return nil
	}
	cfg, err := config.Project(ctx, ing.Project)
	if err != nil {
		return errors.Fmt("read project config: %w", err)
	}

	// Setup clustering ingestion.
	opts := ingestion.Options{
		TaskIndex:                 payload.TaskIndex,
		Project:                   ing.Project,
		PartitionTime:             ing.PartitionTime,
		Realm:                     ing.Project + ":" + ing.SubRealm,
		InvocationID:              ing.IngestedInvocationID,
		BuildStatus:               payload.Build.Status,
		BuildGardenerRotations:    payload.Build.GardenerRotations,
		PreferBuganizerComponents: cfg.BugManagement.GetDefaultBugSystem() != configpb.BugSystem_MONORAIL,
	}

	if payload.PresubmitRun != nil {
		opts.PresubmitRun = &ingestion.PresubmitRun{
			ID:     payload.PresubmitRun.PresubmitRunId,
			Owner:  payload.PresubmitRun.Owner,
			Mode:   payload.PresubmitRun.Mode,
			Status: payload.PresubmitRun.Status,
		}
		opts.BuildCritical = payload.PresubmitRun.Critical
		if payload.PresubmitRun.Critical && ing.BuildStatus == pb.BuildStatus_BUILD_STATUS_FAILURE &&
			payload.PresubmitRun.Status == pb.PresubmitRunStatus_PRESUBMIT_RUN_STATUS_SUCCEEDED {
			logging.Warningf(ctx, "Inconsistent data from LUCI CV: build %v/%v was critical to presubmit run %v/%v and failed, but presubmit run succeeded.",
				payload.Build.Host, payload.Build.Id, payload.PresubmitRun.PresubmitRunId.System, payload.PresubmitRun.PresubmitRunId.Id)
		}
	}

	failingRDBVerdicts := filterToTestVariantsWithUnexpectedFailures(input.Verdicts)

	changepointPartitionTime := payload.Invocation.CreationTime.AsTime()
	testVariantBranchStats, err := queryTestVariantAnalysisForClustering(ctx, failingRDBVerdicts, ing.Project, changepointPartitionTime, input.SourcesByID)
	if err != nil {
		return errors.Fmt("query test variant analysis for clustering: %w", err)
	}

	verdicts := make([]ingestion.TestVerdict, 0, len(failingRDBVerdicts))
	for i, tv := range failingRDBVerdicts {
		var s *pb.Sources
		if tv.SourcesId != "" {
			s = input.SourcesByID[tv.SourcesId]
		}
		verdicts = append(verdicts, ingestion.TestVerdict{
			Verdict:           tv,
			Sources:           s,
			TestVariantBranch: testVariantBranchStats[i],
		})
	}

	// Clustering ingestion is designed to behave gracefully in case of
	// a task retry. Given the same options and same test variants (in
	// the same order), the IDs and content of the chunks it writes is
	// designed to be stable. If chunks already exist, it will skip them.
	if err := e.clustering.Ingest(ctx, opts, verdicts); err != nil {
		err = errors.Fmt("ingesting for clustering: %w", err)
		return transient.Tag.Apply(err)
	}
	return nil
}

// queryTestVariantAnalysisForClustering queries test variant analysis for
// the specified test verdicts. The returned slice has exactly one entry
// for each verdict in `tvs`. If analysis is not available for a given
// verdict, the corresponding item in the response will be nil.
func queryTestVariantAnalysisForClustering(ctx context.Context, tvs []*rdbpb.TestVariant, project string, partitionTime time.Time, sourcesMap map[string]*pb.Sources) (tvbs []*clusteringpb.TestVariantBranch, err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/verdictingester.queryTestVariantAnalysisForClustering")
	defer func() { tracing.End(s, err) }()

	cfg, err := config.Get(ctx)
	if err != nil {
		return nil, errors.Fmt("read config: %w", err)
	}
	tvaQueriesEnabled := cfg.TestVariantAnalysis != nil && cfg.TestVariantAnalysis.Enabled &&
		cfg.Clustering != nil && cfg.Clustering.QueryTestVariantAnalysisEnabled

	var result []*clusteringpb.TestVariantBranch
	if tvaQueriesEnabled {
		var err error
		result, err = changepoints.QueryStatsForClustering(ctx, tvs, project, partitionTime, sourcesMap)
		if err != nil {
			return nil, errors.Fmt("read test variant branch analysis: %w", err)
		}
	} else {
		// Use nil analysis for each verdict.
		result = make([]*clusteringpb.TestVariantBranch, len(tvs))
	}
	return result, nil
}
