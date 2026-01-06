// Copyright 2025 The LUCI Authors.
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

package permissions

import (
	"context"

	"go.chromium.org/luci/server/auth/realms"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
)

// WorkUnitAccessChecker checks the access level of work units or work unit-scoped
// resources (e.g. test results, artifacts, exonerations) within a root invocation.
//
// Unlike VerifyWorkUnitsAccess, which also reads the realms of the given work units,
// this checker is optimized for scenarios where the caller has already read the work units
// and knows their realms.
type WorkUnitAccessChecker struct {
	// The permission(s) required on the work unit's realm to upgrade limited
	// access to full access.
	upgradeLimitedToFull []realms.Permission
	// The access user has on the root invocation.
	RootInvovcationAccess AccessLevel
}

// NewWorkUnitAccessChecker creates a new WorkUnitAccessChecker.
func NewWorkUnitAccessChecker(ctx context.Context, rootInvID rootinvocations.ID, opts VerifyWorkUnitAccessOptions) (*WorkUnitAccessChecker, error) {
	rootInvRealm, err := rootinvocations.ReadRealm(ctx, rootInvID)
	if err != nil {
		// If the root invocation is not found, returns NotFound appstatus error.
		return nil, err
	}
	access := NoAccess

	hasRootFullAccess, err := hasAllPermissions(ctx, rootInvRealm, opts.Full...)
	if err != nil {
		return nil, err
	}
	if hasRootFullAccess {
		access = FullAccess
	} else {
		hasRootLimitedAccess, err := hasAllPermissions(ctx, rootInvRealm, opts.Limited...)
		if err != nil {
			return nil, err
		}
		if hasRootLimitedAccess {
			access = LimitedAccess
		}
	}

	return &WorkUnitAccessChecker{
		upgradeLimitedToFull:  opts.UpgradeLimitedToFull,
		RootInvovcationAccess: access,
	}, nil
}

// Check returns the access level for the given work unit realm.
func (c *WorkUnitAccessChecker) Check(ctx context.Context, wuRealm string) (AccessLevel, error) {
	if c.RootInvovcationAccess != LimitedAccess {
		// If root invocation has full access or no access, return it directly.
		return c.RootInvovcationAccess, nil
	}

	// Check if we can upgrade to full access based on the work unit's realm.
	canUpgrade, err := hasAllPermissions(ctx, wuRealm, c.upgradeLimitedToFull...)
	if err != nil {
		return NoAccess, err
	}
	if canUpgrade {
		return FullAccess, nil
	}
	return LimitedAccess, nil
}
