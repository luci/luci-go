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

package pbutil

import (
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/validate"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// PopulateModuleIdentifierHashes computes the module variant hash field on ModuleIdentifier.
// It should only be called on ModuleIdentifier that have been validated (such as those
// retrieved from storage).
func PopulateModuleIdentifierHashes(id *pb.ModuleIdentifier) {
	id.ModuleVariantHash = VariantHash(id.ModuleVariant)
}

// ValidateModuleIdentifierForStorage validates a module identifier
// that is being uploaded for storage.
//
// Unlike ValidateModuleIdentifierForQuery, this method always requires the
// module variant to be set (only the variant hash is not enough).
//
// N.B. This does not validate the module ID against the configured schemes; this
// should also be applied at upload time. (And must not be applied at other times
// to ensure old modules uploaded under old schemes continue to be queryable.)
func ValidateModuleIdentifierForStorage(id *pb.ModuleIdentifier) error {
	if id == nil {
		return validate.Unspecified()
	}
	// Module name
	if err := ValidateModuleName(id.ModuleName); err != nil {
		return errors.Fmt("module_name: %w", err)
	}

	// Module scheme
	isLegacyModule := id.ModuleName == LegacyModuleName
	if err := ValidateModuleScheme(id.ModuleScheme, isLegacyModule); err != nil {
		return errors.Fmt("module_scheme: %w", err)
	}

	// Module variant
	if err := ValidateVariant(id.ModuleVariant); err != nil {
		return errors.Fmt("module_variant: %w", err)
	}
	if id.ModuleVariantHash != "" {
		// If clients set both the hash and the variant, they should be consistent.
		// Clients may set both in upload contexts if they are passing back a
		// module ID retrieved via another query.
		expectedVariantHash := VariantHash(id.ModuleVariant)
		if id.ModuleVariantHash != expectedVariantHash {
			return errors.Fmt("module_variant_hash: expected %s (to match module_variant) or for value to be unset", expectedVariantHash)
		}
	}
	return nil
}

// ValidateModuleIdentifierForQuery validates a module identifier
// is suitable as an input to a query RPC.
//
// Unlike ValidateModuleIdentifierForStorage, this method allows either
// the ModuleVariant or ModuleVariantHash to be specified.
func ValidateModuleIdentifierForQuery(id *pb.ModuleIdentifier) error {
	if id == nil {
		return validate.Unspecified()
	}
	// Module name
	if err := ValidateModuleName(id.ModuleName); err != nil {
		return errors.Fmt("module_name: %w", err)
	}

	// Module scheme
	isLegacyModule := id.ModuleName == LegacyModuleName
	if err := ValidateModuleScheme(id.ModuleScheme, isLegacyModule); err != nil {
		return errors.Fmt("module_scheme: %w", err)
	}

	// Module variant.
	if id.ModuleVariant == nil && id.ModuleVariantHash == "" {
		return errors.New("at least one of module_variant and module_variant_hash must be set")
	}
	if id.ModuleVariant != nil {
		if err := ValidateVariant(id.ModuleVariant); err != nil {
			return errors.Fmt("module_variant: %w", err)
		}
	}
	if id.ModuleVariantHash != "" {
		if err := ValidateVariantHash(id.ModuleVariantHash); err != nil {
			return errors.Fmt("module_variant_hash: %w", err)
		}
		if id.ModuleVariant != nil {
			// If clients set both the hash and the variant, they should be consistent.
			// Clients may commonly set both if they are passing back a structured test ID
			// retrieved via another query.
			expectedVariantHash := VariantHash(id.ModuleVariant)
			if id.ModuleVariantHash != expectedVariantHash {
				return errors.Fmt("module_variant_hash: expected %s (to match module_variant) or for value to be unset", expectedVariantHash)
			}
		}
	}
	return nil
}
