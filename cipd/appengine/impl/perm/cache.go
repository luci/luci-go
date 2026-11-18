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

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
)

// boolCache is a simple map of comparable to bool for caching permission
// resolutions.
type boolCache[K comparable] map[K]bool

// getOrCompute retrieves the value for `key`, or, if it is not yet in the
// cache, computes it with `cb`.
func (b *boolCache[K]) getOrCompute(key K, cb func() (bool, error)) (bool, error) {
	if has, ok := (*b)[key]; ok {
		return has, nil
	}

	has, err := cb()
	if err != nil {
		return false, err
	}

	if *b == nil {
		(*b) = boolCache[K]{key: has}
	} else {
		(*b)[key] = has
	}
	return has, nil
}

// realmPerm is realm + Permission tuple meant for use with
// [Cache.checkedPerms].
type realmPerm struct {
	realm string
	perm  Permission
}

// Cache caches identity-scoped permission lookups (e.g. realm permissions,
// group memberships) across one or more prefixes within a single request.
//
// Not safe for concurrent use.
//
// TODO: Cache group memberships, hold metadata.Storage for legacy fallback, and
// add ForPrefix.
type Cache struct {
	ident  identity.Identity
	authdb authdb.DB

	checkedPerms boolCache[realmPerm]
}

// NewCache returns a new [Cache], extracting the current identity and authdb
// from `ctx`.
func NewCache(ctx context.Context) *Cache {
	return &Cache{
		ident:  auth.CurrentIdentity(ctx),
		authdb: auth.GetState(ctx).DB(),
	}
}

// HasPerm checks the realm permission in `realm`.
func (r *Cache) HasPerm(ctx context.Context, realm string, perm Permission) (has bool, err error) {
	// TODO: Skip fake (non-realm) permissions and pass bound attributes.
	if realm == "" {
		return false, nil
	}

	return r.checkedPerms.getOrCompute(realmPerm{realm, perm}, func() (bool, error) {
		return r.authdb.HasPermission(ctx, r.ident, perm.perm, realm, nil)
	})
}
