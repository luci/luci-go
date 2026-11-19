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

	"go.chromium.org/luci/cipd/appengine/impl/prefixcfg"
)

// PrefixCache evaluates permissions against a single prefix and its resolved
// configuration.
//
// TODO: Track writeAllowed, lazy PrefixMetadata, and checked legacy roles.
type PrefixCache struct {
	pcache *Cache
	prefix string
	cfg    *prefixcfg.Entry
}

// Prefix returns the CIPD package prefix this PrefixCache is bound to.
func (p *PrefixCache) Prefix() string { return p.prefix }

// Config returns the resolved [prefixcfg.Entry] this PrefixCache is bound to.
func (p *PrefixCache) Config() *prefixcfg.Entry { return p.cfg }

// CanDiagnose checks to see if the caller is allowed to diagnose ACL problems.
//
// This is based on [DiagnoseACL].
func (p *PrefixCache) CanDiagnose(ctx context.Context) (bool, error) {
	return p.CheckOne(ctx, DiagnoseACL)
}

// CheckOne checks to see if the current identity has `perm`.
//
// The realm (if any) is implied by the `prefix` that this PrefixCache is
// bound to.
func (p *PrefixCache) CheckOne(ctx context.Context, perm Permission) (has bool, err error) {
	// TODO: Global write policy.

	// TODO: Group.

	if has, err = p.pcache.HasPerm(ctx, p.cfg.Realm, perm); has || err != nil {
		return
	}

	// TODO: Legacy fallback.

	return
}

// Check verifies that the current user has `required` on `p.prefix`.
//
// TODO: Support Set (multiple permissions) and optional permissions.
func (p *PrefixCache) Check(ctx context.Context, required Permission) error {
	if required == (Permission{}) {
		panic("required permission must not be empty")
	}
	has, err := p.CheckOne(ctx, required)
	if err != nil {
		return err
	}
	if !has {
		return (&MissingPerms{pchk: p, perm: required}).AsError(ctx)
	}
	return nil
}
