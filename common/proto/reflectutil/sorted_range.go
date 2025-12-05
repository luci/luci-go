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
	"iter"
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
func MapRangeSorted(m protoreflect.Map, keyKind protoreflect.Kind) iter.Seq2[protoreflect.MapKey, protoreflect.Value] {
	if m.Len() == 0 {
		return func(yield func(protoreflect.MapKey, protoreflect.Value) bool) {}
	}

	switch keyKind {
	case protoreflect.BoolKind:
		return sortedRangeMapBool(m)
	case protoreflect.Int32Kind, protoreflect.Int64Kind:
		return sortedRangeMapInt(m)
	case protoreflect.Uint32Kind, protoreflect.Uint64Kind:
		return sortedRangeMapUint(m)
	case protoreflect.StringKind:
		return sortedRangeMapString(m)
	default:
		panic("impossible")
	}
}

var falseKey = protoreflect.MapKey(protoreflect.ValueOfBool(false))
var trueKey = protoreflect.MapKey(protoreflect.ValueOfBool(true))

func sortedRangeMapBool(m protoreflect.Map) iter.Seq2[protoreflect.MapKey, protoreflect.Value] {
	return func(yield func(protoreflect.MapKey, protoreflect.Value) bool) {
		if falseVal := m.Get(falseKey); falseVal.IsValid() {
			if !yield(falseKey, falseVal) {
				return
			}
		}
		if trueVal := m.Get(trueKey); trueVal.IsValid() {
			if !yield(trueKey, trueVal) {
				return
			}
		}
	}
}

func sortedRangeMapInt(m protoreflect.Map) iter.Seq2[protoreflect.MapKey, protoreflect.Value] {
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

	return func(yield func(protoreflect.MapKey, protoreflect.Value) bool) {
		for _, item := range items {
			if !yield(item.mk, item.v) {
				return
			}
		}
	}
}

func sortedRangeMapUint(m protoreflect.Map) iter.Seq2[protoreflect.MapKey, protoreflect.Value] {
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

	return func(yield func(protoreflect.MapKey, protoreflect.Value) bool) {
		for _, item := range items {
			if !yield(item.mk, item.v) {
				return
			}
		}
	}
}

func sortedRangeMapString(m protoreflect.Map) iter.Seq2[protoreflect.MapKey, protoreflect.Value] {
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

	return func(yield func(protoreflect.MapKey, protoreflect.Value) bool) {
		for _, item := range items {
			if !yield(item.mk, item.v) {
				return
			}
		}
	}
}
