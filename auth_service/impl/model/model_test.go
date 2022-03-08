// Copyright 2021 The LUCI Authors.
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

package model

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"
	"go.chromium.org/luci/server/auth/service/protocol"

	. "github.com/smartystreets/goconvey/convey"
)

var (
	testCreatedTS  = time.Date(2020, time.August, 16, 15, 20, 0, 0, time.UTC)
	testModifiedTS = time.Date(2021, time.August, 16, 12, 20, 0, 0, time.UTC)
)

func testAuthVersionedEntityMixin() AuthVersionedEntityMixin {
	return AuthVersionedEntityMixin{
		ModifiedTS:    testModifiedTS,
		ModifiedBy:    "user:test-modifier@example.com",
		AuthDBRev:     1337,
		AuthDBPrevRev: 1336,
	}
}

func testSecurityConfig() *protocol.SecurityConfig {
	return &protocol.SecurityConfig{
		InternalServiceRegexp: []string{`.*\.example.com`},
	}
}

func testSecurityConfigBlob() []byte {
	blob, err := proto.Marshal(testSecurityConfig())
	if err != nil {
		panic(err)
	}
	return blob
}

func testAuthGlobalConfig(ctx context.Context) *AuthGlobalConfig {
	return &AuthGlobalConfig{
		Kind:                     "AuthGlobalConfig",
		ID:                       "root",
		AuthVersionedEntityMixin: testAuthVersionedEntityMixin(),
		OAuthClientID:            "test-client-id",
		OAuthAdditionalClientIDs: []string{
			"additional-client-id-0",
			"additional-client-id-1",
		},
		OAuthClientSecret: "test-client-secret",
		TokenServerURL:    "https://token-server.example.com",
		SecurityConfig:    testSecurityConfigBlob(),
	}
}

func testAuthReplicationState(ctx context.Context, rev int64) *AuthReplicationState {
	return &AuthReplicationState{
		Kind:       "AuthReplicationState",
		ID:         "self",
		Parent:     RootKey(ctx),
		AuthDBRev:  rev,
		ModifiedTS: testModifiedTS,
		PrimaryID:  "test-primary-id",
	}
}

func testAuthGroup(ctx context.Context, name string, members []string) *AuthGroup {
	if members == nil {
		members = []string{
			fmt.Sprintf("user:%s-m1@example.com", name),
			fmt.Sprintf("user:%s-m2@example.com", name),
		}
	}
	return &AuthGroup{
		Kind:                     "AuthGroup",
		ID:                       name,
		Parent:                   RootKey(ctx),
		AuthVersionedEntityMixin: testAuthVersionedEntityMixin(),
		Members:                  members,
		Globs:                    []string{"user:*@example.com"},
		Nested:                   []string{"nested-" + name},
		Description:              fmt.Sprintf("This is a test auth group %q.", name),
		Owners:                   "owners-" + name,
		CreatedTS:                testCreatedTS,
		CreatedBy:                "user:test-creator@example.com",
	}
}

func testIPAllowlist(ctx context.Context, name string, subnets []string) *AuthIPAllowlist {
	if subnets == nil {
		subnets = []string{
			"127.0.0.1/10",
			"127.0.0.1/20",
		}
	}
	return &AuthIPAllowlist{
		Kind:                     "AuthIPWhitelist",
		ID:                       name,
		Parent:                   RootKey(ctx),
		AuthVersionedEntityMixin: testAuthVersionedEntityMixin(),
		Subnets:                  subnets,
		Description:              fmt.Sprintf("This is a test AuthIPAllowlist %q.", name),
		CreatedTS:                testCreatedTS,
		CreatedBy:                "user:test-creator@example.com",
	}
}

func testAuthDBSnapshot(ctx context.Context, rev int64) *AuthDBSnapshot {
	return &AuthDBSnapshot{
		Kind:           "AuthDBSnapshot",
		ID:             rev,
		AuthDBDeflated: []byte("test-db-deflated"),
		AuthDBSha256:   "test-sha-256",
		CreatedTS:      testCreatedTS,
	}
}

