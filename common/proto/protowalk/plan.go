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
	"sort"

	"google.golang.org/protobuf/reflect/protoreflect"
)

type planItem struct {
	processor FieldProcessor
	ProcessAttr
	recurseAttr
}

// plan is a specific list of fields to process and/or recurse into.
//
// It is computed by `makePlan(msg, processors)`.
type plan struct {
	desc       protoreflect.MessageDescriptor
	fieldOrder []protoreflect.FieldNumber
	items      map[protoreflect.FieldNumber][]planItem
	numProcs   int
}

func (p plan) each(cb func(protoreflect.FieldDescriptor, []planItem)) {
	fields := p.desc.Fields()
	for _, fieldNum := range p.fieldOrder {
		cb(fields.ByNumber(fieldNum), p.items[fieldNum])
	}
}

// makePlan generates a plan from msg+processors.
//
// The returned plan is traversable in a deterministic order (using the sorted
// order of the affected Fields' tag numbers in the message).
//
// For each affected field, one or more processors will either need to directly
// process the field, or will need to recurse deeper.
func makePlan(desc protoreflect.MessageDescriptor, processors []FieldProcessor, t tempCache) plan {
	size := desc.Fields().Len()
	ret := plan{
		fieldOrder: make([]protoreflect.FieldNumber, 0, size),
		desc:       desc,
		items:      make(map[protoreflect.FieldNumber][]planItem, size),
	}

	// Now for each processor's cache entry, go over its msg+processor data, and
	// merge it into the overall plan.
	for i, proc := range processors {
		added := false
		for _, entry := range t[desc][proc] {
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
				processor:   processors[i],
				ProcessAttr: entry.ProcessAttr,
				recurseAttr: entry.recurseAttr,
			})
			if !added {
				ret.numProcs += 1
				added = true
			}
		}
	}

	return ret
}
