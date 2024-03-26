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

// Package verdictingester defines the top-level task queue which ingests
// test verdicts from ResultDB and pushes it into LUCI Analysis's analysis
// and BigQuery export pipelines.
package verdictingester

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/analysis/clusteredfailures"
	"go.chromium.org/luci/analysis/internal/changepoints"
	tvbexporter "go.chromium.org/luci/analysis/internal/changepoints/bqexporter"
	"go.chromium.org/luci/analysis/internal/clustering/chunkstore"
	"go.chromium.org/luci/analysis/internal/clustering/ingestion"
	clusteringpb "go.chromium.org/luci/analysis/internal/clustering/proto"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/ingestion/control"
	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testresults/gerritchangelists"
	"go.chromium.org/luci/analysis/internal/testverdicts"
	"go.chromium.org/luci/analysis/internal/tracing"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"

	// Add support for Spanner transactions in TQ.
	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

const (
	resultIngestionTaskClass  = "result-ingestion"
	resultIngestionQueue      = "result-ingestion"
	verdictIngestionTaskClass = "verdict-ingestion"
	verdictIngestionQueue     = "verdict-ingestion"

	// ingestionEarliest is the oldest data that may be ingested by
	// LUCI Analysis.
	// This is an offset relative to the current time, and should be kept
	// in sync with the data retention period in Spanner and BigQuery.
	ingestionEarliest = -90 * 24 * time.Hour

	// ingestionLatest is the newest data that may be ingested by
	// LUCI Analysis.
	// This is an offset relative to the current time. It is designed to
	// allow for clock drift.
	ingestionLatest = 24 * time.Hour
)

var (
	taskCounter = metric.NewCounter(
		"analysis/ingestion/task_completion",
		"The number of completed LUCI Analysis ingestion tasks, by build project and outcome.",
		nil,
		// The LUCI Project.
		field.String("project"),
		// "success", "failed_validation",
		// "ignored_no_invocation", "ignored_has_ancestor",
		// "ignored_invocation_not_found", "ignored_resultdb_permission_denied",
		// "ignored_project_not_allowlisted".
		field.String("outcome"))

	testVariantReadMask = &fieldmaskpb.FieldMask{
		Paths: []string{
			"test_id",
			"variant_hash",
			"status",
			"variant",
			"test_metadata",
			"sources_id",
			"exonerations.*.explanation_html",
			"exonerations.*.reason",
			"results.*.result.name",
			"results.*.result.result_id",
			"results.*.result.expected",
			"results.*.result.status",
			"results.*.result.summary_html",
			"results.*.result.start_time",
			"results.*.result.duration",
			"results.*.result.tags",
			"results.*.result.failure_reason",
			"results.*.result.skip_reason",
			"results.*.result.properties",
		},
	}

	buildReadMask = &field_mask.FieldMask{
		Paths: []string{"builder", "infra.resultdb", "status", "input", "output", "ancestor_ids"},
	}
)

// Options configures test result ingestion.
type Options struct {
}

type verdictIngester struct {
	// clustering is used to ingest test failures for clustering.
	clustering *ingestion.Ingester
	// verdictExporter is used to export test verdictExporter.
	verdictExporter *testverdicts.Exporter
	// testVariantBranchExporter is use to export change point analysis results.
	testVariantBranchExporter *tvbexporter.Exporter
}

var verdictIngestion = tq.RegisterTaskClass(tq.TaskClass{
	ID:        verdictIngestionTaskClass,
	Prototype: &taskspb.IngestTestVerdicts{},
	Queue:     verdictIngestionQueue,
	Kind:      tq.Transactional,
})

var resultIngestion = tq.RegisterTaskClass(tq.TaskClass{
	ID:        resultIngestionTaskClass,
	Prototype: &taskspb.IngestTestResults{},
	Queue:     resultIngestionQueue,
	Kind:      tq.Transactional,
})

