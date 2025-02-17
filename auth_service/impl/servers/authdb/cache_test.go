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

package authdb

import (
	"context"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/impl/model"
	"go.chromium.org/luci/auth_service/impl/model/graph"
)

func storeTestAuthDBSnapshot(ctx context.Context, realms *protocol.Realms,
	groups []*protocol.AuthGroup, rev int64, t *ftt.Test) {
	testRequest := &protocol.ReplicationPushRequest{
		Revision: &protocol.AuthDBRevision{
			AuthDbRev: rev,
		},
		AuthDb: &protocol.AuthDB{
			Realms: realms,
			Groups: groups,
		},
	}
	blob, err := proto.Marshal(testRequest)
	assert.Loosely(t, err, should.BeNil)

	testReplicationState := &model.AuthReplicationState{
		AuthDBRev: rev,
	}
	err = model.StoreAuthDBSnapshot(ctx, testReplicationState, blob)
	assert.Loosely(t, err, should.BeNil)
}

func storeTestAuthDBSnapshotLatest(ctx context.Context, rev int64, t *ftt.Test) {
	authDBSnapshotLatest := &model.AuthDBSnapshotLatest{
		Kind:         "AuthDBSnapshotLatest",
		ID:           "latest",
		AuthDBRev:    rev,
		AuthDBSha256: "test-sha-256",
	}
	err := datastore.Put(ctx, authDBSnapshotLatest)
	assert.Loosely(t, err, should.BeNil)
}

func TestPermissionsProvider(t *testing.T) {
	t.Parallel()

	testTime := time.Date(2023, 2, 1, 0, 0, 0, 0, time.UTC)

	snapComp := cmp.AllowUnexported(
		PermissionsSnapshot{}, graph.Graph{}, graph.GroupNode{})

	ftt.Run("CachingPermissionsProvider works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx, tc := testclock.UseTime(ctx, testTime)
		const revInitial = 1000
		const revUpdated = 1001

		group1 := &protocol.AuthGroup{
			Name:    "gr1",
			Members: []string{"someone@example.com"},
		}

		expectedInitial := &PermissionsSnapshot{
			authDBRev:       revInitial,
			permissionNames: []string{"luci.dev.p1", "luci.dev.p2", "luci.dev.p3"},
			permissionsMap: map[string][]*model.RealmPermissions{
				"group:gr1": {
					{
						Name:        "p:r",
						Permissions: []uint32{0, 1, 2},
					},
				},
			},
			groupsGraph: graph.NewGraph(
				[]model.GraphableGroup{model.GraphableGroup(group1)}),
		}
		expectedUpdated := &PermissionsSnapshot{
			authDBRev:       revUpdated,
			permissionNames: []string{"luci.dev.p1", "luci.dev.p2", "luci.dev.p3"},
			permissionsMap: map[string][]*model.RealmPermissions{
				"group:gr1": {
					{
						Name:        "p:r",
						Permissions: []uint32{0, 1, 2},
					},
					{
						Name:        "p:r2",
						Permissions: []uint32{2},
					},
				},
				"group:gr2": {
					{
						Name:        "p:r",
						Permissions: []uint32{1, 2},
					},
				},
			},
			groupsGraph: graph.NewGraph(
				[]model.GraphableGroup{model.GraphableGroup(group1)}),
		}

		provider := &CachingPermissionsProvider{}

		// Getting the initial permissions snapshot fails, since there's nothing in
		// datastore yet.
		_, err := provider.Get(ctx)
		assert.Loosely(t, errors.Is(err, datastore.ErrNoSuchEntity), should.BeTrue)

		// Set up initial revision with 1 group with permissions in 1 realm.
		realmsInitial := &protocol.Realms{
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
					},
				},
			},
		}
		groups := []*protocol.AuthGroup{group1}

		// Store realms in snapshot & set latest snapshot to its revision number.
		storeTestAuthDBSnapshot(ctx, realmsInitial, groups, revInitial, t)
		storeTestAuthDBSnapshotLatest(ctx, revInitial, t)

		// Check the permissions snapshot was fetched.
		snapshot1, err := provider.Get(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, snapshot1, should.Match(expectedInitial, snapComp))

		// Add permission to gr1 & new gr2 in realms.
		realmsUpdated := &protocol.Realms{
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

		// Update snapshot & latest snapshot revision number.
		storeTestAuthDBSnapshot(ctx, realmsUpdated, groups, revUpdated, t)
		storeTestAuthDBSnapshotLatest(ctx, revUpdated, t)

		// At a later time, calling again returns the exact same permission
		// snapshot since maxStaleness time has not been exceeded.
		tc.Add(maxStaleness - 1)
		snapshot2, err := provider.Get(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, snapshot2, should.Match(expectedInitial, snapComp))
		// It should literally be the same object.
		assert.Loosely(t, snapshot2, should.Equal(snapshot1))

		// Now check the permissions snapshot is updated once the cached copy is too
		// stale.
		tc.Add(1)
		snapshot3, err := provider.Get(ctx)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, snapshot3, should.Match(expectedUpdated, snapComp))
	})
}
