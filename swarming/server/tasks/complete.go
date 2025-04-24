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

package tasks

import (
	"context"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/tq"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/notifications"
	"go.chromium.org/luci/swarming/server/tasks/taskspb"
)

// CompleteOp represents an operation to complete a running task.
//
// The actual transaction happens as part of the BotInfo update, see
// botapi/task_update.go.
type CompleteOp struct {
	// Request is the TaskRequest entity for the task to complete.
	Request *model.TaskRequest

	// BotID is ID of the bot completing the task.
	BotID string

	// PerformanceStats is performance stats reported by the bot.
	PerformanceStats *model.PerformanceStats

	// Updates of the ask.
	// Canceled is a bool on whether the task has been canceled at set up stage.
	Canceled bool
	// Abandoned is a bool on whether the task has been abandoned.
	Abandoned bool
	// CASOutputRoot is the digest of the output root uploaded to RBE-CAS.
	CASOutputRoot model.CASReference
	// CIPDPins is resolved versions of all the CIPD packages used in the task.
	CIPDPins model.CIPDInput
	// CostUSD is an approximate bot time cost spent executing this task.
	CostUSD float64
	// Duration is the time spent in seconds for this task, excluding overheads.
	Duration *float64
	// ExitCode is the task process exit code for tasks in COMPLETED state.
	ExitCode *int64
	// HardTimeout is a bool on whether a hard timeout occurred.
	HardTimeout bool
	// IOTimeout is a bool on whether an I/O timeout occurred.
	IOTimeout bool
	// Output is the data to append to the stdout content for the task.
	Output []byte
	// OutputChunkStart is the index of output in the stdout stream.
	OutputChunkStart int64
	// TaskError is a bool on whether the bot abandons the task when reporting
	// a task error.
	// It is essentially the same as `Abaondoned`. But it is not controlled by
	// m.allowAbandoningTasks.
	// TODO: merge it with Abandoned after removing m.allowAbandoningTasks.
	TaskError bool
	// ClientError is the client errors.
	ClientError *ClientError
}

// ClientError is the client errors.
type ClientError struct {
	// MissingCAS is the missing CAS digests in CLIENT_ERROR state.
	MissingCAS []model.CASReference `json:"missing_cas,omitempty"`
	// MissingCIPD is the missing CIPD packages in CLIENT_ERROR state.
	MissingCIPD []model.CIPDPackage `json:"missing_cipd,omitempty"`
}

type CompleteTxnOutcome struct {
	// Updated is a bool on whether the update is performed.
	// True if updated, false if the update cannot be performed, e.g. the task
	// has been completed.
	Updated bool

	// BotEventType is the bot event type determined by the task completion
	// outcome.
	BotEventType model.BotEventType
}

