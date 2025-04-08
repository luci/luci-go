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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/model"
)

// CompleteOp represents an operation to complete a running task.
//
// The actual transaction happens as part of the BotInfo update, see
// botapi/task_update.go.
type CompleteOp struct {
	// TaskRequest entity key.
	RequestKey *datastore.Key

	// BotID is ID of the bot completing the task.
	BotID string

	// PerformanceStats is performance stats reported by the bot.
	PerformanceStats *model.PerformanceStats

	// Updates of the ask.
	// Canceled is a bool on whether the task has been canceled at set up stage.
	Canceled bool
	// TODO: add fields to handle other type of task completion, e.g. abandon a task.

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
	return nil, status.Errorf(codes.Unimplemented, "not implemented yet")
}
