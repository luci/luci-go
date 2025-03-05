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
	"fmt"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
)

// ClaimOp represents an operation of a bot claiming a pending task.
//
// It has 2 parts: the transactional part and the post-transaction follow up.
// The actual transaction happens as part of the BotInfo update, see
// botapi/claim.go.
type ClaimOp struct {
	// Request is the TaskRequest the slice of which is being claimed.
	Request *model.TaskRequest
	// TaskToRunKey is the key of TaskToRun being claimed (identifies the slice).
	TaskToRunKey *datastore.Key
	// ClaimID is the identifier of the claim for idempotency.
	ClaimID string
	// LifecycleTasks is used to emit TQ tasks related to Swarming task lifecycle.
	LifecycleTasks LifecycleTasks
	// ServerVersion is the current Swarming server version.
	ServerVersion string
}

// BotDetails are details of the claiming bot relevant for the claim operation.
type BotDetails struct {
	// Dimensions is the full set of bot dimensions.
	Dimensions model.BotDimensions
	// Version is the version of the claiming bot.
	Version string
	// LogsCloudProject is where the bot keep its Cloud Logging logs.
	LogsCloudProject string
	// IdleSince is when this bot finished its previous task.
	IdleSince time.Time
}

// ClaimTxnOutcome is returned by ClaimTxn.
type ClaimTxnOutcome struct {
	Unavailable string // a reason if the task to run can't be claimed
	Claimed     bool   // true if claimed, false if was already claimed with the same ID

	// The stored TaskResultSummary, if any.
	trs *model.TaskResultSummary
	// The consumed TaskToRun, if any.
	ttr *model.TaskToRun
	// When the transaction callback was running.
	txnTime time.Time
}

// ClaimTxn is called inside a transaction to mark the task slice as claimed.
//
// This updates all necessary task entities to indicate the task is running now.
// It returns an ClaimTxnOutcome which then should be passed to Finished call
// after the transaction to do any post-transaction followups, like reporting
// metrics.
//
// May be retried a bunch of times in case the transaction is retried.
func (op *ClaimOp) ClaimTxn(ctx context.Context, bot *BotDetails) (*ClaimTxnOutcome, error) {
	botID := bot.Dimensions.BotID()
	if botID == "" {
		panic(fmt.Sprintf("no bot ID in dimensions %q", bot.Dimensions))
	}

	ttr := &model.TaskToRun{Key: op.TaskToRunKey}
	trs := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, op.Request.Key)}
	switch err := datastore.Get(ctx, ttr, trs); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		// This happens if at least one entity is missing.
		return &ClaimTxnOutcome{Unavailable: "No such task"}, nil
	case err != nil:
		return nil, errors.Annotate(err, "fetching task entities").Err()
	}

	// Recheck TaskToRun is still unclaimed.
	if !ttr.IsReapable() {
		switch existing := ttr.ClaimID.Get(); {
		case existing == "":
			return &ClaimTxnOutcome{Unavailable: "The task slice has expired"}, nil
		case existing != op.ClaimID:
			return &ClaimTxnOutcome{Unavailable: fmt.Sprintf("Already claimed by %q", existing)}, nil
		default:
			// This task is already claimed by us.
			return &ClaimTxnOutcome{Claimed: false}, nil
		}
	}

	// The task must still be pending, since only pending tasks are "reapable".
	if trs.State != apipb.TaskState_PENDING {
		logging.Errorf(ctx, "The task %s is in unexpected state %s",
			model.RequestKeyToTaskID(op.Request.Key, model.AsRequest), trs.State)
	}

	now := clock.Now(ctx).UTC()
	idleSince := bot.IdleSince
	if idleSince.IsZero() {
		idleSince = now
	}

	trsServerVers := stringset.NewFromSlice(trs.ServerVersions...)
	trsServerVers.Add(op.ServerVersion)

	// Create TaskRunResult representing a running task assigned to a bot.
	run := &model.TaskRunResult{
		Key:            model.TaskRunResultKey(ctx, op.Request.Key),
		RequestCreated: op.Request.Created,
		RequestTags:    op.Request.Tags,
		RequestName:    op.Request.Name,
		RequestUser:    op.Request.User,
		BotID:          botID,
		DeadAfter:      datastore.NewUnindexedOptional(now.Add(time.Duration(op.Request.BotPingToleranceSecs) * time.Second)),
		TaskResultCommon: model.TaskResultCommon{
			State:               apipb.TaskState_RUNNING,
			Modified:            now,
			BotVersion:          bot.Version,
			BotDimensions:       bot.Dimensions,
			BotIdleSince:        datastore.NewUnindexedOptional(idleSince),
			BotLogsCloudProject: bot.LogsCloudProject,
			ServerVersions:      []string{op.ServerVersion},
			CurrentTaskSlice:    int64(ttr.TaskSliceIndex()),
			Started:             datastore.NewIndexedNullable(now),
			ResultDBInfo:        trs.ResultDBInfo,
		},
	}

	// Update TaskResultSummary to indicate the task is running now.
	trs.TaskResultCommon = run.TaskResultCommon
	trs.BotID.Set(botID)
	trs.TryNumber.Set(1)

	// Repopulate ServerVersions which was clobbered in TaskResultCommon
	// assignment above. Note that TaskRunResult was touched only by one server
	// version (we just created it), but TaskResultSummary can be touched by many.
	trs.ServerVersions = trsServerVers.ToSortedSlice()

	// Mark TaskToRun as claimed.
	ttr.Consume(op.ClaimID)

	// Store this all back.
	if err := datastore.Put(ctx, ttr, trs, run); err != nil {
		return nil, errors.Annotate(err, "storing task entities").Err()
	}
	// Emit PubSub notification indicating the task is running now.
	if err := op.LifecycleTasks.sendOnTaskUpdate(ctx, op.Request, trs); err != nil {
		return nil, errors.Annotate(err, "sending task updates").Err()
	}

	// Store some state for reporting metrics in Finished.
	return &ClaimTxnOutcome{
		Claimed: true,
		trs:     trs,
		ttr:     ttr,
		txnTime: now,
	}, nil
}

// Finished is called after ClaimTxn transaction lands.
func (op *ClaimOp) Finished(ctx context.Context, outcome *ClaimTxnOutcome) {
	if outcome.Claimed {
		onTaskStatusChangeSchedulerLatency(ctx, outcome.trs)
		onTaskToRunConsumed(ctx, outcome.ttr, outcome.trs, outcome.txnTime)
	}
}