// CompleteTxn updates the task and marks it as completed.
func (m *managerImpl) CompleteTxn(ctx context.Context, op *CompleteOp) (*CompleteTxnOutcome, error) {
	if op.Abandoned && !m.allowAbandoningTasks {
		return &CompleteTxnOutcome{Updated: false}, nil
	}

	tr := op.Request
	taskID := model.RequestKeyToTaskID(tr.Key, model.AsRunResult)

	trs := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, tr.Key)}
	trr := &model.TaskRunResult{Key: model.TaskRunResultKey(ctx, tr.Key)}
	switch err := datastore.Get(ctx, trs, trr); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, status.Errorf(codes.NotFound, "task %q not found", taskID)
	case err != nil:
		return nil, status.Errorf(codes.Internal, "failed to get task %q: %s", taskID, err)
	}

	if trr.BotID != op.BotID {
		return nil, status.Errorf(codes.InvalidArgument, "task %q is not running by bot %q", taskID, op.BotID)
	}

	newState := op.calculateState(trr)
	if newState == apipb.TaskState_RUNNING {
		return nil, status.Errorf(codes.Internal, "expect to complete task %q, not only update", taskID)
	}

	if !trs.IsActive() {
		if newState != trs.State {
			logging.Errorf(
				ctx,
				"cannot complete task %q with state %s, because it is already completed with state %s",
				taskID, newState, trs.State)
		}
		if op.ExitCode != nil {
			if trs.ExitCode.IsSet() && trs.ExitCode.Get() != *op.ExitCode {
				logging.Errorf(
					ctx,
					"cannot complete task %q with exit_code %d, because it is already completed with exit_code %d",
					taskID, op.ExitCode, trs.ExitCode.Get())
			}
			if trs.DurationSecs.IsSet() && op.Duration != nil && trs.DurationSecs.Get() != *op.Duration {
				logging.Errorf(
					ctx,
					"cannot complete task %q with duration %f, because it is already completed with duration %f",
					taskID, *op.Duration, trs.DurationSecs.Get())
			}
		}
		// The task has completed, cannot make any updates to it.
		// If any of the above checks fails, there's no way for the caller to
		// handle them, so return no error.
		return &CompleteTxnOutcome{Updated: false}, nil
	}

	toPut := []any{trs, trr}
	now := clock.Now(ctx).UTC()
	trr.Completed.Set(now)

	// Some common updates, e.g. output, cost, modified timestamp, server versions.
	commonUpdates := &UpdateOp{
		Request:          tr,
		BotID:            op.BotID,
		CostUSD:          op.CostUSD,
		Output:           op.Output,
		OutputChunkStart: op.OutputChunkStart,
		trr:              trr,
		trs:              trs,
		now:              now,
	}
	outputChunks, err := m.runUpdateTxn(ctx, commonUpdates)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to save output for task %q", taskID)
	}
	for _, oc := range outputChunks {
		toPut = append(toPut, oc)
	}

	trr.State = newState
	trr.Killing = false
	if op.Duration != nil {
		trr.DurationSecs.Set(*op.Duration)
	}
	if op.ExitCode != nil {
		trr.ExitCode.Set(*op.ExitCode)
		trr.Failure = *op.ExitCode != 0
	}

	// Handle special completion cases.
	if newState == apipb.TaskState_TIMED_OUT {
		setExitCodeAndDurationFallbacks(trr, now)
		trr.Failure = true
	}
	if newState == apipb.TaskState_CANCELED {
		// Reset duration, exit_code since the task didn't actually run.
		trr.ExitCode.Unset()
		trr.DurationSecs.Unset()
	}
	if op.Abandoned || op.TaskError {
		setExitCodeAndDurationFallbacks(trr, now)
		trr.InternalFailure = true
		// Set Abandoned timestamp only if the task ends with "BOT_DIED".
		// If the task is abandoned during cancellation, its Abandoned timestamp
		// has been set when cancellation happened.
		if newState == apipb.TaskState_BOT_DIED {
			trr.Abandoned = datastore.NewIndexedNullable(now)
		}
	}
	if op.ClientError != nil {
		setExitCodeAndDurationFallbacks(trr, now)
		trr.MissingCAS = op.ClientError.MissingCAS
		trr.MissingCIPD = op.ClientError.MissingCIPD
		trr.InternalFailure = true
		if newState == apipb.TaskState_CLIENT_ERROR {
			trr.Abandoned = datastore.NewIndexedNullable(now)
		}
	}

	trr.CASOutputRoot = op.CASOutputRoot
	trr.CIPDPins = op.CIPDPins
	trr.DeadAfter.Unset()

	if op.PerformanceStats != nil {
		perfStatsV := *op.PerformanceStats
		perfStats := &perfStatsV
		perfStats.Key = model.PerformanceStatsKey(ctx, tr.Key)
		toPut = append(toPut, perfStats)
	}

	// Copy the common fields from trr to trs, and update other fields.
	trsServerVers := trs.ServerVersions
	trs.TaskResultCommon = trr.TaskResultCommon
	// Do not copy trr.ServerVersions, these entities may be touched by
	// different versions.
	trs.ServerVersions = trsServerVers

	if err := datastore.Put(ctx, toPut...); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to save task %q: %s", taskID, err)
	}

	var botEventType model.BotEventType
	if !op.Abandoned && !op.TaskError && op.ClientError == nil {
		// No need to calculate BotEventType for abandoning a task.
		botEventType, err = calculateBotEventType(trs.State)
		if err != nil {
			logging.Errorf(ctx, "failed to calculate bot event type for completing %q: %s", taskID, err)
			return nil, status.Errorf(codes.Internal, "failed to calculate bot event type for completing %q", taskID)
		}
	}

	// PubSub notification and update BuildTask
	if err := notifications.SendOnTaskUpdate(ctx, m.disp, tr, trs); err != nil {
		logging.Errorf(ctx, "failed to enqueue pubsub notification tasks for completing %q: %s", taskID, err)
		return nil, status.Errorf(codes.Internal, "failed to create resultdb client")
	}

	// Cancel Child tasks and finalize ResultDB invocation
	err = m.disp.AddTask(ctx, &tq.Task{
		Payload: &taskspb.FinalizeTask{
			TaskId: taskID,
		},
	})
	if err != nil {
		logging.Errorf(ctx, "failed to enqueue finalization task for completing %q: %s", taskID, err)
		return nil, status.Errorf(codes.Internal, "failed to enqueue finalization task for completing %q", taskID)
	}

	// Report metrics in case the transaction actually lands.
	txndefer.Defer(ctx, func(ctx context.Context) {
		onTaskCompleted(ctx, trs)
	})

	return &CompleteTxnOutcome{Updated: true, BotEventType: botEventType}, nil
}

