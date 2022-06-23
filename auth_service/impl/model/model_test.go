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

	"go.chromium.org/luci/auth_service/impl/info"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/service/protocol"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	. "go.chromium.org/luci/common/testing/assertions"
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

func getAllDatastoreEntities(ctx context.Context, entityKind string, parent *datastore.Key) ([]datastore.PropertyMap, error) {
	query := datastore.NewQuery(entityKind).Ancestor(parent)
	var entities []datastore.PropertyMap
	err := datastore.GetAll(ctx, query, &entities)
	return entities, err
}

func getProp(pm datastore.PropertyMap, key string) interface{} {
	ps := pm.Slice(key)
	if ps == nil {
		return nil
	}
	return ps[0].Value()
}

func getDatastoreKey(pm datastore.PropertyMap) *datastore.Key {
	return getProp(pm, "$key").(*datastore.Key)
}

func getStringProp(pm datastore.PropertyMap, key string) string {
	return getProp(pm, key).(string)
}

func getBoolProp(pm datastore.PropertyMap, key string) bool {
	return getProp(pm, key).(bool)
}

func getInt64Prop(pm datastore.PropertyMap, key string) int64 {
	return getProp(pm, key).(int64)
}

func getTimeProp(pm datastore.PropertyMap, key string) time.Time {
	return getProp(pm, key).(time.Time)
}

