// Copyright 2024 The LUCI Authors.
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

// Package resultingester defines the task queue which ingests test results
// from ResultDB and pushes it into:
// - Test results table (for exoneration analysis)
// - Test results BigQuery export
// - Changepoint analysis
package resultingester

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/resultdb/pbutil"
	resultpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	tvbexporter "go.chromium.org/luci/analysis/internal/changepoints/bqexporter"
	"go.chromium.org/luci/analysis/internal/checkpoints"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/gerritchangelists"
	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/testresults/exporter"
	"go.chromium.org/luci/analysis/internal/tracing"
	analysispb "go.chromium.org/luci/analysis/proto/v1"

	// Add support for Spanner transactions in TQ.
	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

const (
	resultIngestionTaskClass = "result-ingestion"
	resultIngestionQueue     = "result-ingestion"
)

// Keep checkpoints for 90 days, so that task retries within 90 days are safe.
const checkpointTTL = 90 * 24 * time.Hour

var resultIngestion = tq.RegisterTaskClass(tq.TaskClass{
	ID:        resultIngestionTaskClass,
	Prototype: &taskspb.IngestTestResults{},
	Queue:     resultIngestionQueue,
	Kind:      tq.FollowsContext,
})

// RegisterTaskHandler registers the handler for result ingestion tasks.
func RegisterTaskHandler(srv *server.Server) error {
	resultsClient, err := exporter.NewClient(srv.Context, srv.Options.CloudProject)
	if err != nil {
		return errors.Annotate(err, "create test results BigQuery client").Err()
	}

	tvbBQClient, err := tvbexporter.NewClient(srv.Context, srv.Options.CloudProject)
	if err != nil {
		return err
	}
	srv.RegisterCleanup(func(ctx context.Context) {
		err := tvbBQClient.Close()
		if err != nil {
			logging.Errorf(ctx, "Cleaning up result ingestion test variant branch BQExporter client: %s", err)
		}
	})

	o := &orchestrator{
		sinks: []IngestionSink{
			IngestForExoneration{},
			NewTestResultsExporter(resultsClient),
			NewIngestForChangepointAnalysis(tvbexporter.NewExporter(tvbBQClient)),
		},
	}
	resultIngestion.AttachHandler(func(ctx context.Context, payload proto.Message) error {
		task := payload.(*taskspb.IngestTestResults)
		return o.run(ctx, task)
	})
	return nil
}

// Schedule enqueues a task to ingest test results from a build.
func Schedule(ctx context.Context, task *taskspb.IngestTestResults) {
	project, _ := realms.Split(task.Notification.RootInvocationRealm)
	tq.MustAddTask(ctx, &tq.Task{
		Title:   fmt.Sprintf("%s-%s-subinvocation-%s-page-%v", project, task.Notification.RootInvocation, task.Notification.Invocation, task.TaskIndex),
		Payload: task,
	})
}

// Inputs captures all input into a test results analysis or export.
type Inputs struct {
	// The LUCI Project of the export root.
	Project string
	// The subrealm of the export root. This is the realm, minus the LUCI project.
	SubRealm string
	// The ResultDB host.
	ResultDBHost string
	// The invocation ID of the export root.
	RootInvocationID string
	// The invocation ID of the invocation we are ingesting.
	InvocationID string
	// The page number we are ingesting.
	PageNumber int
	// The start of the data retention period for the data being ingested.
	PartitionTime time.Time
	// The sources tested by the invocation. May be nil if no sources
	// are specified.
	Sources *analysispb.Sources
	// The immediate parent resultdb invocation.
	Parent *resultpb.Invocation
	// The test verdicts to ingest.
	Verdicts []*resultpb.RunTestVerdict
}

// IngestionSink is the interface implemented by consumers of test results
// in the ingestion pipeline.
type IngestionSink interface {
	// A short identifier for the sink, for use in errors.
	Name() string
	// Ingest the test results.
	Ingest(ctx context.Context, input Inputs) error
}

// orchestrator orchestrates the ingestion of test results to the
// specified sinks.
type orchestrator struct {
	// The set of ingestion sinks to write to, in order.
	sinks []IngestionSink
}

