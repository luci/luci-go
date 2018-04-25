// Copyright 2018 The LUCI Authors.
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

package skylarkproto

import (
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/golang/protobuf/proto"
)

var typeRegistry struct {
	m     sync.RWMutex
	types map[reflect.Type]*MessageType
}

// MessageType contains information about the structure of a proto message
//
// It is extracted via reflection from a proto message struct type.
type MessageType struct {
	name       string               // fully qualified proto message name
	ptr        reflect.Type         // ~ *Struct{}
	fields     map[string]fieldDesc // keyed by proto field name
	fieldNames []string             // sorted list of keys in 'fields'
}

// fieldDesc holds type information about some proto field in a message.
type fieldDesc struct {
	typ   reflect.Type                          // type of the value
	value func(msg reflect.Value) reflect.Value // extracts field value from Struct{}
}

// GetMessageType extract type description for protobuf message of given type.
//
// 'typ' is expected to represent a pointer to a protobuf struct, as returned
// by proto.MessageType(...). Returns an error otherwise.
func GetMessageType(typ reflect.Type) (*MessageType, error) {
	typeRegistry.m.RLock()
	cached := typeRegistry.types[typ]
	typeRegistry.m.RUnlock()
	if cached != nil {
		return cached, nil
	}

	zero := reflect.Zero(typ) // (*Struct)(nil)
	name := proto.MessageName(zero.Interface().(proto.Message))
	if name == "" {
		return nil, fmt.Errorf("%q is not a registered proto message type", typ.Name())
	}

	typeRegistry.m.Lock()
	defer typeRegistry.m.Unlock()
	if typeRegistry.types == nil {
		typeRegistry.types = map[reflect.Type]*MessageType{}
	}

	strct := typ.Elem() // Struct{}
	props := proto.GetProperties(strct)

	fields := map[string]fieldDesc{}
	names := []string{}
	for _, prop := range props.Prop {
		prop := prop
		f, ok := strct.FieldByName(prop.Name)
		if !ok {
			panic("property from proto.GetProperties is not defined in the go struct")
		}
		fields[prop.OrigName] = fieldDesc{
			typ: f.Type,
			value: func(msg reflect.Value) reflect.Value {
				return msg.FieldByName(prop.Name)
			},
		}
		names = append(names, prop.OrigName)
	}

	sort.Strings(names)

	newTyp := &MessageType{
		name:       name,
		ptr:        typ,
		fields:     fields,
		fieldNames: names,
	}
	typeRegistry.types[typ] = newTyp
	return newTyp, nil
}

// Name returns fully qualified proto message name.
func (m *MessageType) Name() string {
	return m.name
}

// Type returns proto message type (pointer to a proto struct).
func (m *MessageType) Type() reflect.Type {
	return m.ptr
}
