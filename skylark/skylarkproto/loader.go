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
	"bytes"
	"compress/gzip"
	"fmt"
	"io/ioutil"

	"github.com/golang/protobuf/proto"
	descpb "github.com/golang/protobuf/protoc-gen-go/descriptor"

	"github.com/google/skylark"
	"github.com/google/skylark/skylarkstruct"
)

// LoadProtoModule loads a protobuf module, specified by its full path,
// for example "a/b/c.proto".
// The module should be registered in the process's protobuf descriptors set.
// Returns a dict with single struct named after the proto package.
// It has all message constructors defined inside.
func LoadProtoModule(name string) (skylark.StringDict, error) {
	desc, err := loadFileDesc(name)
	if err != nil {
		return nil, err
	}

	pkg := desc.GetPackage()
	dict := skylark.StringDict{}

	// Add constructors for all messages defined at the package level.
	for _, msg := range desc.MessageType {
		val, err := messageCtor(pkg, msg)
		if err != nil {
			return nil, err
		}
		dict[msg.GetName()] = val
	}

	// Inject enum constants for enums defined at the package level.
	for _, enum := range desc.EnumType {
		injectEnumValues(dict, enum)
	}

	return skylark.StringDict{
		pkg: skylarkstruct.FromStringDict(skylark.String(pkg), dict),
	}, nil
}

// messageCtor returns a skylark callable that acts as constructor of new
// proto messages (of a type described by the given descriptor) and has
// constructors for the nested messages available as attributes.
func messageCtor(ns string, desc *descpb.DescriptorProto) (skylark.Value, error) {
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
	ctor := skylark.NewBuiltin(name, func(_ *skylark.Thread, _ *skylark.Builtin, args skylark.Tuple, kwargs []skylark.Tuple) (skylark.Value, error) {
		if len(args) != 0 {
			return nil, fmt.Errorf("proto message constructors accept only keyword arguments")
		}
		msg := NewMessage(t)
		for _, kv := range kwargs {
			if len(kv) != 2 {
				panic("expecting key-value pair")
			}
			if err := msg.SetField(string(kv[0].(skylark.String)), kv[1]); err != nil {
				return nil, err
			}
		}
		return msg, nil
	})

	// Attach constructors of the nested messages to it as attributes.
	attrs := skylark.StringDict{}
	for _, nested := range desc.NestedType {
		if attrs[nested.GetName()], err = messageCtor(name, nested); err != nil {
			return nil, err
		}
	}

	// Inject enum constants for nested enums.
	for _, enum := range desc.EnumType {
		injectEnumValues(attrs, enum)
	}

	return &builtinWithAttrs{ctor, attrs}, nil
}

// injectEnumValues takes enum constants defined in 'enum' and puts them
// directly into the given dict as integers.
func injectEnumValues(d skylark.StringDict, enum *descpb.EnumDescriptorProto) {
	for _, val := range enum.Value {
		d[val.GetName()] = skylark.MakeInt(int(val.GetNumber()))
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