func (op *CompleteOp) calculateState(trr *model.TaskRunResult) apipb.TaskState {
	switch {
	case op.Canceled:
		return apipb.TaskState_CANCELED
	case op.HardTimeout || op.IOTimeout:
		return apipb.TaskState_TIMED_OUT
	case trr.Killing:
		// trr.Killing almost always sets the state to `KILLED`, except when
		// the task is also timed out.
		return apipb.TaskState_KILLED
	case op.ClientError != nil:
		return apipb.TaskState_CLIENT_ERROR
	case op.Abandoned, op.TaskError:
		return apipb.TaskState_BOT_DIED
	case op.ExitCode != nil:
		return apipb.TaskState_COMPLETED
	default:
		return apipb.TaskState_RUNNING
	}
}

func setExitCodeAndDurationFallbacks(trr *model.TaskRunResult, now time.Time) {
	if !trr.ExitCode.IsSet() {
		trr.ExitCode.Set(-1)
	}
	if !trr.DurationSecs.IsSet() {
		trr.DurationSecs.Set(now.Sub(trr.Started.Get()).Seconds())
	}
}

func calculateBotEventType(taskState apipb.TaskState) (model.BotEventType, error) {
	switch taskState {
	case apipb.TaskState_COMPLETED,
		apipb.TaskState_TIMED_OUT,
		apipb.TaskState_CANCELED:
		return model.BotEventTaskCompleted, nil
	case apipb.TaskState_KILLED:
		return model.BotEventTaskKilled, nil
	default:
		return "", errors.Reason("unexpected task state %s", taskState.String()).Err()
	}
}

func (m *managerImpl) finalizeResultDBInvocation(ctx context.Context, taskID string) error {
	reqKey, err := model.TaskIDToRequestKey(ctx, taskID)
	if err != nil {
		return err
	}

	tr, err := model.FetchTaskRequest(ctx, reqKey)
	switch {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return errors.Annotate(err, "task %q not found", taskID).Tag(tq.Fatal).Err()
	case err != nil:
		return errors.Annotate(err, "failed to get task %q", taskID).Tag(transient.Tag).Err()
	}
	if tr.ResultDBUpdateToken == "" {
		return nil
	}

	trs := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, reqKey)}
	switch err := datastore.Get(ctx, trs); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return errors.Annotate(err, "task %q not found", taskID).Tag(tq.Fatal).Err()
	case err != nil:
		return errors.Annotate(err, "failed to get task %q", taskID).Tag(transient.Tag).Err()
	}
	if trs.ResultDBInfo.Hostname == "" {
		return errors.Reason("task result %q misses resultdb info", taskID).Tag(tq.Fatal).Err()
	}
	project, _ := realms.Split(trs.RequestRealm)
	client, err := m.rdb.MakeClient(ctx, trs.ResultDBInfo.Hostname, project)
	if err != nil {
		return errors.Annotate(err, "failed to create resultdb client").Tag(transient.Tag).Err()
	}
	return client.FinalizeInvocation(ctx, trs.ResultDBInfo.Invocation, tr.ResultDBUpdateToken)
}
