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

package model

import (
	"context"
	"testing"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/rpcpb"
)

func TestGetAuthorizationSnapshot(t *testing.T) {
	t.Parallel()
	ftt.Run("Returns realms object", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		const testAuthDBRev = 12345

		testRealms := &protocol.Realms{
			Permissions: []*protocol.Permission{
				{Name: "luci.dev.p1"},
				{Name: "luci.dev.p2"},
				{Name: "luci.dev.p3"},
			},
			Realms: []*protocol.Realm{
				{
					Name: "p:@root",
				},
				{
					Name: "p:r",
					Bindings: []*protocol.Binding{
						{
							Permissions: []uint32{0, 1},
							Principals:  []string{"group:gr1"},
						},
						{
							Permissions: []uint32{0, 1, 2},
							Principals:  []string{"group:gr3", "group:gr4"},
						},
						{
							Permissions: []uint32{1, 2},
							Principals:  []string{"group:gr2"},
						},
					},
				},
			},
		}
		testGroups := []*protocol.AuthGroup{
			{Name: "group-a", Members: []string{"a@example.com"}},
			{Name: "group-b", Nested: []string{"group-a"}},
		}
		testRequest := &protocol.ReplicationPushRequest{
			Revision: &protocol.AuthDBRevision{
				AuthDbRev: testAuthDBRev,
			},
			AuthDb: &protocol.AuthDB{
				Realms: testRealms,
				Groups: testGroups,
			},
		}
		blob, err := proto.Marshal(testRequest)
		assert.Loosely(t, err, should.BeNil)

		testReplicationState := &AuthReplicationState{
			AuthDBRev: testAuthDBRev,
		}
		err = StoreAuthDBSnapshot(ctx, testReplicationState, blob)
		assert.Loosely(t, err, should.BeNil)

		authDBSnapshotLatest := &AuthDBSnapshotLatest{
			Kind:         "AuthDBSnapshotLatest",
			ID:           "latest",
			AuthDBRev:    testAuthDBRev,
			AuthDBSha256: "test-sha-256",
			ModifiedTS:   testModifiedTS,
		}
		err = datastore.Put(ctx, authDBSnapshotLatest)
		assert.Loosely(t, err, should.BeNil)

		actual, err := GetAuthDBFromSnapshot(ctx, testAuthDBRev)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual.Realms, should.Match(testRealms))
		assert.Loosely(t, actual.Groups, should.Match(testGroups))
	})
}

func TestAnalyzePrincipalPermissions(t *testing.T) {
	t.Parallel()
	ftt.Run("Returns correct permissions on valid group", t, func(t *ftt.Test) {
		testRealms := &protocol.Realms{
			Permissions: []*protocol.Permission{
				{Name: "luci.dev.p1"},
				{Name: "luci.dev.p2"},
				{Name: "luci.dev.p3"},
				{Name: "luci.dev.p4"},
			},
			Realms: []*protocol.Realm{
				{
					Name: "p:@root",
				},
				{
					Name: "p:r",
					Bindings: []*protocol.Binding{
						{
							Permissions: []uint32{0, 1},
							Principals:  []string{"group:gr1"},
						},
						{
							Permissions: []uint32{0, 1, 2},
							Principals:  []string{"group:gr3", "group:gr4"},
						},
						{
							Permissions: []uint32{1, 2},
							Principals:  []string{"group:gr1", "group:gr2"},
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
			},
		}
		expectedResult := map[string][]*rpcpb.RealmPermissions{
			"group:gr1": {
				{
					Name:        "p:r",
					Permissions: []string{"luci.dev.p1", "luci.dev.p2", "luci.dev.p3"},
				},
				{
					Name:        "p:r2",
					Permissions: []string{"luci.dev.p3"},
				},
			},
			"group:gr2": {
				{
					Name:        "p:r",
					Permissions: []string{"luci.dev.p2", "luci.dev.p3"},
				},
			},
			"group:gr3": {
				{
					Name:        "p:r",
					Permissions: []string{"luci.dev.p1", "luci.dev.p2", "luci.dev.p3"},
				},
			},
			"group:gr4": {
				{
					Name:        "p:r",
					Permissions: []string{"luci.dev.p1", "luci.dev.p2", "luci.dev.p3"},
				},
			},
		}
		actualPermissions, err := AnalyzePrincipalPermissions(testRealms)
		assert.Loosely(t, actualPermissions, should.Match(expectedResult))
		assert.Loosely(t, err, should.BeNil)
	})
}
