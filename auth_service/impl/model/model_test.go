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
	"encoding/hex"
	"fmt"
	"sort"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/service/protocol"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/auth_service/api/configspb"
	"go.chromium.org/luci/auth_service/api/taskspb"
	"go.chromium.org/luci/auth_service/impl/info"
	"go.chromium.org/luci/auth_service/impl/util/zlib"
	"go.chromium.org/luci/auth_service/internal/permissions"
	"go.chromium.org/luci/auth_service/testsupport"
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

func testAuthGroup(ctx context.Context, name string) *AuthGroup {
	members := []string{
		fmt.Sprintf("user:%s-m1@example.com", name),
		fmt.Sprintf("user:%s-m2@example.com", name),
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

// emptyAuthGroup creates a new AuthGroup, that owns itself, with no members.
func emptyAuthGroup(ctx context.Context, name string) *AuthGroup {
	return &AuthGroup{
		Kind:   "AuthGroup",
		ID:     name,
		Parent: RootKey(ctx),
		AuthVersionedEntityMixin: AuthVersionedEntityMixin{
			ModifiedTS:    testModifiedTS,
			ModifiedBy:    "user:test-modifier@example.com",
			AuthDBRev:     1,
			AuthDBPrevRev: 0,
		},
		Description: fmt.Sprintf("This is a test auth group %q.", name),
		Owners:      name,
		CreatedTS:   testCreatedTS,
		CreatedBy:   "user:test-creator@example.com",
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

func testAuthRealmsGlobals(ctx context.Context) *AuthRealmsGlobals {
	return &AuthRealmsGlobals{
		AuthVersionedEntityMixin: testAuthVersionedEntityMixin(),
		Kind:                     "AuthRealmsGlobals",
		ID:                       "globals",
		Parent:                   RootKey(ctx),
	}
}

func testAuthProjectRealms(ctx context.Context, projectName string) *AuthProjectRealms {
	return &AuthProjectRealms{
		AuthVersionedEntityMixin: testAuthVersionedEntityMixin(),
		Kind:                     "AuthProjectRealms",
		ID:                       projectName,
		Parent:                   RootKey(ctx),
	}
}

func testAuthProjectRealmsMeta(ctx context.Context, projectName string, cfgRev string) *AuthProjectRealmsMeta {
	return &AuthProjectRealmsMeta{
		Kind:         "AuthProjectRealmsMeta",
		ID:           "meta",
		Parent:       projectRealmsKey(ctx, projectName),
		ConfigRev:    cfgRev,
		PermsRev:     "permissions.cfg:123",
		ConfigDigest: "test config digest",
		ModifiedTS:   testCreatedTS,
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

// isPropIndexed returns true if any property with the given key is indexed.
func isPropIndexed(pm datastore.PropertyMap, key string) bool {
	ps := pm.Slice(key)
	for _, p := range ps {
		if p.IndexSetting() != datastore.NoIndex {
			return true
		}
	}
	return false
}

////////////////////////////////////////////////////////////////////////////////

func TestGetReplicationState(t *testing.T) {
	t.Parallel()

	ftt.Run("Testing GetReplicationState", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		state := testAuthReplicationState(ctx, 12345)

		_, err := GetReplicationState(ctx)
		assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))

		assert.Loosely(t, datastore.Put(ctx, state), should.BeNil)

		actual, err := GetReplicationState(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Match(state))
	})
}

func TestGetAuthGroup(t *testing.T) {
	t.Parallel()

	ftt.Run("Testing GetAuthGroup", t, func(t *ftt.Test) {
		t.Run("returns group if it exists", func(t *ftt.Test) {
			ctx := memory.Use(context.Background())

			// Group doesn't exist yet.
			_, err := GetAuthGroup(ctx, "test-auth-group-1")
			assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))

			// Put the group in datastore.
			authGroup := testAuthGroup(ctx, "test-auth-group-1")
			assert.Loosely(t, datastore.Put(ctx, authGroup), should.BeNil)

			// Group should be returned now that it exists.
			actual, err := GetAuthGroup(ctx, "test-auth-group-1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, actual, should.Match(authGroup))
		})

		t.Run("returned group always has Owners", func(t *ftt.Test) {
			ctx := memory.Use(context.Background())

			// Put a group without Owners in datastore.
			authGroup := testAuthGroup(ctx, "test-auth-group-1")
			authGroup.Owners = ""
			assert.Loosely(t, datastore.Put(ctx, authGroup), should.BeNil)

			// Returned group should have Owners set to AdminGroup.
			actual, err := GetAuthGroup(ctx, "test-auth-group-1")
			assert.Loosely(t, err, should.BeNil)
			expected := testAuthGroup(ctx, "test-auth-group-1")
			expected.Owners = AdminGroup
			assert.Loosely(t, actual, should.Match(expected))
		})

		t.Run("handles empty name", func(t *ftt.Test) {
			actual, err := GetAuthGroup(context.Background(), "")
			assert.Loosely(t, err, should.ErrLike(ErrInvalidName))
			assert.Loosely(t, actual, should.BeNil)
		})
	})
}

func TestGetAllAuthGroups(t *testing.T) {
	t.Parallel()

	ftt.Run("Testing GetAllAuthGroups", t, func(t *ftt.Test) {
		t.Run("returns groups alphabetically", func(t *ftt.Test) {
			ctx := memory.Use(context.Background())

			// Out of order alphabetically by ID.
			assert.Loosely(t, datastore.Put(ctx,
				testAuthGroup(ctx, "test-auth-group-3"),
				testAuthGroup(ctx, "test-auth-group-1"),
				testAuthGroup(ctx, "test-auth-group-2"),
			), should.BeNil)

			actualAuthGroups, err := GetAllAuthGroups(ctx)
			assert.Loosely(t, err, should.BeNil)

			// Returned in alphabetical order.
			assert.Loosely(t, actualAuthGroups, should.Match([]*AuthGroup{
				testAuthGroup(ctx, "test-auth-group-1"),
				testAuthGroup(ctx, "test-auth-group-2"),
				testAuthGroup(ctx, "test-auth-group-3"),
			}))
		})

		t.Run("returned groups always has Owners", func(t *ftt.Test) {
			ctx := memory.Use(context.Background())

			authGroup1 := testAuthGroup(ctx, "test-auth-group-1")
			authGroup1.Owners = ""
			assert.Loosely(t, datastore.Put(ctx,
				authGroup1,
				testAuthGroup(ctx, "test-auth-group-2"),
			), should.BeNil)

			actualAuthGroups, err := GetAllAuthGroups(ctx)
			assert.Loosely(t, err, should.BeNil)

			expectedAuthGroup1 := testAuthGroup(ctx, "test-auth-group-1")
			expectedAuthGroup1.Owners = AdminGroup
			assert.Loosely(t, actualAuthGroups, should.Match([]*AuthGroup{
				expectedAuthGroup1,
				testAuthGroup(ctx, "test-auth-group-2"),
			}))
		})
	})
}

func TestCreateAuthGroup(t *testing.T) {
	t.Parallel()

	ftt.Run("CreateAuthGroup", t, func(t *ftt.Test) {
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
			Identity: "user:someone@example.com",
		})
		ctx = clock.Set(ctx, testclock.New(testCreatedTS))
		ctx = info.SetImageVersion(ctx, "test-version")
		ctx, taskScheduler := tq.TestingContext(txndefer.FilterRDS(ctx), nil)

		t.Run("empty group name", func(t *ftt.Test) {
			group := testAuthGroup(ctx, "")

			_, err := CreateAuthGroup(ctx, group, "Go pRPC API", false)
			assert.Loosely(t, err, should.Equal(ErrInvalidName))
		})

		t.Run("invalid group name", func(t *ftt.Test) {
			group := testAuthGroup(ctx, "foo^")

			_, err := CreateAuthGroup(ctx, group, "Go pRPC API", false)
			assert.Loosely(t, err, should.Equal(ErrInvalidName))
		})

		t.Run("external group name", func(t *ftt.Test) {
			group := testAuthGroup(ctx, "mdb/foo")

			_, err := CreateAuthGroup(ctx, group, "Go pRPC API", false)
			assert.Loosely(t, err, should.Equal(ErrInvalidName))
		})

		t.Run("group name that already exists", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx,
				testAuthGroup(ctx, "foo"),
			), should.BeNil)

			group := testAuthGroup(ctx, "foo")

			_, err := CreateAuthGroup(ctx, group, "Go pRPC API", false)
			assert.Loosely(t, err, should.Equal(ErrAlreadyExists))
		})

		t.Run("invalid member identities", func(t *ftt.Test) {
			group := testAuthGroup(ctx, "foo")
			group.Members = []string{"no-prefix@google.com"}

			_, err := CreateAuthGroup(ctx, group, "Go pRPC API", false)
			assert.Loosely(t, err, should.ErrLike(ErrInvalidIdentity))
			assert.Loosely(t, err, should.ErrLike("bad identity string \"no-prefix@google.com\""))
		})

		t.Run("invalid identity globs", func(t *ftt.Test) {
			group := testAuthGroup(ctx, "foo")
			group.Globs = []string{"*@no-prefix.com"}

			_, err := CreateAuthGroup(ctx, group, "Go pRPC API", false)
			assert.Loosely(t, err, should.ErrLike(ErrInvalidIdentity))
			assert.Loosely(t, err, should.ErrLike("bad identity glob string \"*@no-prefix.com\""))
		})

		t.Run("all referenced groups must exist", func(t *ftt.Test) {
			group := testAuthGroup(ctx, "foo")
			group.Owners = "bar"
			group.Nested = []string{"baz", "qux"}

			_, err := CreateAuthGroup(ctx, group, "Go pRPC API", false)
			assert.Loosely(t, err, should.ErrLike(ErrInvalidReference))
			assert.Loosely(t, err, should.ErrLike("some referenced groups don't exist: baz, qux, bar"))
		})

		t.Run("owner must exist", func(t *ftt.Test) {
			group := testAuthGroup(ctx, "foo")
			group.Owners = "bar"

			_, err := CreateAuthGroup(ctx, group, "Go pRPC API", false)
			assert.Loosely(t, err, should.ErrLike("bar"))
		})

		t.Run("with empty owners uses 'administrators' group", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx,
				testAuthGroup(ctx, AdminGroup),
			), should.BeNil)

			group := emptyAuthGroup(ctx, "foo")
			group.Owners = ""

			createdGroup, err := CreateAuthGroup(ctx, group, "Go pRPC API", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, createdGroup.Owners, should.Equal(AdminGroup))
		})

		t.Run("group can own itself", func(t *ftt.Test) {
			group := emptyAuthGroup(ctx, "foo")
			group.Owners = "foo"

			createdGroup, err := CreateAuthGroup(ctx, group, "Go pRPC API", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, createdGroup.Owners, should.Equal(createdGroup.ID))
		})

		t.Run("group can have project members and globs", func(t *ftt.Test) {
			group := emptyAuthGroup(ctx, "foo")
			group.Members = []string{"project:abc"}
			group.Globs = []string{"project:proj-*-test"}

			createdGroup, err := CreateAuthGroup(ctx, group, "Go pRPC API", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, createdGroup.ID, should.Equal(group.ID))
			assert.Loosely(t, createdGroup.Members, should.Match(createdGroup.Members))
			assert.Loosely(t, createdGroup.Globs, should.Match(createdGroup.Globs))
		})

		t.Run("successfully writes to datastore", func(t *ftt.Test) {
			group := emptyAuthGroup(ctx, "foo")

			createdGroup, err := CreateAuthGroup(ctx, group, "Go pRPC API", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, createdGroup.ID, should.Equal(group.ID))
			assert.Loosely(t, createdGroup.Description, should.Equal(group.Description))
			assert.Loosely(t, createdGroup.Owners, should.Equal(group.Owners))
			assert.Loosely(t, createdGroup.Members, should.Match(group.Members))
			assert.Loosely(t, createdGroup.Globs, should.Match(group.Globs))
			assert.Loosely(t, createdGroup.Nested, should.Match(group.Nested))
			assert.Loosely(t, createdGroup.CreatedBy, should.Equal("user:someone@example.com"))
			assert.Loosely(t, createdGroup.CreatedTS.Unix(), should.Equal(testCreatedTS.Unix()))
			assert.Loosely(t, createdGroup.ModifiedBy, should.Equal("user:someone@example.com"))
			assert.Loosely(t, createdGroup.ModifiedTS.Unix(), should.Equal(testCreatedTS.Unix()))
			assert.Loosely(t, createdGroup.AuthDBRev, should.Equal(1))
			assert.Loosely(t, createdGroup.AuthDBPrevRev, should.BeZero)

			fetchedGroup, err := GetAuthGroup(ctx, "foo")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fetchedGroup.ID, should.Equal(group.ID))
			assert.Loosely(t, fetchedGroup.Description, should.Equal(group.Description))
			assert.Loosely(t, fetchedGroup.Owners, should.Equal(group.Owners))
			assert.Loosely(t, fetchedGroup.Members, should.Match(group.Members))
			assert.Loosely(t, fetchedGroup.Globs, should.Match(group.Globs))
			assert.Loosely(t, fetchedGroup.Nested, should.Match(group.Nested))
			assert.Loosely(t, fetchedGroup.CreatedBy, should.Equal("user:someone@example.com"))
			assert.Loosely(t, fetchedGroup.CreatedTS.Unix(), should.Equal(testCreatedTS.Unix()))
			assert.Loosely(t, fetchedGroup.ModifiedBy, should.Equal("user:someone@example.com"))
			assert.Loosely(t, fetchedGroup.ModifiedTS.Unix(), should.Equal(testCreatedTS.Unix()))
			assert.Loosely(t, fetchedGroup.AuthDBRev, should.Equal(1))
			assert.Loosely(t, fetchedGroup.AuthDBPrevRev, should.BeZero)
		})

		t.Run("updates AuthDB revision only on successful write", func(t *ftt.Test) {
			// Create a group.
			{
				group1 := emptyAuthGroup(ctx, "foo")

				createdGroup1, err := CreateAuthGroup(ctx, group1, "Go pRPC API", false)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, createdGroup1.AuthDBRev, should.Equal(1))
				assert.Loosely(t, createdGroup1.AuthDBPrevRev, should.BeZero)

				state1, err := GetReplicationState(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, state1.AuthDBRev, should.Equal(1))
				tasks := taskScheduler.Tasks()
				assert.Loosely(t, tasks, should.HaveLength(2))
				processChangeTask := tasks[0]
				assert.Loosely(t, processChangeTask.Class, should.Equal("process-change-task"))
				assert.Loosely(t, processChangeTask.Payload, should.Match(&taskspb.ProcessChangeTask{AuthDbRev: 1}))
				replicationTask := tasks[1]
				assert.Loosely(t, replicationTask.Class, should.Equal("replication-task"))
				assert.Loosely(t, replicationTask.Payload, should.Match(&taskspb.ReplicationTask{AuthDbRev: 1}))
			}

			// Create a second group.
			{
				group2 := emptyAuthGroup(ctx, "foo2")

				createdGroup2, err := CreateAuthGroup(ctx, group2, "Go pRPC API", false)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, createdGroup2.AuthDBRev, should.Equal(2))
				assert.Loosely(t, createdGroup2.AuthDBPrevRev, should.BeZero)

				state2, err := GetReplicationState(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, state2.AuthDBRev, should.Equal(2))
				tasks := taskScheduler.Tasks()
				assert.Loosely(t, tasks, should.HaveLength(4))
			}

			// Try to create another group the same as the second, which should fail.
			{
				_, err := CreateAuthGroup(ctx, emptyAuthGroup(ctx, "foo2"), "Go pRPC API", false)
				assert.Loosely(t, err, should.ErrLike("already exists"))

				state3, err := GetReplicationState(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, state3.AuthDBRev, should.Equal(2))
				tasks := taskScheduler.Tasks()
				assert.Loosely(t, tasks, should.HaveLength(4))
			}
		})

		t.Run("creates historical group entities", func(t *ftt.Test) {
			// Create a group.
			{
				group := emptyAuthGroup(ctx, "foo")

				_, err := CreateAuthGroup(ctx, group, "test historical comment", false)
				assert.Loosely(t, err, should.BeNil)

				entities, err := getAllDatastoreEntities(ctx, "AuthGroupHistory", HistoricalRevisionKey(ctx, 1))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, entities, should.HaveLength(1))
				historicalEntity := entities[0]
				assert.Loosely(t, getDatastoreKey(historicalEntity).String(), should.Equal("dev~app::/AuthGlobalConfig,\"root\"/Rev,1/AuthGroupHistory,\"foo\""))
				assert.Loosely(t, getStringProp(historicalEntity, "description"), should.Equal(group.Description))
				assert.Loosely(t, getStringProp(historicalEntity, "owners"), should.Equal(group.Owners))
				assert.Loosely(t, getStringSliceProp(historicalEntity, "members"), should.Match(group.Members))
				assert.Loosely(t, getStringSliceProp(historicalEntity, "globs"), should.Match(group.Globs))
				assert.Loosely(t, getStringSliceProp(historicalEntity, "nested"), should.Match(group.Nested))
				assert.Loosely(t, getStringProp(historicalEntity, "created_by"), should.Equal("user:someone@example.com"))
				assert.Loosely(t, getTimeProp(historicalEntity, "created_ts").Unix(), should.Equal(testCreatedTS.Unix()))
				assert.Loosely(t, getStringProp(historicalEntity, "modified_by"), should.Equal("user:someone@example.com"))
				assert.Loosely(t, getTimeProp(historicalEntity, "modified_ts").Unix(), should.Equal(testCreatedTS.Unix()))
				assert.Loosely(t, getInt64Prop(historicalEntity, "auth_db_rev"), should.Equal(1))
				assert.Loosely(t, getProp(historicalEntity, "auth_db_prev_rev"), should.BeNil)
				assert.Loosely(t, getBoolProp(historicalEntity, "auth_db_deleted"), should.BeFalse)
				assert.Loosely(t, getStringProp(historicalEntity, "auth_db_change_comment"), should.Equal("test historical comment"))
				assert.Loosely(t, getStringProp(historicalEntity, "auth_db_app_version"), should.Equal("test-version"))

				// Check no properties are indexed.
				for k := range historicalEntity {
					assert.Loosely(t, isPropIndexed(historicalEntity, k), should.BeFalse)
				}
			}

			// Create a second group.
			{
				group := emptyAuthGroup(ctx, "foo2")

				_, err := CreateAuthGroup(ctx, group, "Go pRPC API", false)
				assert.Loosely(t, err, should.BeNil)

				entities, err := getAllDatastoreEntities(ctx, "AuthGroupHistory", HistoricalRevisionKey(ctx, 2))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, entities, should.HaveLength(1))
				historicalEntity := entities[0]
				assert.Loosely(t, getDatastoreKey(historicalEntity).String(), should.Equal("dev~app::/AuthGlobalConfig,\"root\"/Rev,2/AuthGroupHistory,\"foo2\""))
				assert.Loosely(t, getStringProp(historicalEntity, "description"), should.Equal(group.Description))
				assert.Loosely(t, getStringProp(historicalEntity, "owners"), should.Equal(group.Owners))
				assert.Loosely(t, getStringSliceProp(historicalEntity, "members"), should.Match(group.Members))
				assert.Loosely(t, getStringSliceProp(historicalEntity, "globs"), should.Match(group.Globs))
				assert.Loosely(t, getStringSliceProp(historicalEntity, "nested"), should.Match(group.Nested))
				assert.Loosely(t, getStringProp(historicalEntity, "created_by"), should.Equal("user:someone@example.com"))
				assert.Loosely(t, getTimeProp(historicalEntity, "created_ts").Unix(), should.Equal(testCreatedTS.Unix()))
				assert.Loosely(t, getStringProp(historicalEntity, "modified_by"), should.Equal("user:someone@example.com"))
				assert.Loosely(t, getTimeProp(historicalEntity, "modified_ts").Unix(), should.Equal(testCreatedTS.Unix()))
				assert.Loosely(t, getInt64Prop(historicalEntity, "auth_db_rev"), should.Equal(2))
				assert.Loosely(t, getProp(historicalEntity, "auth_db_prev_rev"), should.BeNil)
				assert.Loosely(t, getBoolProp(historicalEntity, "auth_db_deleted"), should.BeFalse)
				assert.Loosely(t, getStringProp(historicalEntity, "auth_db_change_comment"), should.Equal("Go pRPC API"))
				assert.Loosely(t, getStringProp(historicalEntity, "auth_db_app_version"), should.Equal("test-version"))

				// Check no properties are indexed.
				for k := range historicalEntity {
					assert.Loosely(t, isPropIndexed(historicalEntity, k), should.BeFalse)
				}
			}
		})
	})
}