func (o *orchestrator) run(ctx context.Context, payload *taskspb.IngestTestResults) error {
	if err := validatePayload(payload); err != nil {
		return tq.Fatal.Apply(errors.Annotate(err, "validate payload").Err())
	}
	n := payload.Notification

	// For reading the root invocation, use a ResultDB client acting as
	// the project of the root invocation.
	rootProject, _ := realms.Split(n.RootInvocationRealm)
	ctx = logging.SetFields(ctx, logging.Fields{"Project": rootProject, "RootInvocation": n.RootInvocation, "Invocation": n.Invocation})

	isProjectEnabled, err := config.IsProjectEnabledForIngestion(ctx, rootProject)
	if err != nil {
		return transient.Tag.Apply(err)
	}
	if !isProjectEnabled {
		// Project not enabled for data ingestion.
		return nil
	}

	// Use the creation time of the root invocation as the partition time.
	// Do not use other properties of the root invocation as it may not
	// have finalized yet and is subject to change.
	partitionTime := n.RootCreateTime.AsTime()

	// For reading the to-be-ingested invocation, use a ResultDB client
	// acting as the project of that invocation.
	project, _ := realms.Split(n.InvocationRealm)
	invClient, err := resultdb.NewClient(ctx, n.ResultdbHost, project)
	if err != nil {
		return transient.Tag.Apply(err)
	}

	parentInv, err := invClient.GetInvocation(ctx, n.Invocation)
	code := status.Code(err)
	if code == codes.NotFound {
		logging.Warningf(ctx, "Parent invocation not found.")
		return tq.Fatal.Apply(errors.Fmt("read parent invocation: %w", err))
	}
	if code == codes.PermissionDenied {
		// Invocation not found, end the task gracefully.
		logging.Warningf(ctx, "Parent invocation permission denied.")
		return nil
	}
	if err != nil {
		// Other error.
		return transient.Tag.Apply(errors.Annotate(err, "read parent invocation").Err())
	}

	req := &resultpb.QueryRunTestVerdictsRequest{
		Invocation:  n.Invocation,
		PageSize:    10_000,
		ResultLimit: 100,
		PageToken:   payload.PageToken,
	}
	rsp, err := invClient.QueryRunTestVerdicts(ctx, req)
	code = status.Code(err)
	if code == codes.PermissionDenied {
		// Invocation permission denied, end the task gracefully.
		logging.Warningf(ctx, "Invocation query test results permission denied.")
		return nil
	}
	if err != nil {
		// Other error.
		return transient.Tag.Apply(errors.Annotate(err, "query test run verdicts").Err())
	}

	input, err := prepareInputs(ctx, payload, parentInv, partitionTime, rsp.RunTestVerdicts)
	if err != nil {
		return transient.Tag.Apply(errors.Annotate(err, "prepare ingestion inputs").Err())
	}

	if rsp.NextPageToken != "" {
		if err := scheduleNextTask(ctx, payload, rsp.NextPageToken); err != nil {
			return transient.Tag.Apply(err)
		}
	}

	// Only run ingestion sinks if there is something to ingest.
	if len(input.Verdicts) > 0 {
		for _, sink := range o.sinks {
			if err := sink.Ingest(ctx, input); err != nil {
				return transient.Tag.Apply(errors.Annotate(err, "ingest: %q", sink.Name()).Err())
			}
		}
	}

	return nil
}

func prepareInputs(ctx context.Context, task *taskspb.IngestTestResults, parent *resultpb.Invocation, partitionTime time.Time, tvs []*resultpb.RunTestVerdict) (Inputs, error) {
	notification := task.Notification
	project, subRealm := realms.Split(notification.RootInvocationRealm)

	rootInvocationID, err := pbutil.ParseInvocationName(notification.RootInvocation)
	if err != nil {
		return Inputs{}, errors.Annotate(err, "parse root invocation name").Err()
	}
	invocationID, err := pbutil.ParseInvocationName(notification.Invocation)
	if err != nil {
		return Inputs{}, errors.Annotate(err, "parse invocation name").Err()
	}

	result := Inputs{
		Project:          project,
		SubRealm:         subRealm,
		ResultDBHost:     notification.ResultdbHost,
		RootInvocationID: rootInvocationID,
		InvocationID:     invocationID,
		PageNumber:       int(task.TaskIndex),
		PartitionTime:    partitionTime,
		Parent:           parent,
		Verdicts:         tvs,
	}

	if notification.Sources != nil {
		// The data is going into the project of the export root, so we should
		// query with the authority of that project.
		sources, err := gerritchangelists.PopulateOwnerKinds(ctx, project, notification.Sources)
		if err != nil {
			return Inputs{}, errors.Annotate(err, "populate changelist owner kinds").Err()
		}
		result.Sources = sources
	}
	return result, nil
}

