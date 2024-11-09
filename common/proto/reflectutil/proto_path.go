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

package reflectutil

import (
	"fmt"
	"strings"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/common/errors"
)

// PathItem is a single step in a Path.
type PathItem interface {
	fmt.Stringer

	// Retrieve applies this PathItem to the given Value.
	//
	// If it fails, returns a Value where v.IsValid() == false.
	Retrieve(protoreflect.Value) protoreflect.Value
}

// PathField is a PathItem which indicates field access in a Message.
type PathField struct{ protoreflect.FieldDescriptor }

var _ PathItem = PathField{}

// Retrieve implements PathItem
func (f PathField) Retrieve(v protoreflect.Value) (ret protoreflect.Value) {
	if msg, ok := v.Interface().(protoreflect.Message); ok {
		ret = msg.Get(f.FieldDescriptor)
	}
	return
}

// String implements fmt.Stringer.
//
// This makes the most sense when used with Path.String(); this formats as
// `.proto_field_name`..
func (f PathField) String() string {
	return fmt.Sprintf(".%s", f.FieldDescriptor.Name())
}

// PathListIdx is a PathItem which indicates the indexing of
// a repeated field.
type PathListIdx int

var _ PathItem = PathListIdx(0)

// Retrieve implements PathItem
func (i PathListIdx) Retrieve(v protoreflect.Value) (ret protoreflect.Value) {
	if lst, ok := v.Interface().(protoreflect.List); ok {
		ret = lst.Get(int(i))
	}
	return
}

// String implements fmt.Stringer.
//
// This makes the most sense when used with Path.String(); this formats as
// `[index_value]`.
func (i PathListIdx) String() string {
	return fmt.Sprintf("[%d]", i)
}

// PathMapKey is a PathItem which indicates the indexing of
// a map field.
type PathMapKey protoreflect.MapKey

var _ PathItem = PathMapKey{}

// Retrieve implements PathItem
func (k PathMapKey) Retrieve(v protoreflect.Value) (ret protoreflect.Value) {
	if m, ok := v.Interface().(protoreflect.Map); ok {
		ret = m.Get(protoreflect.MapKey(k))
	}
	return
}

// String implements fmt.Stringer.
//
// This makes the most sense when used with Path.String(); this formats as
// `[map_key]` for integer/bool types, and `["map_key"]` for string types.
func (k PathMapKey) String() string {
	mk := protoreflect.MapKey(k)
	if strk, ok := mk.Interface().(string); ok {
		return fmt.Sprintf("[%q]", strk)
	}
	return fmt.Sprintf("[%s]", mk.String())
}

// MustMakePathItem will return a PathItem from `item` or panic.
//
// `item` can be a:
//   - int                          (PathListIdx)
//   - protoreflect.FieldDescriptor (PathField)
//   - protoreflect.MapKey          (PathMapKey)
func MustMakePathItem(item any) PathItem {
	switch x := item.(type) {
	case int:
		return PathListIdx(x)
	case protoreflect.FieldDescriptor:
		return PathField{x}
	case protoreflect.MapKey:
		return PathMapKey(x)
	}
	panic(errors.Reason("unknown proto path item: %T", item).Err())
}

// Path is a series of tokens to indicate some (possibly nested) field
// within a Message.
type Path []PathItem

// String implements fmt.Stringer.
//
// Formats the Path in a way that makes sense to humans.
func (p Path) String() string {
	bld := strings.Builder{}
	for _, itm := range p {
		bld.WriteString(itm.String())
	}
	return bld.String()
}

// Retrieve applies this Path to a proto.Message, returning the Value
// indicated.
//
// If the Path doesn't match a path in `msg` (i.e. the Path came from
// a different type of Message), returns an invalid Value.
func (p Path) Retrieve(msg proto.Message) protoreflect.Value {
	v := protoreflect.ValueOf(msg.ProtoReflect())

	for _, itm := range p {
		v = itm.Retrieve(v)
	}

	return v
}