func TestUpdateAuthGroup(t *testing.T) {
	t.Parallel()

	ftt.Run("UpdateAuthGroup", t, func(t *ftt.Test) {
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"owners-foo"},
		})
		ctx = clock.Set(ctx, testclock.New(testCreatedTS))
		ctx = info.SetImageVersion(ctx, "test-version")
		ctx, taskScheduler := tq.TestingContext(txndefer.FilterRDS(ctx), nil)

		// A test group to be put in Datastore for updating.
		group := emptyAuthGroup(ctx, "foo")
		group.Owners = "owners-foo"

		// Etag to use for the group, derived from the last-modified time.
		etag := `W/"MjAyMS0wOC0xNlQxMjoyMDowMFo="`

		// Set current auth DB revision to 10.
		assert.Loosely(t, datastore.Put(ctx, testAuthReplicationState(ctx, 10)), should.BeNil)

		t.Run("can't update external group", func(t *ftt.Test) {
			group.ID = "mdb/foo"
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)
			_, err := UpdateAuthGroup(ctx, group, nil, etag, "Go pRPC API", false)
			assert.Loosely(t, err, should.ErrLike("cannot update external group"))
			assert.Loosely(t, taskScheduler.Tasks(), should.BeEmpty)
		})

		t.Run("can't update if not an owner", func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
			})
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)
			_, err := UpdateAuthGroup(ctx, group, nil, etag, "Go pRPC API", false)
			assert.Loosely(t, err, should.Equal(ErrPermissionDenied))
			assert.Loosely(t, taskScheduler.Tasks(), should.BeEmpty)
		})

		t.Run("can update if admin", func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:someone@example.com",
				IdentityGroups: []string{AdminGroup},
			})
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)

			group.Description = "new description"
			_, err := UpdateAuthGroup(ctx, group, nil, etag, "Go pRPC API", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
		})

		t.Run("extra validation is completed for admin group", func(t *ftt.Test) {
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:someone@example.com",
				IdentityGroups: []string{AdminGroup},
			})

			// Set up the admin group, a self-owned internal group and an
			// external group.
			adminAuthGroup := emptyAuthGroup(ctx, AdminGroup)
			adminAuthGroup.Members = []string{"user:someone@example.com"}
			assert.Loosely(t, datastore.Put(ctx,
				adminAuthGroup,
				emptyAuthGroup(ctx, "foo"),
				emptyAuthGroup(ctx, "sys/admins@example.com")), should.BeNil)

			t.Run("can't change admin owners", func(t *ftt.Test) {
				// Attempt to change the owners of the admin group.
				adminAuthGroup.Owners = "foo"
				_, err := UpdateAuthGroup(ctx, adminAuthGroup, nil, etag, "Go pRPC API", false)
				assert.Loosely(t, err, should.ErrLike(fmt.Sprintf("changing %q group owners is forbidden", AdminGroup)))
				assert.Loosely(t, taskScheduler.Tasks(), should.BeEmpty)
			})

			t.Run("invalid admin group change is rejected", func(t *ftt.Test) {
				// Attempt to add a glob to the admin group.
				adminAuthGroup.Globs = []string{"user:*@example.com"}
				_, err := UpdateAuthGroup(ctx, adminAuthGroup, nil, etag, "Go pRPC API", false)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.BeEmpty)
				adminAuthGroup.Globs = []string{}

				// Attempt to nest a nonexistent external group.
				adminAuthGroup.Nested = []string{"unknown/group"}
				_, err = UpdateAuthGroup(ctx, adminAuthGroup, nil, etag, "Go pRPC API", false)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.BeEmpty)
				adminAuthGroup.Nested = []string{}

				// Attempt to empty the admin group.
				adminAuthGroup.Members = []string{}
				_, err = UpdateAuthGroup(ctx, adminAuthGroup, nil, etag, "Go pRPC API", false)
				assert.Loosely(t, err, should.NotBeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.BeEmpty)
			})

			t.Run("valid admin group change succeeds", func(t *ftt.Test) {
				adminAuthGroup.Nested = []string{"sys/admins@example.com"}
				_, err := UpdateAuthGroup(ctx, adminAuthGroup, nil, etag, "Go pRPC API", false)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
			})
		})

		t.Run("can't delete if etag doesn't match", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)
			_, err := UpdateAuthGroup(ctx, group, nil, "bad-etag", "Go pRPC API", false)
			assert.Loosely(t, err, should.ErrLike(ErrConcurrentModification))
			assert.Loosely(t, taskScheduler.Tasks(), should.BeEmpty)
		})

		t.Run("group name that doesn't exist", func(t *ftt.Test) {
			group.ID = "non-existent-group"
			_, err := UpdateAuthGroup(ctx, group, nil, etag, "Go pRPC API", false)
			assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))
			assert.Loosely(t, taskScheduler.Tasks(), should.BeEmpty)
		})

		t.Run("invalid member identities", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)

			group.Members = []string{"no-prefix@google.com"}

			_, err := UpdateAuthGroup(ctx, group, &fieldmaskpb.FieldMask{Paths: []string{"members"}}, etag, "Go pRPC API", false)
			assert.Loosely(t, err, should.ErrLike(ErrInvalidIdentity))
			assert.Loosely(t, err, should.ErrLike("bad identity string \"no-prefix@google.com\""))
			assert.Loosely(t, taskScheduler.Tasks(), should.BeEmpty)
		})

		t.Run("project member identities", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)

			group.Members = []string{"project:abc"}

			updatedGroup, err := UpdateAuthGroup(ctx, group, &fieldmaskpb.FieldMask{Paths: []string{"members"}}, etag, "Go pRPC API", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, updatedGroup.Members, should.Match(group.Members))
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
		})

		t.Run("invalid identity globs", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)

			group.Globs = []string{"*@no-prefix.com"}

			_, err := UpdateAuthGroup(ctx, group, &fieldmaskpb.FieldMask{Paths: []string{"globs"}}, etag, "Go pRPC API", false)
			assert.Loosely(t, err, should.ErrLike(ErrInvalidIdentity))
			assert.Loosely(t, err, should.ErrLike("bad identity glob string \"*@no-prefix.com\""))
			assert.Loosely(t, taskScheduler.Tasks(), should.BeEmpty)
		})

		t.Run("project identity globs", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)

			group.Globs = []string{"project:*"}

			updatedGroup, err := UpdateAuthGroup(ctx, group, &fieldmaskpb.FieldMask{Paths: []string{"globs"}}, etag, "Go pRPC API", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, updatedGroup.Globs, should.Match(group.Globs))
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
		})

		t.Run("all nested groups must exist", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)

			group.Nested = []string{"baz", "qux"}

			_, err := UpdateAuthGroup(ctx, group, &fieldmaskpb.FieldMask{Paths: []string{"nested"}}, etag, "Go pRPC API", false)
			assert.Loosely(t, err, should.ErrLike(ErrInvalidReference))
			assert.Loosely(t, err, should.ErrLike("some referenced groups don't exist"))
			assert.Loosely(t, taskScheduler.Tasks(), should.BeEmpty)
		})

		t.Run("owner must exist", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)

			group.Owners = "bar"

			_, err := UpdateAuthGroup(ctx, group, &fieldmaskpb.FieldMask{Paths: []string{"owners"}}, etag, "Go pRPC API", false)
			assert.Loosely(t, err, should.ErrLike("bar"))
			assert.Loosely(t, taskScheduler.Tasks(), should.BeEmpty)
		})

		t.Run("with empty owners uses 'administrators' group", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, testAuthGroup(ctx, AdminGroup)), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)

			group.Owners = ""

			updatedGroup, err := UpdateAuthGroup(ctx, group, &fieldmaskpb.FieldMask{Paths: []string{"owners"}}, etag, "Go pRPC API", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, updatedGroup.Owners, should.Equal(AdminGroup))
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
		})

		t.Run("skips commit if no changes", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)

			updatedGroup, err := UpdateAuthGroup(ctx, group, nil, etag, "Go pRPC API", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, updatedGroup, should.Match(group))
			assert.Loosely(t, taskScheduler.Tasks(), should.BeEmpty)
		})

		t.Run("successfully writes to datastore", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, emptyAuthGroup(ctx, "new-owner-group")), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, emptyAuthGroup(ctx, "new-nested-group")), should.BeNil)

			group.Description = "updated description"
			group.Owners = "new-owner-group"
			group.Members = []string{"user:updated@example.com"}
			group.Globs = []string{"user:*@updated.com"}
			group.Nested = []string{"new-nested-group"}

			updatedGroup, err := UpdateAuthGroup(ctx, group, nil, etag, "Go pRPC API", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
			assert.Loosely(t, updatedGroup.ID, should.Equal(group.ID))
			assert.Loosely(t, updatedGroup.Description, should.Equal(group.Description))
			assert.Loosely(t, updatedGroup.Owners, should.Equal(group.Owners))
			assert.Loosely(t, updatedGroup.Members, should.Match(group.Members))
			assert.Loosely(t, updatedGroup.Globs, should.Match(group.Globs))
			assert.Loosely(t, updatedGroup.Nested, should.Match(group.Nested))
			assert.Loosely(t, updatedGroup.CreatedBy, should.Equal("user:test-creator@example.com"))
			assert.Loosely(t, updatedGroup.CreatedTS.Unix(), should.Equal(testCreatedTS.Unix()))
			assert.Loosely(t, updatedGroup.ModifiedBy, should.Equal("user:someone@example.com"))
			assert.Loosely(t, updatedGroup.ModifiedTS.Unix(), should.Equal(testCreatedTS.Unix()))
			assert.Loosely(t, updatedGroup.AuthDBRev, should.Equal(11))
			assert.Loosely(t, updatedGroup.AuthDBPrevRev, should.Equal(1))

			fetchedGroup, err := GetAuthGroup(ctx, "foo")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fetchedGroup.ID, should.Equal(group.ID))
			assert.Loosely(t, fetchedGroup.Description, should.Equal(group.Description))
			assert.Loosely(t, fetchedGroup.Owners, should.Equal(group.Owners))
			assert.Loosely(t, fetchedGroup.Members, should.Match(group.Members))
			assert.Loosely(t, fetchedGroup.Globs, should.Match(group.Globs))
			assert.Loosely(t, fetchedGroup.Nested, should.Match(group.Nested))
			assert.Loosely(t, fetchedGroup.CreatedBy, should.Equal("user:test-creator@example.com"))
			assert.Loosely(t, fetchedGroup.CreatedTS.Unix(), should.Equal(testCreatedTS.Unix()))
			assert.Loosely(t, fetchedGroup.ModifiedBy, should.Equal("user:someone@example.com"))
			assert.Loosely(t, fetchedGroup.ModifiedTS.Unix(), should.Equal(testCreatedTS.Unix()))
			assert.Loosely(t, fetchedGroup.AuthDBRev, should.Equal(11))
			assert.Loosely(t, fetchedGroup.AuthDBPrevRev, should.Equal(1))
		})

		t.Run("updates AuthDB revision only on successful write", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, emptyAuthGroup(ctx, "new-owner-group")), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, emptyAuthGroup(ctx, "new-nested-group")), should.BeNil)

			// Update a group, should succeed and bump AuthDB revision.
			group.Description = "updated description"
			group.Owners = "new-owner-group"
			group.Members = []string{"user:updated@example.com"}
			group.Globs = []string{"user:*@updated.com"}
			group.Nested = []string{"new-nested-group"}

			updatedGroup, err := UpdateAuthGroup(ctx, group, nil, etag, "Go pRPC API", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, updatedGroup.AuthDBRev, should.Equal(11))
			assert.Loosely(t, updatedGroup.AuthDBPrevRev, should.Equal(1))

			state, err := GetReplicationState(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, state.AuthDBRev, should.Equal(11))
			tasks := taskScheduler.Tasks()
			assert.Loosely(t, tasks, should.HaveLength(2))
			processChangeTask := tasks[0]
			assert.Loosely(t, processChangeTask.Class, should.Equal("process-change-task"))
			assert.Loosely(t, processChangeTask.Payload, should.Match(&taskspb.ProcessChangeTask{AuthDbRev: 11}))
			replicationTask := tasks[1]
			assert.Loosely(t, replicationTask.Class, should.Equal("replication-task"))
			assert.Loosely(t, replicationTask.Payload, should.Match(&taskspb.ReplicationTask{AuthDbRev: 11}))

			// Update a group, should fail (due to bad etag) and *not* bump AuthDB revision.
			_, err = UpdateAuthGroup(ctx, group, nil, "bad-etag", "Go pRPC API", false)
			assert.Loosely(t, err, should.ErrLike("permission denied"))

			state, err = GetReplicationState(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, state.AuthDBRev, should.Equal(11))
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
		})

		t.Run("creates historical group entities", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, emptyAuthGroup(ctx, "new-owner-group")), should.BeNil)
			assert.Loosely(t, datastore.Put(ctx, emptyAuthGroup(ctx, "new-nested-group")), should.BeNil)

			// Update a group, should succeed and bump AuthDB revision.
			group.Description = "updated description"
			group.Owners = "new-owner-group"
			group.Members = []string{"user:updated@example.com"}
			group.Globs = []string{"user:*@updated.com"}
			group.Nested = []string{"new-nested-group"}

			_, err := UpdateAuthGroup(ctx, group, nil, etag, "Go pRPC API", false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))

			entities, err := getAllDatastoreEntities(ctx, "AuthGroupHistory", HistoricalRevisionKey(ctx, 11))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, entities, should.HaveLength(1))
			historicalEntity := entities[0]
			assert.Loosely(t, getDatastoreKey(historicalEntity).String(), should.Equal("dev~app::/AuthGlobalConfig,\"root\"/Rev,11/AuthGroupHistory,\"foo\""))
			assert.Loosely(t, getStringProp(historicalEntity, "description"), should.Equal(group.Description))
			assert.Loosely(t, getStringProp(historicalEntity, "owners"), should.Equal(group.Owners))
			assert.Loosely(t, getStringSliceProp(historicalEntity, "members"), should.Match(group.Members))
			assert.Loosely(t, getStringSliceProp(historicalEntity, "globs"), should.Match(group.Globs))
			assert.Loosely(t, getStringSliceProp(historicalEntity, "nested"), should.Match(group.Nested))
			assert.Loosely(t, getStringProp(historicalEntity, "created_by"), should.Equal("user:test-creator@example.com"))
			assert.Loosely(t, getTimeProp(historicalEntity, "created_ts").Unix(), should.Equal(testCreatedTS.Unix()))
			assert.Loosely(t, getStringProp(historicalEntity, "modified_by"), should.Equal("user:someone@example.com"))
			assert.Loosely(t, getTimeProp(historicalEntity, "modified_ts").Unix(), should.Equal(testCreatedTS.Unix()))
			assert.Loosely(t, getInt64Prop(historicalEntity, "auth_db_rev"), should.Equal(11))
			assert.Loosely(t, getProp(historicalEntity, "auth_db_prev_rev"), should.Equal(1))
			assert.Loosely(t, getBoolProp(historicalEntity, "auth_db_deleted"), should.BeFalse)
			assert.Loosely(t, getStringProp(historicalEntity, "auth_db_change_comment"), should.Equal("Go pRPC API"))
			assert.Loosely(t, getStringProp(historicalEntity, "auth_db_app_version"), should.Equal("test-version"))

			// Check no properties are indexed.
			for k := range historicalEntity {
				assert.Loosely(t, isPropIndexed(historicalEntity, k), should.BeFalse)
			}
		})

		t.Run("cyclic dependencies", func(t *ftt.Test) {
			// Use admin creds for simplicity.
			ctx := auth.WithState(ctx, &authtest.FakeState{
				Identity:       "user:someone@example.com",
				IdentityGroups: []string{AdminGroup},
			})

			// Initial state is a tree with 4 groups like this:
			//
			//      A
			//     / \
			//    B1 B2
			//   /
			//  C
			//
			a := emptyAuthGroup(ctx, "A")
			a.Nested = []string{"B1", "B2"}
			b1 := emptyAuthGroup(ctx, "B1")
			b1.Nested = []string{"C"}
			b2 := emptyAuthGroup(ctx, "B2")
			c := emptyAuthGroup(ctx, "C")
			assert.Loosely(t, datastore.Put(ctx, []*AuthGroup{a, b1, b2, c}), should.BeNil)

			t.Run("self-reference", func(t *ftt.Test) {
				//   A
				//  /
				// A
				a.Nested = []string{"A"}

				_, err := UpdateAuthGroup(ctx, a, &fieldmaskpb.FieldMask{Paths: []string{"nested"}}, "", "Go pRPC API", false)
				assert.Loosely(t, err, should.ErrLike("groups can't have cyclic dependencies: A -> A"))
				assert.Loosely(t, taskScheduler.Tasks(), should.BeEmpty)
			})

			t.Run("cycle of length 2", func(t *ftt.Test) {
				//   A
				//  /
				// B2
				//  \
				//   A
				b2.Nested = []string{"A"}

				_, err := UpdateAuthGroup(ctx, b2, &fieldmaskpb.FieldMask{Paths: []string{"nested"}}, "", "Go pRPC API", false)
				assert.Loosely(t, err, should.ErrLike("groups can't have cyclic dependencies: B2 -> A -> B2"))
				assert.Loosely(t, taskScheduler.Tasks(), should.BeEmpty)
			})

			t.Run("cycle of length 3", func(t *ftt.Test) {
				//   A
				//  /
				// B1
				//  \
				//   C
				//  /
				// A
				c.Nested = []string{"A"}

				_, err := UpdateAuthGroup(ctx, c, &fieldmaskpb.FieldMask{Paths: []string{"nested"}}, "", "Go pRPC API", false)
				assert.Loosely(t, err, should.ErrLike("groups can't have cyclic dependencies: C -> A -> B1 -> C"))
				assert.Loosely(t, taskScheduler.Tasks(), should.BeEmpty)
			})

			t.Run("cycle not at root", func(t *ftt.Test) {
				//   B1
				//  /
				// C
				//  \
				//   B1
				c.Nested = []string{"B1"}

				_, err := UpdateAuthGroup(ctx, c, &fieldmaskpb.FieldMask{Paths: []string{"nested"}}, "", "Go pRPC API", false)
				assert.Loosely(t, err, should.ErrLike("groups can't have cyclic dependencies: C -> B1 -> C"))
				assert.Loosely(t, taskScheduler.Tasks(), should.BeEmpty)
			})

			t.Run("diamond shape", func(t *ftt.Test) {
				//      A
				//     / \
				//    B1 B2
				//     \ /
				//      C
				b2.Nested = []string{"C"}

				_, err := UpdateAuthGroup(ctx, b2, &fieldmaskpb.FieldMask{Paths: []string{"nested"}}, "", "Go pRPC API", false)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(2))
			})
		})
	})
}

