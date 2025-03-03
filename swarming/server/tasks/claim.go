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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

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
}

// ClaimTxn is called inside a transaction to mark the task slice as claimed.
//
// This updates all necessary task entities to indicate the task is running now.
// It also mutates `op` state itself, which is then used in Finished call that
// happens after the transaction.
//
// May be retried a bunch of times in case the transaction is retried.
func (op *ClaimOp) ClaimTxn(ctx context.Context, bot *BotDetails) (*ClaimTxnOutcome, error) {
	return nil, errors.Reason("not implemented").Err()
}

// Finished is called after ClaimTxn transaction lands.
func (op *ClaimOp) Finished(ctx context.Context, outcome *ClaimTxnOutcome) {
	// TODO
}