// RegisterTaskHandler registers the handler for result ingestion tasks.
func RegisterTaskHandler(srv *server.Server) error {
	ctx := srv.Context
	cfg, err := config.Get(ctx)
	if err != nil {
		return err
	}
	chunkStore, err := chunkstore.NewClient(ctx, cfg.ChunkGcsBucket)
	if err != nil {
		return err
	}
	srv.RegisterCleanup(func(context.Context) {
		chunkStore.Close()
	})

	cf, err := clusteredfailures.NewClient(ctx, srv.Options.CloudProject)
	if err != nil {
		return err
	}
	srv.RegisterCleanup(func(context.Context) {
		err := cf.Close()
		if err != nil {
			logging.Errorf(ctx, "Cleaning up clustered failures client: %s", err)
		}
	})

	verdictClient, err := testverdicts.NewClient(ctx, srv.Options.CloudProject)
	if err != nil {
		return err
	}
	srv.RegisterCleanup(func(ctx context.Context) {
		err := verdictClient.Close()
		if err != nil {
			logging.Errorf(ctx, "Cleaning up test verdicts client: %s", err)
		}
	})

	tvbBQClient, err := tvbexporter.NewClient(ctx, srv.Options.CloudProject)
	if err != nil {
		return err
	}
	srv.RegisterCleanup(func(ctx context.Context) {
		err := tvbBQClient.Close()
		if err != nil {
			logging.Errorf(ctx, "Cleaning up test variant branch BQExporter client: %s", err)
		}
	})

	analysis := analysis.NewClusteringHandler(cf)
	ri := &verdictIngester{
		clustering:                ingestion.New(chunkStore, analysis),
		verdictExporter:           testverdicts.NewExporter(verdictClient),
		testVariantBranchExporter: tvbexporter.NewExporter(tvbBQClient),
	}
	verdictHandler := func(ctx context.Context, payload proto.Message) error {
		task := payload.(*taskspb.IngestTestVerdicts)
		return ri.ingestTestVerdicts(ctx, task)
	}
	verdictIngestion.AttachHandler(verdictHandler)

	resultHandler := func(ctx context.Context, payload proto.Message) error {
		task := payload.(*taskspb.IngestTestResults)
		itvTask := &taskspb.IngestTestVerdicts{
			PartitionTime: task.PartitionTime,
			Build:         task.Build,
			PresubmitRun:  task.PresubmitRun,
			PageToken:     task.PageToken,
			TaskIndex:     task.TaskIndex,
		}
		return ri.ingestTestVerdicts(ctx, itvTask)
	}
	resultIngestion.AttachHandler(resultHandler)

	return nil
}

// Schedule enqueues a task to ingest test verdicts from a build.
func Schedule(ctx context.Context, task *taskspb.IngestTestVerdicts) {
	tq.MustAddTask(ctx, &tq.Task{
		Title:   fmt.Sprintf("%s-%s-%d-page-%v", task.Build.Project, task.Build.Host, task.Build.Id, task.TaskIndex),
		Payload: task,
	})
}

