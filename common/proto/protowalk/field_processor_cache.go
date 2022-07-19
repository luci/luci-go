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
	"sync"

	"google.golang.org/protobuf/reflect/protoreflect"
)

// fieldProcessorSelectors maps from all the registered processors to an
// id, which can be used with registeredFieldProcessorsID to find the callback
// function and data type, and will be used internally in caches.
var (
	fieldProcessorSelectors   map[reflect.Type]FieldSelector
	fieldProcessorSelectorsMu sync.RWMutex
)

// fieldProcessorCacheKey is the key for the global field processor cache
type fieldProcessorCacheKey struct {
	message    protoreflect.FullName
	processorT reflect.Type
}

// fieldProcessorCacheValue is a single cache value for the global field
// processor cache.
type fieldProcessorCacheValue struct {
	// fieldNum is the field 'tag number' in the proto message for the
	// corresponding field. We store this instead of the FieldDescriptor because
	// it's only 4 bytes, rather than a 16+byte interface.
	fieldNum protoreflect.FieldNumber

	// All cacheable attributes
	ProcessAttr
	recurseAttr
}

// fieldProcessorCacheEntry corresponds to a single Message+FieldProcessor
// combination.
//
// Each value is the combined result of asking FieldProcessor.ShouldProcess on the
// field, plus any recursion attribute (if this field is a Message (single,
// repeated or map)) which recursively contains fields which need to be
// processed by this FieldProcessor.
//
// This is always kept ordered by field number, and is immutable.
type fieldProcessorCacheEntry []fieldProcessorCacheValue

// globalFieldProcessorCache maps (Message+FieldProcessor) combinations to
// a (possibly empty) slice which indicates which fields need to be processed
// and/or recursed by the FieldProcessor.
var globalFieldProcessorCache = map[fieldProcessorCacheKey]fieldProcessorCacheEntry{}
var globalFieldProcessorCacheMu sync.RWMutex

// resetGlobalFieldProcessorCache is only used in tests.
func resetGlobalFieldProcessorCache() {
	globalFieldProcessorCacheMu.Lock()
	defer globalFieldProcessorCacheMu.Unlock()
	for k := range globalFieldProcessorCache {
		delete(globalFieldProcessorCache, k)
	}
}

// generateCacheEntry returns the fieldProcessorCacheEntry for this
// message/processor combination.
//
// This will calculate and return the cache entry.
func generateCacheEntry(msg protoreflect.MessageDescriptor, processor *procBundle) (ret fieldProcessorCacheEntry) {
	fields := msg.Fields()
	for f := 0; f < fields.Len(); f++ {
		field := fields.Get(f)
		value := fieldProcessorCacheValue{
			fieldNum:    field.Number(),
			ProcessAttr: processor.sel(field),
		}

		if !value.ProcessAttr.Valid() {
			panic(fmt.Errorf("(%T).ShouldProcess returned invalid ProcessAttr value: %d",
				processor, value.ProcessAttr))
		}

		if field.IsMap() {
			if mapVal := field.MapValue(); mapVal.Kind() == protoreflect.MessageKind {
				if len(setCacheEntry(mapVal.Message(), processor)) > 0 {
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
			if len(setCacheEntry(field.Message(), processor)) > 0 {
				if field.IsList() {
					value.recurseAttr = recurseRepeated
				} else {
					value.recurseAttr = recurseOne
				}
			}
		}

		// We want an entry in the cache if we have to process the field:
		//   * directly (i.e. processor applies directly to field), OR
		//   * recursively (i.e. the field is a message kind, and that
		//     message contains a field (or another recursion) that we
		//     must follow).
		if value.ProcessAttr != ProcessNever || value.recurseAttr != recurseNone {
			ret = append(ret, value)
		}
	}

	return
}

// setCacheEntry will ensure that globalFieldProcessorCache is populated for
// `msg` for the given `processor`.
//
// Returns the entry for this message/processor combination.
func setCacheEntry(msg protoreflect.MessageDescriptor, processor *procBundle) (ret fieldProcessorCacheEntry) {
	key := fieldProcessorCacheKey{
		message:    msg.FullName(),
		processorT: processor.proc,
	}

	globalFieldProcessorCacheMu.RLock()
	ret, ok := globalFieldProcessorCache[key]
	globalFieldProcessorCacheMu.RUnlock()
	if ok {
		return
	}

	ret = generateCacheEntry(msg, processor)

	globalFieldProcessorCacheMu.Lock()
	if ce, ok := globalFieldProcessorCache[key]; !ok {
		globalFieldProcessorCache[key] = ret
	} else {
		ret = ce
	}
	globalFieldProcessorCacheMu.Unlock()

	return
}
