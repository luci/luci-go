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
	"fmt"
	"reflect"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/proto/reflectutil"
)

// fieldsImpl actually implements the Fields function, using `path` as context
// for the overall path through the proto message.
func fieldsImpl(path reflectutil.Path, msg protoreflect.Message, processors []*procBundle) Results {
	ret := make(Results, len(processors))
	plan := makePlan(msg.Descriptor(), processors)

	type accumulation struct {
		recurseAttr
		procs    []*procBundle
		procIdxs []int
	}

	plan.each(func(field protoreflect.FieldDescriptor, items []planItem) {
		acc := accumulation{
			procs:    make([]*procBundle, 0, len(processors)),
			procIdxs: make([]int, 0, len(processors)),
		}

		var copiedFieldPath reflectutil.Path

		for _, item := range items {
			proc := processors[item.processorIdx]

			if acc.recurseAttr.set(item.recurseAttr) {
				acc.procs = append(acc.procs, processors[item.processorIdx])
				acc.procIdxs = append(acc.procIdxs, item.processorIdx)
			}

			if item.applies(msg.Has(field)) {
				if data, applied := proc.fp.Process(field, msg); applied {
					if copiedFieldPath == nil {
						copiedFieldPath = make(reflectutil.Path, len(path)+1)
						copy(copiedFieldPath, path)
						copiedFieldPath[len(path)] = reflectutil.MustMakePathItem(field)
					}
					ret[item.processorIdx] = append(ret[item.processorIdx], Result{copiedFieldPath, data})
				}
			}
		}

		if acc.recurseAttr != recurseNone {
			recursePath := append(path, reflectutil.MustMakePathItem(field))

			switch acc.recurseAttr {
			case recurseRepeated:
				lst := msg.Get(field).List()
				for i := 0; i < lst.Len(); i++ {
					for procInnerIdx, rslts := range fieldsImpl(append(recursePath, reflectutil.MustMakePathItem(i)), lst.Get(i).Message(), acc.procs) {
						procIdx := acc.procIdxs[procInnerIdx]
						ret[procIdx] = append(ret[procIdx], rslts...)
					}
				}

			case recurseOne:
				if msg.Has(field) {
					for procInnerIdx, rslts := range fieldsImpl(recursePath, msg.Get(field).Message(), acc.procs) {
						procIdx := acc.procIdxs[procInnerIdx]
						ret[procIdx] = append(ret[procIdx], rslts...)
					}
				}

			default:
				if !acc.recurseAttr.isMap() {
					panic("impossible")
				}

				reflectutil.MapRangeSorted(msg.Get(field).Map(), acc.recurseAttr.mapKeyKind(), func(mk protoreflect.MapKey, v protoreflect.Value) bool {
					for procInnerIdx, rslts := range fieldsImpl(append(recursePath, reflectutil.MustMakePathItem(mk)), v.Message(), acc.procs) {
						procIdx := acc.procIdxs[procInnerIdx]
						ret[procIdx] = append(ret[procIdx], rslts...)
					}
					return true
				})
			}
		}
	})

	return ret
}

// lookupProcBundles prepares a procBundle for each registered FieldProcessor.
//
// If a FieldProcessor is no registered, this panics.
func lookupProcBundles(processors ...FieldProcessor) []*procBundle {
	procs := make([]*procBundle, len(processors))
	fieldProcessorSelectorsMu.RLock()
	defer fieldProcessorSelectorsMu.RUnlock()
	for i, p := range processors {
		t := reflect.TypeOf(p)
		sel, ok := fieldProcessorSelectors[t]
		if !ok {
			panic(fmt.Sprintf("unknown FieldProcessor %T (did you forget to call RegisterFieldProcessor?)", p))
		}
		procs[i] = &procBundle{t, sel, p}
	}
	return procs
}

// Fields recursively processes the fields of `msg` with the given sequence of
// FieldProcessors.
func Fields(msg proto.Message, processors ...FieldProcessor) Results {
	if len(processors) == 0 {
		return nil
	}
	// 32 is a guess at how deep the Path could get.
	//
	// There's really no way to know ahead of time (since proto messages could
	// have a recursive structure, allowing the expression of trees, etc.)
	return fieldsImpl(make(reflectutil.Path, 0, 32), msg.ProtoReflect(), lookupProcBundles(processors...))
}
