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

package data

import (
	"cmp"
	"errors"
	"maps"
	"slices"

	"google.golang.org/protobuf/encoding/protojson"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// ValueSet represents a collection of [orchestratorpb.Value], unique by
// typeUrl
type ValueSet struct {
	data map[string]*orchestratorpb.Value
}

// Get retrieves the value from the set (if it exists) or nil otherwise.
func (v ValueSet) Get(typeUrl string) *orchestratorpb.Value {
	return v.data[typeUrl]
}

// Set sets the given value in this set, overriding any existing value of the
// same type.
func (v ValueSet) Set(val *orchestratorpb.Value) {
	v.data[val.GetValue().GetTypeUrl()] = val
}

// Add inserts the given value into this set, but only if it does not already
// exist.
//
// Returns `true` if the value was inserted, `false` if it was already present.
func (v ValueSet) Add(val *orchestratorpb.Value) bool {
	key := val.GetValue().GetTypeUrl()
	if _, ok := v.data[key]; ok {
		return false
	}
	v.data[key] = val
	return true
}

// Redact removes all data (value.value and value_json), from this ValueSet,
// setting the omitted reason as `NO_ACCESS`.
//
// This retains only the type_urls of the redacted data.
func (v ValueSet) Redact() {
	for _, val := range v.data {
		Redact(val)
	}
}

// Filter will:
//   - Remove data from unwanted types (setting the omit reason to `UNWANTED`).
//   - Replace value data for types where JSONPB encoding is desired (and set
//     has_unknown_fields if this process could not fully reserialize the
//     value).
//
// If set, `mopt.Resolver` will be used for decoding and encoding all data
// where JSONPB encoding is desired. If there is no type available to decode
// in the resolver, this will leave the binary encoded data as-is.
func (v ValueSet) Filter(ti *TypeInfo, mopt protojson.MarshalOptions) error {
	var errs []error
	for _, val := range v.data {
		if err := Filter(val, ti, mopt); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// ToSlice returns the values in the set, sorted by type_url.
func (v ValueSet) ToSlice() []*orchestratorpb.Value {
	ret := make([]*orchestratorpb.Value, 0, len(v.data))
	for _, val := range v.data {
		ret = append(ret, val)
	}
	slices.SortFunc(ret, func(a, b *orchestratorpb.Value) int {
		return cmp.Compare(a.GetValue().GetTypeUrl(), b.GetValue().GetTypeUrl())
	})
	return ret
}

// ValueSetFromSlice parses a slice of Value to a ValueSet.
func ValueSetFromSlice(vals []*orchestratorpb.Value) ValueSet {
	ret := make(map[string]*orchestratorpb.Value, len(vals))
	maps.Insert(ret, func(yield func(typeUrl string, v *orchestratorpb.Value) bool) {
		for _, val := range vals {
			if !yield(val.GetValue().GetTypeUrl(), val) {
				return
			}
		}
	})
	return ValueSet{ret}
}
