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

package protowalk

import (
	"reflect"
	"sort"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type planItem struct {
	processorIdx int
	ProcessAttr
	recurseAttr
}

type plan struct {
	desc       protoreflect.MessageDescriptor
	fieldOrder []protoreflect.FieldNumber
	items      map[protoreflect.FieldNumber][]planItem
}

func (p plan) each(cb func(protoreflect.FieldDescriptor, []planItem)) {
	fields := p.desc.Fields()
	for _, fieldNum := range p.fieldOrder {
		cb(fields.ByNumber(fieldNum), p.items[fieldNum])
	}
}

type procBundle struct {
	proc reflect.Type
	sel  FieldSelector
	fp   FieldProcessor
}

// makePlan generates a plan from msg+processors.
//
// The returned plan is traversable in a deterministic order (using the sorted
// order of the affected Fields' tag numbers in the message).
//
// For each affected field, one or more processors will either need to directly
// process the field, or will need to recurse deeper.
func makePlan(desc protoreflect.MessageDescriptor, processors []*procBundle) (ret plan) {
	size := desc.Fields().Len()
	ret.fieldOrder = make([]protoreflect.FieldNumber, 0, size)
	ret.desc = desc
	ret.items = make(map[protoreflect.FieldNumber][]planItem, size)

	// Now for each processor, go over its cached (msg+processor) data, and merge
	// it into the overall plan.
	for procIdx, proc := range processors {
		for _, entry := range setCacheEntry(desc, proc) {
			curPlan, ok := ret.items[entry.fieldNum]
			if !ok { // need to add entry.field to ret.fieldOrder
				foIdx := sort.Search(
					len(ret.fieldOrder),
					func(i int) bool {
						return ret.fieldOrder[i] >= entry.fieldNum
					})
				// -1 is invalid, but gets overwritten; if we have a bug, -1 will cause an
				// explosion later.
				ret.fieldOrder = append(ret.fieldOrder, -1)
				copy(ret.fieldOrder[foIdx+1:], ret.fieldOrder[foIdx:])
				ret.fieldOrder[foIdx] = entry.fieldNum
			}

			// and fold the entry into the overall plan.
			ret.items[entry.fieldNum] = append(curPlan, planItem{
				processorIdx: procIdx,
				ProcessAttr:  entry.ProcessAttr,
				recurseAttr:  entry.recurseAttr,
			})
		}
	}

	return
}
