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

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/appengine/impl/prefixcfg"
)

// PrefixCache evaluates permissions against a single prefix and its resolved
// configuration, lazily caching legacy PrefixMetadata and role evaluations for
// `prefix`.
type PrefixCache struct {
	pcache       *Cache
	prefix       string
	cfg          *prefixcfg.Entry
	writeAllowed bool

	gotMeta      bool
	metas        []*repopb.PrefixMetadata
	checkedRoles boolCache[repopb.Role]
}

// Prefix returns the CIPD package prefix this PrefixCache is bound to.
func (p *PrefixCache) Prefix() string { return p.prefix }

// Config returns the resolved [prefixcfg.Entry] this PrefixCache is bound to.
func (p *PrefixCache) Config() *prefixcfg.Entry { return p.cfg }

// GetMetadata returns the inherited PrefixMetadata for `p.prefix`, fetching
// and caching it on first call.
func (p *PrefixCache) GetMetadata(ctx context.Context) ([]*repopb.PrefixMetadata, error) {
	if !p.gotMeta {
		metas, err := p.pcache.metaSvc.GetMetadata(ctx, p.prefix)
		if err != nil {
			return nil, err
		}
		p.metas = metas
		p.gotMeta = true
	}
	return p.metas, nil
}

// HasRole checks if the caller has the given legacy role on `p.prefix`,
// exploiting role implication (OWNER => WRITER => READER) across cached
// evaluations.
func (p *PrefixCache) HasRole(ctx context.Context, role repopb.Role) (bool, error) {
	if role <= repopb.Role_ROLE_UNSPECIFIED || role > repopb.Role_OWNER {
		return false, nil
	}

	for cachedRole, cachedHas := range p.checkedRoles {
		if cachedHas && cachedRole >= role {
			return true, nil
		}
		if !cachedHas && cachedRole <= role {
			return false, nil
		}
	}

	return p.checkedRoles.getOrCompute(role, func() (bool, error) {
		metas, err := p.GetMetadata(ctx)
		if err != nil {
			return false, err
		}
		ok, _, err := HasRole(ctx, p.cfg, metas, role)
		return ok, err
	})
}

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

	if p.cfg.Service.EnableLegacyAclFallback {
		has, err = p.HasRole(ctx, perm.core.role)
		if has && p.cfg.Realm != "" {
			// Only log this if there actually was a Realm we checked against
			// above, otherwise as we start rolling this out this will generate
			// incredible amounts of spam.
			logging.Debugf(ctx, "PrefixCache.CheckOne(prefix=%q, perm=%s): realm=%q: granted via legacy fallback", p.prefix, perm, p.cfg.Realm)
		}
	}

	return
}

// CheckSet checks all permissions in `perms` against global write policy,
// group membership, realm permissions, and (if enabled) legacy role fallback.
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
