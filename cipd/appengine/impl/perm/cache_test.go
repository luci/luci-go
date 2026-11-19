// Copyright 2026 The LUCI Authors.
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

package perm

import (
	"context"
	"errors"
	"testing"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/auth/realms"
)

type countingDB struct {
	authdb.DB
	permCalls  int
	groupCalls int
	err        error
	groupErr   error
}

func (d *countingDB) HasPermission(ctx context.Context, id identity.Identity, perm realms.Permission, realm string, attrs realms.Attrs) (bool, error) {
	d.permCalls++
	if d.err != nil {
		return false, d.err
	}
	return d.DB.HasPermission(ctx, id, perm, realm, attrs)
}

func (d *countingDB) IsMember(ctx context.Context, id identity.Identity, groups []string) (bool, error) {
	d.groupCalls++
	if d.groupErr != nil {
		return false, d.groupErr
	}
	return d.DB.IsMember(ctx, id, groups)
}

func TestCacheHasPerm(t *testing.T) {
	t.Parallel()

	const user = identity.Identity("user:caller@example.com")
	db := &countingDB{
		DB: authtest.NewFakeDB(
			authtest.MockPermission(user, "proj:granted", DiagnoseACL.Perm()),
		),
	}
	ctx := auth.WithState(t.Context(), &authtest.FakeState{
		Identity: user,
		FakeDB:   db,
	})
	cache := NewCache(ctx, nil)

	// Empty realm checks implied group (if any) and skips HasPermission; a
	// permission with no implied group hits neither IsMember nor HasPermission.
	has, err := cache.HasPerm(ctx, "", DiagnoseACL)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeFalse)
	assert.That(t, db.groupCalls, should.Equal(1))
	assert.That(t, db.permCalls, should.Equal(0))

	has, err = cache.HasPerm(ctx, "", WriteInstance)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeFalse)
	assert.That(t, db.groupCalls, should.Equal(1))
	assert.That(t, db.permCalls, should.Equal(0))

	// Granted permission is cached across repeated lookups; implied group lookup
	// is also served from checkedGroups.
	has, err = cache.HasPerm(ctx, "proj:granted", DiagnoseACL)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeTrue)
	assert.That(t, db.groupCalls, should.Equal(1))
	assert.That(t, db.permCalls, should.Equal(1))

	has, err = cache.HasPerm(ctx, "proj:granted", DiagnoseACL)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeTrue)
	assert.That(t, db.groupCalls, should.Equal(1))
	assert.That(t, db.permCalls, should.Equal(1))

	// Transient errors are not cached; subsequent calls retry authdb.
	db.err = errors.New("authdb boom")
	_, err = cache.HasPerm(ctx, "proj:denied", DiagnoseACL)
	assert.ErrIsLike(t, err, "authdb boom")
	assert.That(t, db.groupCalls, should.Equal(1))
	assert.That(t, db.permCalls, should.Equal(2))

	// Denied permission is cached once the error clears.
	db.err = nil
	has, err = cache.HasPerm(ctx, "proj:denied", DiagnoseACL)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeFalse)
	assert.That(t, db.groupCalls, should.Equal(1))
	assert.That(t, db.permCalls, should.Equal(3))

	has, err = cache.HasPerm(ctx, "proj:denied", DiagnoseACL)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeFalse)
	assert.That(t, db.groupCalls, should.Equal(1))
	assert.That(t, db.permCalls, should.Equal(3))
}

func TestCacheHasPermAttrs(t *testing.T) {
	t.Parallel()

	const (
		condUser   = identity.Identity("user:cond@example.com")
		uncondUser = identity.Identity("user:uncond@example.com")
		realm      = "proj:realm"
	)
	db := &countingDB{
		DB: authtest.NewFakeDB(
			authtest.MockPermission(
				condUser, realm, WriteRef.Perm(),
				authtest.RestrictAttribute("cipd.package.refName", "latest"),
			),
			authtest.MockPermission(uncondUser, realm, WriteRef.Perm()),
		),
	}

	condCtx := auth.WithState(t.Context(), &authtest.FakeState{
		Identity: condUser,
		FakeDB:   db,
	})
	condCache := NewCache(condCtx, nil)

	has, err := condCache.HasPerm(condCtx, realm, WriteRef.Bind("latest"))
	assert.NoErr(t, err)
	assert.That(t, has, should.BeTrue)
	assert.That(t, db.permCalls, should.Equal(1))

	// Repeat call hits cache.
	has, err = condCache.HasPerm(condCtx, realm, WriteRef.Bind("latest"))
	assert.NoErr(t, err)
	assert.That(t, has, should.BeTrue)
	assert.That(t, db.permCalls, should.Equal(1))

	// Different bound value uses a distinct cache key.
	has, err = condCache.HasPerm(condCtx, realm, WriteRef.Bind("prod"))
	assert.NoErr(t, err)
	assert.That(t, has, should.BeFalse)
	assert.That(t, db.permCalls, should.Equal(2))

	// Unconditional uses a distinct cache key and passes nil attrs, so a
	// conditionally restricted grant does not satisfy it.
	has, err = condCache.HasPerm(condCtx, realm, WriteRef.Unconditional())
	assert.NoErr(t, err)
	assert.That(t, has, should.BeFalse)
	assert.That(t, db.permCalls, should.Equal(3))

	has, err = condCache.HasPerm(condCtx, realm, WriteRef.Unconditional())
	assert.NoErr(t, err)
	assert.That(t, has, should.BeFalse)
	assert.That(t, db.permCalls, should.Equal(3))

	// Unconditionally granted user satisfies both Unconditional and Bind.
	uncondCtx := auth.WithState(t.Context(), &authtest.FakeState{
		Identity: uncondUser,
		FakeDB:   db,
	})
	uncondCache := NewCache(uncondCtx, nil)

	has, err = uncondCache.HasPerm(uncondCtx, realm, WriteRef.Unconditional())
	assert.NoErr(t, err)
	assert.That(t, has, should.BeTrue)
	assert.That(t, db.permCalls, should.Equal(4))

	has, err = uncondCache.HasPerm(uncondCtx, realm, WriteRef.Bind("prod"))
	assert.NoErr(t, err)
	assert.That(t, has, should.BeTrue)
	assert.That(t, db.permCalls, should.Equal(5))
}

