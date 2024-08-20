// Copyright 2022 The LUCI Authors.
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

package authdb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/julienschmidt/httprouter"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/model"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestAuthDBServing(t *testing.T) {
	testTS := time.Date(2021, time.August, 16, 15, 20, 0, 0, time.UTC)
	testTSMicro := testTS.UnixNano() / 1000
	testHash := "SHA256-hash"
	testDeflated := []byte("deflated-groups")

	testAuthDBSnapshotLatest := func(rid int64) *model.AuthDBSnapshotLatest {
		return &model.AuthDBSnapshotLatest{
			Kind:         "AuthDBSnapshotLatest",
			ID:           "latest",
			AuthDBRev:    rid,
			AuthDBSha256: testHash,
			ModifiedTS:   testTS,
		}
	}

	testAuthDBSnapshot := func(rid int64) *model.AuthDBSnapshot {
		return &model.AuthDBSnapshot{
			Kind:           "AuthDBSnapshot",
			ID:             rid,
			AuthDBDeflated: testDeflated,
			AuthDBSha256:   testHash,
			CreatedTS:      testTS,
		}
	}

	legacyCall := func(server Server, ctx context.Context, rid int64, skipBody bool) []byte {
		rw := httptest.NewRecorder()
		var revIDStr string
		var sb string
		if rid == 0 {
			revIDStr = "latest"
		} else {
			revIDStr = strconv.FormatInt(rid, 10)
		}

		if skipBody {
			sb = "1"
		} else {
			sb = "0"
		}
		rctx := &router.Context{
			Request: (&http.Request{
				URL: &url.URL{
					RawQuery: fmt.Sprintf("skip_body=%s", sb),
				},
			}).WithContext(ctx),
			Params: []httprouter.Param{
				{Key: "revID", Value: revIDStr},
			},
			Writer: rw,
		}
		err := server.HandleLegacyAuthDBServing(rctx)
		assert.Loosely(t, err, should.BeNil)
		return rw.Body.Bytes()
	}

	t.Parallel()
	ctx := memory.Use(context.Background())

	ftt.Run("Testing pRPC API", t, func(t *ftt.Test) {
		server := Server{}
		assert.Loosely(t, datastore.Put(ctx,
			testAuthDBSnapshot(1),
			testAuthDBSnapshot(2),
			testAuthDBSnapshot(3),
			testAuthDBSnapshot(4)), should.BeNil)

		t.Run("Testing Error Codes", func(t *ftt.Test) {
			requestNegative := &rpcpb.GetSnapshotRequest{
				Revision: -1,
			}
			_, err := server.GetSnapshot(ctx, requestNegative)
			assert.Loosely(t, err, convey.Adapt(ShouldHaveGRPCStatus)(codes.InvalidArgument))

			requestNotPresent := &rpcpb.GetSnapshotRequest{
				Revision: 42,
			}
			_, err = server.GetSnapshot(ctx, requestNotPresent)
			assert.Loosely(t, err, convey.Adapt(ShouldHaveGRPCStatus)(codes.NotFound))
		})

		t.Run("Testing valid revision ID", func(t *ftt.Test) {
			request := &rpcpb.GetSnapshotRequest{
				Revision: 2,
			}
			snapshot, err := server.GetSnapshot(ctx, request)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, snapshot, should.Resemble(&rpcpb.Snapshot{
				AuthDbRev:      2,
				AuthDbSha256:   testHash,
				AuthDbDeflated: testDeflated,
				CreatedTs:      timestamppb.New(testTS),
			}))
		})

		t.Run("Testing GetSnapshotLatest", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, testAuthDBSnapshotLatest(4)), should.BeNil)
			request := &rpcpb.GetSnapshotRequest{
				Revision: 0,
			}
			latestSnapshot, err := server.GetSnapshot(ctx, request)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, latestSnapshot, should.Resemble(&rpcpb.Snapshot{
				AuthDbRev:      4,
				AuthDbSha256:   testHash,
				AuthDbDeflated: testDeflated,
				CreatedTs:      timestamppb.New(testTS),
			}))
		})

		t.Run("Testing skipbody", func(t *ftt.Test) {
			requestSnapshotSkip := &rpcpb.GetSnapshotRequest{
				Revision: 3,
				SkipBody: true,
			}
			snapshot, err := server.GetSnapshot(ctx, requestSnapshotSkip)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, snapshot, should.Resemble(&rpcpb.Snapshot{
				AuthDbRev:    3,
				AuthDbSha256: testHash,
				CreatedTs:    timestamppb.New(testTS),
			}))

			assert.Loosely(t, datastore.Put(ctx, testAuthDBSnapshotLatest(4)), should.BeNil)
			requestLatestSkip := &rpcpb.GetSnapshotRequest{
				Revision: 0,
				SkipBody: true,
			}
			latest, err := server.GetSnapshot(ctx, requestLatestSkip)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, latest, should.Resemble(&rpcpb.Snapshot{
				AuthDbRev:    4,
				AuthDbSha256: testHash,
				CreatedTs:    timestamppb.New(testTS),
			}))
		})

		t.Run("Testing GetSnapshot with sharded DB in datastore.", func(t *ftt.Test) {
			shardedSnapshot := &model.AuthDBSnapshot{
				Kind: "AuthDBSnapshot",
				ID:   42,
				ShardIDs: []string{
					"42:shard-1",
					"42:shard-2",
				},
				AuthDBSha256: testHash,
				CreatedTS:    testTS,
			}

			shard1 := &model.AuthDBShard{
				Kind: "AuthDBShard",
				ID:   "42:shard-1",
				Blob: []byte("shard-1-groups"),
			}
			shard2 := &model.AuthDBShard{
				Kind: "AuthDBShard",
				ID:   "42:shard-2",
				Blob: []byte("shard-2-groups"),
			}
			assert.Loosely(t, datastore.Put(ctx, shard1, shard2, shardedSnapshot), should.BeNil)
			requestSnapshot := &rpcpb.GetSnapshotRequest{
				Revision: 42,
			}
			snapshot, err := server.GetSnapshot(ctx, requestSnapshot)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, snapshot, should.Resemble(&rpcpb.Snapshot{
				AuthDbRev:      42,
				AuthDbSha256:   testHash,
				AuthDbDeflated: []byte("shard-1-groupsshard-2-groups"),
				CreatedTs:      timestamppb.New(testTS),
			}))
		})
	})

	ftt.Run("Testing legacy API Server with JSON response", t, func(t *ftt.Test) {
		server := Server{}
		type TestSnapshotJSON struct {
			AuthDBRev      int64  `json:"auth_db_rev"`
			AuthDBDeflated []byte `json:"deflated_body,omitempty"`
			AuthDBSha256   string `json:"sha256"`
			CreatedTS      int64  `json:"created_ts"`
		}
		expectedJSON := func(rid int64, sb bool) ([]byte, error) {
			if sb {
				testDeflated = []byte{}
			}
			return json.Marshal(map[string]any{
				"snapshot": TestSnapshotJSON{
					AuthDBRev:      rid,
					AuthDBDeflated: testDeflated,
					AuthDBSha256:   testHash,
					CreatedTS:      testTSMicro,
				},
			})
		}

		assert.Loosely(t, datastore.Put(ctx,
			testAuthDBSnapshot(1),
			testAuthDBSnapshot(2),
			testAuthDBSnapshot(3),
			testAuthDBSnapshot(4)), should.BeNil)

		t.Run("Testing GetSnapshotLegacy skipBody=true", func(t *ftt.Test) {
			rid := int64(3)
			skipBody := true
			actualBlob := legacyCall(server, ctx, rid, skipBody)
			expectedBlob, err := expectedJSON(rid, skipBody)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualBlob, should.Resemble(expectedBlob))
		})

		t.Run("Testing GetSnapshotLegacy skipBody=false", func(t *ftt.Test) {
			rid := int64(3)
			skipBody := false
			actualBlob := legacyCall(server, ctx, rid, skipBody)
			expectedBlob, err := expectedJSON(rid, skipBody)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualBlob, should.Resemble(expectedBlob))
		})

		t.Run("Testing GetSnapshotLatestLegacy skipBody=true", func(t *ftt.Test) {
			rid := int64(4)
			skipBody := true
			actualBlob := legacyCall(server, ctx, rid, skipBody)
			expectedBlob, err := expectedJSON(rid, skipBody)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualBlob, should.Resemble(expectedBlob))

		})

		t.Run("Testing GetSnapshotLatestLegacy skipBody=false", func(t *ftt.Test) {
			rid := int64(4)
			skipBody := false
			actualBlob := legacyCall(server, ctx, rid, skipBody)
			expectedBlob, err := expectedJSON(rid, skipBody)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actualBlob, should.Resemble(expectedBlob))
		})
	})
}
