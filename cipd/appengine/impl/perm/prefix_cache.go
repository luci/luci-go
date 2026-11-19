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
	"maps"

	"go.chromium.org/luci/cipd/appengine/impl/prefixcfg"
)

// PrefixCache evaluates permissions against a single prefix and its resolved
// configuration.
//
// TODO: Track lazy PrefixMetadata and checked legacy roles.
type PrefixCache struct {
	pcache       *Cache
	prefix       string
	cfg          *prefixcfg.Entry
	writeAllowed bool
}

// Prefix returns the CIPD package prefix this PrefixCache is bound to.
func (p *PrefixCache) Prefix() string { return p.prefix }

// Config returns the resolved [prefixcfg.Entry] this PrefixCache is bound to.
func (p *PrefixCache) Config() *prefixcfg.Entry { return p.cfg }

// CanDiagnose checks to see if the caller is allowed to diagnose ACL problems.
//
// This is based on [DiagnoseACL].
func (p *PrefixCache) CanDiagnose(ctx context.Context) (bool, error) {
	has, _, err := p.CheckOne(ctx, DiagnoseACL)
	return has, err
}

// CheckOne checks to see if the current identity has `perm`.
//
// The realm (if any) is implied by the `prefix` that this PrefixCache is
// bound to.
func (p *PrefixCache) CheckOne(ctx context.Context, perm Permission) (has, policyReject bool, err error) {
	if perm.core.isWrite && !p.writeAllowed {
		return false, true, nil
	}

	if has, err = p.pcache.HasPerm(ctx, p.cfg.Realm, perm); has || err != nil {
		return
	}

	// TODO: Legacy fallback.

	return
}

// CheckSet checks all permissions in `perms` against global write policy,
// group membership, and realm permissions.
//
// If any permissions are missing, they are returned in `missing`.
func (p *PrefixCache) CheckSet(ctx context.Context, perms Set) (*MissingPerms, error) {
	ret := &MissingPerms{pchk: p}
	for perm := range perms {
		has, policyReject, err := p.CheckOne(ctx, perm)
		if err != nil {
			return nil, err
		}
		if !has {
			if policyReject {
				ret.deniedByPolicy.Add(perm)
			} else {
				ret.deniedByACL.Add(perm)
			}
		}
	}
	return ret, nil
}

// Check verifies that the current user has all `required` permissions on
// `p.prefix`, and evaluates any `optional` permissions if `required` succeeds.
//
// Panics if `required` is non-nil but resolves to an empty [Set] (pass untyped
// `nil` when no required permissions are needed).
func (p *PrefixCache) Check(ctx context.Context, required, optional Checkable) (has Set, err error) {
	if required != nil {
		allRequired := required.Perms()
		if len(allRequired) == 0 {
			panic("required Checkable must not resolve to an empty Set (pass untyped nil if no permissions are required)")
		}
		var missingRequired *MissingPerms
		if missingRequired, err = p.CheckSet(ctx, allRequired); err != nil {
			return nil, err
		}
		if err = missingRequired.AsError(ctx); err != nil {
			return nil, err
		}
		has = maps.Clone(allRequired)
	}

	if optional != nil {
		if allOptional := optional.Perms(); len(allOptional) > 0 {
			var missingOptional *MissingPerms
			if missingOptional, err = p.CheckSet(ctx, allOptional); err != nil {
				return nil, err
			}
			for perm := range allOptional {
				if !missingOptional.Has(perm) {
					has.Add(perm)
				}
			}
		}
	}

	return has, nil
}