func TestDeleteAuthGroup(t *testing.T) {
	t.Parallel()

	ftt.Run("DeleteAuthGroup", t, func(t *ftt.Test) {
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{AdminGroup},
		})
		ctx = clock.Set(ctx, testclock.New(testCreatedTS))
		ctx = info.SetImageVersion(ctx, "test-version")
		ctx, taskScheduler := tq.TestingContext(txndefer.FilterRDS(ctx), nil)

		// A test group to be put in Datastore for deletion.
		group := testAuthGroup(ctx, "foo")
		group.Owners = "foo"
		group.AuthDBRev = 0
		group.AuthDBPrevRev = 0

		// Etag to use for the group, derived from the last-modified time.
		etag := `W/"MjAyMS0wOC0xNlQxMjoyMDowMFo="`

		t.Run("can't delete the admin group", func(t *ftt.Test) {
			err := DeleteAuthGroup(ctx, AdminGroup, "", "Go pRPC API", false)
			assert.Loosely(t, err, should.Equal(ErrPermissionDenied))
		})

		t.Run("can't delete external group", func(t *ftt.Test) {
			group.ID = "mdb/foo"
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)
			err := DeleteAuthGroup(ctx, group.ID, "", "Go pRPC API", false)
			assert.Loosely(t, err, should.ErrLike("cannot delete external group"))
		})

		t.Run("can't delete if not an owner or admin", func(t *ftt.Test) {
			ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
				Identity: "user:someone@example.com",
			})
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)
			err := DeleteAuthGroup(ctx, group.ID, "", "Go pRPC API", false)
			assert.Loosely(t, err, should.Equal(ErrPermissionDenied))
		})

		t.Run("can't delete if etag doesn't match", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)
			err := DeleteAuthGroup(ctx, group.ID, "bad-etag", "Go pRPC API", false)
			assert.Loosely(t, err, should.ErrLike(ErrConcurrentModification))
		})

		t.Run("group name that doesn't exist", func(t *ftt.Test) {
			err := DeleteAuthGroup(ctx, "non-existent-group", "", "Go pRPC API", false)
			assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))
		})

		t.Run("can't delete if group owns another group", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)

			ownedGroup := testAuthGroup(ctx, "owned")
			ownedGroup.Owners = group.ID
			assert.Loosely(t, datastore.Put(ctx, ownedGroup), should.BeNil)

			err := DeleteAuthGroup(ctx, group.ID, "", "Go pRPC API", false)
			assert.Loosely(t, err, should.ErrLike(ErrReferencedEntity))
			assert.Loosely(t, err, should.ErrLike("this group is referenced by other groups: [owned]"))
		})

		t.Run("can't delete if group is nested by group", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)

			nestingGroup := testAuthGroup(ctx, "nester")
			nestingGroup.Nested = []string{group.ID}
			assert.Loosely(t, datastore.Put(ctx, nestingGroup), should.BeNil)

			err := DeleteAuthGroup(ctx, group.ID, "", "Go pRPC API", false)
			assert.Loosely(t, err, should.ErrLike(ErrReferencedEntity))
			assert.Loosely(t, err, should.ErrLike("this group is referenced by other groups: [nester]"))
		})

		t.Run("successfully deletes from datastore and updates AuthDB", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)
			err := DeleteAuthGroup(ctx, group.ID, etag, "Go pRPC API", false)
			assert.Loosely(t, err, should.BeNil)

			state1, err := GetReplicationState(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, state1.AuthDBRev, should.Equal(1))
			tasks := taskScheduler.Tasks()
			assert.Loosely(t, tasks, should.HaveLength(2))
			processChangeTask := tasks[0]
			assert.Loosely(t, processChangeTask.Class, should.Equal("process-change-task"))
			assert.Loosely(t, processChangeTask.Payload, should.Match(&taskspb.ProcessChangeTask{AuthDbRev: 1}))
			replicationTask := tasks[1]
			assert.Loosely(t, replicationTask.Class, should.Equal("replication-task"))
			assert.Loosely(t, replicationTask.Payload, should.Match(&taskspb.ReplicationTask{AuthDbRev: 1}))
		})

		t.Run("creates historical group entities", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, group), should.BeNil)
			err := DeleteAuthGroup(ctx, group.ID, "", "Go pRPC API", false)
			assert.Loosely(t, err, should.BeNil)

			entities, err := getAllDatastoreEntities(ctx, "AuthGroupHistory", HistoricalRevisionKey(ctx, 1))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, entities, should.HaveLength(1))
			historicalEntity := entities[0]
			assert.Loosely(t, getDatastoreKey(historicalEntity).String(), should.Equal("dev~app::/AuthGlobalConfig,\"root\"/Rev,1/AuthGroupHistory,\"foo\""))
			assert.Loosely(t, getStringProp(historicalEntity, "description"), should.Equal(group.Description))
			assert.Loosely(t, getStringProp(historicalEntity, "owners"), should.Equal(group.Owners))
			assert.Loosely(t, getStringSliceProp(historicalEntity, "members"), should.Match(group.Members))
			assert.Loosely(t, getStringSliceProp(historicalEntity, "globs"), should.Match(group.Globs))
			assert.Loosely(t, getStringSliceProp(historicalEntity, "nested"), should.Match(group.Nested))
			assert.Loosely(t, getStringProp(historicalEntity, "modified_by"), should.Equal("user:someone@example.com"))
			assert.Loosely(t, getTimeProp(historicalEntity, "modified_ts").Unix(), should.Equal(testCreatedTS.Unix()))
			assert.Loosely(t, getInt64Prop(historicalEntity, "auth_db_rev"), should.Equal(1))
			assert.Loosely(t, getProp(historicalEntity, "auth_db_prev_rev"), should.BeNil)
			assert.Loosely(t, getBoolProp(historicalEntity, "auth_db_deleted"), should.BeTrue)
			assert.Loosely(t, getStringProp(historicalEntity, "auth_db_change_comment"), should.Equal("Go pRPC API"))
			assert.Loosely(t, getStringProp(historicalEntity, "auth_db_app_version"), should.Equal("test-version"))

			// Check no properties are indexed.
			for k := range historicalEntity {
				assert.Loosely(t, isPropIndexed(historicalEntity, k), should.BeFalse)
			}
		})
	})
}