func testAuthDBSnapshotSharded(ctx context.Context, rev int64, shardCount int) (*AuthDBSnapshot, []byte, error) {
	snapshot := &AuthDBSnapshot{
		Kind:         "AuthDBSnapshot",
		ID:           rev,
		ShardIDs:     make([]string, 0, shardCount),
		AuthDBSha256: "test-sha-256",
		CreatedTS:    testCreatedTS,
	}

	var expectedBlob []byte

	for i := 0; i < shardCount; i++ {
		blobShard := []byte(fmt.Sprintf("test-authdb-shard-%v", i))
		expectedBlob = append(expectedBlob, blobShard...)
		hash := sha256.Sum256(blobShard)
		shardID := fmt.Sprintf("%v:%s", rev, hash)
		shard := &AuthDBShard{
			Kind: "AuthDBShard",
			ID:   shardID,
			Blob: blobShard,
		}
		if err := datastore.Put(ctx, shard); err != nil {
			return nil, nil, err
		}
		snapshot.ShardIDs = append(snapshot.ShardIDs, shardID)
	}
	return snapshot, expectedBlob, nil
}

////////////////////////////////////////////////////////////////////////////////

func TestGetReplicationState(t *testing.T) {
	t.Parallel()

	Convey("Testing GetReplicationState", t, func() {
		ctx := memory.Use(context.Background())

		state := testAuthReplicationState(ctx, 12345)

		_, err := GetReplicationState(ctx)
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		So(datastore.Put(ctx, state), ShouldBeNil)

		actual, err := GetReplicationState(ctx)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, state)
	})
}

func TestGetAuthGroup(t *testing.T) {
	t.Parallel()

	Convey("Testing GetAuthGroup", t, func() {
		ctx := memory.Use(context.Background())

		authGroup := testAuthGroup(ctx, "test-auth-group-1", []string{
			"user:a@example.com",
			"user:b@example.com",
		})

		_, err := GetAuthGroup(ctx, "test-auth-group-1")
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		So(datastore.Put(ctx, authGroup), ShouldBeNil)

		actual, err := GetAuthGroup(ctx, "test-auth-group-1")
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, authGroup)
	})
}

func TestGetAllAuthGroups(t *testing.T) {
	t.Parallel()

	Convey("Testing GetAllAuthGroups", t, func() {
		ctx := memory.Use(context.Background())

		// Out of order alphabetically by ID.
		So(datastore.Put(ctx,
			testAuthGroup(ctx, "test-auth-group-3", nil),
			testAuthGroup(ctx, "test-auth-group-1", nil),
			testAuthGroup(ctx, "test-auth-group-2", nil),
		), ShouldBeNil)

		actualAuthGroups, err := GetAllAuthGroups(ctx)
		So(err, ShouldBeNil)

		// Returned in alphabetical order.
		So(actualAuthGroups, ShouldResemble, []*AuthGroup{
			testAuthGroup(ctx, "test-auth-group-1", nil),
			testAuthGroup(ctx, "test-auth-group-2", nil),
			testAuthGroup(ctx, "test-auth-group-3", nil),
		})
	})
}

func TestGetAuthIPAllowlist(t *testing.T) {
	t.Parallel()

	Convey("Testing GetAuthIPAllowlist", t, func() {
		ctx := memory.Use(context.Background())

		authIPAllowlist := testIPAllowlist(ctx, "test-auth-ip-allowlist-1", []string{
			"123.456.789.101/24",
			"123.456.789.112/24",
		})

		_, err := GetAuthIPAllowlist(ctx, "test-auth-ip-allowlist-1")
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		So(datastore.Put(ctx, authIPAllowlist), ShouldBeNil)

		actual, err := GetAuthIPAllowlist(ctx, "test-auth-ip-allowlist-1")
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, authIPAllowlist)
	})
}

