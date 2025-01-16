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

package rpcs

import (
	"bytes"
	"context"
	"testing"

	"github.com/klauspost/compress/zlib"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
	"go.chromium.org/luci/swarming/server/acls"
	"go.chromium.org/luci/swarming/server/model"
)

func TestGetStdout(t *testing.T) {
	t.Parallel()

	state := NewMockedRequestState()
	state.MockPerm("project:visible-realm", acls.PermTasksGet)

	ctx := memory.Use(context.Background())

	putSummary := func(req *datastore.Key, realm string, dedupFrom *datastore.Key, pending bool) {
		var tryNumber datastore.Nullable[int64, datastore.Indexed]
		var dedupFromRunID string
		if dedupFrom != nil {
			dedupFromRunID = model.RequestKeyToTaskID(dedupFrom, model.AsRunResult)
		} else if !pending {
			tryNumber = datastore.NewIndexedNullable(int64(1))
		}
		err := datastore.Put(ctx, &model.TaskResultSummary{
			TaskResultCommon: model.TaskResultCommon{
				StdoutChunks: 1,
			},
			Key:          model.TaskResultSummaryKey(ctx, req),
			RequestRealm: realm,
			TryNumber:    tryNumber,
			DedupedFrom:  dedupFromRunID,
		})
		if err != nil {
			panic(err)
		}
	}

	putLog := func(req *datastore.Key, log string) {
		var compressed bytes.Buffer
		w := zlib.NewWriter(&compressed)
		if _, err := w.Write([]byte(log)); err != nil {
			panic(err)
		}
		if err := w.Close(); err != nil {
			panic(err)
		}
		err := datastore.Put(ctx, &model.TaskOutputChunk{
			Key:   model.TaskOutputChunkKey(ctx, req, 0),
			Chunk: compressed.Bytes(),
		})
		if err != nil {
			panic(err)
		}
	}

	visible, _ := model.TimestampToRequestKey(ctx, TestTime, 1)
	hidden, _ := model.TimestampToRequestKey(ctx, TestTime, 2)
	missing, _ := model.TimestampToRequestKey(ctx, TestTime, 3)
	dedupped, _ := model.TimestampToRequestKey(ctx, TestTime, 4)
	deduppedFrom, _ := model.TimestampToRequestKey(ctx, TestTime, 5)
	pending, _ := model.TimestampToRequestKey(ctx, TestTime, 6)

	visibleID := model.RequestKeyToTaskID(visible, model.AsRequest)
	hiddenID := model.RequestKeyToTaskID(hidden, model.AsRequest)
	missingID := model.RequestKeyToTaskID(missing, model.AsRequest)
	deduppedID := model.RequestKeyToTaskID(dedupped, model.AsRequest)
	pendingID := model.RequestKeyToTaskID(pending, model.AsRequest)

	putSummary(visible, "project:visible-realm", nil, false)
	putSummary(hidden, "project:hidden", nil, false)
	putSummary(dedupped, "project:visible-realm", deduppedFrom, false)
	putSummary(deduppedFrom, "project:doesntmatter", nil, false)
	putSummary(pending, "project:visible-realm", nil, true)

	putLog(visible, "visible log")
	putLog(hidden, "hidden log")
	putLog(deduppedFrom, "dedupped log")

	call := func(taskID string, offset, length int) (*apipb.TaskOutputResponse, error) {
		ctx := MockRequestState(ctx, state)
		return (&TasksServer{}).GetStdout(ctx, &apipb.TaskIdWithOffsetRequest{
			TaskId: taskID,
			Offset: int64(offset),
			Length: int64(length),
		})
	}

	ftt.Run("Missing task ID", t, func(t *ftt.Test) {
		_, err := call("", 0, 100)
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})

	ftt.Run("Bad task ID", t, func(t *ftt.Test) {
		_, err := call("not-a-task-id", 0, 100)
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})

	ftt.Run("Missing task", t, func(t *ftt.Test) {
		// Note: existence of a task is not a secret (task IDs are predictable).
		_, err := call(missingID, 0, 100)
		assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
	})

	ftt.Run("No permissions", t, func(t *ftt.Test) {
		_, err := call(hiddenID, 0, 100)
		assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
	})

	ftt.Run("Negative offset", t, func(t *ftt.Test) {
		_, err := call(visibleID, -1, 100)
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})

	ftt.Run("Negative length", t, func(t *ftt.Test) {
		_, err := call(visibleID, 0, -1)
		assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
	})

	ftt.Run("Read all", t, func(t *ftt.Test) {
		res, err := call(visibleID, 0, 0)
		assert.NoErr(t, err)
		assert.Loosely(t, string(res.Output), should.Equal("visible log"))
	})

	ftt.Run("Read all from dedupped", t, func(t *ftt.Test) {
		res, err := call(deduppedID, 0, 0)
		assert.NoErr(t, err)
		assert.Loosely(t, string(res.Output), should.Equal("dedupped log"))
	})

	ftt.Run("Read all from pending", t, func(t *ftt.Test) {
		res, err := call(pendingID, 0, 0)
		assert.NoErr(t, err)
		assert.Loosely(t, res.Output, should.HaveLength(0))
	})

	ftt.Run("Respects offset and length", t, func(t *ftt.Test) {
		res, err := call(visibleID, 1, len("visible log")-2)
		assert.NoErr(t, err)
		assert.Loosely(t, string(res.Output), should.Equal("isible lo"))
	})
}
