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
	"strings"
	"sync"

	"go.chromium.org/luci/common/errors"
)

var (
	mu            sync.RWMutex
	perms         = map[string]PermissionFlags{}
	forbidChanges bool
)

// PermissionFlags describe how a registered permission is going to be used in
// the current process.
//
// These are purely process-local flags. It is possible for the same single
// permission to be defined with different flags in different processes.
type PermissionFlags uint8

const (
	// UsedInQueryRealms indicates the permission will be used with QueryRealms.
	//
	// It instructs the runtime to build an in-memory index to speed up the query.
	// This flag is necessary for QueryRealms function to work. It implies some
	// performance and memory costs and should be used only with permissions that
	// are going to be passed to QueryRealms.
	UsedInQueryRealms PermissionFlags = 1 << iota
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

// AddFlags appends given flags to a registered permission.
//
// Permissions are usually defined in shared packages used by multiple services.
// Flags that makes sense for one service do not always make sense for another.
// For that reason RegisterPermission and AddFlags are two separate functions.
//
// Must to be called during startup, e.g. in init(). Panics if called when
// the process is already serving requests.
func (p Permission) AddFlags(flags PermissionFlags) {
	mu.Lock()
	defer mu.Unlock()
	if forbidChanges {
		panic("cannot call Permission.AddFlags while already serving requests, do it before starting the serving loop, e.g. in init()")
	}
	perms[p.name] |= flags
}

// clearPermissions removes all registered permissions (for tests).
func clearPermissions() {
	mu.Lock()
	perms = map[string]PermissionFlags{}
	forbidChanges = false
	mu.Unlock()
}

// RegisterPermission adds a new permission with the given name to the process
// registry or returns an existing one.
//
// Panics if the permission name doesn't look like "<service>.<subject>.<verb>".
//
// Must to be called during startup, e.g. in init(). Panics if called when
// the process is already serving requests.
func RegisterPermission(name string) Permission {
	mu.Lock()
	defer mu.Unlock()
	if forbidChanges {
		panic("cannot call RegisterPermission while already serving requests, do it before starting the serving loop, e.g. in init()")
	}
	if err := ValidatePermissionName(name); err != nil {
		panic(err)
	}
	perms[name] |= 0
	return Permission{name: name}
}

// GetPermissions returns the permissions with the matching names. The order of
// the permission is the same as the provided names.
//
// Implicitly calls ForbidPermissionChanges to make sure no new permissions
// are added later.
//
// Returns an error if any of the permission isn't registered.
func GetPermissions(names ...string) ([]Permission, error) {
	mu.RLock()
	var err error
	for _, name := range names {
		if _, ok := perms[name]; !ok {
			err = errors.Fmt("permission not registered: %q", name)
			break
		}
	}
	forbiddenAlready := forbidChanges
	mu.RUnlock()

	if err != nil {
		return nil, err
	}

	if !forbiddenAlready {
		mu.Lock()
		forbidChanges = true
		mu.Unlock()
	}

	perms := make([]Permission, 0, len(names))
	for _, name := range names {
		perms = append(perms, Permission{name})
	}
	return perms, nil
}

// RegisteredPermissions returns a snapshot of all registered permissions along
// with their flags.
//
// Implicitly calls ForbidPermissionChanges to make sure no new permissions
// are added later.
func RegisteredPermissions() map[Permission]PermissionFlags {
	mu.RLock()
	all := make(map[Permission]PermissionFlags, len(perms))
	for name, flags := range perms {
		all[Permission{name: name}] = flags
	}
	forbiddenAlready := forbidChanges
	mu.RUnlock()

	if !forbiddenAlready {
		mu.Lock()
		forbidChanges = true
		mu.Unlock()
	}

	return all
}

// ForbidPermissionChanges explicitly forbids registering new permissions or
// changing their flags.
//
// All permissions should be registered before the server starts running its
// loop. The runtime relies on this when building various caches. After this
// function is called, RegisterPermissions and AddFlags would start panicking.
//
// Intended for internal server code.
func ForbidPermissionChanges() {
	mu.Lock()
	forbidChanges = true
	mu.Unlock()
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
	return errors.Fmt("bad permission %q - must have form <service>.<subject>.<verb>", name)
}
