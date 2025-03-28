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
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/model"
)

// UpdateOp represents an operation of a bot updating the task it's running.
//
// The actual transaction happens as part of the BotInfo update, see
// botapi/task_update.go.
type UpdateOp struct {
	// TaskRequest entity key.
	RequestKey *datastore.Key

	// ID of the bot sending the task update.
	BotID string

	// Performance stats reported by the bot.
	PerformanceStats *model.PerformanceStats

	// Updates of the ask.
	// Canceled is a bool on whether the task has been canceled at set up stage.
	Canceled bool
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

	// LifecycleTasks is used to emit TQ tasks related to Swarming task lifecycle.
	LifecycleTasks LifecycleTasks
	// ServerVersion is the current Swarming server version.
	ServerVersion string
}
