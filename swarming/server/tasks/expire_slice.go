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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/model"
)

// ExpireSliceOp represent an operation of switching to the next task slice.
//
// Despite its name, this can happen for a variety of reasons, not just passage
// of time (see Reason field).
type ExpireSliceOp struct {
	// Request is the task's TaskRequest entity.
	Request *model.TaskRequest
	// ToRunKey is the key of TaskToRun to expire (identifies the slice).
	ToRunKey *datastore.Key
	// Reason is the reason the slice is marked as expired.
	Reason ExpireReason
	// CulpritBotID is the bot that is likely responsible for BOT_INTERNAL_ERROR
	// if known.
	CulpritBotID string
	// Config is a snapshot of the server configuration.
	Config *cfg.Config
}

// ExpireSliceTxnOutcome is returned by ExpireSliceTxn.
type ExpireSliceTxnOutcome struct {
	Expired   bool // if true, actually expired the slice, otherwise did nothing
	Completed bool // if true, actually expired the last task slice
}

// Reason is the reason the slice is marked as expired.
//
// Its literal string value shows up in metric fields.
type ExpireReason string

const (
	ReasonUnspecified ExpireReason = ""
	NoResource        ExpireReason = "no_resource"        // no bots alive that match the requested dimensions
	PermissionDenied  ExpireReason = "permission_denied"  // no access to the RBE instance
	InvalidArgument   ExpireReason = "invalid_argument"   // RBE didn't like something about the reservation
	BotInternalError  ExpireReason = "bot_internal_error" // the bot picked up the reservation and then died
	Expired           ExpireReason = "expired"            // the scheduling deadline exceeded, discovered via RBE PubSub
	ExpiredViaScan    ExpireReason = "expired_via_scan"   // the scheduling deadline exceeded, discovered via scanning
)

// ExpireSliceTxn is called inside a transaction to mark the task slice as
// expired if it's still pending.
//
// If the task has more slices, enqueues the next slice. Otherwise marks the
// whole task as expired by updating TaskResultSummary.
func (m *managerImpl) ExpireSliceTxn(ctx context.Context, op *ExpireSliceOp) (*ExpireSliceTxnOutcome, error) {
	taskID := model.RequestKeyToTaskID(op.Request.Key, model.AsRequest)
	trs := &model.TaskResultSummary{
		Key: model.TaskResultSummaryKey(ctx, op.Request.Key),
	}
	ttr := &model.TaskToRun{
		Key: op.ToRunKey,
	}
	switch err := datastore.Get(ctx, trs, ttr); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		// Nothing to expire
		logging.Warningf(ctx, "task %q not found", taskID)
		return &ExpireSliceTxnOutcome{}, nil
	case err != nil:
		return nil, errors.Fmt("datastore error fetching task result entities for task %s: %w", taskID, err)
	}

	if !ttr.IsReapable() {
		return &ExpireSliceTxnOutcome{}, nil
	}

	toPut := []any{ttr, trs}
	var terminalState apipb.TaskState
	switch op.Reason {
	case Expired, ExpiredViaScan:
		terminalState = apipb.TaskState_EXPIRED
	case NoResource:
		terminalState = apipb.TaskState_NO_RESOURCE
	case BotInternalError:
		terminalState = apipb.TaskState_BOT_DIED
	default:
		terminalState = apipb.TaskState_EXPIRED
	}

	now := clock.Now(ctx).UTC()

	// Update the current TaskToRun.
	curIdx := ttr.TaskSliceIndex()
	if terminalState == apipb.TaskState_EXPIRED {
		delay := now.Sub(ttr.Expiration.Get()).Seconds()
		ttr.ExpirationDelay.Set(max(0.0, delay))
	}
	trs.ConsumeTaskToRun(ttr, "")

	// Create the next TaskToRun if there are more slices to run.
	var newTTR *model.TaskToRun
	if nextIdx := curIdx + 1; nextIdx < len(op.Request.TaskSlices) {
		var err error
		newTTR, err = model.NewTaskToRun(ctx, m.serverProject, op.Request, nextIdx)
		if err != nil {
			return nil, err
		}
		toPut = append(toPut, newTTR)
		if err := EnqueueRBENew(ctx, m.disp, op.Request, newTTR, op.Config); err != nil {
			return nil, err
		}
	}

	// Update TaskResultSummary.
	trs.Modified = now
	trs.ServerVersions = addServerVersion(trs.ServerVersions, m.serverVersion)
	if newTTR != nil {
		trs.ActivateTaskToRun(newTTR)
	} else {
		trs.State = terminalState
		trs.Completed.Set(now)
		trs.Abandoned.Set(now)
		trs.InternalFailure = terminalState == apipb.TaskState_BOT_DIED
		if terminalState == apipb.TaskState_BOT_DIED && op.CulpritBotID != "" {
			trs.BotID.Set(op.CulpritBotID)
		}
		if terminalState == apipb.TaskState_EXPIRED {
			delay := now.Sub(op.Request.Expiration).Seconds()
			trs.ExpirationDelay.Set(max(0.0, delay))
		}
	}

	if err := datastore.Put(ctx, toPut...); err != nil {
		return nil, err
	}

	// Report metrics in case the transaction actually lands.
	txndefer.Defer(ctx, func(ctx context.Context) {
		onTaskExpired(ctx, trs, ttr, string(op.Reason))
	})

	if newTTR == nil {
		if err := m.onTaskComplete(ctx, taskID, op.Request, trs); err != nil {
			return nil, err
		}
	}

	return &ExpireSliceTxnOutcome{
		Expired:   true,
		Completed: newTTR == nil,
	}, nil
}
