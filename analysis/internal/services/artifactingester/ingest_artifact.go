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
	"google.golang.org/protobuf/types/known/fieldmaskpb"

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
	var title string
	if task.RootInvocationNotification != nil {
		rootInv := task.RootInvocationNotification.RootInvocation
		project, _ := realms.Split(rootInv.Realm)
		title = fmt.Sprintf("%s-%s-page-%v", project, rootInv.RootInvocationId, task.TaskIndex)
	}

	if task.Notification != nil {
		project, _ := realms.Split(task.Notification.Realm)
		title = fmt.Sprintf("legacy-%s-%s-page-%v", project, task.Notification.Invocation, task.TaskIndex)
	}

	tq.MustAddTask(ctx, &tq.Task{
		Title:   title,
		Payload: task,
	})
}

var artifactFields = []string{"name", "result_id", "artifact_id", "content_type", "size_bytes"}

type artifactIngester struct {
	antsExporter *antsexporter.Exporter
}

func (a *artifactIngester) run(ctx context.Context, payload *taskspb.IngestArtifacts) error {
	if err := validatePayload(payload); err != nil {
		return tq.Fatal.Apply(errors.Fmt("validate payload: %w", err))
	}

	var project string
	var resultdbHost string
	var invocationName string
	var rootInvocationName string

	// Exactly one of legacyNotification or rootInvocationNotification should be set.
	legacyNotification := payload.Notification
	rootInvocationNotification := payload.RootInvocationNotification

	if rootInvocationNotification != nil {
		project, _ = realms.Split(rootInvocationNotification.RootInvocation.Realm)
		resultdbHost = rootInvocationNotification.ResultdbHost
		rootInvocationName = rootInvocationNotification.RootInvocation.Name
	} else if legacyNotification != nil {
		project, _ = realms.Split(legacyNotification.Realm)
		resultdbHost = legacyNotification.ResultdbHost
		invocationName = legacyNotification.Invocation
	} else {
		// Should never happen, this should be caught by validatePayload.
		return errors.New("logic error: neither notification nor root_invocation_notification specified")
	}

	ctx = logging.SetFields(ctx, logging.Fields{"Project": project, "Invocation": invocationName, "RootInvocation": rootInvocationName})

	isProjectEnabled, err := config.IsProjectEnabledForIngestion(ctx, project)
	if err != nil {
		return transient.Tag.Apply(err)
	}
	if !isProjectEnabled {
		// Project not enabled for data ingestion.
		return nil
	}

	invClient, err := resultdb.NewClient(ctx, resultdbHost, project)
	if err != nil {
		return transient.Tag.Apply(err)
	}

	// Exactly one of invocation or rootInvocation should be set, depends
	// on whether the task was triggered by a legacy InvocationFinalizedNotification
	// or a RootInvocationFinalizedNotification.
	var invocation *resultpb.Invocation
	var rootInvocation *resultpb.RootInvocation

	if rootInvocationNotification != nil {
		rootInvocation = rootInvocationNotification.RootInvocation
	} else {
		invocation, err = invClient.GetInvocation(ctx, legacyNotification.Invocation)
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
	}

	req := &resultpb.QueryArtifactsRequest{
		PageSize:  1000,
		PageToken: payload.PageToken,
		ReadMask: &fieldmaskpb.FieldMask{
			Paths: artifactFields,
		},
	}
	if rootInvocation != nil {
		req.Parent = rootInvocation.Name
	} else {
		req.Invocations = []string{invocation.Name}
	}
	rsp, err := invClient.QueryArtifacts(ctx, req)
	code := status.Code(err)
	if code == codes.PermissionDenied {
		logging.Warningf(ctx, "Query artifacts permission denied.")
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
		if err := ingestAnTSArtifacts(ctx, project, rsp.Artifacts, invocation, rootInvocation, a.antsExporter); err != nil {
			return transient.Tag.Apply(errors.Fmt("ingestAnTSArtifacts  %w", err))
		}
	}
	return nil
}

func ingestAnTSArtifacts(ctx context.Context, project string, artifacts []*resultpb.Artifact, invocation *resultpb.Invocation, rootInvocation *resultpb.RootInvocation, exporter *antsexporter.Exporter) (err error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/artifactingester.ingestAnTSArtifacts")
	defer func() { tracing.End(s, err) }()

	// Only ingest for android project.
	if project != "android" {
		return nil
	}
	exportOptions := antsexporter.ExportOptions{
		Invocation:     invocation,
		RootInvocation: rootInvocation,
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

	var project string
	var resourceID string

	if task.RootInvocationNotification != nil {
		n := task.RootInvocationNotification
		project, _ = realms.Split(n.RootInvocation.Realm)
		// Root invocation id and invocation id can collide, prefix the resourceID for root invocation task to differentiate.
		resourceID = fmt.Sprintf("rootInvocation/%s/%s", n.ResultdbHost, n.RootInvocation.RootInvocationId)
	} else if task.Notification != nil {
		n := task.Notification
		project, _ = realms.Split(n.Realm)
		invID, err := pbutil.ParseInvocationName(n.Invocation)
		if err != nil {
			return errors.Fmt("parse invocation name: %w", err)
		}
		resourceID = fmt.Sprintf("%s/%s", n.ResultdbHost, invID)
	} else {
		// Should never happen, this should be caught by validatePayload.
		return errors.New("logic error: neither notification nor root_invocation_notification specified")
	}

	// Schedule the task transactionally, conditioned on it not having been
	// scheduled before.
	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		key := checkpoints.Key{
			Project:    project,
			ResourceID: resourceID,
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
			Notification:               task.Notification,
			RootInvocationNotification: task.RootInvocationNotification,
			PageToken:                  nextPageToken,
			TaskIndex:                  nextTaskIndex,
		}
		Schedule(ctx, itaTask)

		return nil
	})
	return err
}

// validatePayload performs a basic sanity check on the task payload.
// It does not validate every field comprehensively as the input is expected
// to come from trusted internal systems. Its primary purpose is to catch
// logic errors (e.g. missing required fields).
func validatePayload(payload *taskspb.IngestArtifacts) error {
	if payload.Notification != nil {
		if err := validateNotification(payload.Notification); err != nil {
			return errors.Fmt("notification: %w", err)
		}
	} else if payload.RootInvocationNotification != nil {
		if err := validateRootInvocationNotification(payload.RootInvocationNotification); err != nil {
			return errors.Fmt("root invocation notification: %w", err)
		}
	} else {
		return errors.New("neither notification nor root_invocation_notification specified")
	}

	if payload.Notification != nil && payload.RootInvocationNotification != nil {
		return errors.New("only one of notification or root_invocation_notification should be specified")
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
		return errors.Fmt("invocation name: %w", err)
	}
	if err := realms.ValidateRealmName(n.Realm, realms.GlobalScope); err != nil {
		return errors.Fmt("invocation realm: %w", err)
	}
	return nil
}

func validateRootInvocationNotification(n *resultpb.RootInvocationFinalizedNotification) error {
	if n == nil {
		return errors.New("unspecified")
	}
	if n.ResultdbHost == "" {
		return errors.New("resultdb host is required")
	}
	if n.RootInvocation == nil {
		return errors.New("root invocation is required")
	}
	if err := pbutil.ValidateRootInvocationName(n.RootInvocation.Name); err != nil {
		return errors.Fmt("root invocation name: %w", err)
	}
	if err := realms.ValidateRealmName(n.RootInvocation.Realm, realms.GlobalScope); err != nil {
		return errors.Fmt("root invocation realm: %w", err)
	}
	return nil
}