func TestCacheRawGroupCheck(t *testing.T) {
	t.Parallel()

	const user = identity.Identity("user:member@example.com")
	db := &countingDB{
		DB: authtest.NewFakeDB(
			authtest.MockMembership(user, "granted-group"),
		),
	}
	ctx := auth.WithState(t.Context(), &authtest.FakeState{
		Identity: user,
		FakeDB:   db,
	})
	cache := NewCache(ctx, nil)

	has, err := cache.RawGroupCheck(ctx, "granted-group")
	assert.NoErr(t, err)
	assert.That(t, has, should.BeTrue)
	assert.That(t, db.groupCalls, should.Equal(1))

	has, err = cache.RawGroupCheck(ctx, "granted-group")
	assert.NoErr(t, err)
	assert.That(t, has, should.BeTrue)
	assert.That(t, db.groupCalls, should.Equal(1))

	// Transient errors are not cached.
	db.groupErr = errors.New("group boom")
	_, err = cache.RawGroupCheck(ctx, "denied-group")
	assert.ErrIsLike(t, err, "group boom")
	assert.That(t, db.groupCalls, should.Equal(2))

	db.groupErr = nil
	has, err = cache.RawGroupCheck(ctx, "denied-group")
	assert.NoErr(t, err)
	assert.That(t, has, should.BeFalse)
	assert.That(t, db.groupCalls, should.Equal(3))

	has, err = cache.RawGroupCheck(ctx, "denied-group")
	assert.NoErr(t, err)
	assert.That(t, has, should.BeFalse)
	assert.That(t, db.groupCalls, should.Equal(3))
}

func TestCacheHasPermGroups(t *testing.T) {
	t.Parallel()

	const (
		viewer       = identity.Identity("user:viewer@example.com")
		admin        = identity.Identity("user:admin@example.com")
		fakeRealmUsr = identity.Identity("user:fake-realm@example.com")
		realm        = "proj:realm"
	)
	db := &countingDB{
		DB: authtest.NewFakeDB(
			authtest.MockMembership(viewer, PrefixesViewers),
			authtest.MockMembership(admin, "administrators"),
			authtest.MockPermission(fakeRealmUsr, realm, DeletePackage.Perm()),
		),
	}

	// Implied group grants permission even with empty realm, short-circuits
	// HasPermission when realm is non-empty, and shares checkedGroups across
	// permissions with the same group.
	viewerCtx := auth.WithState(t.Context(), &authtest.FakeState{
		Identity: viewer,
		FakeDB:   db,
	})
	viewerCache := NewCache(viewerCtx, nil)

	has, err := viewerCache.HasPerm(viewerCtx, "", DiagnoseACL)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeTrue)
	assert.That(t, db.groupCalls, should.Equal(1))
	assert.That(t, db.permCalls, should.Equal(0))

	has, err = viewerCache.HasPerm(viewerCtx, realm, DiagnoseACL)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeTrue)
	assert.That(t, db.groupCalls, should.Equal(1))
	assert.That(t, db.permCalls, should.Equal(0))

	has, err = viewerCache.HasPerm(viewerCtx, realm, ListInstances)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeTrue)
	assert.That(t, db.groupCalls, should.Equal(1))
	assert.That(t, db.permCalls, should.Equal(0))

	// groupOnly permission grants via group (with or without realm) and never
	// falls back to HasPermission even when mocked in a realm.
	adminCtx := auth.WithState(t.Context(), &authtest.FakeState{
		Identity: admin,
		FakeDB:   db,
	})
	adminCache := NewCache(adminCtx, nil)

	has, err = adminCache.HasPerm(adminCtx, "", DeletePackage)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeTrue)
	assert.That(t, db.groupCalls, should.Equal(2))
	assert.That(t, db.permCalls, should.Equal(0))

	has, err = adminCache.HasPerm(adminCtx, realm, DeletePackage)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeTrue)
	assert.That(t, db.groupCalls, should.Equal(2))
	assert.That(t, db.permCalls, should.Equal(0))

	fakeCtx := auth.WithState(t.Context(), &authtest.FakeState{
		Identity: fakeRealmUsr,
		FakeDB:   db,
	})
	fakeCache := NewCache(fakeCtx, nil)

	db.groupErr = errors.New("group check boom")
	_, err = fakeCache.HasPerm(fakeCtx, realm, DeletePackage)
	assert.ErrIsLike(t, err, "group check boom")
	assert.That(t, db.groupCalls, should.Equal(3))
	assert.That(t, db.permCalls, should.Equal(0))

	db.groupErr = nil
	has, err = fakeCache.HasPerm(fakeCtx, realm, DeletePackage)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeFalse)
	assert.That(t, db.groupCalls, should.Equal(4))
	assert.That(t, db.permCalls, should.Equal(0))
}
