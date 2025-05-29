// Copyright 2024 The LUCI Authors.
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

package model

import (
	"context"
	"fmt"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authdb"
	"go.chromium.org/luci/server/auth/service/protocol"

	customerrors "go.chromium.org/luci/auth_service/impl/errors"
)

// validateAuthDB returns nil if AuthDB is valid.
func validateAuthDB(ctx context.Context, db *protocol.AuthDB) error {
	// Do surface-level validation of groups first, because it's cheap.
	for _, g := range db.Groups {
		if err := validateAuthGroup(ctx, g); err != nil {
			return err
		}
	}

	if err := authdb.ValidateAuthDB(db); err != nil {
		return errors.Fmt("invalid AuthDB: %w", err)
	}

	return nil
}

// validateAuthGroup does a cursory validation for the given AuthGroup. It
// returns nil if AuthGroup has valid values for the following fields:
//   - Name;
//   - CreatedBy; and
//   - ModifiedBy.
func validateAuthGroup(ctx context.Context, group *protocol.AuthGroup) error {
	if !auth.IsValidGroupName(group.Name) {
		return fmt.Errorf("%w for AuthGroup: %q",
			customerrors.ErrInvalidName, group.Name)
	}

	if _, err := identity.MakeIdentity(group.CreatedBy); err != nil {
		return fmt.Errorf("%w %q in AuthGroup %q's CreatedBy - %w",
			customerrors.ErrInvalidIdentity, group.CreatedBy, group.Name, err)
	}

	if _, err := identity.MakeIdentity(group.ModifiedBy); err != nil {
		return fmt.Errorf("%w %q in AuthGroup %q's ModifiedBy - %w",
			customerrors.ErrInvalidIdentity, group.ModifiedBy, group.Name, err)
	}

	return nil
}
