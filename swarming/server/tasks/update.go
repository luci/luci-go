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
	"bytes"
	"context"
	"io"
	"time"

	"github.com/klauspost/compress/zlib"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/model"
)

// UpdateOp represents an operation of a bot updating the task it's running.
//
// The update is not suppose to complete a task. Use CompleteTxn for that.
type UpdateOp struct {
	// Request is the TaskRequest entity for the task to complete.
	// TODO(b/355013226): Replace Request with RequestKey once
	// BotPingToleranceSecs is no longer used.
	Request *model.TaskRequest

	// BotID is ID of the bot sending the task update.
	BotID string

	// Updates of the task.
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

type UpdateTxnOutcome struct {
	// MustStop indicates whether the bot should stop running the task.
	MustStop bool

	// StopReason is the reason why the task must stop.
	StopReason string
}

// UpdateTxn runs the transactional logic to update a single task.
//
// It is used to perform updates like reporting stdout, or cost, etc.
// The update is not suppose to complete a task. Use CompleteTxn for that.
func (m *managerImpl) UpdateTxn(ctx context.Context, op *UpdateOp) (*UpdateTxnOutcome, error) {
	taskID := model.RequestKeyToTaskID(op.Request.Key, model.AsRunResult)
	trr := &model.TaskRunResult{Key: model.TaskRunResultKey(ctx, op.Request.Key)}
	trs := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, op.Request.Key)}
	switch err := datastore.Get(ctx, trr, trs); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, status.Errorf(codes.NotFound, "task %q not found", taskID)
	case err != nil:
		return nil, status.Errorf(codes.Internal, "failed to get task %q: %s", taskID, err)
	}

	if trr.BotID != op.BotID {
		return nil, status.Errorf(codes.InvalidArgument, "task %q is not running by bot %q", taskID, op.BotID)
	}

	if !trs.IsActive() {
		// It's possible that the task is already abandoned (i.e. the task has
		// completed with BOT_DIED or KILLED states). Inform the bot to stop
		// running it.
		logging.Debugf(ctx, "Task %q is already completed with state %s", taskID, trs.State)
		return &UpdateTxnOutcome{MustStop: true, StopReason: "The task is already marked as completed on the server"}, nil
	}

	toPut := []any{trr, trs}
	op.trr = trr
	op.trs = trs
	op.now = clock.Now(ctx).UTC()

	outputChunks, err := m.runUpdateTxn(ctx, op)
	if err != nil {
		logging.Errorf(ctx, "Failed to update task %q: %s", taskID, err)
		return nil, status.Errorf(codes.Internal, "failed to update task %q", taskID)
	}
	for _, oc := range outputChunks {
		toPut = append(toPut, oc)
	}

	// Update trr.DeadAfter to avoid the python cron job killing the task.
	// TODO(b/355013226): Deprecate trr.DeadAfter after Python -> Go migration
	// is done.
	trr.DeadAfter.Set(op.now.Add(time.Duration(op.Request.BotPingToleranceSecs) * time.Second))

	if err := datastore.Put(ctx, toPut...); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update task %q: %s", taskID, err)
	}

	stopReason := ""
	if trr.Killing {
		stopReason = "Killing task"
	}

	return &UpdateTxnOutcome{MustStop: trr.Killing, StopReason: stopReason}, nil
}

// runUpdateTxn performs the shared updates on task result entities for both
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

	chunkEntities, stdoutChunks, err := op.updateOutput(ctx)
	if err != nil {
		return nil, err
	}

	trr.StdoutChunks = stdoutChunks
	trs.StdoutChunks = stdoutChunks

	return chunkEntities, nil
}

