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

package groups

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/auth_service/impl/model"
)

var (
	testCreatedTS  = time.Date(2020, time.August, 16, 15, 20, 0, 0, time.UTC)
	testModifiedTS = time.Date(2021, time.August, 16, 12, 20, 0, 0, time.UTC)
)

func testAuthVersionedEntityMixin() model.AuthVersionedEntityMixin {
	return model.AuthVersionedEntityMixin{
		ModifiedTS:    testModifiedTS,
		ModifiedBy:    "user:test-modifier@example.com",
		AuthDBRev:     337,
		AuthDBPrevRev: 336,
	}
}

func testGroup(ctx context.Context, name string, members []string) *model.AuthGroup {
	return &model.AuthGroup{
		Kind:                     "AuthGroup",
		AuthVersionedEntityMixin: testAuthVersionedEntityMixin(),
		ID:                       name,
		Parent:                   model.RootKey(ctx),
		Owners:                   model.AdminGroup,
		Members:                  members,
		CreatedTS:                testCreatedTS,
		CreatedBy:                "user:test-creator@example.com",
	}
}

// putRev is a test helper function to put an AuthDB replication state
// into Datastore for the given revision.
func putRev(ctx context.Context, authDBRev int64) error {
	return datastore.Put(ctx, &model.AuthReplicationState{
		Kind:      "AuthReplicationState",
		AuthDBRev: authDBRev,
		Parent:    model.RootKey(ctx),
	})
}

func TestCachingGroupsProvider(t *testing.T) {
	t.Parallel()

	testTime := time.Date(2023, 2, 1, 0, 0, 0, 0, time.UTC)

	ftt.Run("CachingGroupsProvider works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		ctx, tc := testclock.UseTime(ctx, testTime)

		expectedInitial := []*model.AuthGroup{
			testGroup(ctx, "test-group-a", []string{"user:alice@example.com"}),
			testGroup(ctx, "test-group-b", []string{"user:bob@example.com"}),
		}
		expectedUpdated := []*model.AuthGroup{
			testGroup(ctx, "test-group-a",
				[]string{"user:alice@example.com", "user:eve@example.com"}),
			testGroup(ctx, "test-group-b", []string{"user:bob@example.com"}),
		}
		expectedFinal := []*model.AuthGroup{
			testGroup(ctx, "test-group-a",
				[]string{"user:alice@example.com", "user:eve@example.com"}),
			testGroup(ctx, "test-group-b", []string{"user:charlie@example.com"}),
		}

		provider := &CachingGroupsProvider{}

		// Getting the initial groups snapshot fails, since there's nothing in
		// datastore yet.
		_, err := provider.GetAllAuthGroups(ctx, false)
		assert.Loosely(t, errors.Is(err, datastore.ErrNoSuchEntity), should.BeTrue)

		// Set up initial revision with 2 groups.
		assert.Loosely(t, datastore.Put(ctx, []*model.AuthGroup{
			testGroup(ctx, "test-group-a", []string{"user:alice@example.com"}),
			testGroup(ctx, "test-group-b", []string{"user:bob@example.com"}),
		}), should.BeNil)
		assert.Loosely(t, putRev(ctx, 1000), should.BeNil)

		// Check all groups were fetched.
		groups, err := provider.GetAllAuthGroups(ctx, false)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, groups, should.Match(expectedInitial))

		// Add a member to test-group-a, WITHOUT updating the revision.
		assert.Loosely(t, datastore.Put(ctx,
			testGroup(ctx, "test-group-a", []string{
				"user:alice@example.com", "user:eve@example.com"},
			)), should.BeNil)

		// At a later time, calling again returns the exact same groups since
		// the revision hasn't changed.
		groups, err = provider.GetAllAuthGroups(ctx, false)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, groups, should.Match(expectedInitial))

		// Increase the AuthDB replication state revision (making the cached
		// results outdated).
		assert.Loosely(t, putRev(ctx, 1001), should.BeNil)

		// Datastore is updated, but the cached copy is fresh enough.
		groups, err = provider.GetAllAuthGroups(ctx, true)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, groups, should.Match(expectedInitial))

		// Now check the snapshot is updated once the cached copy is too stale.
		tc.Add(maxStaleness)
		groups, err = provider.GetAllAuthGroups(ctx, true)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, groups, should.Match(expectedUpdated))

		// Change member of test-group-b and update the revision.
		assert.Loosely(t, datastore.Put(ctx,
			testGroup(ctx, "test-group-b", []string{"user:charlie@example.com"})),
			should.BeNil)
		assert.Loosely(t, putRev(ctx, 1002), should.BeNil)

		// Refusing staleness returns the latest groups, even if the cached copy
		// is fresh enough.
		groups, err = provider.GetAllAuthGroups(ctx, false)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, groups, should.Match(expectedFinal))
	})
}
