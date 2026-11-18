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
	"go.chromium.org/luci/server/auth/realms"
)

// Permission represents a single registered permission and sufficient context
// to check if a given identity+realm has this permission or not.
//
// TODO: Support conditional attribute bindings, implied groups, fake
// permissions, global write policy, and legacy roles.
type Permission struct {
	// perm is the realms permission for this Permission.
	perm realms.Permission
}

// Perm returns the underlying [realms.Permission].
func (c Permission) Perm() realms.Permission { return c.perm }

func (c Permission) String() string {
	return c.perm.Name()
}

// definePerm registers `permission` as a realms permission, and returns a new
// [Permission].
func definePerm(permission string) Permission {
	return Permission{perm: realms.RegisterPermission(permission)}
}
