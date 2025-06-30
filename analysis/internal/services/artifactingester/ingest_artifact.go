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

// Package artifactingester defines the task queue which ingests artifacts
// from ResultDB and pushes them into BigQuery.
package artifactingester

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

	antsexporter "go.chromium.org/luci/analysis/internal/ants/artifacts/exporter"
	"go.chromium.org/luci/analysis/internal/checkpoints"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb" // Import the new taskspb
	"go.chromium.org/luci/analysis/internal/tracing"

	// Add support for Spanner transactions in TQ.
	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

const (
	artifactIngestionTaskClass = "artifact-ingestion"
	artifactIngestionQueue     = "artifact-ingestion"
)

// Keep checkpoints for 90 days, so that task retries within 90 days are safe.
const checkpointTTL = 90 * 24 * time.Hour

var artifactIngestion = tq.RegisterTaskClass(tq.TaskClass{
	ID:        artifactIngestionTaskClass,
	Prototype: &taskspb.IngestArtifacts{},
	Queue:     artifactIngestionQueue,
	Kind:      tq.FollowsContext,
})

// RegisterTaskHandler registers the handler for artifact ingestion tasks.
func RegisterTaskHandler(srv *server.Server) error {
	ctx := srv.Context
	antsTestAartifactBQClient, err := antsexporter.NewClient(ctx, srv.Options.CloudProject)
	if err != nil {
		return err
	}
	srv.RegisterCleanup(func(ctx context.Context) {
		err := antsTestAartifactBQClient.Close()
		if err != nil {
			logging.Errorf(ctx, "Cleaning up AnTS Test Artifacts BQ exporter client: %s", err)
		}
	})
	artifactIngester := &artifactIngester{
		antsExporter: antsexporter.NewExporter(antsTestAartifactBQClient),
	}
	artifactIngestion.AttachHandler(func(ctx context.Context, payload proto.Message) error {
		task := payload.(*taskspb.IngestArtifacts)
		return artifactIngester.run(ctx, task)
	})
	return nil
}

// Schedule enqueues a task to ingest artifacts.
func Schedule(ctx context.Context, task *taskspb.IngestArtifacts) {
	project, _ := realms.Split(task.Notification.Realm)

	tq.MustAddTask(ctx, &tq.Task{
		Title:   fmt.Sprintf("%s-%s-page-%v", project, task.Notification.Invocation, task.TaskIndex),
		Payload: task,
	})
}

type artifactIngester struct {
	antsExporter *antsexporter.Exporter
}

func (a *artifactIngester) run(ctx context.Context, payload *taskspb.IngestArtifacts) error {
	if err := validatePayload(payload); err != nil {
		return tq.Fatal.Apply(errors.Fmt("validate payload: %w", err))
	}
	n := payload.Notification

	project, _ := realms.Split(n.Realm)
	ctx = logging.SetFields(ctx, logging.Fields{"Project": project, "Invocation": n.Invocation})

	isProjectEnabled, err := config.IsProjectEnabledForIngestion(ctx, project)
	if err != nil {
		return transient.Tag.Apply(err)
	}
	if !isProjectEnabled {
		// Project not enabled for data ingestion.
		return nil
	}

	invClient, err := resultdb.NewClient(ctx, n.ResultdbHost, project)
	if err != nil {
		return transient.Tag.Apply(err)
	}

	invocation, err := invClient.GetInvocation(ctx, n.Invocation)
	code := status.Code(err)
	if code == codes.NotFound {
		// Invocation not found, end the task gracefully.
		logging.Warningf(ctx, "Invocation not found.",
			invocation, project)
		return nil
	}
	if code == codes.PermissionDenied {
		// Invocation not found, end the task gracefully.
		logging.Warningf(ctx, "Invocation permission denied.")
		return nil
	}
	if err != nil {
		// Other error.
		return transient.Tag.Apply(errors.Fmt("read invocation: %w", err))
	}

	req := &resultpb.QueryArtifactsRequest{
		Invocations: []string{n.Invocation},
		PageSize:    1000,
		PageToken:   payload.PageToken,
	}
	rsp, err := invClient.QueryArtifacts(ctx, req)
	code = status.Code(err)
	if code == codes.PermissionDenied {
		logging.Warningf(ctx, "Invocation query artifacts permission denied.")
		return tq.Fatal.Apply(errors.Fmt("query artifacts: %w", err))
	}
	if err != nil {
		// Other error.
		return transient.Tag.Apply(errors.Fmt("query artifacts: %w", err))
	}

	if rsp.NextPageToken != "" {
		if err := scheduleNextArtifactTask(ctx, payload, rsp.NextPageToken); err != nil {
			return transient.Tag.Apply(err)
		}
	}

	if len(rsp.Artifacts) > 0 {
		if err := ingestAnTSArtifacts(ctx, project, rsp.Artifacts, invocation, a.antsExporter); err != nil {
			return transient.Tag.Apply(errors.Fmt("ingestAnTSArtifacts  %w", err))
		}
	}
	return nil
}

