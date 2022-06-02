// Copyright 2020 The LUCI Authors.
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

package realms

import (
	"sort"
	"strings"
	"sync"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
)

var (
	mu    sync.RWMutex
	perms = stringset.New(0)
)

// Permission is a symbol that has form "<service>.<subject>.<verb>", which
// describes some elementary action ("<verb>") that can be done to some category
// of resources ("<subject>"), managed by some particular kind of LUCI service
// ("<service>").
//
// Each individual LUCI service should document what permissions it checks and
// when. It becomes a part of service's public API. Usually services should
// check only permissions of resources they own (e.g. "<service>.<subject>.*"),
// but in exceptional cases they may also check permissions intended for other
// services. This is primarily useful for services that somehow "proxy" access
// to resources.
type Permission struct {
	name string
}

// Name is "<service>.<subject>.<verb>" string.
func (p Permission) Name() string {
	return p.name
}

// String allows permissions to be used as "%s" in format strings.
func (p Permission) String() string {
	return p.name
}

// clearPermissions removes all registered permissions (for tests).
func clearPermissions() {
	mu.Lock()
	perms = stringset.New(0)
	mu.Unlock()
}

// RegisterPermission adds a new permission with the given name to the process
// registry or returns an existing one.
//
// Panics if the permission name doesn't look like "<service>.<subject>.<verb>".
//
// Intended to be called during init() time, but may be called later too.
func RegisterPermission(name string) Permission {
	mu.Lock()
	defer mu.Unlock()
	if err := ValidatePermissionName(name); err != nil {
		panic(err)
	}
	perms.Add(name)
	return Permission{name: name}
}

// GetPermissions returns the permissions with the matching names. The order of
// the permission is the same as the provided names.
//
// Returns an error if any of the permission isn't registered.
func GetPermissions(names ...string) ([]Permission, error) {
	mu.RLock()
	var err error
	for _, name := range names {
		if !perms.Has(name) {
			err = errors.Reason("permission not registered: %q", name).Err()
			break
		}
	}
	mu.RUnlock()

	if err != nil {
		return nil, err
	}

	perms := make([]Permission, 0, len(names))
	for _, name := range names {
		perms = append(perms, Permission{name})
	}
	return perms, nil
}

// RegisteredPermissions returns a snapshot of all registered permissions.
//
// Permissions in the slice are ordered by their name.
func RegisteredPermissions() []Permission {
	mu.RLock()
	all := make([]Permission, 0, perms.Len())
	perms.Iter(func(name string) bool {
		all = append(all, Permission{name: name})
		return true
	})
	mu.RUnlock()
	sort.Slice(all, func(i, j int) bool { return all[i].name < all[j].name })
	return all
}

// ValidatePermissionName returns an error if the permission name is invalid.
//
// It checks the name looks like "<service>.<subject>.<verb>".
func ValidatePermissionName(name string) error {
	if parts := strings.Split(name, "."); len(parts) == 3 {
		good := true
		for _, p := range parts {
			if p == "" {
				good = false
				break
			}
		}
		if good {
			return nil
		}
	}
	return errors.Reason("bad permission %q - must have form <service>.<subject>.<verb>", name).Err()
}
