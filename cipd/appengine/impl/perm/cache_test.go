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
	permCalls int
	err       error
}

func (d *countingDB) HasPermission(ctx context.Context, id identity.Identity, perm realms.Permission, realm string, attrs realms.Attrs) (bool, error) {
	d.permCalls++
	if d.err != nil {
		return false, d.err
	}
	return d.DB.HasPermission(ctx, id, perm, realm, attrs)
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
	cache := NewCache(ctx)

	// Empty realm short-circuits without hitting authdb.
	has, err := cache.HasPerm(ctx, "", DiagnoseACL)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeFalse)
	assert.That(t, db.permCalls, should.Equal(0))

	// Granted permission is cached across repeated lookups.
	has, err = cache.HasPerm(ctx, "proj:granted", DiagnoseACL)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeTrue)
	assert.That(t, db.permCalls, should.Equal(1))

	has, err = cache.HasPerm(ctx, "proj:granted", DiagnoseACL)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeTrue)
	assert.That(t, db.permCalls, should.Equal(1))

	// Transient errors are not cached; subsequent calls retry authdb.
	db.err = errors.New("authdb boom")
	_, err = cache.HasPerm(ctx, "proj:denied", DiagnoseACL)
	assert.ErrIsLike(t, err, "authdb boom")
	assert.That(t, db.permCalls, should.Equal(2))

	// Denied permission is cached once the error clears.
	db.err = nil
	has, err = cache.HasPerm(ctx, "proj:denied", DiagnoseACL)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeFalse)
	assert.That(t, db.permCalls, should.Equal(3))

	has, err = cache.HasPerm(ctx, "proj:denied", DiagnoseACL)
	assert.NoErr(t, err)
	assert.That(t, has, should.BeFalse)
	assert.That(t, db.permCalls, should.Equal(3))
}
