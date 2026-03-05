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
	"slices"

	orchestratorpb "go.chromium.org/turboci/proto/go/graph/orchestrator/v1"
)

// valRefCmp is used for []*ValueRef which are unique by type_url.
func valRefCmp(a, b *orchestratorpb.ValueRef) int {
	return cmp.Compare(a.GetTypeUrl(), b.GetTypeUrl())
}

// SetByTypeIn sets `ref` in `set`.
//
// This assumes that, and maintains, the uniqueby/sorted criteria of the set.
//
// If setting `ref` in `set` would result in a realm conflict, returns `set`
// unmodified and `realmConflict` as true.
func SetByTypeIn(set []*orchestratorpb.ValueRef, ref *orchestratorpb.ValueRef) (ret []*orchestratorpb.ValueRef, realmConflict bool) {
	idx, found := slices.BinarySearchFunc(set, ref, valRefCmp)
	if found {
		if set[idx].GetRealm() != ref.GetRealm() {
			return set, true
		}
		set[idx] = ref
	} else {
		set = slices.Insert(set, idx, ref)
	}
	return set, false
}

// AddByTypeIn adds `ref` in `set` as long as it would not overwrite an existing
// value. If `ref` would overwrite an existing ref, returns `added` == false.
//
// Requires that `set` is already sorted uniquely by `unq`.
func AddByTypeIn(set []*orchestratorpb.ValueRef, ref *orchestratorpb.ValueRef) (newSet []*orchestratorpb.ValueRef, added bool) {
	idx, found := slices.BinarySearchFunc(set, ref, valRefCmp)
	if found {
		return set, false
	}
	return slices.Insert(set, idx, ref), true
}
