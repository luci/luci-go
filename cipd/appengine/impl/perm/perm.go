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

	"go.chromium.org/luci/cipd/api/cipd/v1/repopb"
)

// Checkable is an interface which allows methods to accept either a [Set] or a
// single [Permission].
type Checkable interface {
	fmt.Stringer
	Perms() Set
}

// corePermission is the core aspects of a Permission which don't change
// with binding.
type corePermission struct {
	// perm is the realms permission for this Permission.
	perm realms.Permission

	// groupOnly indicates that this permission is NOT actually grantable via
	// LUCI Realms, but is *only* grantable via `group`.
	//
	// We use this for `cipd.packages.delete` just for internal uniformity (so
	// that the repo implementation can use [DeletePackage] instead of having to
	// 'know' that this permission check is special).
	groupOnly bool

	// isWrite indicates that this permission mutates state and is subject to the
	// prefix's AllowWritersFromRegexp policy.
	//
	// If role is set, this implies that role is >= WRITER, but role >= WRITER
	// does NOT imply this (e.g. "cipd.packages.readLegacyRoles" is implied by
	// OWNER, but is not a write operation).
	isWrite bool

	// role is the smallest legacy role which implies this permission. If the
	// caller has this role, and legacy ACLs are enabled for the prefix in which
	// this is being checked, then they have this permission.
	role repopb.Role

	// group is a CRIA group which implies this permission. If the caller is in
	// this group, then they have this permission.
	group string

	// attrKey is the attribute key of this corePermission (if any).
	//
	// Attrs in use by CIPD are:
	//   * `cipd.instance.metadataKey`
	//   * `cipd.instance.tagKey`
	//   * `cipd.package.refName`
	attrKey string
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
type Permission struct {
	// core is the immutable parts of this Permission.
	core *corePermission

	// attrVal is the bound value for this Permission.
	//
	// If this is empty, it means that there are no attributes for the permission
	// check, and the permission must be granted unconditionally.
	attrVal string
}

// Perm returns the underlying [realms.Permission].
func (c Permission) Perm() realms.Permission {
	return c.core.Perm()
}

// IsGroupOnly reports whether this permission is only grantable via
// [Permission.Group] (and not via LUCI Realms).
func (c Permission) IsGroupOnly() bool { return c.core.groupOnly }

// IsWrite reports whether this permission mutates state and is subject to
// AllowWritersFromRegexp.
func (c Permission) IsWrite() bool { return c.core.isWrite }

// Role returns the smallest legacy role which implies this permission, or
// [repopb.Role_ROLE_UNSPECIFIED].
func (c Permission) Role() repopb.Role { return c.core.role }

// Group returns the CRIA group whose membership implies this permission, or "".
func (c Permission) Group() string { return c.core.group }

// Perms implements [Checkable].
func (c Permission) Perms() Set { return SetOf(c) }

func (c Permission) String() string {
	if c.core == nil {
		return "<invalid>"
	}
	if c.attrVal == "" || c.core.attrKey == "" {
		return c.core.perm.Name()
	}
	return fmt.Sprintf("%s{%s:%q}", c.core.perm.Name(), c.core.attrKey, c.attrVal)
}

func (c Permission) attrs() realms.Attrs {
	if c.attrVal == "" || c.core.attrKey == "" {
		return nil
	}
	return realms.Attrs{c.core.attrKey: c.attrVal}
}

// withAttr modifies c.core.attrKey, and returns a [Bindable] which can be
// bound to produce a [Permission].
//
// Panics if used twice with the same [corePermission].
func (c Permission) withAttr(attrKey string) Bindable {
	if c.core.attrKey != "" {
		panic("impossible: cannot use `withAttr` twice on the same Permission")
	}
	c.core.attrKey = attrKey
	return Bindable{c.core}
}

// withRole modifies c.core.role, and returns c.
//
// Panics if used twice with the same [corePermission].
func (c Permission) withRole(role repopb.Role) Permission {
	if c.core.role != 0 {
		panic("impossible: cannot use `withRole` twice on the same Permission")
	}
	c.core.role = role
	return c
}

// withGroup modifies c.core.group, and returns c.
//
// Panics if used twice with the same [corePermission].
func (c Permission) withGroup(group string) Permission {
	if c.core.group != "" {
		panic("impossible: cannot use `withGroup` twice on the same Permission")
	}
	c.core.group = group
	return c
}

// groupOnly modifies c.core.groupOnly and c.core.group, and returns c.
//
// Panics if used twice with the same [corePermission].
func (c Permission) groupOnly(group string) Permission {
	if c.core.groupOnly {
		panic("impossible: cannot use `groupOnly` twice on the same Permission")
	}
	c = c.withGroup(group)
	c.core.groupOnly = true
	return c
}

// asWrite modifies c.core.isWrite, and returns c.
//
// Panics if used twice with the same [corePermission].
func (c Permission) asWrite() Permission {
	if c.core.isWrite {
		panic("impossible: cannot use `asWrite` twice on the same Permission")
	}
	c.core.isWrite = true
	return c
}

// definePerm registers `permission` as a realms permission, and returns a new
// [Permission].
func definePerm(permission string) Permission {
	// NOTE: We have a test which asserts that no duplicates appear in
	// AllPermissions. Since `definePerm` is private, this should guarantee that
	// all *corePermission in the process are also unique.
	core := &corePermission{perm: realms.RegisterPermission(permission)}
	AllPermissions[core] = struct{}{}
	return Permission{core: core}
}

// Bindable is a 'half bound' permission which knows the attribute key, but not
// the attribute value.
//
// They are used as the base permission definitions for permissions which
// require an attribute.
//
// This intentionally does *not* implement [Checkable] in order to have the
// compiler reject accidental attempts to use such half bound permissions with
// [PrefixCache.Check].
type Bindable struct {
	core *corePermission
}

// Perm returns the underlying [realms.Permission].
func (b Bindable) Perm() realms.Permission { return b.core.Perm() }

// Bind returns a new [Permission], binding the internally saved attrKey to
// the given attrValue.
//
// Does not modify `b`.
func (b Bindable) Bind(attrValue string) Permission {
	return Permission{b.core, attrValue}
}

// Unconditional returns a new [Permission] which has NO bound value, and which
// will be checked as an unconditionally granted permission.
func (b Bindable) Unconditional() Permission {
	return Permission{b.core, ""}
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
		return cmp.Or(
			cmp.Compare(a.core.perm.Name(), b.core.perm.Name()),
			cmp.Compare(a.attrVal, b.attrVal),
		)
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