func TestGetAllAuthIPAllowlists(t *testing.T) {
	t.Parallel()

	Convey("Testing GetAllAuthIPAllowlists", t, func() {
		ctx := memory.Use(context.Background())

		// Out of order alphabetically by ID.
		So(datastore.Put(ctx,
			testIPAllowlist(ctx, "test-allowlist-3", nil),
			testIPAllowlist(ctx, "test-allowlist-1", nil),
			testIPAllowlist(ctx, "test-allowlist-2", nil),
		), ShouldBeNil)

		actualAuthIPAllowlists, err := GetAllAuthIPAllowlists(ctx)
		So(err, ShouldBeNil)

		// Returned in alphabetical order.
		So(actualAuthIPAllowlists, ShouldResemble, []*AuthIPAllowlist{
			testIPAllowlist(ctx, "test-allowlist-1", nil),
			testIPAllowlist(ctx, "test-allowlist-2", nil),
			testIPAllowlist(ctx, "test-allowlist-3", nil),
		})
	})
}

func TestGetAuthGlobalConfig(t *testing.T) {
	t.Parallel()

	Convey("Testing GetAuthGlobalConfig", t, func() {
		ctx := memory.Use(context.Background())

		_, err := GetAuthGlobalConfig(ctx)
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		cfg := testAuthGlobalConfig(ctx)
		err = datastore.Put(ctx, cfg)
		So(err, ShouldBeNil)

		actual, err := GetAuthGlobalConfig(ctx)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, cfg)
	})
}

func TestGetAuthDBSnapshot(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())

	Convey("Testing GetAuthDBSnapshot", t, func() {
		_, err := GetAuthDBSnapshot(ctx, 42)
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		snapshot := testAuthDBSnapshot(ctx, 42)

		err = datastore.Put(ctx, snapshot)
		So(err, ShouldBeNil)

		actual, err := GetAuthDBSnapshot(ctx, 42)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, snapshot)

		err = datastore.Put(ctx, snapshot)
		So(err, ShouldBeNil)

		actual, err = GetAuthDBSnapshot(ctx, 42)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, snapshot)
	})

	Convey("Testing GetAuthDBSnapshotLatest", t, func() {
		_, err := GetAuthDBSnapshotLatest(ctx)
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		snapshot := testAuthDBSnapshot(ctx, 42)

		authDBSnapshotLatest := &AuthDBSnapshotLatest{
			Kind:         "AuthDBSnapshotLatest",
			ID:           "latest",
			AuthDBRev:    snapshot.ID,
			AuthDBSha256: snapshot.AuthDBSha256,
			ModifiedTS:   testModifiedTS,
		}
		err = datastore.Put(ctx, authDBSnapshotLatest)

		So(err, ShouldBeNil)

		actual, err := GetAuthDBSnapshotLatest(ctx)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, authDBSnapshotLatest)
	})

	Convey("Testing unshardAuthDB", t, func() {
		authDBShard1 := &AuthDBShard{
			Kind: "AuthDBShard",
			ID:   "42:7F404D83A3F440591C25A09B0A471EC4BB7D4EA3B50C081BCE37AA879E15EB69",
			Blob: []byte("half-1"),
		}
		authDBShard2 := &AuthDBShard{
			Kind: "AuthDBShard",
			ID:   "42:55915DB56DAD50F22BD882DACEE545FEFCA583CB8B3DACC4E5D9CAC9A4A2460C",
			Blob: []byte("half-2"),
		}
		shardIDs := []string{authDBShard1.ID, authDBShard2.ID}

		expectedBlob := []byte("half-1half-2")
		So(datastore.Put(ctx, authDBShard1), ShouldBeNil)
		So(datastore.Put(ctx, authDBShard2), ShouldBeNil)

		actualBlob, err := unshardAuthDB(ctx, shardIDs)
		So(err, ShouldBeNil)
		So(actualBlob, ShouldResemble, expectedBlob)
	})

	Convey("Testing GetAuthDBSnapshot with sharded DB", t, func() {
		snapshot, expectedAuthDB, err := testAuthDBSnapshotSharded(ctx, 42, 3)
		So(err, ShouldBeNil)
		So(datastore.Put(ctx, snapshot), ShouldBeNil)

		actualSnapshot, err := GetAuthDBSnapshot(ctx, 42)
		So(err, ShouldBeNil)
		So(actualSnapshot.AuthDBDeflated, ShouldResemble, expectedAuthDB)
	})
}
