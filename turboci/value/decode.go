// Copyright 2026 The LUCI Authors.
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

package value

import (
	"cmp"
	"fmt"
	"slices"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// Decode decodes and returns a proto of type `T` from the given ValueRef.
//
// `source` is used to lookup the data for this ValueRef if it's not inline.
//
// If ValueRef has a type_url which does not match `T`, this returns an error.
func Decode[T proto.Message](source DataSource, ref *orchestratorpb.ValueRef) (T, error) {
	var zero T
	typeURL := URL[T]()
	if typeURL != ref.GetTypeUrl() {
		return zero, fmt.Errorf("mismatched types %q != %q", typeURL, ref.GetTypeUrl())
	}

	var binData *anypb.Any
	var jsonData *orchestratorpb.ValueData_JsonAny

	if data := ref.GetInline(); data != nil {
		binData = data
	} else {
		got := source.Retrieve(Digest(ref.GetDigest()))
		binData, jsonData = got.GetBinary(), got.GetJson()
	}

	if binData == nil && jsonData == nil {
		return zero, nil
	}

	if binData != nil {
		if binData.TypeUrl != typeURL {
			// This should never happen; it means a ValueRef for type X pointed to a
			// ValueData of type Y.
			return zero, fmt.Errorf("BUG: mismatched types %q != %q in ValueData", typeURL, binData.TypeUrl)
		}
		ret, err := binData.UnmarshalNew()
		if err != nil {
			return zero, err
		}
		return ret.(T), nil
	}

	if jsonData.GetTypeUrl() != typeURL {
		// This should never happen; it means a ValueRef for type X pointed to a
		// ValueData of type Y.
		return zero, fmt.Errorf("BUG: mismatched types %q != %q in ValueData", typeURL, jsonData.GetTypeUrl())
	}

	ret := zero.ProtoReflect().New().Interface()
	err := protojson.Unmarshal([]byte(jsonData.GetValue()), ret)
	if err != nil {
		return zero, err
	}

	return ret.(T), nil
}

// Lookup finds and decodes the first non-omitted value of type `T` in a set of
// ValueRefs sorted by type_url.
//
// Note that ValueRefs do not need to be unique by type_url (such as for
// edit reason details).
//
// This assumes that `set` is sorted by "type_url" and will do a binary search.
//
// If none is found, or ref is omitted, this returns nil.
func Lookup[T proto.Message](source DataSource, set []*orchestratorpb.ValueRef) (T, error) {
	val := Find(set, URL[T]())
	if val == nil {
		var zero T
		return zero, nil
	}
	return Decode[T](source, val)
}

// Find finds the first non-omitted ValueRef with the given type URL in a
// set of ValueRefs sorted by type_url.
//
// Note that ValueRefs do not need to be unique by type_url (such as for
// edit reason details).
//
// If none is found, this returns nil.
func Find(list []*orchestratorpb.ValueRef, typeURL string) *orchestratorpb.ValueRef {
	idx, found := slices.BinarySearchFunc(list, typeURL, func(ref *orchestratorpb.ValueRef, typeURL string) int {
		return cmp.Compare(ref.GetTypeUrl(), typeURL)
	})
	if found {
		for _, ref := range list[idx:] {
			if ref.GetTypeUrl() != typeURL {
				return nil
			}
			if ref.HasOmitReason() {
				continue
			}
			return ref
		}
	}
	return nil
}

func Results[T proto.Message](source DataSource, check *orchestratorpb.Check) ([]T, error) {
	var zero T
	ret := make([]T, 0, len(check.GetResults()))
	for _, rslt := range check.GetResults() {
		x, err := Lookup[T](source, rslt.GetData())
		if err != nil {
			return nil, err
		}
		if !proto.Equal(zero, x) {
			ret = append(ret, x)
		}
	}
	return ret, nil
}
