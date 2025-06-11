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
	"go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/analysis"
	"go.chromium.org/luci/analysis/internal/analysis/clusteredfailures"
	antsinvocationexporter "go.chromium.org/luci/analysis/internal/ants/invocations/exporter"
	antstestresultexporter "go.chromium.org/luci/analysis/internal/ants/testresults/exporter"
	tvbexporter "go.chromium.org/luci/analysis/internal/changepoints/bqexporter"
	"go.chromium.org/luci/analysis/internal/checkpoints"
	"go.chromium.org/luci/analysis/internal/clustering/chunkstore"
	"go.chromium.org/luci/analysis/internal/clustering/ingestion"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/gerritchangelists"
	"go.chromium.org/luci/analysis/internal/ingestion/control"
	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testverdicts"
	"go.chromium.org/luci/analysis/internal/tracing"
	pb "go.chromium.org/luci/analysis/proto/v1"

	// Add support for Spanner transactions in TQ.
	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

const (
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

	// Keep checkpoints for 90 days, so that task retries within 90 days are safe.
	checkpointTTL = 90 * 24 * time.Hour
)

var (
	taskCounter = metric.NewCounter(
		"analysis/ingestion/task_completion",
		"The number of completed LUCI Analysis ingestion tasks, by build project and outcome.",
		nil,
		// The LUCI Project.
		field.String("project"),
		// "success", "failed_validation",
		// "ignored_no_invocation", "ignored_not_export_root",
		// "ignored_invocation_not_found", "ignored_resultdb_permission_denied",
		// "ignored_project_not_allowlisted".
		field.String("outcome"))

	testVariantReadMask = &fieldmaskpb.FieldMask{
		Paths: []string{
			"test_id",
			"variant_hash",
			"status",
			"status_v2",
			"status_override",
			"variant",
			"test_metadata",
			"sources_id",
			"exonerations.*.explanation_html",
			"exonerations.*.reason",
			"results.*.result.name",
			"results.*.result.result_id",
			"results.*.result.expected",
			"results.*.result.status",
			"results.*.result.status_v2",
			"results.*.result.summary_html",
			"results.*.result.start_time",
			"results.*.result.duration",
			"results.*.result.tags",
			"results.*.result.failure_reason",
			"results.*.result.skip_reason",
			"results.*.result.skipped_reason",
			"results.*.result.properties",
			"results.*.result.framework_extensions",
		},
	}

	buildReadMask = &field_mask.FieldMask{
		Paths: []string{"builder", "infra.resultdb", "status", "input", "output", "ancestor_ids"},
	}
)

