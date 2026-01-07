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

// Package workunitingester ingests work units from ResultDB.
// The task should be scheduled when the root invocation has finalized instead of the low-latency work unit pubsub,
// because the AnTS export requires the root invocation finalize time as its partition time.
package workunitingester

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
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/analysis/internal/checkpoints"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/resultdb"
	"go.chromium.org/luci/analysis/internal/tasks/taskspb"
	"go.chromium.org/luci/analysis/internal/tracing"

	// Add support for Spanner transactions in TQ.
	_ "go.chromium.org/luci/server/tq/txn/spanner"
)

// Keep checkpoints for 90 days, so that task retries within 90 days are safe.
const checkpointTTL = 90 * 24 * time.Hour

const (
	workUnitIngestionTaskClass = "work-unit-ingestion"
	workUnitIngestionQueue     = "work-unit-ingestion"
)

var taskClass = tq.RegisterTaskClass(tq.TaskClass{
	ID:        workUnitIngestionTaskClass,
	Prototype: &taskspb.IngestWorkUnits{},
	Queue:     workUnitIngestionQueue,
	Kind:      tq.FollowsContext,
})

// RegisterTaskHandler registers the task handler for ingesting work units.
func RegisterTaskHandler(srv *server.Server) error {
	ingester := &workUnitIngester{}
	taskClass.AttachHandler(func(ctx context.Context, payload proto.Message) error {
		task := payload.(*taskspb.IngestWorkUnits)
		return ingester.run(ctx, task)
	})
	return nil
}

// Schedule enqueues a task to ingest work units.
func Schedule(ctx context.Context, payload *taskspb.IngestWorkUnits) error {
	rootInvID, err := pbutil.ParseRootInvocationName(payload.RootInvocation)
	if err != nil {
		return err
	}
	project, _ := realms.Split(payload.Realm)

	return tq.AddTask(ctx, &tq.Task{
		Payload: payload,
		Title:   fmt.Sprintf("%s-%s-page-%v", project, rootInvID, payload.TaskIndex),
	})
}

// workUnitIngester ingests work units from ResultDB.
type workUnitIngester struct{}

func (i *workUnitIngester) run(ctx context.Context, task *taskspb.IngestWorkUnits) error {
	if err := validatePayload(task); err != nil {
		return tq.Fatal.Apply(errors.Fmt("validate payload: %w", err))
	}

	rootInvName := task.RootInvocation
	project, _ := realms.Split(task.Realm)
	ctx = logging.SetFields(ctx, logging.Fields{"Project": project, "RootInvocation": rootInvName})

	isProjectEnabled, err := config.IsProjectEnabledForIngestion(ctx, project)
	if err != nil {
		return transient.Tag.Apply(err)
	}
	if !isProjectEnabled {
		// Project not enabled for data ingestion.
		return nil
	}

	rdbHost := task.ResultdbHost
	// Create ResultDB client.
	rdbClient, err := resultdb.NewClient(ctx, rdbHost, project)
	if err != nil {
		return transient.Tag.Apply(err)
	}

	rootInvocation, err := rdbClient.GetRootInvocation(ctx, rootInvName)
	code := status.Code(err)
	if code == codes.NotFound {
		// Invocation not found, end the task gracefully.
		logging.Warningf(ctx, "Root invocation not found.",
			rootInvocation, project)
		return nil
	}
	if code == codes.PermissionDenied {
		// Invocation not found, end the task gracefully.
		logging.Warningf(ctx, "Root invocation permission denied.")
		return nil
	}
	if err != nil {
		// Other error.
		return transient.Tag.Apply(errors.Fmt("read root invocation: %w", err))
	}

	// Fetch work units.
	req := &rdbpb.QueryWorkUnitsRequest{
		Parent:    rootInvName,
		PageToken: task.PageToken,
		PageSize:  1000,
		View:      rdbpb.WorkUnitView_WORK_UNIT_VIEW_BASIC,
	}

	rsp, err := rdbClient.QueryWorkUnits(ctx, req)
	code = status.Code(err)
	if code == codes.PermissionDenied {
		logging.Warningf(ctx, "Query work unit permission denied.")
		return tq.Fatal.Apply(errors.Fmt("query work unit: %w", err))
	}
	if err != nil {
		// Other error.
		return transient.Tag.Apply(errors.Fmt("query work unit: %w", err))
	}

	// Schedule next task before the actual export.
	if rsp.NextPageToken != "" {
		if err := scheduleNextWorkUnitTask(ctx, task, rsp.NextPageToken); err != nil {
			return transient.Tag.Apply(errors.Fmt("schedule next work unit task: %w", err))
		}
	}
	// TODO: Export work units.
	return nil
}

// scheduleNextWorkUnitTask schedules a task to ingest the next page of work units,
// starting at the given page token.
// If a continuation task for this task has been previously scheduled
// (e.g. in a previous try of this task), this method does nothing.
func scheduleNextWorkUnitTask(ctx context.Context, task *taskspb.IngestWorkUnits, nextPageToken string) (retErr error) {
	ctx, s := tracing.Start(ctx, "go.chromium.org/luci/analysis/internal/services/workunitingester.scheduleNextWorkUnitTask")
	defer func() { tracing.End(s, retErr) }()

	if nextPageToken == "" {
		// If the next page token is "", it means ResultDB returned the
		// last page. We should not schedule a continuation task.
		panic("next page token cannot be the empty page token")
	}
	project, _ := realms.Split(task.Realm)
	rdbHost := task.ResultdbHost
	rootInvID, err := pbutil.ParseRootInvocationName(task.RootInvocation)
	if err != nil {
		return err
	}
	// Schedule the task transactionally, conditioned on it not having been
	// scheduled before.
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		key := checkpoints.Key{
			Project:    project,
			ResourceID: fmt.Sprintf("%s/%s", rdbHost, rootInvID),
			ProcessID:  "work-unit-ingestion/schedule-continuation",
			Uniquifier: fmt.Sprintf("%v", task.TaskIndex),
		}
		exists, err := checkpoints.Exists(ctx, key)
		if err != nil {
			return errors.Fmt("test existence of checkpoint: %w", err)
		}
		if exists {
			// Next task has already been created in the past. Do not create
			// it again.
			return nil
		}
		// Insert checkpoint.
		m := checkpoints.Insert(ctx, key, checkpointTTL)
		span.BufferWrite(ctx, m)

		nextTask := proto.Clone(task).(*taskspb.IngestWorkUnits)
		nextTask.PageToken = nextPageToken
		nextTask.TaskIndex = task.TaskIndex + 1

		if err := Schedule(ctx, nextTask); err != nil {
			return errors.Fmt("schedule task: %w", err)
		}
		return nil
	})
	return err
}

func validatePayload(payload *taskspb.IngestWorkUnits) error {
	if payload.ResultdbHost == "" {
		return errors.New("resultdb host is required")
	}
	if err := pbutil.ValidateRootInvocationName(payload.RootInvocation); err != nil {
		return errors.Fmt("root invocation name: %w", err)
	}
	if err := realms.ValidateRealmName(payload.Realm, realms.GlobalScope); err != nil {
		return errors.Fmt("realm: %w", err)
	}
	if payload.TaskIndex <= 0 {
		return errors.New("task index must be positive")
	}
	return nil
}