func TestGetAuthIPAllowlist(t *testing.T) {
	t.Parallel()

	ftt.Run("Testing GetAuthIPAllowlist", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		authIPAllowlist := testIPAllowlist(ctx, "test-auth-ip-allowlist-1", []string{
			"123.456.789.101/24",
			"123.456.789.112/24",
		})

		_, err := GetAuthIPAllowlist(ctx, "test-auth-ip-allowlist-1")
		assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))

		assert.Loosely(t, datastore.Put(ctx, authIPAllowlist), should.BeNil)

		actual, err := GetAuthIPAllowlist(ctx, "test-auth-ip-allowlist-1")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Match(authIPAllowlist))
	})
}

func TestGetAllAuthIPAllowlists(t *testing.T) {
	t.Parallel()

	ftt.Run("Testing GetAllAuthIPAllowlists", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		// Out of order alphabetically by ID.
		assert.Loosely(t, datastore.Put(ctx,
			testIPAllowlist(ctx, "test-allowlist-3", nil),
			testIPAllowlist(ctx, "test-allowlist-1", nil),
			testIPAllowlist(ctx, "test-allowlist-2", nil),
		), should.BeNil)

		actualAuthIPAllowlists, err := GetAllAuthIPAllowlists(ctx)
		assert.Loosely(t, err, should.BeNil)

		// Returned in alphabetical order.
		assert.Loosely(t, actualAuthIPAllowlists, should.Match([]*AuthIPAllowlist{
			testIPAllowlist(ctx, "test-allowlist-1", nil),
			testIPAllowlist(ctx, "test-allowlist-2", nil),
			testIPAllowlist(ctx, "test-allowlist-3", nil),
		}))
	})
}

