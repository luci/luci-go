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

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/model"
)

// UpdateOp represents an operation of a bot updating the task it's running.
//
// The update is not suppose to complete a task. Use CompleteTxn for that.
type UpdateOp struct {
	// TaskRequest entity key.
	RequestKey *datastore.Key

	// ID of the bot sending the task update.
	BotID string

	// Updates of the ask.
	// CostUSD is an approximate bot time cost spent executing this task.
	CostUSD float64
	// Output is the data to append to the stdout content for the task.
	Output []byte
	// OutputChunkStart is the index of output in the stdout stream.
	OutputChunkStart int64

	trr *model.TaskRunResult
	trs *model.TaskResultSummary
	now time.Time
}

// runUpdateTxn performs the shared updates on task result entities for both
// task update and completion.
//
// Updates op.trr and op.trs in place, and return the new/updated TaskOutputChunk.
// updateTxn performs the shared updates on task result entities for both
// task update and completion.
//
// Updates op.trr and op.trs in place, and return the new/updated TaskOutputChunk.
func (m *managerImpl) runUpdateTxn(ctx context.Context, op *UpdateOp) ([]*model.TaskOutputChunk, error) {
	trr := op.trr
	trs := op.trs

	trr.Modified = op.now
	trr.CostUSD = max(trr.CostUSD, op.CostUSD)
	trr.ServerVersions = addServerVersion(trr.ServerVersions, m.serverVersion)

	trs.Modified = op.now
	trs.CostUSD = trr.CostUSD
	trs.ServerVersions = addServerVersion(trs.ServerVersions, m.serverVersion)

	if len(op.Output) == 0 {
		return nil, nil
	}

	outputChunks, stdoutChunks, err := op.updateOutput(ctx)
	if err != nil {
		return nil, err
	}

	trr.StdoutChunks = stdoutChunks
	trs.StdoutChunks = stdoutChunks

	return outputChunks, nil
}

func (op *UpdateOp) updateOutput(ctx context.Context) ([]*model.TaskOutputChunk, int64, error) {
	return nil, 0, errors.New("not implemented yet")
}

func addServerVersion(versions []string, newVersion string) []string {
	versionSet := stringset.NewFromSlice(versions...)
	versionSet.Add(newVersion)
	return versionSet.ToSortedSlice()
}