func (op *UpdateOp) updateOutput(ctx context.Context) ([]*model.TaskOutputChunk, int64, error) {
	output := op.Output

	type chunk struct {
		content     []byte
		outputStart int // output start inside a model.TaskOutputChunk.
	}
	var chunks []chunk
	var chunkEntities []*model.TaskOutputChunk
	outputChunkStart := op.OutputChunkStart
	numChunks := op.trr.StdoutChunks
	for len(output) > 0 {
		chunkNumber := outputChunkStart / model.ChunkSize
		if chunkNumber > model.MaxChunks {
			logging.Warningf(ctx, "Dropping output\n%d bytes were lost", len(op.Output))
			break
		}
		start := int(outputChunkStart % model.ChunkSize)
		nextStart := min(model.ChunkSize-start, len(output))
		outputChunk := &model.TaskOutputChunk{
			Key: model.TaskOutputChunkKey(ctx, op.Request.Key, chunkNumber),
		}
		chunks = append(
			chunks,
			chunk{content: output[:nextStart], outputStart: start})
		chunkEntities = append(chunkEntities, outputChunk)

		output = output[nextStart:]
		numChunks = max(numChunks, chunkNumber+1)
		outputChunkStart = (chunkNumber + 1) * model.ChunkSize
	}

	if len(chunkEntities) == 0 {
		return nil, numChunks, nil
	}

	merr := make(errors.MultiError, len(chunkEntities))
	if err := datastore.Get(ctx, chunkEntities); err != nil {
		if !errors.As(err, &merr) {
			return nil, numChunks, err
		}
	}

	for i, oc := range chunkEntities {
		if merr[i] != nil {
			if errors.Is(merr[i], datastore.ErrNoSuchEntity) {
				chunkEntities[i] = &model.TaskOutputChunk{
					Key: oc.Key,
				}
				oc = chunkEntities[i]
			} else {
				return nil, numChunks, errors.WrapIf(merr[i], "failed to get output chunk #%d", i)
			}
		}

		chunkContent, err := decompress(oc.Chunk)
		if err != nil {
			return nil, numChunks, errors.Fmt("failed to decompress output chunk #%d: %w", i, err)
		}

		if len(chunkContent) < chunks[i].outputStart {
			// There's a gap between the existing content and the new one,
			// fill up with empty data.
			chunkContent = append(chunkContent, bytes.Repeat([]byte{0}, chunks[i].outputStart-len(chunkContent))...)
		}

		// Append/merge new content with the existing.
		start := int32(chunks[i].outputStart)
		end := start + int32(len(chunks[i].content))
		origChunk := chunkContent
		chunkContent = append(chunkContent[:start], chunks[i].content...)
		if len(origChunk) > int(end) {
			chunkContent = append(chunkContent, origChunk[end:]...)
		}

		compressed, err := compress(chunkContent)
		if err != nil {
			return nil, numChunks, errors.Fmt("failed to compress output chunk #%d: %w", i, err)
		}
		oc.Chunk = compressed
	}

	return chunkEntities, numChunks, nil
}

func addServerVersion(versions []string, newVersion string) []string {
	versionSet := stringset.NewFromSlice(versions...)
	versionSet.Add(newVersion)
	return versionSet.ToSortedSlice()
}

func decompress(compressed []byte) ([]byte, error) {
	if len(compressed) == 0 {
		return nil, nil
	}
	zr, err := zlib.NewReader(bytes.NewReader(compressed))
	if err != nil {
		return nil, errors.Fmt("opening zlib reader: %w", err)
	}
	defer func() { _ = zr.Close() }()
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, errors.Fmt("decompressing: %w", err)
	}
	if err := zr.Close(); err != nil {
		return nil, errors.Fmt("closing zlib reader: %w", err)
	}
	return raw, nil
}

func compress(raw []byte) ([]byte, error) {
	buf := &bytes.Buffer{}
	zw := zlib.NewWriter(buf)
	if _, err := zw.Write(raw); err != nil {
		return nil, errors.Fmt("compressing: %w", err)
	}
	if err := zw.Close(); err != nil {
		return nil, errors.Fmt("closing zlib writer: %w", err)
	}
	return buf.Bytes(), nil
}
