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

package repo

import (
	"context"

	"go.chromium.org/luci/cipd/appengine/impl/perm"
	"go.chromium.org/luci/cipd/appengine/impl/prefixcfg"
)

// newPermissionCache returns a new [perm.Cache] bound to `impl.meta` and the
// current identity and authdb in `ctx`.
func (impl *repoImpl) newPermissionCache(ctx context.Context) *perm.Cache {
	return perm.NewCache(ctx, impl.meta)
}

// prefixPermChecker makes a new [perm.PrefixCache] bound to:
//   - the current service and project configuration for `prefix`
//   - the current identity (in ctx)
//   - the current authdb (in ctx)
func (impl *repoImpl) prefixPermChecker(ctx context.Context, prefix string) (*perm.PrefixCache, error) {
	cfg, err := impl.lookupPrefixCfg(ctx, prefix)
	if err != nil {
		return nil, err
	}
	return impl.newPermissionCache(ctx).ForPrefix(prefix, cfg), nil
}

// hasPermission checks that the current user has all the given `required`
// permissions for `prefix`, or returns an error if not.
//
// This MAY fall back to [perm.HasRole] if legacy fallback is enabled for the
// prefix and the user does not have permission via groups or realms.
//
// In addition, you may provide `optional` permissions which will be checked
// as well.
//
// Returns the resolved [prefixcfg.Entry] for `prefix`, the set of permissions
// (`has`, containing all of `required` plus any `optional` the user has), or
// an error if the caller does not have all `required`.
//
// Callers that also need the inherited PrefixMetadata after checking
// permissions should construct a [perm.PrefixCache] directly via
// [repoImpl.prefixPermChecker], call [perm.PrefixCache.Check], and then call
// [perm.PrefixCache.GetMetadata].
//
// If the error was actually a permission denial, it will be a [status.Errorf]
// formatted error with the code [codes.PermissionDenied]. Otherwise this will
// be a regular Go error (wrapped as [codes.Internal] by
// [grpcutil.GRPCifyAndLogErr]).
func (impl *repoImpl) hasPermission(ctx context.Context, prefix string, required, optional perm.Checkable) (cfg *prefixcfg.Entry, has perm.Set, err error) {
	pchk, err := impl.prefixPermChecker(ctx, prefix)
	if err != nil {
		return nil, nil, err
	}
	has, err = pchk.Check(ctx, required, optional)
	if err != nil {
		return nil, nil, err
	}
	return pchk.Config(), has, nil
}
