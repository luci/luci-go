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
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateModuleIdentifierForStorage(t *testing.T) {
	t.Parallel()
	ftt.Run("ValidateModuleIdentifierForStorage", t, func(t *ftt.Test) {
		id := &pb.ModuleIdentifier{
			ModuleName:   "module",
			ModuleScheme: "scheme",
			ModuleVariant: &pb.Variant{
				Def: map[string]string{
					"k": "v",
				},
			},
		}

		t.Run("Valid", func(t *ftt.Test) {
			assert.Loosely(t, ValidateModuleIdentifierForStorage(id), should.BeNil)
		})
		t.Run("Nil", func(t *ftt.Test) {
			assert.Loosely(t, ValidateModuleIdentifierForStorage(nil), should.ErrLike("unspecified"))
		})
		t.Run("Module name", func(t *ftt.Test) {
			// The implementation calls into ValidateModuleName. That already has its own tests, so
			// just verify it is called.
			id.ModuleName = ""
			assert.Loosely(t, ValidateModuleIdentifierForStorage(id), should.ErrLike("module_name: unspecified"))
		})
		t.Run("Module scheme", func(t *ftt.Test) {
			t.Run("Empty", func(t *ftt.Test) {
				// The implementation calls into ValidateModule. That already has its own tests, so
				// just verify it is called.
				id.ModuleScheme = ""
				assert.Loosely(t, ValidateModuleIdentifierForStorage(id), should.ErrLike("module_scheme: unspecified"))
			})
			t.Run("Legacy is reserved", func(t *ftt.Test) {
				id.ModuleName = "module"
				id.ModuleScheme = "legacy"
				assert.Loosely(t, ValidateModuleIdentifierForStorage(id), should.ErrLike(`module_scheme: must not be set to "legacy" except in the "legacy" module`))
			})
			t.Run("Legacy module requires legacy scheme", func(t *ftt.Test) {
				id.ModuleName = "legacy"
				id.ModuleScheme = "scheme"
				assert.Loosely(t, ValidateModuleIdentifierForStorage(id), should.ErrLike(`module_scheme: must be set to "legacy" in the "legacy" module`))
			})
		})
		t.Run("Module Variant", func(t *ftt.Test) {
			t.Run("Empty", func(t *ftt.Test) {
				id.ModuleVariant = nil
				assert.Loosely(t, ValidateModuleIdentifierForStorage(id), should.ErrLike("module_variant: unspecified"))
			})
			t.Run("Invalid", func(t *ftt.Test) {
				id.ModuleVariant = &pb.Variant{
					Def: map[string]string{
						"\x00": "v",
					},
				}
				assert.Loosely(t, ValidateModuleIdentifierForStorage(id), should.ErrLike("module_variant: \"\\x00\":\"v\": key: does not match pattern"))
			})
		})
		t.Run("Module Variant Hash", func(t *ftt.Test) {
			t.Run("Unset", func(t *ftt.Test) {
				id.ModuleVariantHash = ""
				assert.Loosely(t, ValidateModuleIdentifierForStorage(id), should.BeNil)
			})
			t.Run("Set and matches", func(t *ftt.Test) {
				id.ModuleVariant = &pb.Variant{
					Def: map[string]string{
						"k": "v",
					},
				}
				id.ModuleVariantHash = "b1618cc2bf370a7c"
				assert.Loosely(t, ValidateModuleIdentifierForStorage(id), should.BeNil)
			})
			t.Run("Set and mismatches", func(t *ftt.Test) {
				id.ModuleVariant = &pb.Variant{
					Def: map[string]string{
						"k": "v",
					},
				}
				id.ModuleVariantHash = "blah"
				assert.Loosely(t, ValidateModuleIdentifierForStorage(id), should.ErrLike("module_variant_hash: expected b1618cc2bf370a7c (to match module_variant) or for value to be unset"))
			})
		})
	})
}

