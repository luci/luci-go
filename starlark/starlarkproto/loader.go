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

package starlarkproto

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"

	"github.com/golang/protobuf/proto"
	descpb "github.com/golang/protobuf/protoc-gen-go/descriptor"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

// LoadProtoModule loads a protobuf module, specified by its full path,
// for example "a/b/c.proto".
// The module should be registered in the process's protobuf descriptors set.
// Returns a dict with single struct named after the proto package.
// It has all message constructors defined inside.
func LoadProtoModule(name string) (starlark.StringDict, error) {
	desc, err := loadFileDesc(name)
	if err != nil {
		return nil, err
	}

	pkg := desc.GetPackage()
	dict := starlark.StringDict{}

	// Add constructors for all messages defined at the package level.
	for _, msg := range desc.MessageType {
		val, err := newMessageCtor(pkg, msg)
		if err != nil {
			return nil, err
		}
		dict[msg.GetName()] = val
	}

	// Inject enum constants for enums defined at the package level.
	for _, enum := range desc.EnumType {
		injectEnumValues(dict, enum)
	}

	return starlark.StringDict{
		pkg: starlarkstruct.FromStringDict(starlark.String(pkg), dict),
	}, nil
}

// newMessageCtor returns a starlark callable that acts as constructor of new
// proto messages (of a type described by the given descriptor) and has
// constructors for the nested messages available as attributes.
func newMessageCtor(ns string, desc *descpb.DescriptorProto) (starlark.Value, error) {
	// Get type descriptor for the proto message.
	name := ns + "." + desc.GetName()
	typ := proto.MessageType(name)
	if typ == nil {
		return nil, fmt.Errorf("could not find %q in the protobuf lib type registry", name)
	}
	t, err := GetMessageType(typ)
	if err != nil {
		return nil, err
	}

	// Constructor function that uses the type to instantiate the messages.
	ctor := starlark.NewBuiltin(name, func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if len(args) != 0 {
			return nil, fmt.Errorf("proto message constructors accept only keyword arguments")
		}
		msg := NewMessage(t)
		for _, kv := range kwargs {
			if len(kv) != 2 {
				panic("expecting key-value pair")
			}
			if err := msg.SetField(string(kv[0].(starlark.String)), kv[1]); err != nil {
				return nil, err
			}
		}
		return msg, nil
	})

	// Attach constructors of the nested messages to it as attributes.
	attrs := starlark.StringDict{}
	for _, nested := range desc.NestedType {
		// map<...> fields are represented by magical map message types. We
		// represent maps using Starlark dicts, so we skip map message types.
		if !nested.GetOptions().GetMapEntry() {
			if attrs[nested.GetName()], err = newMessageCtor(name, nested); err != nil {
				return nil, err
			}
		}
	}

	// Inject enum constants for nested enums.
	for _, enum := range desc.EnumType {
		injectEnumValues(attrs, enum)
	}

	return &messageCtor{
		Builtin: ctor,
		attrs:   attrs,
		typ:     t,
	}, nil
}

// injectEnumValues takes enum constants defined in 'enum' and puts them
// directly into the given dict as integers.
func injectEnumValues(d starlark.StringDict, enum *descpb.EnumDescriptorProto) {
	for _, val := range enum.Value {
		d[val.GetName()] = starlark.MakeInt(int(val.GetNumber()))
	}
}

// loadFileDesc loads a FileDescriptorProto for a given proto module, specified
// by its full path.
//
// The module should be registered in the process's protobuf descriptors set.
func loadFileDesc(name string) (*descpb.FileDescriptorProto, error) {
	gzblob := proto.FileDescriptor(name)
	if gzblob == nil {
		return nil, fmt.Errorf("no such proto file registered")
	}

	r, err := gzip.NewReader(bytes.NewReader(gzblob))
	if err != nil {
		return nil, fmt.Errorf("failed to open gzip reader - %s", err)
	}
	defer r.Close()

	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("failed to uncompress descriptor - %s", err)
	}

	fd := &descpb.FileDescriptorProto{}
	if err := proto.Unmarshal(b, fd); err != nil {
		return nil, fmt.Errorf("malformed FileDescriptorProto - %s", err)
	}

	return fd, nil
}
