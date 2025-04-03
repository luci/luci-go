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
)

// AbandonOp represents an operation of a bot abandoning a task.
//
// This happens if the bot dies midway through task execution.
type AbandonOp struct {
	// BotID is the bot that abandons tasks.
	BotID string
	// TaskID is the task that is being abandoned.
	TaskID string
	// LifecycleTasks is used to emit TQ tasks related to Swarming task lifecycle.
	LifecycleTasks LifecycleTasks
	// ServerVersion is the current Swarming server version.
	ServerVersion string
}

// AbandonTxn runs the transactional logic to finalize the abandoned task.
func (op *AbandonOp) AbandonTxn(ctx context.Context) error {
	if !op.LifecycleTasks.ShouldAbandonTasks() {
		return nil
	}
	// TODO
	return nil
}

// Finished is called once the transaction lands to report metrics etc.
func (op *AbandonOp) Finished(ctx context.Context) {
	// TODO
}