func TestUpdateAllAuthIPAllowlists(t *testing.T) {
	t.Parallel()

	ftt.Run("Testing updateAllAuthIPAllowlists", t, func(t *ftt.Test) {
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{AdminGroup},
		})
		ctx = testsupport.SetTestContextSigner(ctx, "test-app-id", "test-app-id@example.com")
		ctx = clock.Set(ctx, testclock.New(testCreatedTS))
		ctx = info.SetImageVersion(ctx, "test-version")
		ctx, taskScheduler := tq.TestingContext(txndefer.FilterRDS(ctx), nil)

		assert.Loosely(t, datastore.Put(ctx,
			testIPAllowlist(ctx, "test-allowlist-1", nil),
			testIPAllowlist(ctx, "test-allowlist-2", []string{"0.0.0.0/24", "127.0.0.1/20"}),
		), should.BeNil)

		baseSubnetMap := map[string][]string{
			"test-allowlist-1": {"127.0.0.1/10", "127.0.0.1/20"},
			"test-allowlist-2": {"0.0.0.0/24", "127.0.0.1/20"},
		}
		baseAllowlistSlice := []*AuthIPAllowlist{
			testIPAllowlist(ctx, "test-allowlist-1", nil),
			testIPAllowlist(ctx, "test-allowlist-2", []string{"0.0.0.0/24", "127.0.0.1/20"}),
		}

		allowlistToCreate := &AuthIPAllowlist{
			AuthVersionedEntityMixin: AuthVersionedEntityMixin{
				ModifiedTS:    testCreatedTS,
				ModifiedBy:    "user:test-app-id@example.com",
				AuthDBRev:     1,
				AuthDBPrevRev: 0,
			},
			Kind:        "AuthIPWhitelist",
			ID:          "test-allowlist-3",
			Parent:      RootKey(ctx),
			Subnets:     []string{"123.4.5.6"},
			Description: "Imported from ip_allowlist.cfg",
			CreatedTS:   testCreatedTS,
			CreatedBy:   "user:test-app-id@example.com",
		}

		t.Run("no-op for identical subnet map", func(t *ftt.Test) {
			assert.Loosely(t, updateAllAuthIPAllowlists(ctx, baseSubnetMap, "Go pRPC API"), should.BeNil)
			allowlists, err := GetAllAuthIPAllowlists(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, allowlists, should.Match(baseAllowlistSlice))
			assert.Loosely(t, taskScheduler.Tasks(), should.BeEmpty)

		})

		t.Run("Create allowlist entity", func(t *ftt.Test) {
			baseSubnetMap["test-allowlist-3"] = []string{"123.4.5.6"}
			assert.Loosely(t, updateAllAuthIPAllowlists(ctx, baseSubnetMap, "Go pRPC API"), should.BeNil)
			allowlists, err := GetAllAuthIPAllowlists(ctx)
			assert.Loosely(t, err, should.BeNil)
			expectedSlice := append(baseAllowlistSlice, allowlistToCreate)
			assert.Loosely(t, allowlists, should.Match(expectedSlice))
		})

		t.Run("Update allowlist entity", func(t *ftt.Test) {
			baseSubnetMap["test-allowlist-1"] = []string{"122.22.44.66"}
			assert.Loosely(t, updateAllAuthIPAllowlists(ctx, baseSubnetMap, "Go pRPC API"), should.BeNil)
			allowlists, err := GetAllAuthIPAllowlists(ctx)
			baseAllowlistSlice[0].AuthVersionedEntityMixin = AuthVersionedEntityMixin{
				ModifiedTS:    testCreatedTS,
				ModifiedBy:    "user:test-app-id@example.com",
				AuthDBRev:     1,
				AuthDBPrevRev: 1337,
			}
			baseAllowlistSlice[0].Subnets = []string{"122.22.44.66"}
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, allowlists, should.Match(baseAllowlistSlice))
		})

		t.Run("Delete allowlist entity", func(t *ftt.Test) {
			delete(baseSubnetMap, "test-allowlist-1")
			assert.Loosely(t, updateAllAuthIPAllowlists(ctx, baseSubnetMap, "Go pRPC API"), should.BeNil)
			allowlists, err := GetAllAuthIPAllowlists(ctx)
			expectedSlice := baseAllowlistSlice[1:]
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, allowlists, should.Match(expectedSlice))
		})

		t.Run("Multiple allowlist entity changes", func(t *ftt.Test) {
			baseSubnetMap["test-allowlist-3"] = []string{"123.4.5.6"}
			baseSubnetMap["test-allowlist-1"] = []string{"122.22.44.66"}
			delete(baseSubnetMap, "test-allowlist-2")
			assert.Loosely(t, updateAllAuthIPAllowlists(ctx, baseSubnetMap, "Go pRPC API"), should.BeNil)
			allowlists, err := GetAllAuthIPAllowlists(ctx)
			allowlist0Copy := *baseAllowlistSlice[0]
			allowlist0Copy.AuthVersionedEntityMixin = AuthVersionedEntityMixin{
				ModifiedTS:    testCreatedTS,
				ModifiedBy:    "user:test-app-id@example.com",
				AuthDBRev:     1,
				AuthDBPrevRev: 1337,
			}
			allowlist0Copy.Subnets = baseSubnetMap["test-allowlist-1"]
			expectedAllowlists := []*AuthIPAllowlist{&allowlist0Copy, allowlistToCreate}
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, allowlists, should.Match(expectedAllowlists))

			state1, err := GetReplicationState(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, state1.AuthDBRev, should.Equal(1))
			tasks := taskScheduler.Tasks()
			assert.Loosely(t, tasks, should.HaveLength(2))
			processChangeTask := tasks[0]
			assert.Loosely(t, processChangeTask.Class, should.Equal("process-change-task"))
			assert.Loosely(t, processChangeTask.Payload, should.Match(&taskspb.ProcessChangeTask{AuthDbRev: 1}))
			replicationTask := tasks[1]
			assert.Loosely(t, replicationTask.Class, should.Equal("replication-task"))
			assert.Loosely(t, replicationTask.Payload, should.Match(&taskspb.ReplicationTask{AuthDbRev: 1}))

			entities, err := getAllDatastoreEntities(ctx, "AuthIPWhitelistHistory", HistoricalRevisionKey(ctx, 1))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, entities, should.HaveLength(3))
			historicalEntity := entities[0]
			assert.Loosely(t, getDatastoreKey(historicalEntity).String(), should.Equal("dev~app::/AuthGlobalConfig,\"root\"/Rev,1/AuthIPWhitelistHistory,\"test-allowlist-1\""))
			assert.Loosely(t, getStringProp(historicalEntity, "description"), should.Equal(baseAllowlistSlice[0].Description))
			assert.Loosely(t, getStringSliceProp(historicalEntity, "subnets"), should.Match(allowlist0Copy.Subnets))
			assert.Loosely(t, getStringProp(historicalEntity, "modified_by"), should.Equal("user:test-app-id@example.com"))
			assert.Loosely(t, getTimeProp(historicalEntity, "modified_ts").Unix(), should.Equal(testCreatedTS.Unix()))
			assert.Loosely(t, getInt64Prop(historicalEntity, "auth_db_rev"), should.Equal(1))
			assert.Loosely(t, getInt64Prop(historicalEntity, "auth_db_prev_rev"), should.Equal(1337))
			assert.Loosely(t, getStringProp(historicalEntity, "auth_db_change_comment"), should.Equal("Go pRPC API"))
			assert.Loosely(t, getStringProp(historicalEntity, "auth_db_app_version"), should.Equal("test-version"))

			historicalEntity = entities[1]
			assert.Loosely(t, getDatastoreKey(historicalEntity).String(), should.Equal("dev~app::/AuthGlobalConfig,\"root\"/Rev,1/AuthIPWhitelistHistory,\"test-allowlist-2\""))
			assert.Loosely(t, getStringProp(historicalEntity, "description"), should.Equal(baseAllowlistSlice[1].Description))
			assert.Loosely(t, getStringSliceProp(historicalEntity, "subnets"), should.Match(baseAllowlistSlice[1].Subnets))
			assert.Loosely(t, getStringProp(historicalEntity, "modified_by"), should.Equal("user:test-app-id@example.com"))
			assert.Loosely(t, getTimeProp(historicalEntity, "modified_ts").Unix(), should.Equal(testCreatedTS.Unix()))
			assert.Loosely(t, getInt64Prop(historicalEntity, "auth_db_rev"), should.Equal(1))
			assert.Loosely(t, getInt64Prop(historicalEntity, "auth_db_prev_rev"), should.Equal(1337))
			assert.Loosely(t, getStringProp(historicalEntity, "auth_db_change_comment"), should.Equal("Go pRPC API"))
			assert.Loosely(t, getStringProp(historicalEntity, "auth_db_app_version"), should.Equal("test-version"))
			assert.Loosely(t, getBoolProp(historicalEntity, "auth_db_deleted"), should.BeTrue)

			historicalEntity = entities[2]
			assert.Loosely(t, getDatastoreKey(historicalEntity).String(), should.Equal("dev~app::/AuthGlobalConfig,\"root\"/Rev,1/AuthIPWhitelistHistory,\"test-allowlist-3\""))
			assert.Loosely(t, getStringProp(historicalEntity, "description"), should.Equal(allowlistToCreate.Description))
			assert.Loosely(t, getStringSliceProp(historicalEntity, "subnets"), should.Match(allowlistToCreate.Subnets))
			assert.Loosely(t, getStringProp(historicalEntity, "modified_by"), should.Equal("user:test-app-id@example.com"))
			assert.Loosely(t, getTimeProp(historicalEntity, "modified_ts").Unix(), should.Equal(testCreatedTS.Unix()))
			assert.Loosely(t, getInt64Prop(historicalEntity, "auth_db_rev"), should.Equal(1))
			assert.Loosely(t, getProp(historicalEntity, "auth_db_prev_rev"), should.BeNil)
			assert.Loosely(t, getStringProp(historicalEntity, "auth_db_change_comment"), should.Equal("Go pRPC API"))
			assert.Loosely(t, getStringProp(historicalEntity, "auth_db_app_version"), should.Equal("test-version"))

			// Check no properties are indexed.
			for k := range historicalEntity {
				assert.Loosely(t, isPropIndexed(historicalEntity, k), should.BeFalse)
			}
		})
	})
}