func (i *verdictIngester) ingestTestVerdicts(ctx context.Context, payload *taskspb.IngestTestVerdicts) error {
	if err := validateRequest(ctx, payload); err != nil {
		project := "(unknown)"
		if payload.GetBuild().GetProject() != "" {
			project = payload.Build.Project
		}
		taskCounter.Add(ctx, 1, project, "failed_validation")
		return tq.Fatal.Apply(err)
	}

	isProjectEnabled, err := isProjectEnabledForIngestion(ctx, payload.Build.Project)
	if err != nil {
		return transient.Tag.Apply(err)
	}
	if !isProjectEnabled {
		taskCounter.Add(ctx, 1, payload.Build.Project, "ignored_project_not_allowlisted")
		return nil
	}

	if !payload.Build.HasInvocation {
		// Build does not have a ResultDB invocation to ingest.
		logging.Debugf(ctx, "Skipping ingestion of build %s-%d because it has no ResultDB invocation.",
			payload.Build.Host, payload.Build.Id)
		taskCounter.Add(ctx, 1, payload.Build.Project, "ignored_no_invocation")
		return nil
	}

	if payload.Build.IsIncludedByAncestor {
		// Yes. Do not ingest this build to avoid ingesting the same test
		// results multiple times.
		taskCounter.Add(ctx, 1, payload.Build.Project, "ignored_has_ancestor")
		return nil
	}

	rdbHost := payload.Build.ResultdbHost
	invName := control.BuildInvocationName(payload.Build.Id)
	rc, err := resultdb.NewClient(ctx, rdbHost, payload.Build.Project)
	if err != nil {
		return transient.Tag.Apply(err)
	}
	inv, err := rc.GetInvocation(ctx, invName)
	code := status.Code(err)
	if code == codes.NotFound {
		// Invocation not found, end the task gracefully.
		logging.Warningf(ctx, "Invocation %s for project %s not found.",
			invName, payload.Build.Project)
		taskCounter.Add(ctx, 1, payload.Build.Project, "ignored_invocation_not_found")
		return nil
	}
	if code == codes.PermissionDenied {
		// Invocation not found, end the task gracefully.
		logging.Warningf(ctx, "Permission denied to read invocation %s for project %s.",
			invName, payload.Build.Project)
		taskCounter.Add(ctx, 1, payload.Build.Project, "ignored_resultdb_permission_denied")
		return nil
	}
	if err != nil {
		logging.Warningf(ctx, "GetInvocation has error code %s.", code)
		return transient.Tag.Apply(errors.Annotate(err, "get invocation").Err())
	}

	ingestion, err := extractIngestionContext(payload, inv)
	if err != nil {
		return err
	}

	// Query test variants from ResultDB.
	req := &rdbpb.QueryTestVariantsRequest{
		Invocations: []string{inv.Name},
		ResultLimit: 100,
		PageSize:    10000,
		ReadMask:    testVariantReadMask,
		PageToken:   payload.PageToken,
	}
	rsp, err := rc.QueryTestVariants(ctx, req)
	if err != nil {
		err = errors.Annotate(err, "query test variants").Err()
		return transient.Tag.Apply(err)
	}

	sources, err := gerritchangelists.PopulateOwnerKinds(ctx, payload.Build.Project, rsp.Sources)
	if err != nil {
		err = errors.Annotate(err, "populate changelist owner kinds").Err()
		return transient.Tag.Apply(err)
	}

	// Schedule a task to deal with the next page of verdicts (if needed).
	// Do this immediately, so that task can commence while we are still
	// inserting the verdicts for this page.
	if rsp.NextPageToken != "" {
		if err := scheduleNextTask(ctx, payload, rsp.NextPageToken); err != nil {
			err = errors.Annotate(err, "schedule next task").Err()
			return transient.Tag.Apply(err)
		}
	}

	// Record the test verdicts for test history.
	err = recordTestResults(ctx, ingestion, rsp.TestVariants, sources)
	if err != nil {
		// If any transaction failed, the task will be retried and the tables will be
		// eventual-consistent.
		return errors.Annotate(err, "record test verdicts").Err()
	}

	nextPageToken := rsp.NextPageToken

	// Ingest for test variant analysis (change point analysis).
	// Note that this is different from the ingestForTestVariantAnalysis below
	// which should eventually be removed.
	// See go/luci-test-variant-analysis-design for details.
	err = ingestForChangePointAnalysis(ctx, i.testVariantBranchExporter, rsp.TestVariants, sources, payload)
	if err != nil {
		return errors.Annotate(err, "change point analysis").Err()
	}

	// Ingest the test verdicts for clustering. This should occur
	// after test variant analysis ingestion as it queries the results of the
	// above analysis.
	err = ingestForClustering(ctx, i.clustering, payload, ingestion, rsp.TestVariants, sources)
	if err != nil {
		return err
	}

	err = ingestForVerdictExport(ctx, i.verdictExporter, rsp.TestVariants, sources, inv, payload)
	if err != nil {
		return errors.Annotate(err, "export verdicts").Err()
	}

	if nextPageToken == "" {
		// In the last task.
		taskCounter.Add(ctx, 1, payload.Build.Project, "success")
	}
	return nil
}