var verdictIngestion = tq.RegisterTaskClass(tq.TaskClass{
	ID:        verdictIngestionTaskClass,
	Prototype: &taskspb.IngestTestVerdicts{},
	Queue:     verdictIngestionQueue,
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

	antsTestResultsBQClient, err := antstestresultexporter.NewClient(ctx, srv.Options.CloudProject)
	if err != nil {
		return err
	}
	srv.RegisterCleanup(func(ctx context.Context) {
		err := antsTestResultsBQClient.Close()
		if err != nil {
			logging.Errorf(ctx, "Cleaning up AnTS Test Results BQ exporter client: %s", err)
		}
	})

	antsInvocationBQClient, err := antsinvocationexporter.NewClient(ctx, srv.Options.CloudProject)
	if err != nil {
		return err
	}
	srv.RegisterCleanup(func(ctx context.Context) {
		err := antsInvocationBQClient.Close()
		if err != nil {
			logging.Errorf(ctx, "Cleaning up AnTS Invocation BQ exporter client: %s", err)
		}
	})

	analysis := analysis.NewClusteringHandler(cf)
	o := &orchestrator{
		sinks: []IngestionSink{
			// Record the test verdicts for test history.
			&TestResultsRecorder{},
			// Ingest for test variant analysis (change point analysis).
			// See go/luci-test-variant-analysis-design for details.
			&ChangePointExporter{exporter: tvbexporter.NewExporter(tvbBQClient)},
			// Ingest the test verdicts for clustering.
			// IMPORTANT: This should occur after test variant analysis ingestion as
			// it queries the results of the above analysis.
			&ClusteringExporter{clustering: ingestion.New(chunkStore, analysis)},
			&VerdictExporter{exporter: testverdicts.NewExporter(verdictClient)},
			&AnTSTestResultExporter{exporter: antstestresultexporter.NewExporter(antsTestResultsBQClient)},
			// This should occur after ants test result exporter, because we want to export ants invocation
			// after test result are all exported. See go/export-result-f1 for details.
			&AnTSTInvocationExporter{exporter: antsinvocationexporter.NewExporter(antsInvocationBQClient)}},
	}
	verdictHandler := func(ctx context.Context, payload proto.Message) error {
		task := payload.(*taskspb.IngestTestVerdicts)
		return o.run(ctx, task)
	}
	verdictIngestion.AttachHandler(verdictHandler)
	return nil
}

// Schedule enqueues a task to ingest test verdicts from a build.
func Schedule(ctx context.Context, task *taskspb.IngestTestVerdicts) {
	tq.MustAddTask(ctx, &tq.Task{
		Title:   fmt.Sprintf("%s-%s-page-%v", task.Project, task.IngestionId, task.TaskIndex),
		Payload: task,
	})
}

// Inputs captures all input into an analysis or export.
type Inputs struct {
	Invocation *rdbpb.Invocation
	// The test Verdicts to ingest.
	Verdicts    []*rdbpb.TestVariant
	SourcesByID map[string]*pb.Sources
	Payload     *taskspb.IngestTestVerdicts
	LastPage    bool // Whether this is the last page of test verdicts.
}

// IngestionSink is the interface implemented by consumers of test verdicts
// in the ingestion pipeline.
type IngestionSink interface {
	// A short identifier for the sink, for use in errors.
	Name() string
	// Ingest the test verdicts.
	Ingest(ctx context.Context, input Inputs) error
}

// orchestrator orchestrates the ingestion of test verdicts to the
// specified sinks.
type orchestrator struct {
	// The set of ingestion sinks to write to, in order.
	sinks []IngestionSink
}

func (o *orchestrator) run(ctx context.Context, payload *taskspb.IngestTestVerdicts) error {
	if err := validateRequest(ctx, payload); err != nil {
		project := "(unknown)"
		if payload.Project != "" {
			project = payload.Project
		}
		taskCounter.Add(ctx, 1, project, "failed_validation")
		return tq.Fatal.Apply(err)
	}

	isProjectEnabled, err := config.IsProjectEnabledForIngestion(ctx, payload.Project)
	if err != nil {
		return transient.Tag.Apply(err)
	}
	if !isProjectEnabled {
		taskCounter.Add(ctx, 1, payload.Project, "ignored_project_not_allowlisted")
		return nil
	}

	if payload.Invocation == nil {
		if payload.Build != nil {
			// Ingestion has a build that does not have a ResultDB invocation to ingest.
			logging.Debugf(ctx, "Skipping ingestion of build %s-%d because it has no ResultDB invocation.",
				payload.Build.Host, payload.Build.Id)
			taskCounter.Add(ctx, 1, payload.Project, "ignored_no_invocation")
			return nil
		}
		return errors.New("ingestion with no build and no invocation should not be scheduled")
	}

	rdbHost := payload.Invocation.ResultdbHost
	invName := pbutil.InvocationName(payload.Invocation.InvocationId)
	rc, err := resultdb.NewClient(ctx, rdbHost, payload.Project)
	if err != nil {
		return transient.Tag.Apply(err)
	}
	inv, err := rc.GetInvocation(ctx, invName)
	code := status.Code(err)
	if code == codes.NotFound {
		// Invocation not found, end the task gracefully.
		logging.Warningf(ctx, "Invocation %s for project %s not found.",
			invName, payload.Project)
		taskCounter.Add(ctx, 1, payload.Project, "ignored_invocation_not_found")
		return nil
	}
	if code == codes.PermissionDenied {
		// Invocation not found, end the task gracefully.
		logging.Warningf(ctx, "Permission denied to read invocation %s for project %s.",
			invName, payload.Project)
		taskCounter.Add(ctx, 1, payload.Project, "ignored_resultdb_permission_denied")
		return nil
	}
	if err != nil {
		logging.Warningf(ctx, "GetInvocation has error code %s.", code)
		return transient.Tag.Apply(errors.Fmt("get invocation: %w", err))
	}

	if !inv.IsExportRoot {
		// Invocation is not an export root. Do not ingest.
		logging.Debugf(ctx, "Skipping ingestion of invocation %s for project %s because it is not an export root.",
			invName, payload.Project)
		taskCounter.Add(ctx, 1, payload.Project, "ignored_not_export_root")
		return nil
	}
	// Query test variants from ResultDB.
	req := &rdbpb.QueryTestVariantsRequest{
		Invocations: []string{inv.Name},
		ResultLimit: 100,
		PageSize:    10000,
		ReadMask:    testVariantReadMask,
		PageToken:   payload.PageToken,
	}
	if payload.UseNewIngestionOrder {
		req.OrderBy = `status_v2_effective`
	}

	rsp, err := rc.QueryTestVariants(ctx, req)
	if err != nil {
		err = errors.Fmt("query test variants: %w", err)
		return transient.Tag.Apply(err)
	}

	sources, err := gerritchangelists.PopulateOwnerKindsBatch(ctx, payload.Project, rsp.Sources)
	if err != nil {
		err = errors.Fmt("populate changelist owner kinds: %w", err)
		return transient.Tag.Apply(err)
	}

	// Schedule a task to deal with the next page of verdicts (if needed).
	// Do this immediately, so that task can commence while we are still
	// inserting the verdicts for this page.
	if rsp.NextPageToken != "" {
		if err := scheduleNextTask(ctx, payload, rsp.NextPageToken); err != nil {
			err = errors.Fmt("schedule next task: %w", err)
			return transient.Tag.Apply(err)
		}
	}
	nextPageToken := rsp.NextPageToken

	input := Inputs{
		Invocation:  inv,
		Verdicts:    rsp.TestVariants,
		SourcesByID: sources,
		Payload:     payload,
		LastPage:    nextPageToken == "",
	}

	for _, sink := range o.sinks {
		if err := sink.Ingest(ctx, input); err != nil {
			return transient.Tag.Apply(errors.Fmt("ingest: %q %w", sink.Name(), err))
		}
	}

	if nextPageToken == "" {
		// In the last task.
		taskCounter.Add(ctx, 1, payload.Project, "success")
	}
	return nil
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
func scheduleNextTask(ctx context.Context, task *taskspb.IngestTestVerdicts, nextPageToken string) (retErr error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/verdictingester.scheduleNextTask")
	defer func() { tracing.End(s, retErr) }()

	if nextPageToken == "" {
		// If the next page token is "", it means ResultDB returned the
		// last page. We should not schedule a continuation task.
		panic("next page token cannot be the empty page token")
	}

	// Schedule the task transactionally, conditioned on it not having been
	// scheduled before.
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		key := checkpoints.Key{
			Project:    task.Project,
			ResourceID: fmt.Sprintf("%s/%s", task.Invocation.ResultdbHost, task.Invocation.InvocationId),
			ProcessID:  "verdict-ingestion/schedule-continuation",
			Uniquifier: fmt.Sprintf("%v", task.TaskIndex),
		}
		exists, err := checkpoints.Exists(ctx, key)
		if err != nil {
			return errors.Fmt("test existance of checkpoint: %w", err)
		}
		if exists {
			// Next task has already been created in the past. Do not create
			// it again.
			// This can happen if the ingestion task failed after
			// it scheduled the ingestion task for the next page,
			// and was subsequently retried.
			return nil
		}
		// Insert checkpoint.
		m := checkpoints.Insert(ctx, key, 30*24*time.Hour)
		span.BufferWrite(ctx, m)

		nextTaskIndex := task.TaskIndex + 1

		itvTask := &taskspb.IngestTestVerdicts{
			PartitionTime:        task.PartitionTime,
			IngestionId:          task.IngestionId,
			Project:              task.Project,
			Invocation:           task.Invocation,
			Build:                task.Build,
			PresubmitRun:         task.PresubmitRun,
			PageToken:            nextPageToken,
			TaskIndex:            nextTaskIndex,
			UseNewIngestionOrder: task.UseNewIngestionOrder,
		}
		Schedule(ctx, itvTask)

		return nil
	})
	return err
}

func validateRequest(ctx context.Context, payload *taskspb.IngestTestVerdicts) error {
	if payload.IngestionId == "" {
		return errors.New("ingestion ID must be specified")
	}
	if err := pbutil.ValidateProject(payload.Project); err != nil {
		return errors.Fmt("project: %w", err)
	}
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
	if payload.Build != nil {
		if err := control.ValidateBuildResult(payload.Build); err != nil {
			return err
		}
	}
	if payload.Invocation != nil {
		if err := control.ValidateInvocationResult(payload.Invocation); err != nil {
			return err
		}
	}
	return nil
}