func TestAuthGlobalConfig(t *testing.T) {
	t.Parallel()

	ftt.Run("Testing GetAuthGlobalConfig", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		_, err := GetAuthGlobalConfig(ctx)
		assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))

		cfg := testAuthGlobalConfig(ctx)
		err = datastore.Put(ctx, cfg)
		assert.Loosely(t, err, should.BeNil)

		actual, err := GetAuthGlobalConfig(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Match(cfg))
	})

	ftt.Run("Testing updateAuthGlobalConfig", t, func(t *ftt.Test) {
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{AdminGroup},
		})
		ctx = testsupport.SetTestContextSigner(ctx, "test-app-id", "test-app-id@example.com")
		ctx = clock.Set(ctx, testclock.New(testCreatedTS))
		ctx = info.SetImageVersion(ctx, "test-version")
		ctx, taskScheduler := tq.TestingContext(txndefer.FilterRDS(ctx), nil)
		oauthcfgpb := &configspb.OAuthConfig{
			PrimaryClientId:     "new-test-client-id",
			PrimaryClientSecret: "new-test-client-secret",
			ClientIds: []string{
				"new-test-client-id-1",
				"new-test-client-id-2",
			},
			TokenServerUrl: "https://new-token-server-url.example.com",
		}
		seccfgpb := testSecurityConfig()

		t.Run("Creating new AuthGlobalConfig", func(t *ftt.Test) {
			assert.Loosely(t, updateAuthGlobalConfig(ctx, oauthcfgpb, seccfgpb, "Go pRPC API"), should.BeNil)
			updatedCfg, err := GetAuthGlobalConfig(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, updatedCfg, should.Match(&AuthGlobalConfig{
				AuthVersionedEntityMixin: AuthVersionedEntityMixin{
					ModifiedTS:    testCreatedTS,
					ModifiedBy:    "user:test-app-id@example.com",
					AuthDBRev:     1,
					AuthDBPrevRev: 0,
				},
				Kind:                     "AuthGlobalConfig",
				ID:                       "root",
				OAuthClientID:            "new-test-client-id",
				OAuthAdditionalClientIDs: []string{"new-test-client-id-1", "new-test-client-id-2"},
				OAuthClientSecret:        "new-test-client-secret",
				TokenServerURL:           "https://new-token-server-url.example.com",
				SecurityConfig:           testSecurityConfigBlob(),
			}))
			state1, err := GetReplicationState(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, state1.AuthDBRev, should.Equal(1))
			tasks := taskScheduler.Tasks()
			assert.Loosely(t, tasks, should.HaveLength(2))
			processChangeTask := tasks[0]
			assert.Loosely(t, processChangeTask.Class, should.Equal("process-change-task"))
			assert.Loosely(t, processChangeTask.Payload, should.Match(&taskspb.ProcessChangeTask{AuthDbRev: 1}))
			replicationTask := tasks[1]
			assert.Loosely(t, replicationTask.Class, should.Equal("replication-task"))
			assert.Loosely(t, replicationTask.Payload, should.Match(&taskspb.ReplicationTask{AuthDbRev: 1}))

			entities, err := getAllDatastoreEntities(ctx, "AuthGlobalConfigHistory", HistoricalRevisionKey(ctx, 1))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, entities, should.HaveLength(1))
			historicalEntity := entities[0]
			assert.Loosely(t, getDatastoreKey(historicalEntity).String(), should.Equal("dev~app::/AuthGlobalConfig,\"root\"/Rev,1/AuthGlobalConfigHistory,\"root\""))
			assert.Loosely(t, getStringProp(historicalEntity, "oauth_client_id"), should.Equal("new-test-client-id"))
			assert.Loosely(t, getStringProp(historicalEntity, "oauth_client_secret"), should.Equal("new-test-client-secret"))
			assert.Loosely(t, getStringProp(historicalEntity, "token_server_url"), should.Equal("https://new-token-server-url.example.com"))
			assert.Loosely(t, getStringSliceProp(historicalEntity, "oauth_additional_client_ids"), should.Match([]string{"new-test-client-id-1", "new-test-client-id-2"}))
			assert.Loosely(t, getStringProp(historicalEntity, "modified_by"), should.Equal("user:test-app-id@example.com"))
			assert.Loosely(t, getTimeProp(historicalEntity, "modified_ts").Unix(), should.Equal(testCreatedTS.Unix()))
			assert.Loosely(t, getInt64Prop(historicalEntity, "auth_db_rev"), should.Equal(1))
			assert.Loosely(t, getProp(historicalEntity, "auth_db_prev_rev"), should.BeNil)
			assert.Loosely(t, getStringProp(historicalEntity, "auth_db_change_comment"), should.Equal("Go pRPC API"))
			assert.Loosely(t, getStringProp(historicalEntity, "auth_db_app_version"), should.Equal("test-version"))
			assert.Loosely(t, getBoolProp(historicalEntity, "auth_db_deleted"), should.BeFalse)
		})

		t.Run("Updating AuthGlobalConfig", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, testAuthGlobalConfig(ctx)), should.BeNil)
			assert.Loosely(t, updateAuthGlobalConfig(ctx, oauthcfgpb, seccfgpb, "Go pRPC API"), should.BeNil)
			updatedCfg, err := GetAuthGlobalConfig(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, updatedCfg, should.Match(&AuthGlobalConfig{
				AuthVersionedEntityMixin: AuthVersionedEntityMixin{
					ModifiedTS:    testCreatedTS,
					ModifiedBy:    "user:test-app-id@example.com",
					AuthDBRev:     1,
					AuthDBPrevRev: 1337,
				},
				Kind:                     "AuthGlobalConfig",
				ID:                       "root",
				OAuthClientID:            "new-test-client-id",
				OAuthAdditionalClientIDs: []string{"new-test-client-id-1", "new-test-client-id-2"},
				OAuthClientSecret:        "new-test-client-secret",
				TokenServerURL:           "https://new-token-server-url.example.com",
				SecurityConfig:           testSecurityConfigBlob(),
			}))
			state1, err := GetReplicationState(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, state1.AuthDBRev, should.Equal(1))
			tasks := taskScheduler.Tasks()
			assert.Loosely(t, tasks, should.HaveLength(2))
			processChangeTask := tasks[0]
			assert.Loosely(t, processChangeTask.Class, should.Equal("process-change-task"))
			assert.Loosely(t, processChangeTask.Payload, should.Match(&taskspb.ProcessChangeTask{AuthDbRev: 1}))
			replicationTask := tasks[1]
			assert.Loosely(t, replicationTask.Class, should.Equal("replication-task"))
			assert.Loosely(t, replicationTask.Payload, should.Match(&taskspb.ReplicationTask{AuthDbRev: 1}))

			entities, err := getAllDatastoreEntities(ctx, "AuthGlobalConfigHistory", HistoricalRevisionKey(ctx, 1))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, entities, should.HaveLength(1))
			historicalEntity := entities[0]
			assert.Loosely(t, getDatastoreKey(historicalEntity).String(), should.Equal("dev~app::/AuthGlobalConfig,\"root\"/Rev,1/AuthGlobalConfigHistory,\"root\""))
			assert.Loosely(t, getStringProp(historicalEntity, "oauth_client_id"), should.Equal("new-test-client-id"))
			assert.Loosely(t, getStringProp(historicalEntity, "oauth_client_secret"), should.Equal("new-test-client-secret"))
			assert.Loosely(t, getStringProp(historicalEntity, "token_server_url"), should.Equal("https://new-token-server-url.example.com"))
			assert.Loosely(t, getByteSliceProp(historicalEntity, "security_config"), should.Match(testSecurityConfigBlob()))
			assert.Loosely(t, getStringSliceProp(historicalEntity, "oauth_additional_client_ids"), should.Match([]string{"new-test-client-id-1", "new-test-client-id-2"}))
			assert.Loosely(t, getStringProp(historicalEntity, "modified_by"), should.Equal("user:test-app-id@example.com"))
			assert.Loosely(t, getTimeProp(historicalEntity, "modified_ts").Unix(), should.Equal(testCreatedTS.Unix()))
			assert.Loosely(t, getInt64Prop(historicalEntity, "auth_db_rev"), should.Equal(1))
			assert.Loosely(t, getInt64Prop(historicalEntity, "auth_db_prev_rev"), should.Equal(1337))
			assert.Loosely(t, getStringProp(historicalEntity, "auth_db_change_comment"), should.Equal("Go pRPC API"))
			assert.Loosely(t, getStringProp(historicalEntity, "auth_db_app_version"), should.Equal("test-version"))
			assert.Loosely(t, getBoolProp(historicalEntity, "auth_db_deleted"), should.BeFalse)
		})
		t.Run("no-op for identical configs", func(t *ftt.Test) {
			// Set up an initial global config.
			assert.Loosely(t, datastore.Put(ctx, testAuthGlobalConfig(ctx)), should.BeNil)

			// Update global config with identical configs.
			sameOauthCfg := &configspb.OAuthConfig{
				PrimaryClientId:     "test-client-id",
				PrimaryClientSecret: "test-client-secret",
				ClientIds: []string{
					"additional-client-id-0",
					"additional-client-id-1",
				},
				TokenServerUrl: "https://token-server.example.com",
			}
			assert.Loosely(t, updateAuthGlobalConfig(ctx, sameOauthCfg, seccfgpb, "Go pRPC API"), should.BeNil)

			// Check this is a no-op.
			assert.Loosely(t, taskScheduler.Tasks(), should.HaveLength(0))

			// AuthGlobalConfig should be unchanged.
			updatedCfg, err := GetAuthGlobalConfig(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, updatedCfg, should.Match(testAuthGlobalConfig(ctx)))
		})
	})
}

