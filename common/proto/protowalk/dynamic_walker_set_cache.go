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

	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/data/stringset"
)

// tempCacheField is a single cache value for tempCache which indicates what
// behavior we need for a single field in a proto message for a single
// processor.
type tempCacheField struct {
	// fieldNum is the field 'tag number' in the proto message for the
	// corresponding field. We store this instead of the FieldDescriptor because
	// it's only 4 bytes, rather than a 16+byte interface.
	fieldNum protoreflect.FieldNumber

	// ProcessAttr will be set if this field needs to be processed by the
	// FieldProcessor.
	ProcessAttr

	// recurseAttr will be set if we need to descend into this field in order to
	// see if any of the values it contains need processing.
	recurseAttr
}

// tempCacheEntry corresponds to a single Message+FieldProcessor
// combination.
//
// Each value is the combined result of asking FieldProcessor.ShouldProcess on the
// field, plus any recursion attribute (if this field is a Message (single,
// repeated or map)) which recursively contains fields which need to be
// processed by this FieldProcessor.
//
// This is always kept ordered by field number, and is immutable.
type tempCacheEntry []tempCacheField

type tempCacheEntryBuilder struct {
	ret tempCacheEntry
	tmp map[protoreflect.FieldNumber]tempCacheField
}

func newTempCacheEntryBuilder() *tempCacheEntryBuilder {
	return &tempCacheEntryBuilder{
		ret: make(tempCacheEntry, 0),
		tmp: map[protoreflect.FieldNumber]tempCacheField{},
	}
}

type tempCache map[protoreflect.MessageDescriptor]map[FieldProcessor]tempCacheEntry

func (t tempCache) populate(msgD protoreflect.MessageDescriptor, processors []FieldProcessor) {
	// TODO - setCacheEntry/generateCacheEntry/makePlan can be collapsed into
	// a single walk of msgD which directly outputs a plan. The current code
	// structure is because the previous iteration of this library allowed
	// dynamically mixing and matching {msg x processors} - we would
	// walk each {msg x processor} once (code in this file) and then we would
	// quickly merge them (makePlan) to actually execute.
	//
	// The current version of this library fully cooks the {msg x processors} plan
	// directly into the Walker instances - there is now no need to separate the
	// {msg x processor} computation.
	for _, proc := range processors {
		t.setCacheEntry(msgD, proc, stringset.New(1), map[protoreflect.MessageDescriptor]*tempCacheEntryBuilder{})
	}
}

// generateCacheEntry returns the tempCacheEntry for this
// message/processor combination.
//
// This will calculate and return the cache entry.
//
// If msg has recursive fields, we may not be able to get the final cache values
// for those fields until we get up one level, so use tmpRet to keep the temporary
// cache values for them.
func (t tempCache) generateCacheEntry(msg protoreflect.MessageDescriptor, processor FieldProcessor, visitedSubMsgs stringset.Set, tmp map[protoreflect.MessageDescriptor]*tempCacheEntryBuilder) *tempCacheEntryBuilder {
	fields := msg.Fields()

	for f := 0; f < fields.Len(); f++ {
		finalRet := true
		field := fields.Get(f)
		value := tempCacheField{
			fieldNum:    field.Number(),
			ProcessAttr: processor.ShouldProcess(field),
		}

		if !value.ProcessAttr.Valid() {
			panic(fmt.Errorf("(%T).ShouldProcess returned invalid ProcessAttr value: %d",
				processor, value.ProcessAttr))
		}

		if field.IsMap() {
			if mapVal := field.MapValue(); mapVal.Kind() == protoreflect.MessageKind {
				if len(t.setCacheEntry(mapVal.Message(), processor, visitedSubMsgs, tmp)) > 0 {
					switch field.MapKey().Kind() {
					case protoreflect.BoolKind:
						value.recurseAttr = recurseMapBool
					case protoreflect.Int32Kind, protoreflect.Int64Kind:
						value.recurseAttr = recurseMapInt
					case protoreflect.Uint32Kind, protoreflect.Uint64Kind:
						value.recurseAttr = recurseMapUint
					case protoreflect.StringKind:
						value.recurseAttr = recurseMapString
					}
				}
			}
		} else if field.Kind() == protoreflect.MessageKind {
			if visitedSubMsgs.Add(string(field.FullName())) {
				if len(t.setCacheEntry(field.Message(), processor, visitedSubMsgs, tmp)) > 0 {
					if field.IsList() {
						value.recurseAttr = recurseRepeated
					} else {
						value.recurseAttr = recurseOne
					}
				}
			} else {
				// Found a recursive message, for example
				// message Outer {
				//	 message Inner {
				//	 	 string value = 1;
				//	   Inner next = 2;
				//	 }
				//   Inner inner = 1;
				// }
				// And we're processing .inner.next.
				// We should not call setCacheEntry for it again because it will
				// get us to an infinite loop.
				// And it will reuse the cache entry for .inner, so we really don't
				// need to call setCacheEntry.
				finalRet = false
				if field.IsList() {
					value.recurseAttr = recurseRepeated
				} else {
					value.recurseAttr = recurseOne
				}

				for _, v := range tmp[field.Message()].ret {
					if v.ProcessAttr != ProcessNever {
						finalRet = true
						break
					}
				}
			}
		}

		// We want an entry in the cache if we have to process the field:
		//   * directly (i.e. processor applies directly to field), OR
		//   * recursively (i.e. the field is a message kind, and that
		//     message contains a field (or another recursion) that we
		//     must follow).
		if value.ProcessAttr != ProcessNever || value.recurseAttr != recurseNone {
			if bdr, ok := tmp[msg]; ok {
				bdr.tmp[value.fieldNum] = value
				if finalRet {
					bdr.ret = append(bdr.ret, value)
					delete(bdr.tmp, value.fieldNum)
				}
			}
		}
	}
	return tmp[msg]
}

// setCacheEntry will ensure that the walker cache is populated for
// `msg` for the given `processor`.
//
// Returns the entry for this message/processor combination.
func (t tempCache) setCacheEntry(msg protoreflect.MessageDescriptor, processor FieldProcessor, visitedSubMsgs stringset.Set, tmp map[protoreflect.MessageDescriptor]*tempCacheEntryBuilder) (ret tempCacheEntry) {
	msgMap := t[msg]
	if msgMap == nil {
		msgMap = map[FieldProcessor]tempCacheEntry{}
		t[msg] = msgMap
	} else {
		var ok bool
		ret, ok = msgMap[processor]
		if ok {
			return
		}
	}

	if _, ok := tmp[msg]; !ok {
		tmp[msg] = newTempCacheEntryBuilder()
	}
	ceb := t.generateCacheEntry(msg, processor, visitedSubMsgs, tmp)
	if len(ceb.tmp) > 0 {
		// We haven't got the final values for some fields.
		// Do not set cache for now.
		return ceb.ret
	}

	if len(ceb.ret) == 0 {
		// The message doesn't need to be processed by the processor.
		return ceb.ret
	}

	ret = ceb.ret
	// for mutually recursive messages, we may actually have already processed
	// this combination by this point - prefer the previous computation.
	if ce, ok := t[msg][processor]; !ok {
		t[msg][processor] = ret
	} else {
		ret = ce
	}

	return
}