// scheduleNextTask schedules a task to continue the ingestion,
// starting at the given page token.
// If a continuation task for this task has been previously scheduled
// (e.g. in a previous try of this task), this method does nothing.
func scheduleNextTask(ctx context.Context, task *taskspb.IngestTestResults, nextPageToken string) (retErr error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/resultingester.scheduleNextTask")
	defer func() { tracing.End(s, retErr) }()

	if nextPageToken == "" {
		// If the next page token is "", it means ResultDB returned the
		// last page. We should not schedule a continuation task.
		panic("next page token cannot be the empty page token")
	}
	rootProject, _ := realms.Split(task.Notification.RootInvocationRealm)
	rdbHost := task.Notification.ResultdbHost
	rootInvID, err := pbutil.ParseInvocationName(task.Notification.RootInvocation)
	if err != nil {
		return errors.Annotate(err, "parse root invocation name").Err()
	}
	invID, err := pbutil.ParseInvocationName(task.Notification.Invocation)
	if err != nil {
		return errors.Annotate(err, "parse invocation name").Err()
	}

	// Schedule the task transactionally, conditioned on it not having been
	// scheduled before.
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		key := checkpoints.Key{
			Project:    rootProject,
			ResourceID: fmt.Sprintf("%s/%s/%s", rdbHost, rootInvID, invID),
			ProcessID:  "result-ingestion/schedule-continuation",
			Uniquifier: fmt.Sprintf("%v", task.TaskIndex),
		}
		exists, err := checkpoints.Exists(ctx, key)
		if err != nil {
			return errors.Annotate(err, "test existance of checkpoint").Err()
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
		m := checkpoints.Insert(ctx, key, checkpointTTL)
		span.BufferWrite(ctx, m)

		nextTaskIndex := task.TaskIndex + 1

		itrTask := &taskspb.IngestTestResults{
			Notification: task.Notification,
			PageToken:    nextPageToken,
			TaskIndex:    nextTaskIndex,
		}
		Schedule(ctx, itrTask)

		return nil
	})
	return err
}

func validatePayload(payload *taskspb.IngestTestResults) error {
	if err := validateNotification(payload.Notification); err != nil {
		return errors.Annotate(err, "notification").Err()
	}
	if payload.TaskIndex <= 0 {
		return errors.New("task index must be positive")
	}
	return nil
}

func validateNotification(n *resultpb.InvocationReadyForExportNotification) error {
	if n == nil {
		return errors.New("unspecified")
	}
	if n.RootCreateTime == nil {
		return errors.New("root create time is required")
	}
	if n.ResultdbHost == "" {
		return errors.New("resultdb host is required")
	}
	if err := pbutil.ValidateInvocationName(n.RootInvocation); err != nil {
		return errors.Annotate(err, "root invocation name").Err()
	}
	if err := pbutil.ValidateInvocationName(n.Invocation); err != nil {
		return errors.Annotate(err, "invocation name").Err()
	}
	if err := realms.ValidateRealmName(n.RootInvocationRealm, realms.GlobalScope); err != nil {
		return errors.Annotate(err, "root invocation realm").Err()
	}
	if err := realms.ValidateRealmName(n.InvocationRealm, realms.GlobalScope); err != nil {
		return errors.Annotate(err, "invocation realm").Err()
	}
	if n.Sources != nil {
		// If the invocation has sources, they must be valid.
		if err := pbutil.ValidateSources(n.Sources); err != nil {
			return errors.Annotate(err, "sources").Err()
		}
	}
	return nil
}
