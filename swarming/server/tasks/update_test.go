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
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/model"
	"go.chromium.org/luci/swarming/server/tqtasks"
)

func TestUpdateOp(t *testing.T) {
	t.Parallel()

	ftt.Run("With mocks", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		now := time.Date(2044, time.February, 3, 4, 5, 0, 0, time.UTC)
		ctx, _ = testclock.UseTime(ctx, now)
		ctx, tqt := tqtasks.TestingContext(ctx)

		taskID := "65aba3a3e6b99200"
		reqKey, err := model.TaskIDToRequestKey(ctx, taskID)
		assert.NoErr(t, err)

		mgr := NewManager(tqt.Tasks, "swarming-proj", "cur-version", nil, false)

		type entities struct {
			tr  *model.TaskRequest
			trs *model.TaskResultSummary
			trr *model.TaskRunResult
		}
		createTaskEntities := func(state apipb.TaskState) entities {
			tr := &model.TaskRequest{
				Key:                  reqKey,
				BotPingToleranceSecs: 300,
			}
			trc := model.TaskResultCommon{
				State: state,
				ResultDBInfo: model.ResultDBInfo{
					Hostname:   "resultdb.example.com",
					Invocation: "invocations/task-65aba3a3e6b99201",
				},
				ServerVersions: []string{"v1"},
				Started:        datastore.NewIndexedNullable(now.Add(-time.Hour)),
			}

			trs := &model.TaskResultSummary{
				Key:              model.TaskResultSummaryKey(ctx, reqKey),
				TaskResultCommon: trc,
				RequestRealm:     "project:realm",
			}
			trs.ServerVersions = addServerVersion(trs.ServerVersions, "v2")

			trr := &model.TaskRunResult{
				Key:              model.TaskRunResultKey(ctx, reqKey),
				TaskResultCommon: trc,
				BotID:            "bot1",
				DeadAfter:        datastore.NewUnindexedOptional(now.Add(time.Hour)),
			}
			assert.NoErr(t, datastore.Put(ctx, tr, trs, trr))
			return entities{
				tr:  tr,
				trs: trs,
				trr: trr,
			}
		}

		t.Run("task_already_complete", func(t *ftt.Test) {
			ents := createTaskEntities(apipb.TaskState_BOT_DIED)
			op := &UpdateOp{
				Request: ents.tr,
				BotID:   "bot1",
			}
			outcome, err := mgr.UpdateTxn(ctx, op)
			assert.NoErr(t, err)
			assert.That(t, outcome.MustStop, should.BeTrue)
		})

		t.Run("update_task", func(t *ftt.Test) {
			ents := createTaskEntities(apipb.TaskState_RUNNING)
			op := &UpdateOp{
				Request:          ents.tr,
				BotID:            "bot1",
				CostUSD:          10,
				Output:           bytes.Repeat([]byte("a"), 100),
				OutputChunkStart: 0,
			}
			outcome, err := mgr.UpdateTxn(ctx, op)
			assert.NoErr(t, err)
			assert.That(t, outcome.MustStop, should.BeFalse)

			trs := &model.TaskResultSummary{Key: model.TaskResultSummaryKey(ctx, reqKey)}
			trr := &model.TaskRunResult{Key: model.TaskRunResultKey(ctx, reqKey)}
			assert.NoErr(t, datastore.Get(ctx, trs, trr))
			assert.That(t, trs.CostUSD, should.Equal(op.CostUSD))
			assert.That(t, trs.ServerVersions, should.Match([]string{"cur-version", "v1", "v2"}))
			assert.That(t, trs.StdoutChunks, should.Equal(int64(1)))
			assert.That(t, trs.Modified, should.Match(now))
			assert.That(t, trr.DeadAfter.Get(), should.Match(now.Add(300*time.Second)))
		})
	})
}