// isProjectEnabledForIngestion returns if the LUCI project is enabled for
// ingestion. By default, all LUCI projects are enabled for ingestion, but
// it is possible to limit ingestion to an allowlisted set in the
// service configuration.
func isProjectEnabledForIngestion(ctx context.Context, project string) (bool, error) {
	cfg, err := config.Get(ctx)
	if err != nil {
		return false, errors.Annotate(err, "get service config").Err()
	}
	if !cfg.Ingestion.GetProjectAllowlistEnabled() {
		return true, nil
	}
	allowList := cfg.Ingestion.GetProjectAllowlist()
	for _, entry := range allowList {
		if project == entry {
			return true, nil
		}
	}
	return false, nil
}

// filterToTestVariantsWithUnexpectedFailures filters the given list of
// test variants to only those with unexpected failures.
func filterToTestVariantsWithUnexpectedFailures(tvs []*rdbpb.TestVariant) []*rdbpb.TestVariant {
	var results []*rdbpb.TestVariant
	for _, tv := range tvs {
		if hasUnexpectedFailures(tv) {
			results = append(results, tv)
		}
	}
	return results
}

func hasUnexpectedFailures(tv *rdbpb.TestVariant) bool {
	if tv.Status == rdbpb.TestVariantStatus_UNEXPECTEDLY_SKIPPED ||
		tv.Status == rdbpb.TestVariantStatus_EXPECTED {
		return false
	}

	for _, trb := range tv.Results {
		tr := trb.Result
		if !tr.Expected && tr.Status != rdbpb.TestStatus_PASS && tr.Status != rdbpb.TestStatus_SKIP {
			// If any result is an unexpected failure, LUCI Analysis should save this test variant.
			return true
		}
	}
	return false
}

// scheduleNextTask schedules a task to continue the ingestion,
// starting at the given page token.
// If a continuation task for this task has been previously scheduled
// (e.g. in a previous try of this task), this method does nothing.
func scheduleNextTask(ctx context.Context, task *taskspb.IngestTestVerdicts, nextPageToken string) error {
	if nextPageToken == "" {
		// If the next page token is "", it means ResultDB returned the
		// last page. We should not schedule a continuation task.
		panic("next page token cannot be the empty page token")
	}
	buildID := control.BuildID(task.Build.Host, task.Build.Id)

	// Schedule the task transactionally, conditioned on it not having been
	// scheduled before.
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		entries, err := control.Read(ctx, []string{buildID})
		if err != nil {
			return errors.Annotate(err, "read ingestion record").Err()
		}

		entry := entries[0]
		if entry == nil {
			return errors.Reason("build %v does not have ingestion record", buildID).Err()
		}
		if task.TaskIndex >= entry.TaskCount {
			// This should nver happen.
			panic("current ingestion task not recorded on ingestion control record")
		}
		nextTaskIndex := task.TaskIndex + 1
		if nextTaskIndex != entry.TaskCount {
			// Next task has already been created in the past. Do not create
			// it again.
			// This can happen if the ingestion task failed after
			// it scheduled the ingestion task for the next page,
			// and was subsequently retried.
			return nil
		}
		entry.TaskCount = entry.TaskCount + 1
		if err := control.InsertOrUpdate(ctx, entry); err != nil {
			return errors.Annotate(err, "update ingestion record").Err()
		}

		itvTask := &taskspb.IngestTestVerdicts{
			PartitionTime: task.PartitionTime,
			Build:         task.Build,
			PresubmitRun:  task.PresubmitRun,
			PageToken:     nextPageToken,
			TaskIndex:     nextTaskIndex,
		}
		Schedule(ctx, itvTask)

		return nil
	})
	return err
}