func TestAuthRealmsConfig(t *testing.T) {
	t.Parallel()

	getCtx := func() (context.Context, *tqtesting.Scheduler) {
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
			Identity: "user:someone@example.com",
		})
		ctx = testsupport.SetTestContextSigner(ctx, "test-app-id", "test-app-id@example.com")
		ctx = clock.Set(ctx, testclock.New(testCreatedTS))
		ctx = info.SetImageVersion(ctx, "test-version")
		return tq.TestingContext(txndefer.FilterRDS(ctx), nil)
	}

	ftt.Run("Testing GetAuthRealmsGlobals", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		_, err := GetAuthRealmsGlobals(ctx)
		assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))

		realmGlobals := testAuthRealmsGlobals(ctx)
		err = datastore.Put(ctx, realmGlobals)
		assert.Loosely(t, err, should.BeNil)

		actual, err := GetAuthRealmsGlobals(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Match(realmGlobals))
	})

	ftt.Run("Testing GetAuthProjectRealms", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		testProject := "testproject"
		_, err := GetAuthProjectRealms(ctx, testProject)
		assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))

		projectRealms := testAuthProjectRealms(ctx, testProject)
		err = datastore.Put(ctx, projectRealms)
		assert.Loosely(t, err, should.BeNil)

		actual, err := GetAuthProjectRealms(ctx, testProject)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Match(projectRealms))
	})

	ftt.Run("Testing GetAllAuthProjectRealms", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		// Querying when there are no project realms should succeed.
		actual, err := GetAllAuthProjectRealms(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.BeEmpty)

		// Put 2 project realms in datastore.
		projectRealmsA := testAuthProjectRealms(ctx, "testproject-a")
		projectRealmsB := testAuthProjectRealms(ctx, "testproject-b")
		assert.Loosely(t, datastore.Put(ctx, projectRealmsA, projectRealmsB), should.BeNil)

		actual, err = GetAllAuthProjectRealms(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.HaveLength(2))
		// No guarantees on order, so sort the output before comparing.
		sort.Slice(actual, func(i, j int) bool {
			return actual[i].ID < actual[j].ID
		})
		assert.Loosely(t, actual, should.Match([]*AuthProjectRealms{
			projectRealmsA,
			projectRealmsB,
		}))
	})

	ftt.Run("Testing updateAuthProjectRealms", t, func(t *ftt.Test) {
		permissionsRev := "permissions.cfg:abc"
		proj1Realms := &protocol.Realms{
			Permissions: makeTestPermissions("luci.dev.p2", "luci.dev.z", "luci.dev.p1"),
			Realms: []*protocol.Realm{
				{
					Name: "proj1:@root",
					Bindings: []*protocol.Binding{
						{
							// Permissions p2, z, p1.
							Permissions: []uint32{0, 1, 2},
							Principals:  []string{"group:gr1"},
						},
					},
				},
			},
		}
		expandedRealms := []*ExpandedRealms{
			{
				CfgRev: &RealmsCfgRev{
					ProjectID:    "proj1",
					ConfigRev:    "a1b2c3",
					ConfigDigest: "test config digest",
					PermsRev:     "ignored-perms.cfg:321",
				},
				Realms: proj1Realms,
			},
		}
		expectedRealms, err := ToStorableRealms(proj1Realms)
		assert.Loosely(t, err, should.BeNil)

		t.Run("created for a new project", func(t *ftt.Test) {
			ctx, ts := getCtx()

			// Check updating realms for a new project works.
			err := updateAuthProjectRealms(ctx, expandedRealms, permissionsRev, "Go pRPC API")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ts.Tasks(), should.HaveLength(2))
			// Check the newly added project realms are as expected.
			authProjectRealms, err := GetAuthProjectRealms(ctx, "proj1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, authProjectRealms.ID, should.Equal("proj1"))
			assert.Loosely(t, authProjectRealms.ModifiedBy, should.Equal("user:test-app-id@example.com"))
			assert.Loosely(t, authProjectRealms.Realms, should.Match(expectedRealms))
			authProjectRealmsMeta, err := GetAuthProjectRealmsMeta(ctx, "proj1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, authProjectRealmsMeta, should.Match(&AuthProjectRealmsMeta{
				Kind:         "AuthProjectRealmsMeta",
				ID:           "meta",
				Parent:       projectRealmsKey(ctx, "proj1"),
				ConfigRev:    "a1b2c3",
				PermsRev:     permissionsRev,
				ConfigDigest: "test config digest",
				ModifiedTS:   testCreatedTS,
			}))
		})

		t.Run("updated for an existing project", func(t *ftt.Test) {
			ctx, ts := getCtx()

			originalAuthProjectRealms := testAuthProjectRealms(ctx, "proj1")
			originalAuthProjectRealmsMeta := testAuthProjectRealmsMeta(ctx, "proj1", "abc123")
			assert.Loosely(t, datastore.Put(ctx, originalAuthProjectRealms, originalAuthProjectRealmsMeta), should.BeNil)

			// Check updating realms for an existing project works.
			err = updateAuthProjectRealms(ctx, expandedRealms, permissionsRev, "Go pRPC API")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ts.Tasks(), should.HaveLength(2))
			// Check the newly added project realms are as expected.
			authProjectRealms, err := GetAuthProjectRealms(ctx, "proj1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, authProjectRealms.ID, should.Equal("proj1"))
			assert.Loosely(t, authProjectRealms.ModifiedBy, should.Equal("user:test-app-id@example.com"))
			assert.Loosely(t, authProjectRealms.Realms, should.Match(expectedRealms))
			authProjectRealmsMeta, err := GetAuthProjectRealmsMeta(ctx, "proj1")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, authProjectRealmsMeta, should.Match(&AuthProjectRealmsMeta{
				Kind:         "AuthProjectRealmsMeta",
				ID:           "meta",
				Parent:       projectRealmsKey(ctx, "proj1"),
				ConfigRev:    expandedRealms[0].CfgRev.ConfigRev,
				PermsRev:     permissionsRev,
				ConfigDigest: expandedRealms[0].CfgRev.ConfigDigest,
				ModifiedTS:   testCreatedTS,
			}))
		})
	})

	ftt.Run("Testing DeleteAuthProjectRealms", t, func(t *ftt.Test) {
		ctx, ts := getCtx()

		testProject := "testproject"
		projectRealms := testAuthProjectRealms(ctx, testProject)
		err := datastore.Put(ctx, projectRealms)
		assert.Loosely(t, err, should.BeNil)

		_, err = GetAuthProjectRealms(ctx, testProject)
		assert.Loosely(t, err, should.BeNil)

		err = deleteAuthProjectRealms(ctx, testProject, "Go pRPC API")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, ts.Tasks(), should.HaveLength(2))

		err = deleteAuthProjectRealms(ctx, testProject, "Go pRPC API")
		assert.Loosely(t, err, should.ErrLike(datastore.ErrNoSuchEntity))
	})

	ftt.Run("Testing GetAuthProjectRealmsMeta", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		testProject := "testproject"
		_, err := GetAuthProjectRealmsMeta(ctx, testProject)
		assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))

		projectRealms := testAuthProjectRealms(ctx, testProject)
		projectRealmsMeta := makeAuthProjectRealmsMeta(ctx, testProject)

		assert.Loosely(t, datastore.Put(ctx, projectRealms), should.BeNil)
		assert.Loosely(t, datastore.Put(ctx, projectRealmsMeta), should.BeNil)

		actual, err := GetAuthProjectRealmsMeta(ctx, testProject)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Match(projectRealmsMeta))
		actualID, err := actual.ProjectID()
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actualID, should.Equal(testProject))
	})

	ftt.Run("Testing GetAllAuthProjectRealmsMeta", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		testProjects := []string{"testproject-1", "testproject-2", "testproject-3"}
		projectRealms := make([]*AuthProjectRealms, len(testProjects))
		projectRealmsMeta := make([]*AuthProjectRealmsMeta, len(testProjects))
		for i, project := range testProjects {
			projectRealms[i] = testAuthProjectRealms(ctx, project)
			projectRealmsMeta[i] = makeAuthProjectRealmsMeta(ctx, project)
		}

		assert.Loosely(t, datastore.Put(ctx, projectRealms, projectRealmsMeta), should.BeNil)

		actual, err := GetAllAuthProjectRealmsMeta(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Match(projectRealmsMeta))
		for idx, proj := range testProjects {
			id, err := actual[idx].ProjectID()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, id, should.Equal(proj))
		}
	})

	ftt.Run("Testing updateAuthRealmsGlobals", t, func(t *ftt.Test) {
		t.Run("no previous AuthRealmsGlobals entity present", func(t *ftt.Test) {
			ctx, ts := getCtx()

			permCfg := &configspb.PermissionsConfig{
				Role: []*configspb.PermissionsConfig_Role{
					{
						Name: "role/test.role",
						Permissions: []*protocol.Permission{
							{
								Name: "test.perm.create",
							},
						},
					},
				},
			}

			err := updateAuthRealmsGlobals(ctx, permCfg, "Go pRPC API")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ts.Tasks(), should.HaveLength(2))

			fetched, err := GetAuthRealmsGlobals(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fetched.PermissionsList.GetPermissions(), should.Match(
				[]*protocol.Permission{
					{
						Name:     "test.perm.create",
						Internal: false,
					},
				}))
		})

		t.Run("updating permissions entity already present", func(t *ftt.Test) {
			ctx, ts := getCtx()
			authRealmGlobals := testAuthRealmsGlobals(ctx)
			assert.Loosely(t, datastore.Put(ctx, authRealmGlobals), should.BeNil)

			permsCfg := &configspb.PermissionsConfig{
				Role: []*configspb.PermissionsConfig_Role{
					{
						Name: "role/test.role",
						Permissions: []*protocol.Permission{
							{
								Name: "test.perm.create",
							},
						},
						Includes: []string{},
					},
					{
						Name: "role/test.role.two",
						Permissions: []*protocol.Permission{
							{
								Name: "testtwo.perm.delete",
							},
						},
					},
					{
						Name: "role/luci.internal.testint.role",
						Permissions: []*protocol.Permission{
							{
								Name:     "testint.perm.schedule",
								Internal: true,
							},
						},
					},
				},
			}

			assert.Loosely(t, updateAuthRealmsGlobals(ctx, permsCfg, "Go pRPC API"), should.BeNil)
			assert.Loosely(t, ts.Tasks(), should.HaveLength(2))

			fetched, err := GetAuthRealmsGlobals(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fetched.PermissionsList.GetPermissions(), should.Match(
				[]*protocol.Permission{
					{
						Name:     "test.perm.create",
						Internal: false,
					},
					{
						Name:     "testint.perm.schedule",
						Internal: true,
					},
					{
						Name:     "testtwo.perm.delete",
						Internal: false,
					},
				}))
		})

		t.Run("skip update if permissions unchanged", func(t *ftt.Test) {
			ctx, ts := getCtx()
			authRealmGlobals := testAuthRealmsGlobals(ctx)
			authRealmGlobals.PermissionsList = &permissions.PermissionsList{
				Permissions: []*protocol.Permission{
					{
						Name:     "test.perm.create",
						Internal: false,
					},
					{
						Name:     "testint.perm.schedule",
						Internal: true,
					},
					{
						Name:     "testtwo.perm.delete",
						Internal: false,
					},
				},
			}
			assert.Loosely(t, datastore.Put(ctx, authRealmGlobals), should.BeNil)

			permsCfg := &configspb.PermissionsConfig{
				Role: []*configspb.PermissionsConfig_Role{
					{
						Name: "role/test.role",
						Permissions: []*protocol.Permission{
							{
								Name: "test.perm.create",
							},
						},
						Includes: []string{},
					},
					{
						Name: "role/test.role.two",
						Permissions: []*protocol.Permission{
							{
								Name: "testtwo.perm.delete",
							},
						},
					},
					{
						Name: "role/luci.internal.testint.role",
						Permissions: []*protocol.Permission{
							{
								Name:     "testint.perm.schedule",
								Internal: true,
							},
						},
					},
				},
			}

			assert.Loosely(t, updateAuthRealmsGlobals(ctx, permsCfg, "Go pRPC API"), should.BeNil)
			assert.Loosely(t, ts.Tasks(), should.HaveLength(0))

			fetched, err := GetAuthRealmsGlobals(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fetched.PermissionsList.GetPermissions(), should.Match(
				[]*protocol.Permission{
					{
						Name:     "test.perm.create",
						Internal: false,
					},
					{
						Name:     "testint.perm.schedule",
						Internal: true,
					},
					{
						Name:     "testtwo.perm.delete",
						Internal: false,
					},
				}))
		})
	})
}