func TestUpdateOutput(t *testing.T) {
	t.Parallel()

	ftt.Run("updateOutput", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		taskID := "65aba3a3e6b99200"
		reqKey, err := model.TaskIDToRequestKey(ctx, taskID)
		assert.NoErr(t, err)

		type entities struct {
			tr     *model.TaskRequest
			trr    *model.TaskRunResult
			chunks []*model.TaskOutputChunk
		}
		createTaskEntities := func(stdoutChunks int64, lastChunkLegth int) entities {
			tr := &model.TaskRequest{
				Key:                  reqKey,
				BotPingToleranceSecs: 300,
			}
			trr := &model.TaskRunResult{
				Key: model.TaskRunResultKey(ctx, reqKey),
				TaskResultCommon: model.TaskResultCommon{
					StdoutChunks: stdoutChunks,
				},
			}

			chunks := make([]*model.TaskOutputChunk, stdoutChunks)
			for i := int64(0); i < stdoutChunks; i++ {
				content := bytes.Repeat([]byte("a"), model.ChunkSize)
				if i == stdoutChunks-1 {
					content = content[:lastChunkLegth]

				}
				compressed, err := compress(content)
				assert.NoErr(t, err)
				chunks[i] = &model.TaskOutputChunk{
					Key:   model.TaskOutputChunkKey(ctx, reqKey, i),
					Chunk: compressed,
				}
			}
			assert.NoErr(t, datastore.Put(ctx, tr, trr, chunks))
			return entities{
				tr:     tr,
				trr:    trr,
				chunks: chunks,
			}
		}

		call := func(tr *model.TaskRequest, trr *model.TaskRunResult, outputChunkStart int64, outputLength int) ([]*model.TaskOutputChunk, int64) {
			op := &UpdateOp{
				Request:          tr,
				BotID:            "bot1",
				trr:              trr,
				Output:           bytes.Repeat([]byte("a"), outputLength),
				OutputChunkStart: outputChunkStart,
			}

			chunks, stdoutChunks, err := op.updateOutput(ctx)
			assert.NoErr(t, err)
			return chunks, stdoutChunks
		}

		compare := func(chunk *model.TaskOutputChunk, expected []byte) {
			decompressed, err := decompress(chunk.Chunk)
			assert.NoErr(t, err)
			assert.That(t, decompressed, should.Match(expected))
		}

		t.Run("first_time_update_output_less_than_a_chunk", func(t *ftt.Test) {
			ents := createTaskEntities(0, 0)
			chunks, stdoutChunks := call(ents.tr, ents.trr, 0, 100)
			assert.That(t, stdoutChunks, should.Equal(int64(1)))
			assert.That(t, len(chunks), should.Equal(1))
			compare(chunks[0], bytes.Repeat([]byte("a"), 100))
		})

		t.Run("first_time_update_output_multiple_chunks", func(t *ftt.Test) {
			ents := createTaskEntities(0, 0)
			chunks, stdoutChunks := call(ents.tr, ents.trr, 0, model.ChunkSize+100)
			assert.That(t, stdoutChunks, should.Equal(int64(2)))
			assert.That(t, len(chunks), should.Equal(2))
			compare(chunks[0], bytes.Repeat([]byte("a"), model.ChunkSize))
			compare(chunks[1], bytes.Repeat([]byte("a"), 100))
		})

		t.Run("new_output_from_a_fresh_chunk_no_gap", func(t *ftt.Test) {
			ents := createTaskEntities(1, model.ChunkSize)
			chunks, stdoutChunks := call(ents.tr, ents.trr, model.ChunkSize, model.ChunkSize+100)
			assert.That(t, stdoutChunks, should.Equal(int64(3)))
			assert.That(t, len(chunks), should.Equal(2))
			compare(chunks[0], bytes.Repeat([]byte("a"), model.ChunkSize))
			compare(chunks[1], bytes.Repeat([]byte("a"), 100))

		})

		t.Run("new_output_from_a_fresh_chunk_with_gap", func(t *ftt.Test) {
			ents := createTaskEntities(1, model.ChunkSize)
			chunks, stdoutChunks := call(ents.tr, ents.trr, model.ChunkSize+5, model.ChunkSize+100)
			assert.That(t, stdoutChunks, should.Equal(int64(3)))
			assert.That(t, len(chunks), should.Equal(2))
			expectedChunk := bytes.Repeat([]byte{0}, 5)
			expectedChunk = append(expectedChunk, bytes.Repeat([]byte("a"), model.ChunkSize-5)...)
			compare(chunks[0], expectedChunk)
			compare(chunks[1], bytes.Repeat([]byte("a"), 105))
		})

		t.Run("new_output_append_to_existing_chunk_no_gap", func(t *ftt.Test) {
			ents := createTaskEntities(1, 100)
			chunks, stdoutChunks := call(ents.tr, ents.trr, 100, model.ChunkSize-100)
			assert.That(t, stdoutChunks, should.Equal(int64(1)))
			assert.That(t, len(chunks), should.Equal(1))
			compare(chunks[0], bytes.Repeat([]byte("a"), model.ChunkSize))
		})

		t.Run("new_output_append_to_existing_chunk_with_gap", func(t *ftt.Test) {
			ents := createTaskEntities(2, 100)
			chunks, stdoutChunks := call(ents.tr, ents.trr, model.ChunkSize+110, 500)
			assert.That(t, stdoutChunks, should.Equal(int64(2)))
			assert.That(t, len(chunks), should.Equal(1))
			expectedChunk := bytes.Repeat([]byte("a"), 100)
			expectedChunk = append(expectedChunk, bytes.Repeat([]byte{0}, 10)...)
			expectedChunk = append(expectedChunk, bytes.Repeat([]byte("a"), 500)...)
			compare(chunks[0], expectedChunk)
		})

		t.Run("new_output_merge_to_existing_chunk_shorter_than_existing", func(t *ftt.Test) {
			ents := createTaskEntities(2, 100)
			// The end of new content is before the end of existing content.
			chunks, stdoutChunks := call(ents.tr, ents.trr, model.ChunkSize+20, 50)
			assert.That(t, stdoutChunks, should.Equal(int64(2)))
			assert.That(t, len(chunks), should.Equal(1))
			expectedChunk := bytes.Repeat([]byte("a"), 100)
			compare(chunks[0], expectedChunk)
		})

		t.Run("new_output_merge_to_existing_chunk_longer_than_existing", func(t *ftt.Test) {
			ents := createTaskEntities(2, 100)
			// The end of new content is before the end of existing content.
			chunks, stdoutChunks := call(ents.tr, ents.trr, model.ChunkSize+20, 100)
			assert.That(t, stdoutChunks, should.Equal(int64(2)))
			assert.That(t, len(chunks), should.Equal(1))
			expectedChunk := bytes.Repeat([]byte("a"), 120)
			compare(chunks[0], expectedChunk)
		})
	})
}
