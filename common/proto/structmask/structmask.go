// Copyright 2021 The LUCI Authors.
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

// Package structmask implements a functionality similar to
// google.protobuf.FieldMask, but which applies only to google.protobuf.Struct.
//
// A google.protobuf.FieldMask can refer only to valid protobuf fields and
// "google.golang.org/protobuf" asserts that when serializing the field mask.
// It makes this mechanism unusable for targeting "unusual" struct fields
// (for example ones containing '.'). Additionally, google.protobuf.FieldMask
// doesn't support wildcard matches (with '*', since it is not a valid proto
// field name).
package structmask

import (
	"fmt"

	"google.golang.org/protobuf/types/known/structpb"
)

// Filter knows how to use StructMask to filter google.protobuf.Struct.
//
// Construct it using NewFilter.
type Filter struct {
	root *node
}

// NewFilter returns a filter that filters structs according to the struct mask.
//
// Returns an error if the struct mask is malformed. If `mask` is empty, returns
// a filter that doesn't actually filter anything.
func NewFilter(mask []*StructMask) (*Filter, error) {
	root, err := parseMask(mask)
	if err != nil {
		return nil, err
	}
	return &Filter{root}, nil
}

// Apply returns a shallow copy of the struct, selecting only elements matching
// the mask.
//
// The result may reuse fields of the original struct (i.e. it copies pointers,
// whenever possible, not actual objects). In extreme case of mask `*` it will
// return `s` as is.
//
// If you need to modify the result, consider explicitly making a deep copy with
// proto.Clone first.
//
// If given `nil`, returns `nil` as well.
func (f *Filter) Apply(s *structpb.Struct) *structpb.Struct {
	if s == nil || f.root == nil || f.root == leafNode {
		return s
	}
	filtered := f.root.filterStruct(s)
	if filtered == nil {
		return &structpb.Struct{}
	}
	// During merging we use `nil` as a stand in for "empty set after filtering",
	// in lists to make it distinct from NullValue representing the real `null`.
	// `nil` is not allowed in *structpb.Struct. Convert them all to real Nulls.
	fillNulls(filtered)
	return filtered.GetStructValue()
}

////////////////////////////////////////////////////////////////////////////////

// leafNode is a sentinel node meaning "grab the rest of the value unfiltered".
var leafNode = &node{}

// node contains a filter tree applying to some struct path element and its
// children.
//
// *node pointers have two special values:
//
//	nil - no filter is present (e.g. if `star == nil`, then do not recurse).
//
// /  leafNode - a filter that grabs all remaining values unfiltered.
type node struct {
	star   *node            // a filter to apply to all dict fields or list indexes, if any
	fields map[string]*node // a filter for individual dict fields
}

// filter recursively applies the filter tree `n` to an input value.
//
// It returns a filtered value with possible "gaps" in lists (represented by
// nils in structpb.ListValue.Values slice). These gaps appear when the filter
// filters out the entire list element. They are needed because the parent
// node may still fill them in. It needs to know where gaps are to do so safely.
// Note that representing them with structpb.NullValue is dangerous, since
// structs can have genuine `null`s in them.
//
// For example, a filter `a.*.x` applied to a `{"a": [{"x": 1}, {"y": 2}]}`
// results in `{"a": [{"x": 1}, <gap>]}`. Similarly `*.*.y` applied to the same
// input results in `{"a": [<gap>, {"y": 2}]}`. When we join these filters, we
// get the result with all gaps filled in: `{"a": [{"x": 2}, {"y": 2}]}`. Note
// that since filter paths start with different tokens (`*` vs `a`) the filter
// nodes that actually produce gaps reside in different branches of the tree,
// separated by multiple layers. This necessitates the merging and gap filling
// to be recursive (see `merge`).
//
// Since a correctly constructed *structpb.Value isn't allowed to have `nil`s,
// all gaps left in the final result are converted to `null` at the very end of
// the filtering by `fillNulls`. This is documented in the StructMask proto doc
// in the section that talks about "exceptional conditions".
func (n *node) filter(val *structpb.Value) *structpb.Value {
	if n == leafNode {
		return val
	}

	// Since `n` is not a leafNode, it is actually `.<something>`, i.e. it needs
	// to filter inner guts of `val`. We can "dive" only into dicts and lists.
	// Trying to "explore" a scalar value results in "no match" result,
	// represented by `nil`. Note that it is distinct from NullValue.
	//
	// Also if `n` is the last `.*` of the mask, return `val` unchanged as is
	// without even diving into it or checking additional masks in `fields`. This
	// avoids useless memory allocation of structpb.Struct/structpb.ListValue
	// wrappers.
	switch v := val.Kind.(type) {
	case *structpb.Value_StructValue:
		if n.star == leafNode {
			return val
		}
		return n.filterStruct(v.StructValue)
	case *structpb.Value_ListValue:
		if n.star == leafNode {
			return val
		}
		return n.filterList(v.ListValue)
	default:
		return nil
	}
}

