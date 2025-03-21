// Copyright 2024 The LUCI Authors.
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

package botapi

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/swarming/server/botsrv"
	"go.chromium.org/luci/swarming/server/model"
)

// TaskUpdateRequest is sent by the bot.
type TaskUpdateRequest struct {
	// Session is a serialized Swarming Bot Session proto.
	Session []byte `json:"session"`

	// TaskID is the TaskResultSummary packed key of the task to update.
	//
	// Required.
	TaskID string `json:"task_id"`

	// Performance stats
	// BotOverhead is the total overhead in seconds, summing overhead from all
	// sections defined below.
	BotOverhead float64 `json:"bot_overhead,omitempty"`
	// CacheTrimStats is stats of cache trimming before the dependency installations.
	CacheTrimStats model.OperationStats `json:"cache_trim_stats,omitempty"`
	// CIPDStats is stats of installing CIPD packages before the task.
	CIPDStats model.OperationStats `json:"cipd_stats,omitempty"`
	// IsolatedStats is stats of CAS operations, including CAS dependencies
	// download before the task and CAS uploading operation after the task.
	IsolatedStats IsolatedStats `json:"isolated_stats,omitempty"`
	// NamedCachesInstall is stats of named cache operations, including mounting
	// before the task and unmounting after the task.
	NamedCachesStats NamedCachesStats `json:"named_caches_stats,omitempty"`
	// CleanupStats is stats of work directory cleanup operation after the task.
	CleanupStats model.OperationStats `json:"cleanup_stats,omitempty"`

	// Task updates
	// CASOutputRoot is the digest of the output root uploaded to RBE-CAS.
	CASOutputRoot model.CASReference `json:"cas_output_root,omitempty"`
	// CIPDPins is resolved versions of all the CIPD packages used in the task.
	CIPDPins model.CIPDInput `json:"cipd_pins,omitempty"`
	// CostUSD is an approximate bot time cost spent executing this task.
	CostUSD float64 `json:"cost_usd,omitempty"`
	// Duration is the time spent in seconds for this task, excluding overheads.
	Duration float64 `json:"duration,omitempty"`
	// ExitCode is the task process exit code for tasks in COMPLETED state.
	ExitCode int64 `json:"exit_code,omitempty"`
	// HardTimeout is a bool on whether a hard timeout occurred.
	HardTimeout bool `json:"hard_timeout,omitempty"`
	// IOTimeout is a bool on whether an I/O timeout occurred.
	IOTimeout bool `json:"io_timeout,omitempty"`
	// Output is the data to append to the stdout content for the task.
	Output string `json:"output,omitempty"`
	// OutputChunkStart is the index of output in the stdout stream.
	OutputChunkStart int64 `json:"output_chunk_start,omitempty"`
	// Canceled is a bool on whether the task has been canceled at set up stage.
	//
	// It is set only for the case where the server receives a request to cancel
	// the task after a bot claims it but before the bot actually starts to run
	// it.
	// * If the server receives the cancel request before the task is claimed,
	//   the task will be ended with "CANCELED" state right away and no bot can
	//   claim it.
	// * If the server receives the cancel request after the bot starts to run
	//   the task, the bot will end the task gracefully and send a task update
	//   with `duration` set when it's done. The task will then be ended with
	//   "KILLED" state.
	//
	// Task in this case will be ended with "CANCELED" state as well.
	Canceled bool `json:"canceled,omitempty"`
}

// IsolatedStats is stats of CAS operations.
type IsolatedStats struct {
	// Download is stats of CAS dependencies download before the task.
	Download model.CASOperationStats `json:"download,omitempty"`
	// Upload is stats of CAS uploading operation after the task.
	Upload model.CASOperationStats `json:"upload,omitempty"`
}

// NamedCachesStats is stats of named cache operations.
type NamedCachesStats struct {
	// Install is stats of named cache mounting before the task.
	Install model.OperationStats `json:"install,omitempty"`
	// Uninstall is stats of named cache unmounting after the task.
	Uninstall model.OperationStats `json:"uninstall,omitempty"`
}

func (r *TaskUpdateRequest) ExtractSession() []byte { return r.Session }
func (r *TaskUpdateRequest) ExtractDebugRequest() any {
	rv := *r
	rv.Session = nil
	rv.Output = fmt.Sprintf("output len: %d, content redacted", len(rv.Output))
	return &rv
}

// TaskUpdateResponse is returned by the server.
type TaskUpdateResponse struct {
	// MustStop indicates whether the bot should stop the current task.
	MustStop bool `json:"must_stop"`

	// OK indicates whether the update was processed successfully.
	OK bool `json:"ok"`

	// Session is a serialized bot session proto.
	//
	// If not empty, contains the refreshed session.
	Session []byte `json:"session,omitempty"`
}

// TaskUpdate implements handler that collects task state updates.
//
// TODO: Doc, implement.
func (srv *BotAPIServer) TaskUpdate(ctx context.Context, body *TaskUpdateRequest, r *botsrv.Request) (botsrv.Response, error) {
	return nil, status.Errorf(codes.Unimplemented, "not implemented yet")
}