func ingestAnTSArtifacts(ctx context.Context, project string, artifacts []*resultpb.Artifact, invocation *resultpb.Invocation, exporter *antsexporter.Exporter) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/artifactingester.ingestAnTSArtifacts")
	defer func() { tracing.End(s, err) }()

	// Only ingest for android project.
	if project != "android" {
		return nil
	}
	exportOptions := antsexporter.ExportOptions{
		Invocation: invocation,
	}
	err = exporter.Export(ctx, artifacts, exportOptions)
	if err != nil {
		return errors.Fmt("export: %w", err)
	}

	return nil
}

// scheduleNextArtifactTask schedules a task to continue the artifact ingestion,
// starting at the given page token.
// If a continuation task for this task has been previously scheduled
// (e.g. in a previous try of this task), this method does nothing.
func scheduleNextArtifactTask(ctx context.Context, task *taskspb.IngestArtifacts, nextPageToken string) (retErr error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/artifactingester.scheduleNextArtifactTask")
	defer func() { tracing.End(s, retErr) }()

	if nextPageToken == "" {
		// If the next page token is "", it means ResultDB returned the
		// last page. We should not schedule a continuation task.
		panic("next page token cannot be the empty page token")
	}
	project, _ := realms.Split(task.Notification.Realm)
	rdbHost := task.Notification.ResultdbHost
	invID, err := pbutil.ParseInvocationName(task.Notification.Invocation)
	if err != nil {
		return errors.Fmt("parse invocation name: %w", err)
	}

	// Schedule the task transactionally, conditioned on it not having been
	// scheduled before.
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		key := checkpoints.Key{
			Project:    project,
			ResourceID: fmt.Sprintf("%s/%s", rdbHost, invID),
			ProcessID:  "artifact-ingestion/schedule-continuation",
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
		m := checkpoints.Insert(ctx, key, checkpointTTL)
		span.BufferWrite(ctx, m)

		nextTaskIndex := task.TaskIndex + 1

		itaTask := &taskspb.IngestArtifacts{
			Notification: task.Notification,
			PageToken:    nextPageToken,
			TaskIndex:    nextTaskIndex,
		}
		Schedule(ctx, itaTask)

		return nil
	})
	return err
}

func validatePayload(payload *taskspb.IngestArtifacts) error {
	if err := validateNotification(payload.Notification); err != nil {
		return errors.Fmt("notification: %w", err)
	}
	if payload.TaskIndex <= 0 {
		return errors.New("task index must be positive")
	}
	return nil
}

func validateNotification(n *resultpb.InvocationFinalizedNotification) error {
	if n == nil {
		return errors.New("unspecified")
	}
	if n.ResultdbHost == "" {
		return errors.New("resultdb host is required")
	}
	if err := pbutil.ValidateInvocationName(n.Invocation); err != nil {
		return errors.Fmt("root invocation name: %w", err)
	}
	if err := realms.ValidateRealmName(n.Realm, realms.GlobalScope); err != nil {
		return errors.Fmt("invocation realm: %w", err)
	}
	return nil
}
