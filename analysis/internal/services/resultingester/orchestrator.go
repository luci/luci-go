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
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/gerritchangelists"
	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	analysispb "go.chromium.org/luci/analysis/proto/v1"

	// Add support for Spanner transactions in TQ.
	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

const (
	resultIngestionTaskClass = "result-ingestion"
	resultIngestionQueue     = "result-ingestion"
)

var resultIngestion = tq.RegisterTaskClass(tq.TaskClass{
	ID:        resultIngestionTaskClass,
	Prototype: &taskspb.IngestTestResults{},
	Queue:     resultIngestionQueue,
	Kind:      tq.FollowsContext,
})

// RegisterTaskHandler registers the handler for result ingestion tasks.
func RegisterTaskHandler(srv *server.Server) error {
	o := &orchestrator{
		sinks: []IngestionSink{
			IngestForExoneration{},
			// TODO(meiring): Add sinks for changepoint ingestion and BigQuery export.
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
	// The invocation ID of the export root.
	RootInvocationID string
	// The invocation ID of the invocation we are ingesting.
	InvocationID string
	// The start of the data retention period for the data being ingested.
	PartitionTime time.Time
	// The sources tested by the invocation. May be nil if no sources
	// are specified.
	Sources *analysispb.Sources
	// The test verdicts to ingest.
	Verdicts []*resultpb.RunTestVerdict
}

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
	n := payload.Notification

	// For reading the root invocation, use a ResultDB client acting as
	// the project of the root invocation.
	rootProject, _ := realms.Split(n.RootInvocationRealm)
	ctx = logging.SetFields(ctx, logging.Fields{"Project": rootProject, "RootInvocation": n.RootInvocation, "Invocation": n.Invocation})

	rootClient, err := resultdb.NewClient(ctx, n.ResultdbHost, rootProject)
	if err != nil {
		return transient.Tag.Apply(err)
	}

	rootInv, err := rootClient.GetInvocation(ctx, n.RootInvocation)
	code := status.Code(err)
	if code == codes.NotFound {
		// Invocation not found, end the task gracefully.
		logging.Warningf(ctx, "Root invocation not found.")
		return tq.Fatal.Apply(errors.Annotate(err, "read root invocation").Err())
	}
	if code == codes.PermissionDenied {
		// Invocation not found, end the task gracefully.
		logging.Warningf(ctx, "Root invocation permission denied.")
		return nil
	}
	if err != nil {
		// Other error.
		return transient.Tag.Apply(errors.Annotate(err, "read root invocation").Err())
	}

	// For reading the to-be-ingested invocation, use a ResultDB client
	// acting as the project of that invocation.
	project, _ := realms.Split(n.InvocationRealm)
	invClient, err := resultdb.NewClient(ctx, n.ResultdbHost, project)
	if err != nil {
		return transient.Tag.Apply(err)
	}

	req := &resultpb.QueryRunTestVerdictsRequest{
		Invocation:  n.Invocation,
		PageSize:    10_000,
		ResultLimit: 100,
		PageToken:   payload.PageToken,
	}
	rsp, err := invClient.QueryRunTestVerdicts(ctx, req)
	code = status.Code(err)
	if code == codes.NotFound {
		// Invocation not found, end the task gracefully.
		logging.Warningf(ctx, "Invocation not found.")
		return tq.Fatal.Apply(errors.Annotate(err, "query test verdicts").Err())
	}
	if code == codes.PermissionDenied {
		// Invocation not found, end the task gracefully.
		logging.Warningf(ctx, "Invocation permission denied.")
		return nil
	}
	if err != nil {
		// Other error.
		return transient.Tag.Apply(errors.Annotate(err, "query run test variants").Err())
	}

	// TODO: create continuation task (conditional on unreached checkpoint).

	input, err := prepareInputs(ctx, n, rootInv, rsp.RunTestVerdicts)
	if err != nil {
		return transient.Tag.Apply(errors.Annotate(err, "prepare ingestion inputs").Err())
	}

	for _, sink := range o.sinks {
		if err := sink.Ingest(ctx, input); err != nil {
			return transient.Tag.Apply(errors.Annotate(err, "ingest: %q", sink.Name()).Err())
		}
	}

	return nil
}

func prepareInputs(ctx context.Context, notification *resultpb.InvocationReadyForExportNotification, rootInv *resultpb.Invocation, tvs []*resultpb.RunTestVerdict) (Inputs, error) {
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
		RootInvocationID: rootInvocationID,
		InvocationID:     invocationID,
		PartitionTime:    rootInv.CreateTime.AsTime(),
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
