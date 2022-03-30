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
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestAuthDBServing(t *testing.T) {
	testTS := time.Date(2021, time.August, 16, 15, 20, 0, 0, time.UTC)
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

	t.Parallel()
	ctx := memory.Use(context.Background())

	Convey("Testing pRPC API", t, func() {
		server := Server{}
		So(datastore.Put(ctx,
			testAuthDBSnapshot(1),
			testAuthDBSnapshot(2),
			testAuthDBSnapshot(3),
			testAuthDBSnapshot(4)), ShouldBeNil)

		Convey("Testing Error Codes", func() {
			requestNegative := &rpcpb.GetSnapshotRequest{
				Revision: -1,
			}
			_, err := server.GetSnapshot(ctx, requestNegative)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument)

			requestNotPresent := &rpcpb.GetSnapshotRequest{
				Revision: 42,
			}
			_, err = server.GetSnapshot(ctx, requestNotPresent)
			So(err, ShouldHaveGRPCStatus, codes.NotFound)

			requestNotPresent.Revision, err = getLatestRevision(ctx, requestNotPresent)
			So(err, ShouldHaveGRPCStatus, codes.NotFound)
		})

		Convey("Testing valid revision ID", func() {
			request := &rpcpb.GetSnapshotRequest{
				Revision: 2,
			}
			snapshot, err := server.GetSnapshot(ctx, request)
			So(err, ShouldBeNil)
			So(snapshot, ShouldResembleProto, &rpcpb.Snapshot{
				AuthDbRev:      2,
				AuthDbSha256:   testHash,
				AuthDbDeflated: testDeflated,
				CreatedTs:      timestamppb.New(testTS),
			})
		})

		Convey("Testing GetSnapshotLatest", func() {
			So(datastore.Put(ctx, testAuthDBSnapshotLatest(4)), ShouldBeNil)
			request := &rpcpb.GetSnapshotRequest{
				Revision: 0,
			}
			latestSnapshot, err := server.GetSnapshot(ctx, request)
			So(err, ShouldBeNil)
			So(latestSnapshot, ShouldResembleProto, &rpcpb.Snapshot{
				AuthDbRev:      4,
				AuthDbSha256:   testHash,
				AuthDbDeflated: testDeflated,
				CreatedTs:      timestamppb.New(testTS),
			})
		})

		Convey("Testing skipbody", func() {
			requestSnapshotSkip := &rpcpb.GetSnapshotRequest{
				Revision: 3,
				SkipBody: true,
			}
			snapshot, err := server.GetSnapshot(ctx, requestSnapshotSkip)
			So(err, ShouldBeNil)
			So(snapshot, ShouldResembleProto, &rpcpb.Snapshot{
				AuthDbRev:    3,
				AuthDbSha256: testHash,
				CreatedTs:    timestamppb.New(testTS),
			})

			So(datastore.Put(ctx, testAuthDBSnapshotLatest(4)), ShouldBeNil)
			requestLatestSkip := &rpcpb.GetSnapshotRequest{
				Revision: 0,
				SkipBody: true,
			}
			latest, err := server.GetSnapshot(ctx, requestLatestSkip)
			So(err, ShouldBeNil)
			So(latest, ShouldResembleProto, &rpcpb.Snapshot{
				AuthDbRev:    4,
				AuthDbSha256: testHash,
				CreatedTs:    timestamppb.New(testTS),
			})
		})

		Convey("Testing GetSnapshot with sharded DB in datastore.", func() {
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
			So(datastore.Put(ctx, shard1, shard2, shardedSnapshot), ShouldBeNil)
			requestSnapshot := &rpcpb.GetSnapshotRequest{
				Revision: 42,
			}
			snapshot, err := server.GetSnapshot(ctx, requestSnapshot)
			So(err, ShouldBeNil)
			So(snapshot, ShouldResembleProto, &rpcpb.Snapshot{
				AuthDbRev:      42,
				AuthDbSha256:   testHash,
				AuthDbDeflated: []byte("shard-1-groupsshard-2-groups"),
				CreatedTs:      timestamppb.New(testTS),
			})
		})
	})
}
