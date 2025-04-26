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
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/cfg"
	"go.chromium.org/luci/swarming/server/model"
)

type ExpireSliceOp struct {
	// Request is the task's TaskRequest entity.
	Request *model.TaskRequest
	// ToRunKey is the key of TaskToRun to expire (identifies the slice).
	ToRunKey *datastore.Key
	// Reason is the reason the slice is marked as expired.
	Reason ExpireReason
	// Details is the details of the reason.
	Details string
	// CulpritBotID is the bot that is likely responsible for BOT_INTERNAL_ERROR
	// if known.
	CulpritBotID string
	// Config is a snapshot of the server configuration.
	Config *cfg.Config
}

// Reason is the reason the slice is marked as expired.
type ExpireReason string

const (
	ReasonUnspecified ExpireReason = ""
	NoResource        ExpireReason = "no_resource"        // no bots alive that match the requested dimensions
	PermissionDenied  ExpireReason = "permission_denied"  // no access to the RBE instance
	InvalidArgument   ExpireReason = "invalid_argument"   // RBE didn't like something about the reservation
	BotInternalError  ExpireReason = "bot_internal_error" // the bot picked up the reservation and then died
	Expired           ExpireReason = "expired"            // the scheduling deadline exceeded
)

// ExpireSliceTxn is called inside a transaction to mark the task slice as
// expired if it's still pending.
//
// If the task has more slices, enqueues the next slice. Otherwise marks the
// whole task as expired by updating TaskResultSummary.
func (m *managerImpl) ExpireSliceTxn(ctx context.Context, op *ExpireSliceOp) error {
	taskID := model.RequestKeyToTaskID(op.Request.Key, model.AsRequest)
	trs := &model.TaskResultSummary{
		Key: model.TaskResultSummaryKey(ctx, op.Request.Key),
	}
	ttr := &model.TaskToRun{
		Key: op.ToRunKey,
	}
	switch err := datastore.Get(ctx, trs, ttr); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return errors.Reason("task %q not found", taskID).Err()
	case err != nil:
		return errors.Annotate(err, "datastore error fetching task result entities for task %s", taskID).Err()
	}

	if !ttr.IsReapable() {
		return nil
	}

	tpPut := []any{ttr, trs}
	var terminalState apipb.TaskState
	switch op.Reason {
	case Expired:
		terminalState = apipb.TaskState_EXPIRED
	case NoResource:
		terminalState = apipb.TaskState_NO_RESOURCE
	case BotInternalError:
		terminalState = apipb.TaskState_BOT_DIED
	default:
		terminalState = apipb.TaskState_EXPIRED
	}

	now := clock.Now(ctx).UTC()

	// Update current TaskToRun
	curIdx := ttr.TaskSliceIndex()
	if terminalState == apipb.TaskState_EXPIRED {
		delay := now.Sub(ttr.Expiration.Get()).Seconds()
		ttr.ExpirationDelay.Set(max(0.0, delay))
	}
	ttr.Consume("")

	// Create the next TaskToRun if there are more slices to run.
	var newTTR *model.TaskToRun
	var err error
	nextIdx := curIdx + 1
	if nextIdx < len(op.Request.TaskSlices) {
		newTTR, err = model.NewTaskToRun(ctx, m.serverProject, op.Request, nextIdx)
		if err != nil {
			return err
		}
		tpPut = append(tpPut, newTTR)
		if err := EnqueueRBENew(ctx, m.disp, op.Request, newTTR, op.Config); err != nil {
			return err
		}
	}

	// Update TaskResultSummary
	trs.Modified = now
	trs.ServerVersions = addServerVersion(trs.ServerVersions, m.serverVersion)
	if newTTR != nil {
		trs.CurrentTaskSlice = int64(nextIdx)
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

	if err := datastore.Put(ctx, tpPut...); err != nil {
		return err
	}

	if newTTR != nil {
		return nil
	}

	return m.onTaskComplete(ctx, taskID, op.Request, trs)
}
