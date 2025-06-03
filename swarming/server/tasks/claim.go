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
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/notifications"
)

// ClaimOp represents an operation of a bot claiming a pending task.
type ClaimOp struct {
	// Request is the TaskRequest the slice of which is being claimed.
	Request *model.TaskRequest
	// TaskToRunKey is the key of TaskToRun being claimed (identifies the slice).
	TaskToRunKey *datastore.Key
	// ClaimID is the identifier of the claim for idempotency.
	ClaimID string
	// BotDimensions is the full set of bot dimensions.
	BotDimensions model.BotDimensions
	// BotVersion is the version of the claiming bot.
	BotVersion string
	// BotLogsCloudProject is where the bot keep its Cloud Logging logs.
	BotLogsCloudProject string
	// BotIdleSince is when this bot finished its previous task.
	BotIdleSince time.Time
	// BotOwners is the list of bot owners from bots.cfg.
	BotOwners []string
}

// ClaimOpOutcome is returned by ClaimTxn.
type ClaimOpOutcome struct {
	Unavailable string // a reason if the task to run can't be claimed
	Claimed     bool   // true if claimed, false if was already claimed with the same ID
}

// ClaimTxn is called inside a transaction to mark the task slice as claimed.
func (m *managerImpl) ClaimTxn(ctx context.Context, op *ClaimOp) (*ClaimOpOutcome, error) {
	botID := op.BotDimensions.BotID()
	if botID == "" {
		panic(fmt.Sprintf("no bot ID in dimensions %q", op.BotDimensions))
	}

	ttr := &model.TaskToRun{Key: op.TaskToRunKey}
	trs := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, op.Request.Key)}
	switch err := datastore.Get(ctx, ttr, trs); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		// This happens if at least one entity is missing.
		return &ClaimOpOutcome{Unavailable: "No such task"}, nil
	case err != nil:
		return nil, errors.Fmt("fetching task entities: %w", err)
	}

	// Recheck TaskToRun is still unclaimed.
	if !ttr.IsReapable() {
		switch existing := ttr.ClaimID.Get(); {
		case existing == "":
			return &ClaimOpOutcome{Unavailable: "The task slice has expired"}, nil
		case existing != op.ClaimID:
			return &ClaimOpOutcome{Unavailable: fmt.Sprintf("Already claimed by %q", existing)}, nil
		default:
			// This task is already claimed by us.
			return &ClaimOpOutcome{Claimed: false}, nil
		}
	}

	// The task must still be pending, since only pending tasks are "reapable".
	if trs.State != apipb.TaskState_PENDING {
		logging.Errorf(ctx, "The task %s is in unexpected state %s",
			model.RequestKeyToTaskID(op.Request.Key, model.AsRequest), trs.State)
	}

	now := clock.Now(ctx).UTC()
	idleSince := op.BotIdleSince
	if idleSince.IsZero() {
		idleSince = now
	}

	trsServerVers := stringset.NewFromSlice(trs.ServerVersions...)
	trsServerVers.Add(m.serverVersion)

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
			BotVersion:          op.BotVersion,
			BotDimensions:       op.BotDimensions,
			BotIdleSince:        datastore.NewUnindexedOptional(idleSince),
			BotLogsCloudProject: op.BotLogsCloudProject,
			BotOwners:           op.BotOwners,
			ServerVersions:      []string{m.serverVersion},
			CurrentTaskSlice:    int64(ttr.TaskSliceIndex()),
			Started:             datastore.NewIndexedNullable(now),
			ResultDBInfo:        trs.ResultDBInfo,
		},
	}

	// Update TaskResultSummary to indicate the task is running now.
	trs.TaskResultCommon = run.TaskResultCommon
	trs.BotID.Set(botID)
	trs.TryNumber.Set(1)

	// Mark TaskToRun as claimed (this updates `trs` as well).
	trs.ConsumeTaskToRun(ttr, op.ClaimID)

	// Repopulate ServerVersions which was clobbered in TaskResultCommon
	// assignment above. Note that TaskRunResult was touched only by one server
	// version (we just created it), but TaskResultSummary can be touched by many.
	trs.ServerVersions = trsServerVers.ToSortedSlice()

	// Store this all back.
	if err := datastore.Put(ctx, ttr, trs, run); err != nil {
		return nil, errors.Fmt("storing task entities: %w", err)
	}
	// Emit PubSub notification indicating the task is running now.
	if err := notifications.SendOnTaskUpdate(ctx, m.disp, op.Request, trs); err != nil {
		return nil, errors.Fmt("sending task updates: %w", err)
	}

	// Report metrics in case the transaction actually lands.
	txndefer.Defer(ctx, func(ctx context.Context) {
		onTaskStatusChangeSchedulerLatency(ctx, trs)
		onTaskToRunConsumed(ctx, ttr, trs, now)
	})

	// Store some state for reporting metrics in Finished.
	return &ClaimOpOutcome{Claimed: true}, nil
}
