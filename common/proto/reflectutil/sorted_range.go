// Copyright 2022 The LUCI Authors.
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

package reflectutil

import (
	"sort"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// MapRangeSorted is the same as `m.Range()` except that it iterates through the
// map in sorted order.
//
// The keys AND values of the map are snapshotted before your callback runs;
// This means that addition or removal of keys from the map will not be
// reflected to your callback. Interior modification of values (i.e. Value.field
// = something) MAY be reflected, but wholesale re-assignment of map value will not
// be reflected.
//
// Note that for the purposes of this function and proto reflection in general,
// Int32Kind==Int64Kind and Uint32Kind==Uint64Kind; Providing Int32Kind for a
// map with an int64 key is not an issue (and vice versa).
func MapRangeSorted(m protoreflect.Map, keyKind protoreflect.Kind, cb func(protoreflect.MapKey, protoreflect.Value) bool) {
	if m.Len() == 0 {
		return
	}

	switch keyKind {
	case protoreflect.BoolKind:
		sortedRangeMapBool(m, cb)
	case protoreflect.Int32Kind, protoreflect.Int64Kind:
		sortedRangeMapInt(m, cb)
	case protoreflect.Uint32Kind, protoreflect.Uint64Kind:
		sortedRangeMapUint(m, cb)
	case protoreflect.StringKind:
		sortedRangeMapString(m, cb)
	default:
		panic("impossible")
	}
}

func sortedRangeMapBool(m protoreflect.Map, cb func(protoreflect.MapKey, protoreflect.Value) bool) {
	falseKey := protoreflect.MapKey(protoreflect.ValueOfBool(false))
	if falseVal := m.Get(falseKey); falseVal.IsValid() {
		if !cb(falseKey, falseVal) {
			return
		}
	}
	trueKey := protoreflect.MapKey(protoreflect.ValueOfBool(true))
	if trueVal := m.Get(trueKey); trueVal.IsValid() {
		if !cb(trueKey, trueVal) {
			return
		}
	}
}

func sortedRangeMapInt(m protoreflect.Map, cb func(protoreflect.MapKey, protoreflect.Value) bool) {
	type itemT struct {
		sk int64
		mk protoreflect.MapKey
		v  protoreflect.Value
	}

	items := make([]*itemT, m.Len())
	idx := 0
	m.Range(func(mk protoreflect.MapKey, v protoreflect.Value) bool {
		items[idx] = &itemT{mk.Int(), mk, v}
		idx++
		return true
	})
	sort.Slice(items, func(i, j int) bool {
		return items[i].sk < items[j].sk
	})

	for _, item := range items {
		if !cb(item.mk, item.v) {
			return
		}
	}
}

func sortedRangeMapUint(m protoreflect.Map, cb func(protoreflect.MapKey, protoreflect.Value) bool) {
	type itemT struct {
		sk uint64
		mk protoreflect.MapKey
		v  protoreflect.Value
	}

	items := make([]*itemT, m.Len())
	idx := 0
	m.Range(func(mk protoreflect.MapKey, v protoreflect.Value) bool {
		items[idx] = &itemT{mk.Uint(), mk, v}
		idx++
		return true
	})
	sort.Slice(items, func(i, j int) bool {
		return items[i].sk < items[j].sk
	})

	for _, item := range items {
		if !cb(item.mk, item.v) {
			return
		}
	}
}

func sortedRangeMapString(m protoreflect.Map, cb func(protoreflect.MapKey, protoreflect.Value) bool) {
	type itemT struct {
		sk string
		mk protoreflect.MapKey
		v  protoreflect.Value
	}

	items := make([]*itemT, m.Len())
	idx := 0
	m.Range(func(mk protoreflect.MapKey, v protoreflect.Value) bool {
		items[idx] = &itemT{mk.String(), mk, v}
		idx++
		return true
	})
	sort.Slice(items, func(i, j int) bool {
		return items[i].sk < items[j].sk
	})

	for _, item := range items {
		if !cb(item.mk, item.v) {
			return
		}
	}
}