func getStringSliceProp(pm datastore.PropertyMap, key string) []string {
	vals := []string{}
	ps := pm.Slice(key)
	if ps == nil {
		return nil
	}
	for _, p := range ps {
		vals = append(vals, p.Value().(string))
	}
	return vals
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

func TestCreateAuthGroup(t *testing.T) {
	t.Parallel()

	Convey("CreateAuthGroup", t, func() {
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
			Identity: "user:someone@example.com",
		})
		ctx = clock.Set(ctx, testclock.New(testCreatedTS))
		ctx = info.SetImageVersion(ctx, "test-version")

		Convey("empty group name", func() {
			group := testAuthGroup(ctx, "", nil)

			_, err := CreateAuthGroup(ctx, group)
			So(err, ShouldEqual, ErrInvalidName)
		})

		Convey("invalid group name", func() {
			group := testAuthGroup(ctx, "foo^", nil)

			_, err := CreateAuthGroup(ctx, group)
			So(err, ShouldEqual, ErrInvalidName)
		})

		Convey("external group name", func() {
			group := testAuthGroup(ctx, "mdb/foo", nil)

			_, err := CreateAuthGroup(ctx, group)
			So(err, ShouldEqual, ErrInvalidName)
		})

		Convey("group name that already exists", func() {
			So(datastore.Put(ctx,
				testAuthGroup(ctx, "foo", nil),
			), ShouldBeNil)

			group := testAuthGroup(ctx, "foo", nil)

			_, err := CreateAuthGroup(ctx, group)
			So(err, ShouldEqual, ErrAlreadyExists)
		})

		Convey("all referenced groups must exist", func() {
			group := testAuthGroup(ctx, "foo", nil)
			group.Owners = "bar"
			group.Nested = []string{"baz", "qux"}

			_, err := CreateAuthGroup(ctx, group)
			So(err, ShouldUnwrapTo, ErrInvalidReference)
			So(err, ShouldErrLike, "some referenced groups don't exist: baz, qux, bar")
		})

		Convey("owner must exist", func() {
			group := testAuthGroup(ctx, "foo", nil)
			group.Owners = "bar"
			group.Nested = nil

			_, err := CreateAuthGroup(ctx, group)
			So(err, ShouldErrLike, "bar")
		})

		Convey("with empty owners uses 'administrators' group", func() {
			So(datastore.Put(ctx,
				testAuthGroup(ctx, AdminGroup, nil),
			), ShouldBeNil)

			group := testAuthGroup(ctx, "foo", nil)
			group.Owners = ""
			group.Nested = nil

			createdGroup, err := CreateAuthGroup(ctx, group)
			So(err, ShouldBeNil)
			So(createdGroup.Owners, ShouldEqual, AdminGroup)
		})

		Convey("group can own itself", func() {
			group := testAuthGroup(ctx, "foo", nil)
			group.Owners = "foo"
			group.Nested = nil

			createdGroup, err := CreateAuthGroup(ctx, group)
			So(err, ShouldBeNil)
			So(createdGroup.Owners, ShouldEqual, createdGroup.ID)
		})

		Convey("successfully writes to datastore", func() {
			group := testAuthGroup(ctx, "foo", nil)
			group.Owners = "foo"
			group.Nested = nil

			createdGroup, err := CreateAuthGroup(ctx, group)
			So(err, ShouldBeNil)
			So(createdGroup.ID, ShouldEqual, group.ID)
			So(createdGroup.Description, ShouldEqual, group.Description)
			So(createdGroup.Owners, ShouldEqual, group.Owners)
			So(createdGroup.Members, ShouldResemble, group.Members)
			So(createdGroup.Globs, ShouldResemble, group.Globs)
			So(createdGroup.Nested, ShouldResemble, group.Nested)
			So(createdGroup.CreatedBy, ShouldEqual, "user:someone@example.com")
			So(createdGroup.CreatedTS.Unix(), ShouldEqual, testCreatedTS.Unix())
			So(createdGroup.ModifiedBy, ShouldEqual, "user:someone@example.com")
			So(createdGroup.ModifiedTS.Unix(), ShouldEqual, testCreatedTS.Unix())
			So(createdGroup.AuthDBRev, ShouldEqual, 1)
			So(createdGroup.AuthDBPrevRev, ShouldEqual, 0)

			fetchedGroup, err := GetAuthGroup(ctx, "foo")
			So(err, ShouldBeNil)
			So(fetchedGroup.ID, ShouldEqual, group.ID)
			So(fetchedGroup.Description, ShouldEqual, group.Description)
			So(fetchedGroup.Owners, ShouldEqual, group.Owners)
			So(fetchedGroup.Members, ShouldResemble, group.Members)
			So(fetchedGroup.Globs, ShouldResemble, group.Globs)
			So(fetchedGroup.Nested, ShouldResemble, group.Nested)
			So(fetchedGroup.CreatedBy, ShouldEqual, "user:someone@example.com")
			So(fetchedGroup.CreatedTS.Unix(), ShouldEqual, testCreatedTS.Unix())
			So(fetchedGroup.ModifiedBy, ShouldEqual, "user:someone@example.com")
			So(fetchedGroup.ModifiedTS.Unix(), ShouldEqual, testCreatedTS.Unix())
			So(fetchedGroup.AuthDBRev, ShouldEqual, 1)
			So(fetchedGroup.AuthDBPrevRev, ShouldEqual, 0)
		})

		Convey("updates AuthDB revision only on successful write", func() {
			// Create a group.
			{
				group1 := testAuthGroup(ctx, "foo", nil)
				group1.Owners = "foo"
				group1.Nested = nil

				createdGroup1, err := CreateAuthGroup(ctx, group1)
				So(err, ShouldBeNil)
				So(createdGroup1.AuthDBRev, ShouldEqual, 1)
				So(createdGroup1.AuthDBPrevRev, ShouldEqual, 0)

				state1, err := GetReplicationState(ctx)
				So(err, ShouldBeNil)
				So(state1.AuthDBRev, ShouldEqual, 1)
			}

			// Create a second group.
			{
				group2 := testAuthGroup(ctx, "foo2", nil)
				group2.Owners = "foo2"
				group2.Nested = nil

				createdGroup2, err := CreateAuthGroup(ctx, group2)
				So(err, ShouldBeNil)
				So(createdGroup2.AuthDBRev, ShouldEqual, 2)
				So(createdGroup2.AuthDBPrevRev, ShouldEqual, 0)

				state2, err := GetReplicationState(ctx)
				So(err, ShouldBeNil)
				So(state2.AuthDBRev, ShouldEqual, 2)
			}

			// Try to create another group the same as the second, which should fail.
			{
				_, err := CreateAuthGroup(ctx, testAuthGroup(ctx, "foo2", nil))
				So(err, ShouldBeError)

				state3, err := GetReplicationState(ctx)
				So(err, ShouldBeNil)
				So(state3.AuthDBRev, ShouldEqual, 2)
			}
		})

		Convey("creates historical group entities", func() {
			// Create a group.
			{
				group := testAuthGroup(ctx, "foo", nil)
				group.Owners = "foo"
				group.Nested = nil

				_, err := CreateAuthGroup(ctx, group)
				So(err, ShouldBeNil)

				entities, err := getAllDatastoreEntities(ctx, "AuthGroupHistory", HistoricalRevisionKey(ctx, 1))
				So(err, ShouldBeNil)
				So(entities, ShouldHaveLength, 1)
				historicalEntity := entities[0]
				So(getDatastoreKey(historicalEntity).String(), ShouldEqual, "dev~app::/AuthGlobalConfig,\"root\"/Rev,1/AuthGroupHistory,\"foo\"")
				So(getStringProp(historicalEntity, "description"), ShouldEqual, group.Description)
				So(getStringProp(historicalEntity, "owners"), ShouldEqual, group.Owners)
				So(getStringSliceProp(historicalEntity, "members"), ShouldResemble, group.Members)
				So(getStringSliceProp(historicalEntity, "globs"), ShouldResemble, group.Globs)
				So(getStringSliceProp(historicalEntity, "nested"), ShouldResemble, group.Nested)
				So(getStringProp(historicalEntity, "created_by"), ShouldEqual, "user:someone@example.com")
				So(getTimeProp(historicalEntity, "created_ts").Unix(), ShouldEqual, testCreatedTS.Unix())
				So(getStringProp(historicalEntity, "modified_by"), ShouldEqual, "user:someone@example.com")
				So(getTimeProp(historicalEntity, "modified_ts").Unix(), ShouldEqual, testCreatedTS.Unix())
				So(getInt64Prop(historicalEntity, "auth_db_rev"), ShouldEqual, 1)
				So(getInt64Prop(historicalEntity, "auth_db_prev_rev"), ShouldEqual, 0)
				So(getBoolProp(historicalEntity, "auth_db_deleted"), ShouldBeFalse)
				So(getStringProp(historicalEntity, "auth_db_comment"), ShouldEqual, "Go pRPC API")
				So(getStringProp(historicalEntity, "auth_db_app_version"), ShouldEqual, "test-version")
			}

			// Create a second group.
			{
				group := testAuthGroup(ctx, "foo2", nil)
				group.Owners = "foo2"
				group.Nested = nil

				_, err := CreateAuthGroup(ctx, group)
				So(err, ShouldBeNil)

				entities, err := getAllDatastoreEntities(ctx, "AuthGroupHistory", HistoricalRevisionKey(ctx, 2))
				So(err, ShouldBeNil)
				So(entities, ShouldHaveLength, 1)
				historicalEntity := entities[0]
				So(getDatastoreKey(historicalEntity).String(), ShouldEqual, "dev~app::/AuthGlobalConfig,\"root\"/Rev,2/AuthGroupHistory,\"foo2\"")
				So(getStringProp(historicalEntity, "description"), ShouldEqual, group.Description)
				So(getStringProp(historicalEntity, "owners"), ShouldEqual, group.Owners)
				So(getStringSliceProp(historicalEntity, "members"), ShouldResemble, group.Members)
				So(getStringSliceProp(historicalEntity, "globs"), ShouldResemble, group.Globs)
				So(getStringSliceProp(historicalEntity, "nested"), ShouldResemble, group.Nested)
				So(getStringProp(historicalEntity, "created_by"), ShouldEqual, "user:someone@example.com")
				So(getTimeProp(historicalEntity, "created_ts").Unix(), ShouldEqual, testCreatedTS.Unix())
				So(getStringProp(historicalEntity, "modified_by"), ShouldEqual, "user:someone@example.com")
				So(getTimeProp(historicalEntity, "modified_ts").Unix(), ShouldEqual, testCreatedTS.Unix())
				So(getInt64Prop(historicalEntity, "auth_db_rev"), ShouldEqual, 2)
				So(getInt64Prop(historicalEntity, "auth_db_prev_rev"), ShouldEqual, 0)
				So(getBoolProp(historicalEntity, "auth_db_deleted"), ShouldBeFalse)
				So(getStringProp(historicalEntity, "auth_db_comment"), ShouldEqual, "Go pRPC API")
				So(getStringProp(historicalEntity, "auth_db_app_version"), ShouldEqual, "test-version")
			}
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
		_, err := GetAuthDBSnapshot(ctx, 42, false)
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		snapshot := testAuthDBSnapshot(ctx, 42)

		err = datastore.Put(ctx, snapshot)
		So(err, ShouldBeNil)

		actual, err := GetAuthDBSnapshot(ctx, 42, false)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, snapshot)

		err = datastore.Put(ctx, snapshot)
		So(err, ShouldBeNil)

		actual, err = GetAuthDBSnapshot(ctx, 42, false)
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

		actualSnapshot, err := GetAuthDBSnapshot(ctx, 42, false)
		So(err, ShouldBeNil)
		So(actualSnapshot.AuthDBDeflated, ShouldResemble, expectedAuthDB)

		actualSnapshot, err = GetAuthDBSnapshot(ctx, 42, true)
		So(err, ShouldBeNil)
		So(actualSnapshot.AuthDBDeflated, ShouldBeNil)
	})
}

func TestProtoConversion(t *testing.T) {
	t.Parallel()

	Convey("AuthGroup FromProto and ToProto round trip equivalence", t, func() {
		ctx := memory.Use(context.Background())

		empty := &AuthGroup{
			Kind:   "AuthGroup",
			Parent: RootKey(ctx),
		}

		So(AuthGroupFromProto(ctx, empty.ToProto(true)), ShouldResemble, empty)

		g := testAuthGroup(ctx, "foo-group", nil)
		// Ignore the versioned entity mixin since this doesn't survive the proto conversion round trip.
		g.AuthVersionedEntityMixin = AuthVersionedEntityMixin{}

		So(AuthGroupFromProto(ctx, g.ToProto(true)), ShouldResemble, g)
	})
}