func TestGetAuthDBSnapshot(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())

	ftt.Run("Testing GetAuthDBSnapshot", t, func(t *ftt.Test) {
		_, err := GetAuthDBSnapshot(ctx, 42, false)
		assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))

		snapshot := testAuthDBSnapshot(ctx, 42)

		err = datastore.Put(ctx, snapshot)
		assert.Loosely(t, err, should.BeNil)

		actual, err := GetAuthDBSnapshot(ctx, 42, false)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Match(snapshot))

		err = datastore.Put(ctx, snapshot)
		assert.Loosely(t, err, should.BeNil)

		actual, err = GetAuthDBSnapshot(ctx, 42, false)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Match(snapshot))
	})

	ftt.Run("Testing GetAuthDBSnapshotLatest", t, func(t *ftt.Test) {
		_, err := GetAuthDBSnapshotLatest(ctx)
		assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))

		snapshot := testAuthDBSnapshot(ctx, 42)

		authDBSnapshotLatest := &AuthDBSnapshotLatest{
			Kind:         "AuthDBSnapshotLatest",
			ID:           "latest",
			AuthDBRev:    snapshot.ID,
			AuthDBSha256: snapshot.AuthDBSha256,
			ModifiedTS:   testModifiedTS,
		}
		err = datastore.Put(ctx, authDBSnapshotLatest)

		assert.Loosely(t, err, should.BeNil)

		actual, err := GetAuthDBSnapshotLatest(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Match(authDBSnapshotLatest))
	})

	ftt.Run("Testing unshardAuthDB", t, func(t *ftt.Test) {
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
		assert.Loosely(t, datastore.Put(ctx, authDBShard1), should.BeNil)
		assert.Loosely(t, datastore.Put(ctx, authDBShard2), should.BeNil)

		actualBlob, err := unshardAuthDB(ctx, shardIDs)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actualBlob, should.Match(expectedBlob))
	})

	ftt.Run("Testing GetAuthDBSnapshot with sharded DB", t, func(t *ftt.Test) {
		snapshot, expectedAuthDB, err := testAuthDBSnapshotSharded(ctx, 42, 3)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, datastore.Put(ctx, snapshot), should.BeNil)

		actualSnapshot, err := GetAuthDBSnapshot(ctx, 42, false)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actualSnapshot.AuthDBDeflated, should.Match(expectedAuthDB))

		actualSnapshot, err = GetAuthDBSnapshot(ctx, 42, true)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actualSnapshot.AuthDBDeflated, should.BeNil)
	})
}

func TestStoreAuthDBSnapshot(t *testing.T) {
	t.Parallel()

	ftt.Run("storing AuthDBSnapshot works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		// Set up the test data.
		authDBBlob := []byte("test-authdb-blob")
		expectedDeflated, err := zlib.Compress(authDBBlob)
		assert.Loosely(t, err, should.BeNil)
		blobChecksum := sha256.Sum256(authDBBlob)
		expectedHexDigest := hex.EncodeToString(blobChecksum[:])

		// Store the AuthDBSnapshot.
		err = StoreAuthDBSnapshot(ctx, testAuthReplicationState(ctx, 1), authDBBlob)
		assert.Loosely(t, err, should.BeNil)

		// Check the AuthDBSnapshot stored data.
		authDBSnapshot, err := GetAuthDBSnapshot(ctx, 1, false)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, authDBSnapshot, should.Match(&AuthDBSnapshot{
			Kind:           "AuthDBSnapshot",
			ID:             1,
			AuthDBDeflated: expectedDeflated,
			AuthDBSha256:   expectedHexDigest,
			CreatedTS:      testModifiedTS,
		}))
		decompressedBlob, _ := zlib.Decompress(authDBSnapshot.AuthDBDeflated)
		assert.Loosely(t, decompressedBlob, should.Match(authDBBlob))

		t.Run("no overwriting for existing revision", func(t *ftt.Test) {
			// Attempt to store the AuthDBSnapshot for an existing revision.
			err = StoreAuthDBSnapshot(ctx, testAuthReplicationState(ctx, 1), []byte("test-authdb-blob-changed"))
			assert.Loosely(t, err, should.BeNil)

			// Check the AuthDBSnapshot stored data was not actually changed.
			authDBSnapshot, err := GetAuthDBSnapshot(ctx, 1, false)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, authDBSnapshot, should.Match(&AuthDBSnapshot{
				Kind:           "AuthDBSnapshot",
				ID:             1,
				AuthDBDeflated: expectedDeflated,
				AuthDBSha256:   expectedHexDigest,
				CreatedTS:      testModifiedTS,
			}))
		})
	})

	ftt.Run("AuthDBSnapshotLatest is appropriately updated", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		// Set the latest revision to 24.
		latestSnapshot := &AuthDBSnapshotLatest{
			Kind:      "AuthDBSnapshotLatest",
			ID:        "latest",
			AuthDBRev: 24,
		}
		assert.Loosely(t, datastore.Put(ctx, latestSnapshot), should.BeNil)

		t.Run("updated for later revision", func(t *ftt.Test) {
			assert.Loosely(t, StoreAuthDBSnapshot(ctx, testAuthReplicationState(ctx, 28), nil), should.BeNil)

			// Check the latest AuthDBSnapshot pointer was updated.
			latestSnapshot, err := GetAuthDBSnapshotLatest(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, latestSnapshot.AuthDBRev, should.Equal(28))
		})

		t.Run("not updated for earlier revision", func(t *ftt.Test) {
			assert.Loosely(t, StoreAuthDBSnapshot(ctx, testAuthReplicationState(ctx, 23), nil), should.BeNil)

			// Check the latest AuthDBSnapshot pointer was not updated.
			latestSnapshot, err := GetAuthDBSnapshotLatest(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, latestSnapshot.AuthDBRev, should.Equal(24))
		})
	})

	ftt.Run("unsharding sharded data works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		// Shard an AuthDB compressed blob.
		authDBDeflatedBlob := []byte("this is test data")
		shardIDs, err := shardAuthDB(ctx, 32, authDBDeflatedBlob, 8)
		assert.Loosely(t, err, should.BeNil)

		// Check the blob is reconstructed given the same shard IDs.
		unshardedBlob, err := unshardAuthDB(ctx, shardIDs)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, unshardedBlob, should.Match(authDBDeflatedBlob))
	})
}

func TestDryRun(t *testing.T) {
	t.Parallel()

}

func TestProtoConversion(t *testing.T) {
	t.Parallel()

	ftt.Run("AuthGroup FromProto and ToProto round trip equivalence", t, func(t *ftt.Test) {
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"testers"},
		})

		empty := &AuthGroup{
			Kind:   "AuthGroup",
			Parent: RootKey(ctx),
		}
		emptyGroup, err := empty.ToProto(ctx, true)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, AuthGroupFromProto(ctx, emptyGroup), should.Match(empty))

		g := testAuthGroup(ctx, "foo-group")
		// Ignore the versioned entity mixin since this doesn't survive the proto conversion round trip.
		g.AuthVersionedEntityMixin = AuthVersionedEntityMixin{}

		group, err := g.ToProto(ctx, true)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, AuthGroupFromProto(ctx, group), should.Match(g))
	})

	ftt.Run("AuthGroup ToProto redacts emails", t, func(t *ftt.Test) {
		ctx := auth.WithState(memory.Use(context.Background()), &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"owners-foo"},
		})
		g := testAuthGroup(ctx, "google/testgroup")
		// Ignore the versioned entity mixin since this doesn't survive the proto conversion round trip.
		g.AuthVersionedEntityMixin = AuthVersionedEntityMixin{}

		group, err := g.ToProto(ctx, true)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, group.CallerCanViewMembers, should.Equal(false))
		assert.Loosely(t, group.NumRedacted, should.Equal(2))
	})
}

func TestRealmsToProto(t *testing.T) {
	t.Parallel()

	ftt.Run("Parsing Realms from AuthProjectRealms", t, func(t *ftt.Test) {
		projRealms := &protocol.Realms{
			Permissions: makeTestPermissions("luci.dev.p2", "luci.dev.z", "luci.dev.p1"),
			Realms: []*protocol.Realm{
				{
					Name: "testProj:@root",
					Bindings: []*protocol.Binding{
						{
							// Permissions p2, z, p1.
							Permissions: []uint32{0, 1, 2},
							Principals:  []string{"group:gr1"},
						},
					},
				},
			},
		}

		t.Run("modern format", func(t *ftt.Test) {
			modernFormat, err := ToStorableRealms(projRealms)
			assert.Loosely(t, err, should.BeNil)

			apr := &AuthProjectRealms{
				ID:     "testProj",
				Realms: modernFormat,
			}
			parsed, err := apr.RealmsToProto()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, parsed, should.Match(projRealms))
		})

		t.Run("uncompressed format", func(t *ftt.Test) {
			marshalled, err := proto.Marshal(projRealms)
			assert.Loosely(t, err, should.BeNil)

			apr := &AuthProjectRealms{
				ID:     "testProj",
				Realms: marshalled,
			}
			parsed, err := apr.RealmsToProto()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, parsed, should.Match(projRealms))
		})

		t.Run("legacy format", func(t *ftt.Test) {
			// This is an approximation of what the Python version of
			// Auth Service would have put in Datastore.
			marshalled, err := proto.Marshal(projRealms)
			assert.Loosely(t, err, should.BeNil)
			legacyFormat, err := zlib.Compress(marshalled)
			assert.Loosely(t, err, should.BeNil)

			apr := &AuthProjectRealms{
				ID:     "testProj",
				Realms: legacyFormat,
			}
			parsed, err := apr.RealmsToProto()
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, parsed, should.Match(projRealms))
		})
	})
}

func TestStorableRealms(t *testing.T) {
	t.Parallel()

	ftt.Run("FromStorableRealms is inverse of ToStorableRealms", t, func(t *ftt.Test) {
		projRealms := &protocol.Realms{
			Permissions: makeTestPermissions("luci.dev.p2", "luci.dev.z", "luci.dev.p1"),
			Realms: []*protocol.Realm{
				{
					Name: "testProj:@root",
					Bindings: []*protocol.Binding{
						{
							// Permissions p2, z, p1.
							Permissions: []uint32{0, 1, 2},
							Principals:  []string{"group:gr1"},
						},
					},
				},
			},
		}
		blob, err := ToStorableRealms(projRealms)
		assert.Loosely(t, err, should.BeNil)

		actual, err := FromStorableRealms(blob)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Match(projRealms))
	})
}

func TestValidateAdminGroup(t *testing.T) {
	t.Parallel()

	ftt.Run("Admin group validation works", t, func(t *ftt.Test) {
		ctx := context.Background()

		t.Run("must be self-owned", func(t *ftt.Test) {
			g := &AuthGroup{
				ID:     AdminGroup,
				Owners: "test-group",
			}
			assert.ErrIsLike(t, validateAdminGroup(ctx, g), ErrInvalidArgument)
			assert.ErrIsLike(t, validateAdminGroup(ctx, g), "owned by itself")
		})

		t.Run("cannot have globs", func(t *ftt.Test) {
			g := &AuthGroup{
				ID:     AdminGroup,
				Owners: AdminGroup,
				Globs:  []string{"user:*@example.com"},
			}
			assert.ErrIsLike(t, validateAdminGroup(ctx, g), ErrInvalidArgument)
			assert.ErrIsLike(t, validateAdminGroup(ctx, g),
				"cannot have globs")
		})

		t.Run("cannot have internal subgroups", func(t *ftt.Test) {
			g := &AuthGroup{
				ID:     AdminGroup,
				Owners: AdminGroup,
				Nested: []string{"test-group"},
			}
			assert.ErrIsLike(t, validateAdminGroup(ctx, g), ErrInvalidArgument)
			assert.ErrIsLike(t, validateAdminGroup(ctx, g),
				"can only have external subgroups")
		})

		t.Run("cannot be empty", func(t *ftt.Test) {
			g := &AuthGroup{
				ID:     AdminGroup,
				Owners: AdminGroup,
			}
			assert.ErrIsLike(t, validateAdminGroup(ctx, g), ErrInvalidArgument)
			assert.ErrIsLike(t, validateAdminGroup(ctx, g),
				"cannot be empty")
		})

		t.Run("happy with a member", func(t *ftt.Test) {
			g := &AuthGroup{
				ID:      AdminGroup,
				Owners:  AdminGroup,
				Members: []string{"user:someone@example.com"},
			}
			assert.Loosely(t, validateAdminGroup(ctx, g), should.BeNil)
		})

		t.Run("happy with an external subgroup", func(t *ftt.Test) {
			g := &AuthGroup{
				ID:     AdminGroup,
				Owners: AdminGroup,
				Nested: []string{"sys/admins@example.com"},
			}
			assert.Loosely(t, validateAdminGroup(ctx, g), should.BeNil)
		})
	})
}