func ingestForClustering(ctx context.Context, clustering *ingestion.Ingester, payload *taskspb.IngestTestVerdicts, ing *IngestionContext, testVariants []*rdbpb.TestVariant, sources map[string]*pb.Sources) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/verdictingester.ingestForClustering")
	defer func() { tracing.End(s, err) }()

	cfg, err := config.Project(ctx, ing.Project)
	if err != nil {
		return errors.Annotate(err, "read project config").Err()
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

	failingRDBVerdicts := filterToTestVariantsWithUnexpectedFailures(testVariants)
	testVariantBranchStats, err := queryTestVariantAnalysisForClustering(ctx, failingRDBVerdicts, ing.Project, ing.PartitionTime, sources)
	if err != nil {
		return errors.Annotate(err, "query test variant analysis for clustering").Err()
	}

	verdicts := make([]ingestion.TestVerdict, 0, len(failingRDBVerdicts))
	for i, tv := range failingRDBVerdicts {
		var s *pb.Sources
		if tv.SourcesId != "" {
			s = sources[tv.SourcesId]
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
	if err := clustering.Ingest(ctx, opts, verdicts); err != nil {
		err = errors.Annotate(err, "ingesting for clustering").Err()
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
		return nil, errors.Annotate(err, "read config").Err()
	}
	tvaQueriesEnabled := cfg.TestVariantAnalysis != nil && cfg.TestVariantAnalysis.Enabled &&
		cfg.Clustering != nil && cfg.Clustering.QueryTestVariantAnalysisEnabled

	var result []*clusteringpb.TestVariantBranch
	if tvaQueriesEnabled {
		var err error
		result, err = changepoints.QueryStatsForClustering(ctx, tvs, project, partitionTime, sourcesMap)
		if err != nil {
			return nil, errors.Annotate(err, "read test variant branch analysis").Err()
		}
	} else {
		// Use nil analysis for each verdict.
		result = make([]*clusteringpb.TestVariantBranch, len(tvs))
	}
	return result, nil
}

func ingestForChangePointAnalysis(ctx context.Context, exporter *tvbexporter.Exporter, testVariants []*rdbpb.TestVariant, sources map[string]*pb.Sources, payload *taskspb.IngestTestVerdicts) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/verdictingester.ingestForChangePointAnalysis")
	defer func() { tracing.End(s, err) }()

	cfg, err := config.Get(ctx)
	if err != nil {
		return errors.Annotate(err, "read config").Err()
	}
	tvaEnabled := cfg.TestVariantAnalysis != nil && cfg.TestVariantAnalysis.Enabled
	if !tvaEnabled {
		return nil
	}
	err = changepoints.Analyze(ctx, testVariants, payload, sources, exporter)
	if err != nil {
		return errors.Annotate(err, "analyze test variants").Err()
	}
	return nil
}

func ingestForVerdictExport(ctx context.Context, verdictExporter *testverdicts.Exporter,
	testVariants []*rdbpb.TestVariant, sources map[string]*pb.Sources, inv *rdbpb.Invocation, payload *taskspb.IngestTestVerdicts) (err error) {

	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/verdictingester.ingestForVerdictExport")
	defer func() { tracing.End(s, err) }()

	cfg, err := config.Get(ctx)
	if err != nil {
		return errors.Annotate(err, "read config").Err()
	}
	enabled := cfg.TestVerdictExport != nil && cfg.TestVerdictExport.Enabled
	if !enabled {
		return nil
	}
	// Export test verdicts.
	exportOptions := testverdicts.ExportOptions{
		Payload:     payload,
		Invocation:  inv,
		SourcesByID: sources,
	}
	err = verdictExporter.Export(ctx, testVariants, exportOptions)
	if err != nil {
		return errors.Annotate(err, "export").Err()
	}
	return nil
}

func validateRequest(ctx context.Context, payload *taskspb.IngestTestVerdicts) error {
	if !payload.PartitionTime.IsValid() {
		return errors.New("partition time must be specified and valid")
	}
	t := payload.PartitionTime.AsTime()
	now := clock.Now(ctx)
	if t.Before(now.Add(ingestionEarliest)) {
		return fmt.Errorf("partition time (%v) is too long ago", t)
	} else if t.After(now.Add(ingestionLatest)) {
		return fmt.Errorf("partition time (%v) is too far in the future", t)
	}
	if payload.Build == nil {
		return errors.New("build must be specified")
	}
	if err := control.ValidateBuildResult(payload.Build); err != nil {
		return err
	}
	return nil
}
