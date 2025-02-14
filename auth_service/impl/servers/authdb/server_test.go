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
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/julienschmidt/httprouter"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/model"
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

	t.Parallel()

	ftt.Run("Testing pRPC API", t, func(t *ftt.Test) {
		server := Server{}
		ctx := memory.Use(context.Background())
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
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))

			requestNotPresent := &rpcpb.GetSnapshotRequest{
				Revision: 42,
			}
			_, err = server.GetSnapshot(ctx, requestNotPresent)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
		})

		t.Run("Testing valid revision ID", func(t *ftt.Test) {
			request := &rpcpb.GetSnapshotRequest{
				Revision: 2,
			}
			snapshot, err := server.GetSnapshot(ctx, request)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, snapshot, should.Match(&rpcpb.Snapshot{
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
			assert.Loosely(t, latestSnapshot, should.Match(&rpcpb.Snapshot{
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
			assert.Loosely(t, snapshot, should.Match(&rpcpb.Snapshot{
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
			assert.Loosely(t, latest, should.Match(&rpcpb.Snapshot{
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
			assert.Loosely(t, snapshot, should.Match(&rpcpb.Snapshot{
				AuthDbRev:      42,
				AuthDbSha256:   testHash,
				AuthDbDeflated: []byte("shard-1-groupsshard-2-groups"),
				CreatedTs:      timestamppb.New(testTS),
			}))
		})
	})

	ftt.Run("Testing legacy API Server with JSON response", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		assert.Loosely(t, datastore.Put(ctx, testAuthDBSnapshot(3)),
			should.BeNil)

		server := Server{}

		t.Run("skip_body=1", func(t *ftt.Test) {
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{
					URL: &url.URL{
						RawQuery: "skip_body=1",
					},
				}).WithContext(ctx),
				Params: []httprouter.Param{
					{Key: "revID", Value: "3"},
				},
				Writer: rw,
			}
			err := server.HandleLegacyAuthDBServing(rctx)
			assert.Loosely(t, err, should.BeNil)

			actual := map[string]SnapshotJSON{}
			assert.Loosely(t, json.NewDecoder(rw.Body).Decode(&actual), should.BeNil)

			assert.Loosely(t, actual, should.Match(
				map[string]SnapshotJSON{
					"snapshot": {
						AuthDBRev:    3,
						AuthDBSha256: testHash,
						CreatedTS:    testTSMicro,
					},
				}))
		})

		t.Run("skip_body not set", func(t *ftt.Test) {
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Params: []httprouter.Param{
					{Key: "revID", Value: "3"},
				},
				Writer: rw,
			}
			err := server.HandleLegacyAuthDBServing(rctx)
			assert.Loosely(t, err, should.BeNil)

			actual := map[string]SnapshotJSON{}
			assert.Loosely(t, json.NewDecoder(rw.Body).Decode(&actual), should.BeNil)

			assert.Loosely(t, actual, should.Match(
				map[string]SnapshotJSON{
					"snapshot": {
						AuthDBRev:      3,
						AuthDBDeflated: testDeflated,
						AuthDBSha256:   testHash,
						CreatedTS:      testTSMicro,
					},
				}))
		})

		t.Run("invalid revision name", func(t *ftt.Test) {
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Params: []httprouter.Param{
					{Key: "revID", Value: "test"},
				},
				Writer: rw,
			}
			err := server.HandleLegacyAuthDBServing(rctx)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("invalid negative revision", func(t *ftt.Test) {
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Params: []httprouter.Param{
					{Key: "revID", Value: "-1"},
				},
				Writer: rw,
			}
			err := server.HandleLegacyAuthDBServing(rctx)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("revision not found", func(t *ftt.Test) {
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{}).WithContext(ctx),
				Params: []httprouter.Param{
					{Key: "revID", Value: "4"},
				},
				Writer: rw,
			}
			err := server.HandleLegacyAuthDBServing(rctx)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
		})
	})
	ftt.Run("GetPrincipalPermissions RPC call", t, func(t *ftt.Test) {
		srv := Server{
			permissionsProvider: &CachingPermissionsProvider{},
		}
		ctx := memory.Use(context.Background())

		// Set up Datastore with realms and groups.
		const authDBRev = 1000
		group1 := &protocol.AuthGroup{
			Name: "gr1",
			Members: []string{
				"user:a@example.com", "user:b@example.com", "user:eve@test.com"},
			Globs: []string{"user:*@example.com", "project:*"},
		}
		group2 := &protocol.AuthGroup{
			Name:   "gr2",
			Nested: []string{"gr2-a"},
		}
		group2a := &protocol.AuthGroup{
			Name:    "gr2-a",
			Members: []string{"user:a@example.com"},
		}
		groups := []*protocol.AuthGroup{group1, group2, group2a}
		realms := &protocol.Realms{
			Permissions: []*protocol.Permission{
				{Name: "luci.dev.p1"},
				{Name: "luci.dev.p2"},
				{Name: "luci.dev.p3"},
			},
			Realms: []*protocol.Realm{
				{
					Name: "p:r",
					Bindings: []*protocol.Binding{
						{
							Permissions: []uint32{0, 1, 2},
							Principals:  []string{"group:gr1"},
						},
						{
							Permissions: []uint32{1, 2},
							Principals:  []string{"group:gr2"},
						},
						{
							Permissions: []uint32{0},
							Principals:  []string{"user:u1"},
						},
						{
							Permissions: []uint32{0},
							Principals:  []string{"glob:g1"},
						},
					},
				},
				{
					Name: "p:r2",
					Bindings: []*protocol.Binding{
						{
							Permissions: []uint32{2},
							Principals:  []string{"group:gr1"},
						},
					},
				},
				{
					Name: "p:r3",
					Bindings: []*protocol.Binding{
						{
							Permissions: []uint32{0},
							Principals:  []string{"group:gr2"},
						},
					},
				},
			},
		}
		storeTestAuthDBSnapshot(ctx, realms, groups, authDBRev, t)
		storeTestAuthDBSnapshotLatest(ctx, authDBRev, t)

		gr1Expected := &rpcpb.PrincipalPermissions{
			Name: "group:gr1",
			RealmPermissions: []*rpcpb.RealmPermissions{
				{
					Name:        "p:r",
					Permissions: []string{"luci.dev.p1", "luci.dev.p2", "luci.dev.p3"},
				},
				{
					Name:        "p:r2",
					Permissions: []string{"luci.dev.p3"},
				},
			},
		}

		t.Run("group lookup works", func(t *ftt.Test) {
			req := &rpcpb.GetPrincipalPermissionsRequest{
				Principal: &rpcpb.Principal{
					Kind: rpcpb.PrincipalKind_GROUP,
					Name: "gr1",
				},
			}
			resp, err := srv.GetPrincipalPermissions(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, resp, should.Match(gr1Expected))
		})

		t.Run("nested group lookup works", func(t *ftt.Test) {
			req := &rpcpb.GetPrincipalPermissionsRequest{
				Principal: &rpcpb.Principal{
					Kind: rpcpb.PrincipalKind_GROUP,
					Name: "gr2-a",
				},
			}
			resp, err := srv.GetPrincipalPermissions(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			expected := &rpcpb.PrincipalPermissions{
				Name: "group:gr2-a",
				RealmPermissions: []*rpcpb.RealmPermissions{
					{
						Name:        "p:r",
						Permissions: []string{"luci.dev.p2", "luci.dev.p3"},
					},
					{
						Name:        "p:r3",
						Permissions: []string{"luci.dev.p1"},
					},
				},
			}
			assert.Loosely(t, resp, should.Match(expected))
		})

		t.Run("user lookup works", func(t *ftt.Test) {
			req := &rpcpb.GetPrincipalPermissionsRequest{
				Principal: &rpcpb.Principal{
					Kind: rpcpb.PrincipalKind_IDENTITY,
					Name: "user:eve@test.com",
				},
			}
			resp, err := srv.GetPrincipalPermissions(ctx, req)
			assert.Loosely(t, err, should.BeNil)
			// The user is only in group gr1. They should have the same permissions,
			// but the root principal is different.
			gr1Expected.Name = "user:eve@test.com"
			assert.Loosely(t, resp, should.Match(gr1Expected))
		})

		t.Run("glob lookup works", func(t *ftt.Test) {
			req := &rpcpb.GetPrincipalPermissionsRequest{
				Principal: &rpcpb.Principal{
					Kind: rpcpb.PrincipalKind_GLOB,
					Name: "user:*@example.com",
				},
			}
			resp, err := srv.GetPrincipalPermissions(ctx, req)
			assert.Loosely(t, err, should.BeNil)

			expected := &rpcpb.PrincipalPermissions{
				Name: "user:*@example.com",
				RealmPermissions: []*rpcpb.RealmPermissions{
					{
						Name:        "p:r",
						Permissions: []string{"luci.dev.p1", "luci.dev.p2", "luci.dev.p3"},
					},
					{
						Name:        "p:r2",
						Permissions: []string{"luci.dev.p3"},
					},
				},
			}
			assert.Loosely(t, resp, should.Match(expected))
		})

		t.Run("returns NotFound when group not found in graph", func(t *ftt.Test) {
			// Principal that does not exist should return empty realms permissions.
			req := &rpcpb.GetPrincipalPermissionsRequest{
				Principal: &rpcpb.Principal{
					Kind: rpcpb.PrincipalKind_GROUP,
					Name: "unknown-group",
				},
			}
			resp, err := srv.GetPrincipalPermissions(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, resp, should.BeNil)
		})

		t.Run("returns invalid argument for empty request", func(t *ftt.Test) {
			resp, err := srv.GetPrincipalPermissions(ctx, &rpcpb.GetPrincipalPermissionsRequest{})
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, resp, should.BeNil)
		})
		t.Run("returns invalid argument for empty principal name", func(t *ftt.Test) {
			req := &rpcpb.GetPrincipalPermissionsRequest{
				Principal: &rpcpb.Principal{
					Kind: rpcpb.PrincipalKind_GROUP,
					Name: "",
				},
			}
			resp, err := srv.GetPrincipalPermissions(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, resp, should.BeNil)
		})

		t.Run("returns invalid argument if principal kind is not valid", func(t *ftt.Test) {
			req := &rpcpb.GetPrincipalPermissionsRequest{
				Principal: &rpcpb.Principal{
					Kind: rpcpb.PrincipalKind_PRINCIPAL_KIND_UNSPECIFIED,
					Name: "gr1",
				},
			}
			resp, err := srv.GetPrincipalPermissions(ctx, req)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
			assert.Loosely(t, resp, should.BeNil)
		})
	})
}

func TestMembershipsServing(t *testing.T) {
	t.Parallel()

	ftt.Run("Legacy membership check call", t, func(t *ftt.Test) {
		srv := Server{}

		testDB, err := authdb.NewSnapshotDB(&protocol.AuthDB{
			Groups: []*protocol.AuthGroup{
				{
					Name:    "group-a",
					Members: []string{"user:somebody@example.com"},
					Globs:   []string{"user:tester-*@test.com"},
				},
				{
					Name:   "group-b",
					Nested: []string{"group-a"},
				},
			},
		}, "", 1, false)
		assert.Loosely(t, err, should.BeNil)

		ctx := auth.WithState(context.Background(), &authtest.FakeState{
			FakeDB: testDB,
		})

		t.Run("validates identity", func(t *ftt.Test) {
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{
					URL: &url.URL{
						RawQuery: "identity=somebody@example.com&groups=group-a",
					},
				}).WithContext(ctx),
				Writer: rw,
			}
			err := srv.CheckLegacyMembership(rctx)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("validates groups", func(t *ftt.Test) {
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{
					URL: &url.URL{
						RawQuery: "identity=user:foo@bar.com&groups=",
					},
				}).WithContext(ctx),
				Writer: rw,
			}
			err := srv.CheckLegacyMembership(rctx)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
		})

		t.Run("not a member for unknown group", func(t *ftt.Test) {
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{
					URL: &url.URL{
						RawQuery: "identity=user:somebody@example.com&groups=c",
					},
				}).WithContext(ctx),
				Writer: rw,
			}
			err := srv.CheckLegacyMembership(rctx)
			assert.Loosely(t, err, should.BeNil)

			actual := map[string]any{}
			assert.Loosely(t, json.NewDecoder(rw.Body).Decode(&actual), should.BeNil)
			assert.Loosely(t, actual, should.Match(map[string]any{
				"is_member": false,
			}))
		})

		t.Run("nested works", func(t *ftt.Test) {
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{
					URL: &url.URL{
						RawQuery: "identity=user:somebody@example.com&groups=group-b",
					},
				}).WithContext(ctx),
				Writer: rw,
			}
			err := srv.CheckLegacyMembership(rctx)
			assert.Loosely(t, err, should.BeNil)

			actual := map[string]any{}
			assert.Loosely(t, json.NewDecoder(rw.Body).Decode(&actual), should.BeNil)
			assert.Loosely(t, actual, should.Match(map[string]any{
				"is_member": true,
			}))
		})

		t.Run("glob works", func(t *ftt.Test) {
			rw := httptest.NewRecorder()
			rctx := &router.Context{
				Request: (&http.Request{
					URL: &url.URL{
						RawQuery: "identity=user:tester-a@test.com&groups=group-a",
					},
				}).WithContext(ctx),
				Writer: rw,
			}
			err := srv.CheckLegacyMembership(rctx)
			assert.Loosely(t, err, should.BeNil)

			actual := map[string]any{}
			assert.Loosely(t, json.NewDecoder(rw.Body).Decode(&actual), should.BeNil)
			assert.Loosely(t, actual, should.Match(map[string]any{
				"is_member": true,
			}))
		})
	})
}
