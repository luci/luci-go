// Copyright 2019 The LUCI Authors.
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

package resultdb

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"sort"

	resultspb "go.chromium.org/luci/resultdb/proto/v1"
)

// VariantDefMap contains the key:val pairs that define a Variant.
//
// It is the "Def" part of a VariantDef proto.
type VariantDefMap map[string]string

// Validate validates the receiver VariantDefMap by checking characters and length of the keys and values.
func (d VariantDefMap) Validate() error {
	// TODO(jchinlee): Implement.
	return nil
}

// ID returns a hex SHA256 hash of newline-joined "<key>:<val>" strings from the variant as an ID.
func (d VariantDefMap) ID() string {
	h := sha256.New()
	for _, k := range d.SortedKeys() {
		io.WriteString(h, k)
		io.WriteString(h, ":")
		io.WriteString(h, d[k])
		io.WriteString(h, "\n")
	}

	return hex.EncodeToString(h.Sum(nil))
}

// Proto converts the VariantDefMap to a resultspb.VariantDef proto.
func (d VariantDefMap) Proto() *resultspb.VariantDef {
	return &resultspb.VariantDef{
		Def: d,
	}
}

// SortedKeys returns the keys in the variant def as a sorted slice.
func (d VariantDefMap) SortedKeys() []string {
	keys := make([]string, 0, len(d))
	for k := range d {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// MergeTestVariantMaps gets the test variant def from merging the input maps.
//
// If multiple maps define the same key, the last one wins.
func MergeTestVariantMaps(maps ...VariantDefMap) VariantDefMap {
	def := VariantDefMap{}
	for _, m := range maps {
		for k, v := range m {
			def[k] = v
		}
	}
	return def
}
