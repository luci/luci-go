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
	"encoding"
	"reflect"
	"runtime"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
)

var (
	protoMessageType = reflect.TypeFor[proto.Message]()
	structPBType     = reflect.TypeFor[*structpb.Struct]()
	textUnmarshalTyp = reflect.TypeFor[encoding.TextUnmarshaler]()
)

func getVisibleFields(typs ...reflect.Type) stringset.Set {
	var ret stringset.Set
	for _, typ := range typs {
		if typ == nil {
			continue
		}
		var set stringset.Set
		if typ.Implements(protoMessageType) {
			set = protoVisibleFieldsOf(typ)
		} else {
			set = jsonToVisibleFieldsOf(typ)
		}
		if ret == nil {
			ret = set
		} else {
			ret = ret.Union(set)
		}
	}
	return ret
}

func checkTypOk(typ reflect.Type) error {
	// proto message first
	if typ.Implements(protoMessageType) {
		return nil
	}

	if typ.Kind() == reflect.Map {
		// map[string]* types
		ktyp := typ.Key()
		kknd := ktyp.Kind()
		if !(kknd == reflect.String || kknd == reflect.Int || ktyp.Implements(textUnmarshalTyp)) {
			return errors.New("used with map with non-string, non-int keys.")
		}
	} else {
		// struct pointer types
		if typ.Kind() != reflect.Pointer || typ.Elem().Kind() != reflect.Struct {
			return errors.New("used with non-struct-pointer type.")
		}
	}

	return nil
}

func registerInImpl(o registerOptions, typIn reflect.Type, namespace string, r *Registry, reg *registration, rprop *registeredProperty) error {
	if err := checkTypOk(typIn); err != nil {
		return err
	}

	reg.typIn = typIn
	reg.unknown = o.unknownFields

	if typIn.Implements(protoMessageType) {
		if typIn != structPBType {
			reg.parseInput = protoFromStruct
		}
	} else {
		reg.parseInput = jsonFromStruct(o.jsonUseNumber, typIn.Kind() != reflect.Map)
	}

	rprop.registry = r
	rprop.namespace = namespace
	rprop.typ = typIn
	return nil
}

func registerOutImpl(o registerOptions, typOut reflect.Type, namespace string, r *Registry, reg *registration, rprop *registeredProperty) error {
	if err := checkTypOk(typOut); err != nil {
		return err
	}

	reg.typOut = typOut

	if typOut.Implements(protoMessageType) {
		if typOut != structPBType {
			reg.serializeOutput = protoToJSON(o.protoUseJSONNames)
		}
	} else {
		reg.serializeOutput = anyToJSON
	}

	rprop.registry = r
	rprop.namespace = namespace
	rprop.typ = typOut
	return nil
}

func (r *Registry) register(o registerOptions, namespace string, reg registration) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.final {
		return errors.New("Registry is already finalized.")
	}

	_, file, line, ok := runtime.Caller(2 + o.skipFrames)
	if !ok {
		file = "UNKNOWN"
		line = 0
	}
	reg.file, reg.line = file, line

	if r.regs == nil {
		r.regs = make(map[string]registration)
	}

	if cur, ok := r.regs[namespace]; ok {
		return errors.Fmt("Registry.register: Namespace %q was already registered at %s:%d",
			namespace, cur.file, cur.line)
	}

	// Check to see if any existing registrations conflict with the new
	// registration.
	if namespace == "" {
		r.topLevelFields = getVisibleFields(reg.typIn, reg.typOut)
		r.topLevelStrict = o.strictTopLevel
		// Registering top-level namespace? Check all other registrations.
		for ns, otherReg := range r.regs {
			if r.topLevelFields.Has(ns) {
				return errors.Fmt("Registry.register: cannot register top-level property namespace - existing entry for %q registered at %s:%d",
					ns, otherReg.file, otherReg.line)
			}
		}
	} else {
		// Registering non-top-level? Check to see if there is a top-level namespace,
		// and if so, does it have the namespace we are trying to register.
		if r.topLevelFields != nil {
			if r.topLevelFields.Has(namespace) {
				topLevel := r.regs[""]
				return errors.Fmt("Registry.register: cannot register namespace %q - top-level property namespace registered at %s:%d has conflicting field",
					namespace, topLevel.file, topLevel.line)
			}
		}
	}
	r.regs[namespace] = reg

	return nil
}
