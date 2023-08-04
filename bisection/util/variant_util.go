// Copyright 2023 The LUCI Authors.
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

package util

import (
	"encoding/json"
	"fmt"
	"sort"

	pb "go.chromium.org/luci/bisection/proto/v1"
)

// VariantPB convert json string representation of the variant into protocol buffer.
func VariantPB(variant string) (*pb.Variant, error) {
	v := map[string]string{}
	if err := json.Unmarshal([]byte(variant), &v); err != nil {
		return nil, err
	}
	return &pb.Variant{Def: v}, nil
}

// VariantToStrings returns a key:val string slice representation of the Variant.
// Never returns nil.
func VariantToStrings(vr *pb.Variant) []string {
	if len(vr.GetDef()) == 0 {
		return []string{}
	}
	pairs := make([]string, 0, len(vr.GetDef()))
	for k, v := range vr.GetDef() {
		pairs = append(pairs, fmt.Sprintf("%s:%s", k, v))
	}
	sort.Strings(pairs)
	return pairs
}
