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

package pbutil

import (
	"fmt"
	"sort"

	"go.chromium.org/luci/common/errors"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// ValidateVariantDef returns an error if def is invalid.
func ValidateVariantDef(d *pb.VariantDef) error {
	for k, v := range d.GetDef() {
		p := pb.StringPair{Key: k, Value: v}
		if err := ValidateStringPair(&p); err != nil {
			return errors.Annotate(err, "%q:%q", k, v).Err()
		}
	}
	return nil
}

// VariantDefPairs returns a key:val string slice representation of the VariantDef.
func VariantDefPairs(d *pb.VariantDef) []string {
	keys := SortedVariantDefKeys(d)
	pairs := make([]string, len(keys))
	defMap := d.GetDef()
	for i, k := range keys {
		pairs[i] = fmt.Sprintf("%s:%s", k, defMap[k])
	}
	return pairs
}

// VariantDefFromStrings returns a VariantDef proto given the key:val string slice of its contents.
//
// If a key appears multiple times, the last pair wins.
func VariantDefFromStrings(pairs []string) (*pb.VariantDef, error) {
	if len(pairs) == 0 {
		return nil, nil
	}

	defMap := make(map[string]string, len(pairs))
	for _, p := range pairs {
		pair, err := StringPairFromString(p)
		if err != nil {
			return nil, errors.Annotate(err, "pair %q", p).Err()
		}
		defMap[pair.Key] = pair.Value
	}
	return &pb.VariantDef{Def: defMap}, nil
}

// SortedVariantDefKeys returns the keys in the variant def as a sorted slice.
func SortedVariantDefKeys(d *pb.VariantDef) []string {
	keys := make([]string, 0, len(d.GetDef()))
	for k := range d.GetDef() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
