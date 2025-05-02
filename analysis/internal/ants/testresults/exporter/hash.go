// Copyright 2025 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package exporter

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"hash"
	"sort"
	"strings"
	"unicode/utf16"

	bqpb "go.chromium.org/luci/analysis/proto/bq/legacy"
	"go.chromium.org/luci/common/errors"
)

// putInt writes a 32-bit integer (like Java's int) to the hasher in LittleEndian order.
func putInt(h hash.Hash, val int) error {
	if err := binary.Write(h, binary.LittleEndian, int32(val)); err != nil {
		return fmt.Errorf("write int32 to hasher: %w", err)
	}
	return nil
}

// putString writes a string to the hasher, mimicking Guava's behavior:
// each char is written as 2 bytes (UTF-16), LittleEndian.
func putString(h hash.Hash, s string) error {
	// Convert Go string (UTF-8) to UTF-16 code points (uint16).
	utf16Chars := utf16.Encode([]rune(s))

	// Write each uint16 (2 bytes) to the hasher in LittleEndian order.
	for _, char16 := range utf16Chars {
		if err := binary.Write(h, binary.LittleEndian, char16); err != nil {
			return fmt.Errorf("write uint16 to hasher: %w", err)
		}
	}
	return nil
}

// persistentHashTestIdentifier generates a SHA256 hash for a TestIdentifier,
// replicating the hashing logic used by the AnTS system for compatibility.
//
// Strings are encoded as UTF-16 Little Endian, and their lengths as
// int32 Little Endian, matching the behavior of the original Java implementation.
//
// Note: The ModuleParametersHash field must be pre-computed (e.g., using
// hashParameters) before calling this function.
func persistentHashTestIdentifier(id *bqpb.AntsTestResultRow_TestIdentifier) (string, error) {
	h := sha256.New()
	var err error

	// module
	err = putInt(h, len(id.Module))
	if err != nil {
		return "", errors.Annotate(err, "module length").Err()
	}
	err = putString(h, id.Module)
	if err != nil {
		return "", errors.Annotate(err, "module").Err()
	}

	err = putInt(h, len(id.ModuleParameters))
	if err != nil {
		return "", errors.Annotate(err, "module parameters length").Err()
	}
	err = putString(h, id.ModuleParametersHash)
	if err != nil {
		return "", errors.Annotate(err, "module parameters hash").Err()
	}

	// test_class
	err = putInt(h, len(id.TestClass))
	if err != nil {
		return "", errors.Annotate(err, "test class length").Err()
	}
	err = putString(h, id.TestClass)
	if err != nil {
		return "", errors.Annotate(err, "test class").Err()
	}

	// method
	err = putInt(h, len(id.Method))
	if err != nil {
		return "", errors.Annotate(err, "method length").Err()
	}
	err = putString(h, id.Method)
	if err != nil {
		return "", errors.Annotate(err, "method").Err()
	}

	// method parameters is deprecated, put empty string here for backward compatibility.
	err = putInt(h, 0)
	if err != nil {
		return "", err
	}
	err = putString(h, "")
	if err != nil {
		return "", err
	}

	hashBytes := h.Sum(nil)
	hashString := hex.EncodeToString(hashBytes)
	return hashString, nil
}

// hashParameters generates a SHA256 hash for a slice of module parameters
// (*bqpb.AntsTestResultRow_StringPair), replicating the AnTS hashing logic
// for compatibility.
//
// Returns an empty string if the input slice is nil or empty.
func hashParameters(parameters []*bqpb.AntsTestResultRow_StringPair) string {
	if len(parameters) == 0 {
		return ""
	}

	// Create a copy to avoid modifying the original slice during sorting.
	paramsCopy := make([]*bqpb.AntsTestResultRow_StringPair, len(parameters))
	copy(paramsCopy, parameters)

	// Sort by Name, then Value.
	sort.Slice(paramsCopy, func(i, j int) bool {
		if paramsCopy[i].Name != paramsCopy[j].Name {
			return paramsCopy[i].Name < paramsCopy[j].Name
		}
		return paramsCopy[i].Value < paramsCopy[j].Value
	})

	// Join into "Name1=Value1,Name2=Value2" format.
	var sb strings.Builder
	for i, p := range paramsCopy {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(p.Name)
		sb.WriteString("=")
		sb.WriteString(p.Value)
	}

	h := sha256.New()
	h.Write([]byte(sb.String()))
	return hex.EncodeToString(h.Sum(nil))
}
