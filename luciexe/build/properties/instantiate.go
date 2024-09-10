// Copyright 2024 The LUCI Authors.
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

package properties

import (
	"reflect"

	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/errors"
)

// Instantiate generates a new State from this Registry.
//
// Input values will be populated from `input`, if provided.
//
// If `notify` is provided, it will be invoked every time an output property in
// the State changes.
//
// The version will be monotonically increasing - this version harmonizes with
// the version returned by [State.Serialize], which is intended to allow
// implementations to not process stale notifications.
//
// The callback is invoked outside of a mutex, but should still execute
// quickly (e.g. pushing something to a non-blocking channel, such as
// a [go.chromium.org/luci/common/sync/dispatcher.Channel]). Because it is
// called outside of a mutex, you may see many notifications with out-of-order
// version numbers.
//
// This will finalize the registry, preventing any new registrations. It is
// valid to generate multiple States from one Registry - they will all operate
// independently. This characteristic is especially useful in tests, because you
// can create a single Registry with a single set of registered properties, and
// then generate a new State for each test case.
func (r *Registry) Instantiate(input *structpb.Struct, notify func(version int64)) (*State, error) {
	r.mu.Lock()
	r.final = true
	r.mu.Unlock()

	s := &State{
		registry:    r,
		outputState: make(map[string]*outputPropertyState, len(r.regs)),
		notifyFunc:  notify,
	}

	var err error
	s.initialData, err = r.parseInitialState(input)
	if err != nil {
		return nil, err
	}

	// handle output state
	for namespace, reg := range r.regs {
		if reg.typOut != nil {
			ps := &outputPropertyState{
				toJSON: reg.serializeOutput, // will be nil if typ == structPBType
				data:   makeOutType(reg.typOut),
			}
			s.outputState[namespace] = ps
		}
	}

	return s, nil
}

func (r *Registry) parseInitialState(input *structpb.Struct) (map[string]any, error) {
	if input == nil {
		return nil, nil
	}

	ret := make(map[string]any, len(r.regs))

	decode := func(reg registration, namespace string, sval *structpb.Struct) error {
		if reg.parseInput != nil {
			// We need to convert *Struct to the native type, allocate a new native
			// type and then transform *Struct into this.

			var err error
			if reg.typIn.Kind() == reflect.Map {
				// Map is tricky - parseInput requires *map, but we only want to retain
				// the actual map, not a pointer to it.
				mapPtr := reflect.New(reg.typIn)
				mapPtr.Elem().Set(reflect.MakeMap(reg.typIn))
				if err = reg.parseInput(sval, mapPtr.Interface()); err == nil {
					ret[namespace] = mapPtr.Elem().Interface()
				}
			} else {
				// For structs we just use *struct all the way through.
				structPtr := reflect.New(reg.typIn.Elem()).Interface()
				if err = reg.parseInput(sval, structPtr); err == nil {
					ret[namespace] = structPtr
				}
			}
			if err != nil {
				fmtBit := "[%q]"
				name := namespace
				if name == "" {
					fmtBit = "[%s]"
					name = "top-level"
				}
				return errors.Annotate(err, "Registry.Initialize"+fmtBit, name).Err()
			}
		} else {
			// The native type is *structpb.Struct, so pass it through directly.
			ret[namespace] = sval
		}
		return nil
	}

	var myStruct *structpb.Struct
	myStruct = &structpb.Struct{Fields: make(map[string]*structpb.Value, len(input.Fields))}
	for namespace, v := range input.Fields {
		myStruct.Fields[namespace] = v
	}

	var topLevelReg *registration
	for namespace, reg := range r.regs {
		if reg.typIn != nil {
			if namespace == "" {
				// we will process the top level property last.
				topLevelReg = &reg
				continue
			}

			// see if this input had any data?
			val, ok := myStruct.Fields[namespace]
			if !ok {
				continue
			}
			// Remove this from myStruct - everything left over will be parsed by the
			// top-level regestered property.
			delete(myStruct.Fields, namespace)

			// In all cases, the target value should be a Struct - get it and then parse
			// it into ret[namespace].
			if sval := val.GetStructValue(); sval != nil {
				if err := decode(reg, namespace, sval); err != nil {
					return nil, err
				}
			} else {
				return nil, errors.Reason(
					"properties.Registry.Instantiate - input[%q] - input is not Struct (got %T)", namespace, val.GetKind()).Err()
			}
		}
	}

	if leftover := len(myStruct.Fields); leftover > 0 {
		if topLevelReg != nil {
			if err := decode(*topLevelReg, "", myStruct); err != nil {
				return nil, err
			}
		} else {
			return nil, errors.Reason(
				"properties.Registry.Instantiate - %d leftover top-level properties and no top-level property registered.",
				leftover).Err()
		}
	}

	return ret, nil
}

func makeOutType(typ reflect.Type) any {
	var ret any
	if typ.Kind() == reflect.Map {
		ret = reflect.MakeMap(typ).Interface()
	} else {
		ret = reflect.New(typ.Elem()).Interface()
	}

	if typ == structPBType {
		ret.(*structpb.Struct).Fields = make(map[string]*structpb.Value, 0)
	}
	return ret
}
