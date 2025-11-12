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
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateDefinitionForStorage(t *testing.T) {
	t.Parallel()
	ftt.Run("ValidateDefinitionForStorage", t, func(t *ftt.Test) {
		def := &pb.RootInvocationDefinition{
			System: "system-name",
			Name:   "definition name",
			Properties: &pb.RootInvocationDefinition_Properties{
				Def: map[string]string{
					"key": "value",
				},
			},
		}

		t.Run("Valid", func(t *ftt.Test) {
			assert.Loosely(t, ValidateDefinitionForStorage(def), should.BeNil)
		})

		t.Run("Nil", func(t *ftt.Test) {
			assert.Loosely(t, ValidateDefinitionForStorage(nil), should.ErrLike("unspecified"))
		})

		t.Run("System", func(t *ftt.Test) {
			t.Run("Missing", func(t *ftt.Test) {
				def.System = ""
				assert.Loosely(t, ValidateDefinitionForStorage(def), should.ErrLike("system: unspecified"))
			})
			t.Run("Invalid format", func(t *ftt.Test) {
				def.System = "Invalid-System"
				assert.Loosely(t, ValidateDefinitionForStorage(def), should.ErrLike("system: does not match"))
			})
			t.Run("Too long", func(t *ftt.Test) {
				def.System = "system-" + strings.Repeat("a", maxRootInvocationDefSystemLength)
				assert.Loosely(t, ValidateDefinitionForStorage(def), should.ErrLike("system: must be at most 64 bytes"))
			})
		})

		t.Run("Name", func(t *ftt.Test) {
			t.Run("Missing", func(t *ftt.Test) {
				def.Name = ""
				assert.Loosely(t, ValidateDefinitionForStorage(def), should.ErrLike("name: unspecified"))
			})
			t.Run("Too long", func(t *ftt.Test) {
				def.Name = strings.Repeat("a", maxRootInvocationDefNameLength+1)
				assert.Loosely(t, ValidateDefinitionForStorage(def), should.ErrLike("name: longer than 256 bytes"))
			})
			t.Run("Not in Unicode Normal Form C", func(t *ftt.Test) {
				def.Name = "e\u0301" // Normal Form C would be \u00e9 (é).
				assert.Loosely(t, ValidateDefinitionForStorage(def), should.ErrLike("name: not in unicode normalized form C"))
			})
			t.Run("Contains non-printables", func(t *ftt.Test) {
				def.Name = "a\nb"
				assert.Loosely(t, ValidateDefinitionForStorage(def), should.ErrLike("name: non-printable rune '\\n' at byte index 1"))
			})
		})

		t.Run("Properties", func(t *ftt.Test) {
			t.Run("Missing", func(t *ftt.Test) {
				def.Properties = nil
				assert.Loosely(t, ValidateDefinitionForStorage(def), should.ErrLike("properties: unspecified"))
			})
			// ValidateDefinitionProperties has its own exhaustive test cases below. Simply check the method is called.
			t.Run("Invalid", func(t *ftt.Test) {
				def.Properties = DefinitionProperties("key with spaces", "value")
				assert.Loosely(t, ValidateDefinitionForStorage(def), should.ErrLike(`properties: def["key with spaces"]: key: does not match`))
			})
		})
	})
}

func TestValidateDefinitionProperties(t *testing.T) {
	t.Parallel()
	ftt.Run("ValidateDefinitionProperties", t, func(t *ftt.Test) {
		props := &pb.RootInvocationDefinition_Properties{
			Def: map[string]string{
				"key": "value",
			},
		}

		t.Run("Valid", func(t *ftt.Test) {
			assert.Loosely(t, ValidateDefinitionProperties(props), should.BeNil)
		})
		t.Run("Nil", func(t *ftt.Test) {
			assert.Loosely(t, ValidateDefinitionProperties(nil), should.ErrLike("unspecified"))
		})

		t.Run("Invalid key", func(t *ftt.Test) {
			props.Def["key with spaces"] = "value"
			assert.Loosely(t, ValidateDefinitionProperties(props), should.ErrLike(`def["key with spaces"]: key: does not match`))
		})
		t.Run("Too long key", func(t *ftt.Test) {
			props.Def[strings.Repeat("a", 65)] = "value"
			assert.Loosely(t, ValidateDefinitionProperties(props), should.ErrLike(`aaaaaaa"]: key: length must be less or equal to 64 bytes`))
		})
		t.Run("Invalid value", func(t *ftt.Test) {
			props.Def["key"] = "a\nb"
			err := ValidateDefinitionProperties(props)
			assert.Loosely(t, err, should.ErrLike(`def["key"]: value: non-printable rune '\n' at byte index 1`))
		})
		t.Run("Too long value", func(t *ftt.Test) {
			props.Def["key"] = strings.Repeat("a", 257)
			assert.Loosely(t, ValidateDefinitionProperties(props), should.ErrLike(`def["key"]: value: length must be less or equal to 256 bytes`))
		})
		t.Run("Too large", func(t *ftt.Test) {
			props.Def = make(map[string]string, 10)
			for i := 0; i < 10; i++ {
				props.Def[fmt.Sprintf("key_%d", i)] = strings.Repeat("a", 100)
			}
			assert.Loosely(t, ValidateDefinitionProperties(props), should.ErrLike("got 1110 bytes; exceeds the maximum size of 1024 bytes"))
		})
	})
}

func TestDefinitionPropertiesHash(t *testing.T) {
	t.Parallel()
	ftt.Run("DefinitionPropertiesHash", t, func(t *ftt.Test) {
		t.Run("nil properties", func(t *ftt.Test) {
			hash := DefinitionPropertiesHash(nil)
			assert.Loosely(t, hash, should.Equal(""))
		})

		t.Run("empty properties", func(t *ftt.Test) {
			hash := DefinitionPropertiesHash(&pb.RootInvocationDefinition_Properties{})
			// The hash of an empty string.
			assert.Loosely(t, hash, should.Equal("e3b0c44298fc1c14"))
		})

		t.Run("one property", func(t *ftt.Test) {
			props := DefinitionProperties("key1", "value1")
			hash := DefinitionPropertiesHash(props)
			assert.Loosely(t, hash, should.Equal("b2f3a94de6cce8c7"))
		})

		t.Run("multiple properties", func(t *ftt.Test) {
			props := DefinitionProperties("key1", "value1", "key2", "value2")
			hash := DefinitionPropertiesHash(props)
			assert.Loosely(t, hash, should.Equal("1a7d399bd89d3c57"))
		})

		t.Run("order does not matter", func(t *ftt.Test) {
			props1 := DefinitionProperties("key1", "value1", "key2", "value2")
			props2 := DefinitionProperties("key2", "value2", "key1", "value1")
			hash1 := DefinitionPropertiesHash(props1)
			hash2 := DefinitionPropertiesHash(props2)
			assert.Loosely(t, hash1, should.Equal(hash2))
			assert.Loosely(t, hash1, should.Equal("1a7d399bd89d3c57"))
		})
	})
}
