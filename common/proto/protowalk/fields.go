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
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/proto/reflectutil"
)

// fieldsImpl actually implements the Fields method, using `path` as context
// for the overall path through the proto message.
func (l DynamicWalker) fieldsImpl(path reflectutil.Path, msg protoreflect.Message) map[FieldProcessor][]Result {
	plan := l.plans[msg.Descriptor()]
	ret := make(map[FieldProcessor][]Result, plan.numProcs)
	mergeResults := func(additional map[FieldProcessor][]Result) {
		for key, val := range additional {
			ret[key] = append(ret[key], val...)
		}
	}

	plan.each(func(field protoreflect.FieldDescriptor, items []planItem) {
		var toRecurse recurseAttr

		var copiedFieldPath reflectutil.Path

		for _, item := range items {
			toRecurse.set(item.recurseAttr)

			if item.applies(msg.Has(field)) {
				if data, applied := item.processor.Process(field, msg); applied {
					if copiedFieldPath == nil {
						copiedFieldPath = make(reflectutil.Path, len(path)+1)
						copy(copiedFieldPath, path)
						copiedFieldPath[len(path)] = reflectutil.MustMakePathItem(field)
					}
					ret[item.processor] = append(ret[item.processor], Result{copiedFieldPath, data})
				}
			}
		}

		if toRecurse != recurseNone {
			recursePath := append(path, reflectutil.MustMakePathItem(field))

			switch toRecurse {
			case recurseRepeated:
				lst := msg.Get(field).List()
				for i := 0; i < lst.Len(); i++ {
					mergeResults(l.fieldsImpl(append(recursePath, reflectutil.MustMakePathItem(i)), lst.Get(i).Message()))
				}

			case recurseOne:
				if msg.Has(field) {
					mergeResults(l.fieldsImpl(recursePath, msg.Get(field).Message()))
				}

			default:
				if !toRecurse.isMap() {
					panic("impossible")
				}

				reflectutil.MapRangeSorted(msg.Get(field).Map(), toRecurse.mapKeyKind(), func(mk protoreflect.MapKey, v protoreflect.Value) bool {
					mergeResults(l.fieldsImpl(append(recursePath, reflectutil.MustMakePathItem(mk)), v.Message()))
					return true
				})
			}
		}
	})

	return ret
}
