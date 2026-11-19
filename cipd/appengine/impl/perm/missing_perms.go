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
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/cipd/api/cipd/v1/repopb"
)

// MissingPerms is an error helper for formatting a collection of missing
// permissions.
//
// This understands how to check if the user is allowed to see a detailed error
// message, or only a simple permission denied error.
type MissingPerms struct {
	pchk *PrefixCache

	deniedByPolicy Set
	deniedByACL    Set
}

// Empty reports whether no permissions were missing.
func (m *MissingPerms) Empty() bool {
	return len(m.deniedByPolicy) == 0 && len(m.deniedByACL) == 0
}

// Has reports whether `c` is in the missing set.
func (m *MissingPerms) Has(c Permission) bool {
	return m.deniedByPolicy.Has(c) || m.deniedByACL.Has(c)
}

// AsError formats the missing permission(s) as a gRPC PermissionDenied error
// (or returns nil if none are missing).
func (m *MissingPerms) AsError(ctx context.Context) error {
	if m.Empty() {
		return nil
	}

	canDiag, err := m.pchk.CanDiagnose(ctx)
	if err != nil {
		return err
	}
	if !canDiag {
		// Return a vanilla PermissionDenied error.
		return NoAccessErr(ctx, m.pchk.prefix)
	}

	toReport := m.deniedByACL
	var detailSuffix string
	if len(m.deniedByPolicy) > 0 {
		toReport = m.deniedByPolicy
		detailSuffix = ": rejected by global policy"
	} else if m.pchk.cfg.Service.EnableLegacyAclFallback {
		var maxRole repopb.Role
		for perm := range toReport {
			maxRole = max(maxRole, perm.core.role)
		}
		if maxRole != repopb.Role_ROLE_UNSPECIFIED {
			detailSuffix = fmt.Sprintf(", and also does not have role: %s", maxRole)
		}
	}

	plural := ""
	if len(toReport) > 1 {
		plural = "s"
	}
	return status.Errorf(
		codes.PermissionDenied, "%q is missing the %s permission%s in prefix %q (realm %q)%s",
		m.pchk.pcache.ident, toReport, plural, m.pchk.prefix, m.pchk.cfg.Realm, detailSuffix)
}