func TestValidateModuleIdentifierForQuery(t *testing.T) {
	t.Parallel()
	ftt.Run("ValidateModuleIdentifierForQuery", t, func(t *ftt.Test) {
		id := &pb.ModuleIdentifier{
			ModuleName:   "module",
			ModuleScheme: "scheme",
			ModuleVariant: &pb.Variant{
				Def: map[string]string{
					"k": "v",
				},
			},
		}

		t.Run("Valid", func(t *ftt.Test) {
			assert.Loosely(t, ValidateModuleIdentifierForQuery(id), should.BeNil)
		})
		t.Run("Nil", func(t *ftt.Test) {
			assert.Loosely(t, ValidateModuleIdentifierForQuery(nil), should.ErrLike("unspecified"))
		})
		t.Run("Module name", func(t *ftt.Test) {
			// The implementation calls into ValidateModuleName. That already has its own tests, so
			// just verify it is called.
			id.ModuleName = ""
			assert.Loosely(t, ValidateModuleIdentifierForQuery(id), should.ErrLike("module_name: unspecified"))
		})
		t.Run("Module scheme", func(t *ftt.Test) {
			t.Run("Empty", func(t *ftt.Test) {
				// The implementation calls into ValidateModule. That already has its own tests, so
				// just verify it is called.
				id.ModuleScheme = ""
				assert.Loosely(t, ValidateModuleIdentifierForQuery(id), should.ErrLike("module_scheme: unspecified"))
			})
			t.Run("Legacy is reserved", func(t *ftt.Test) {
				id.ModuleName = "module"
				id.ModuleScheme = "legacy"
				assert.Loosely(t, ValidateModuleIdentifierForQuery(id), should.ErrLike(`module_scheme: must not be set to "legacy" except in the "legacy" module`))
			})
			t.Run("Legacy module requires legacy scheme", func(t *ftt.Test) {
				id.ModuleName = "legacy"
				id.ModuleScheme = "scheme"
				assert.Loosely(t, ValidateModuleIdentifierForQuery(id), should.ErrLike(`module_scheme: must be set to "legacy" in the "legacy" module`))
			})
		})
		t.Run("Module variant and hash unset", func(t *ftt.Test) {
			id.ModuleVariant = nil
			id.ModuleVariantHash = ""
			assert.Loosely(t, ValidateModuleIdentifierForQuery(id), should.ErrLike("at least one of module_variant and module_variant_hash must be set"))
		})
		t.Run("Module Variant", func(t *ftt.Test) {
			t.Run("Unset with hash set", func(t *ftt.Test) {
				id.ModuleVariant = nil
				id.ModuleVariantHash = "b1618cc2bf370a7c"
				assert.Loosely(t, ValidateModuleIdentifierForQuery(id), should.BeNil)
			})
			t.Run("Invalid", func(t *ftt.Test) {
				id.ModuleVariant = &pb.Variant{
					Def: map[string]string{
						"\x00": "v",
					},
				}
				assert.Loosely(t, ValidateModuleIdentifierForQuery(id), should.ErrLike("module_variant: \"\\x00\":\"v\": key: does not match pattern"))
			})
		})
		t.Run("Module Variant Hash", func(t *ftt.Test) {
			t.Run("Invalid", func(t *ftt.Test) {
				id.ModuleVariantHash = "blah"
				assert.Loosely(t, ValidateModuleIdentifierForQuery(id), should.ErrLike("module_variant_hash: variant hash blah must match ^[0-9a-f]{16}$"))
			})
			t.Run("Unset with variant set", func(t *ftt.Test) {
				id.ModuleVariantHash = ""
				id.ModuleVariant = &pb.Variant{}
				assert.Loosely(t, ValidateModuleIdentifierForQuery(id), should.BeNil)
			})
			t.Run("Set and matches", func(t *ftt.Test) {
				id.ModuleVariant = &pb.Variant{
					Def: map[string]string{
						"k": "v",
					},
				}
				id.ModuleVariantHash = "b1618cc2bf370a7c"
				assert.Loosely(t, ValidateModuleIdentifierForQuery(id), should.BeNil)
			})
			t.Run("Set and mismatches", func(t *ftt.Test) {
				id.ModuleVariant = &pb.Variant{
					Def: map[string]string{
						"k": "v",
					},
				}
				id.ModuleVariantHash = "00008cc2bf370000"
				assert.Loosely(t, ValidateModuleIdentifierForQuery(id), should.ErrLike("module_variant_hash: expected b1618cc2bf370a7c (to match module_variant) or for value to be unset"))
			})
		})
	})
}
