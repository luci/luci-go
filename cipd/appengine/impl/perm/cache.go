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

	"go.chromium.org/luci/cipd/appengine/impl/prefixcfg"
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

// Cache caches identity-scoped permission and group lookups across one or more
// prefixes within a single request.
//
// Not safe for concurrent use.
//
// TODO: Hold metadata.Storage for legacy fallback.
type Cache struct {
	ident  identity.Identity
	authdb authdb.DB

	checkedPerms  boolCache[realmPerm]
	checkedGroups boolCache[string]
}

// NewCache returns a new [Cache], extracting the current identity and authdb
// from `ctx`.
func NewCache(ctx context.Context) *Cache {
	return &Cache{
		ident:  auth.CurrentIdentity(ctx),
		authdb: auth.GetState(ctx).DB(),
	}
}

// RawGroupCheck checks if the caller has membership in the given group.
//
// This is exposed only to allow Repo to reuse the group cache for some legacy
// ACL things.
func (r *Cache) RawGroupCheck(ctx context.Context, group string) (bool, error) {
	return r.checkedGroups.getOrCompute(group, func() (bool, error) {
		return r.authdb.IsMember(ctx, r.ident, []string{group})
	})
}

// HasPerm checks whether the current identity has `perm` via its implied group
// (if any) or via LUCI Realms in `realm` (when `realm` is non-empty and `perm`
// is not groupOnly).
func (r *Cache) HasPerm(ctx context.Context, realm string, perm Permission) (has bool, err error) {
	// Check if the current identity has access to the group which implies `perm`
	// (if any).
	if perm.core.group != "" {
		if has, err = r.RawGroupCheck(ctx, perm.core.group); has || err != nil {
			return
		}
	}

	// Next, check if there is a realm, and this permission actually can be
	// granted via realms, if the current identity has this permission.
	if realm != "" && !perm.core.groupOnly {
		return r.checkedPerms.getOrCompute(realmPerm{realm, perm}, func() (bool, error) {
			return r.authdb.HasPermission(ctx, r.ident, perm.Perm(), realm, perm.attrs())
		})
	}

	return false, nil
}

// ForPrefix returns a [PrefixCache] bound to `prefix` and its corresponding
// `cfg` that shares identity-scoped caches with `r`.
func (r *Cache) ForPrefix(prefix string, cfg *prefixcfg.Entry) *PrefixCache {
	return &PrefixCache{
		pcache: r,
		prefix: prefix,
		cfg:    cfg,
	}
}
