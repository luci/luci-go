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
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"

	"go.chromium.org/luci/server/auth/realms"
)

// Checkable is an interface which allows methods to accept either a [Set] or a
// single [Permission].
type Checkable interface {
	fmt.Stringer
	Perms() Set
}

// corePermission is the core aspects of a Permission which don't change
// with binding.
//
// TODO: Support implied groups, fake permissions, global write policy, and
// legacy roles.
type corePermission struct {
	// perm is the realms permission for this Permission.
	perm realms.Permission
}

// Grantable is the common interface between [Permission] and [Bindable].
//
// It also implements [Checkable] (which, for [Bindable] permissions, always
// checks the [Bindable.Unconditional] version of the permission).
type Grantable interface {
	Checkable

	Perm() realms.Permission
}

func (c *corePermission) String() string {
	if c == nil {
		return "<invalid>"
	}
	return c.perm.String()
}

func (c *corePermission) Perms() Set {
	return SetOf(Permission{core: c})
}

func (c *corePermission) Perm() realms.Permission {
	if c == nil {
		return realms.Permission{}
	}
	return c.perm
}

// Permission represents a single registered permission and sufficient context
// to check if a given identity+realm has this permission or not.
//
// If the underlying permission requires binding, this Permission includes
// the bound key+value as well.
//
// TODO: Support conditional attribute bindings, implied groups, fake
// permissions, global write policy, and legacy roles.
type Permission struct {
	// core is the immutable parts of this Permission.
	core *corePermission
}

// Perm returns the underlying [realms.Permission].
func (c Permission) Perm() realms.Permission {
	return c.core.Perm()
}

// Perms implements [Checkable].
func (c Permission) Perms() Set { return SetOf(c) }

func (c Permission) String() string {
	if c.core == nil {
		return "<invalid>"
	}
	return c.core.perm.Name()
}

// definePerm registers `permission` as a realms permission, and returns a new
// [Permission].
func definePerm(permission string) Permission {
	// NOTE: We have a test which asserts that no duplicates appear in
	// AllPermissions. Since `definePerm` is private, this should guarantee that
	// all *corePermission in the process are also unique.
	core := &corePermission{realms.RegisterPermission(permission)}
	AllPermissions[core] = struct{}{}
	return Permission{core: core}
}

// Set is a set of [Permission]s and implements [Checkable].
//
// When using Set with e.g. [PrefixCache.Check], the caller must have *all*
// permissions in this set to pass.
type Set map[Permission]struct{}

// SetOf is a convenience wrapper to turn multiple Permissions into a [Set].
func SetOf(permissions ...Permission) Set {
	ret := make(Set, len(permissions))
	for _, p := range permissions {
		ret.Add(p)
	}
	return ret
}

// Perms implements [Checkable].
func (p Set) Perms() Set { return p }

func (p *Set) Add(c Permission) {
	if c.core == nil {
		return
	}
	if *p == nil {
		*p = Set{c: {}}
	} else {
		(*p)[c] = struct{}{}
	}
}

func (p Set) Has(c Permission) bool {
	_, ret := p[c]
	return ret
}

// Sorted yields all permissions in this Set in sorted order.
func (p Set) Sorted(yield func(Permission) bool) {
	slices.Values(slices.SortedFunc(maps.Keys(p), func(a, b Permission) int {
		return cmp.Compare(a.core.perm.Name(), b.core.perm.Name())
	}))(yield)
}

func (p Set) String() string {
	if len(p) == 0 {
		return "<empty>"
	}
	if len(p) == 1 {
		for p := range p {
			return p.String()
		}
	}

	var ret strings.Builder
	ret.WriteString("[")
	first := true
	for p := range p.Sorted {
		if !first {
			ret.WriteString(",")
		}
		first = false
		ret.WriteString(p.String())
	}
	ret.WriteString("]")
	return ret.String()
}