func (n *node) filterStruct(val *structpb.Struct) *structpb.Value {
	// Apply `*` mask first (if any).
	out := &structpb.Struct{}
	if n.star != nil {
		out.Fields = make(map[string]*structpb.Value, len(val.Fields))
		for k, v := range val.Fields {
			if filtered := n.star.filter(v); filtered != nil {
				out.Fields[k] = filtered
			}
		}
	} else {
		out.Fields = make(map[string]*structpb.Value, len(n.fields))
	}

	// Merge any additional values picked by field masks targeting individual
	// dict keys.
	for key, filter := range n.fields {
		if input, ok := val.Fields[key]; ok {
			if filtered := filter.filter(input); filtered != nil {
				out.Fields[key] = merge(out.Fields[key], filtered)
			}
		}
	}

	// If filtered out all keys, return an "empty set" value represented by nil.
	// Note that this drops genuinely empty dicts as well. Oh, well... This is
	// somewhat negated by the early return on leaf nodes in `filter`.
	if len(out.Fields) == 0 {
		return nil
	}

	return structpb.NewStructValue(out)
}

func (n *node) filterList(val *structpb.ListValue) *structpb.Value {
	// Only `*` is supported. Picking individual list indexes is not implemented
	// yet since it is not clear how to do merging step when there are different
	// masks that use `*` and concrete indexes at the same time. To do the correct
	// merging we need to "remember" original indexes of items in the filtered
	// list and there's no place for it in *structpb.ListValue.
	if n.star == nil {
		return nil
	}
	out := &structpb.ListValue{Values: make([]*structpb.Value, len(val.Values))}
	for i, v := range val.Values {
		// Note that this leaves a `nil` gap if the list element was completely
		// filtered out. It is important for `merge` to know where they are to do
		// merging correctly. These gaps are converted to nulls by `fillNulls` at
		// the very end when all merges are done.
		out.Values[i] = n.star.filter(v)
	}
	return structpb.NewListValue(out)
}

// merge merges `b` into `a`, returning a shallowly combined copy of both.
//
// Both `a` and `b` should be results of a filtering of the same value, thus
// they (if not nil) must have the same type and (if lists) have the same
// length. If they are scalars, they assumed to be equal already.
//
// `nil` represents "empty sets", i.e. if `a` is nil, `b` will be returned as
// is and vice-versa. If both are `nil`, returns `nil` as well.
func merge(a, b *structpb.Value) *structpb.Value {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	case a == b:
		return a
	}
	switch a := a.Kind.(type) {
	case *structpb.Value_StructValue:
		// `b` *must* be a Struct here. Panic if not, it means there's a bug.
		return structpb.NewStructValue(mergeStruct(a.StructValue, b.Kind.(*structpb.Value_StructValue).StructValue))
	case *structpb.Value_ListValue:
		// `b` *must* be a List here. Panic if not, it means there's a bug.
		return structpb.NewListValue(mergeList(a.ListValue, b.Kind.(*structpb.Value_ListValue).ListValue))
	default:
		return b
	}
}

func mergeStruct(a, b *structpb.Struct) *structpb.Struct {
	l := len(a.Fields)
	if len(b.Fields) > l {
		l = len(b.Fields)
	}

	out := &structpb.Struct{
		Fields: make(map[string]*structpb.Value, l),
	}

	for key, aval := range a.Fields {
		out.Fields[key] = merge(aval, b.Fields[key])
	}

	for key, bval := range b.Fields {
		// Already dealt with A&B case above. Pick only B-A.
		if _, ok := a.Fields[key]; !ok {
			out.Fields[key] = bval
		}
	}

	return out
}

func mergeList(a, b *structpb.ListValue) *structpb.ListValue {
	if len(a.Values) != len(b.Values) {
		panic(fmt.Sprintf("unexpected list lengths %d != %d", len(a.Values), len(b.Values)))
	}
	out := &structpb.ListValue{
		Values: make([]*structpb.Value, len(a.Values)),
	}
	for idx := range a.Values {
		out.Values[idx] = merge(a.Values[idx], b.Values[idx])
	}
	return out
}

func fillNulls(v *structpb.Value) {
	switch v := v.Kind.(type) {
	case *structpb.Value_StructValue:
		for key, elem := range v.StructValue.Fields {
			if elem == nil {
				// We leave nils only in lists, not in structs.
				panic(fmt.Sprintf("unexpected nil for struct key %q", key))
			} else {
				fillNulls(elem)
			}
		}
	case *structpb.Value_ListValue:
		for idx, elem := range v.ListValue.Values {
			if elem == nil {
				v.ListValue.Values[idx] = structpb.NewNullValue()
			} else {
				fillNulls(elem)
			}
		}
	}
}
