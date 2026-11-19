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

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MissingPerms is an error helper for formatting a collection of missing
// permissions.
//
// This understands how to check if the user is allowed to see a detailed error
// message, or only a simple permission denied error.
type MissingPerms struct {
	pchk *PrefixCache

	// TODO: Generalize to Set for multi-permission checks.
	perm Permission
}

// Empty reports whether no permissions were missing.
func (m *MissingPerms) Empty() bool {
	return m.perm == (Permission{})
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

	// TODO: Report global write policy rejection and missing legacy role details.

	return status.Errorf(
		codes.PermissionDenied, "%q is missing the %s permission in prefix %q (realm %q)",
		m.pchk.pcache.ident, m.perm, m.pchk.prefix, m.pchk.cfg.Realm)
}
