// Copyright 2019 The LUCI Authors.
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
	"fmt"
	"strings"
	"sync"
	"sync/atomic"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
)

// Loader can instantiate Starlark values that correspond to proto messages.
//
// Holds a pool of descriptors that describe all available proto types. Use
// AddDescriptorSet to seed it. Once seeded, use Module to get a Starlark module
// with symbols defined in some registered `*.proto` file.
//
// Loader is also a Starlark value itself, with the following methods:
//   - add_descriptor_set(ds) - see AddDescriptorSet.
//   - module(path) - see Module.
//
// Can be used concurrently. Non-freezable.
type Loader struct {
	m sync.RWMutex

	files *protoregistry.Files
	types *protoregistry.Types

	dsets   map[*DescriptorSet]struct{}
	mtypes  map[protoreflect.MessageDescriptor]*MessageType
	modules map[string]*starlarkstruct.Module // *.proto file => its top-level symbols

	hash uint32 // unique (within the process) value, used by Hash()
}

// loaderHash is used to give each instance of *Loader its own unique non-reused
// hash value for Hash() method.
var loaderHash uint32 = 1000

// NewLoader instantiates a new loader with empty proto registry.
func NewLoader() *Loader {
	return &Loader{
		files:   &protoregistry.Files{},
		types:   &protoregistry.Types{},
		dsets:   make(map[*DescriptorSet]struct{}, 0),
		mtypes:  make(map[protoreflect.MessageDescriptor]*MessageType, 0),
		modules: make(map[string]*starlarkstruct.Module, 0),
		hash:    atomic.AddUint32(&loaderHash, 1),
	}
}

// Key of the default *Loader in starlark.Thread local store.
const threadLoaderKey = "starlarkproto.Loader"

// DefaultLoader returns a loader installed in the thread via SetDefaultLoader.
//
// Returns nil if there's no default loader.
func DefaultLoader(th *starlark.Thread) *Loader {
	l, _ := th.Local(threadLoaderKey).(*Loader)
	return l
}

// SetDefaultLoader installs the given loader as default in the thread.
//
// It can be obtained via DefaultLoader or proto.default_loader() from Starlark.
// Note that Starlark code has no way of changing the default loader. It's
// responsibility of the hosting environment to prepare the default loader
// (just like it prepares starlark.Thread itself).
func SetDefaultLoader(th *starlark.Thread, l *Loader) {
	th.SetLocal(threadLoaderKey, l)
}

// Types returns a registry for looking up or iterating over descriptor types.
func (l *Loader) Types() *protoregistry.Types {
	return l.types
}

// AddDescriptorSet makes all *.proto files defined in the given descriptor set
// and all its dependencies available for use from Starlark.
//
// AddDescriptorSet is idempotent in a sense that calling AddDescriptorSet(ds)
// multiple times with the exact same 'ds' is not an error. But trying to
// register a proto file through multiple different descriptor sets is an error.
func (l *Loader) AddDescriptorSet(ds *DescriptorSet) error {
	l.m.Lock()
	defer l.m.Unlock()
	return l.addDescriptorSetLocked(ds)
}

// addDescriptorSetLocked implements AddDescriptorSet.
func (l *Loader) addDescriptorSetLocked(ds *DescriptorSet) error {
	if _, ok := l.dsets[ds]; ok {
		return nil
	}
	for _, dep := range ds.deps {
		if err := l.addDescriptorSetLocked(dep); err != nil {
			return fmt.Errorf("%q: %s", ds.name, err)
		}
	}
	for _, fd := range ds.fdps {
		if err := l.addDescriptorLocked(fd); err != nil {
			return fmt.Errorf("%q: %s", ds.name, err)
		}
	}
	l.dsets[ds] = struct{}{}
	return nil
}

// addDescriptor adds a single deserialized FileDescriptorProto.
func (l *Loader) addDescriptorLocked(fd *descriptorpb.FileDescriptorProto) error {
	// Load the file descriptor, resolving all references through 'res' which
	// will capture unresolved ones. Note that per comments in protodesc/desc.go,
	// there would be an option to tell protodesc.NewFile to make this check
	// natively.
	res := &resolver{r: l.files}
	f, err := protodesc.NewFile(fd, res)
	if err != nil {
		return fmt.Errorf("resolving imports in %s: %s", fd.GetName(), err)
	}

	switch {
	case len(res.files) != 0:
		return fmt.Errorf(
			"compiled proto file %s refers to undefined files: %s",
			fd.GetName(), strings.Join(res.files, ", "))
	case len(res.descs) != 0:
		return fmt.Errorf(
			"compiled proto file %s refers to undefined descriptors: %s",
			fd.GetName(), strings.Join(res.descs, ", "))
	}

	if err := l.files.RegisterFile(f); err != nil {
		return fmt.Errorf("registering %s: %s", fd.GetName(), err)
	}

	// TODO(vadimsh): Populate l.types somehow. It is used by encoders/decoders
	// to handle google.protobuf.Any fields (which we currently do not support).

	return nil
}

// resolver wraps protodesc.Resolver by capturing unresolved references.
type resolver struct {
	r protodesc.Resolver

	files []string // unresolvable files
	descs []string // unresolvable descriptors
}

func (r *resolver) FindFileByPath(p string) (protoreflect.FileDescriptor, error) {
	d, err := r.r.FindFileByPath(p)
	if err == protoregistry.NotFound {
		r.files = append(r.files, p)
	}
	return d, err
}

func (r *resolver) FindDescriptorByName(n protoreflect.FullName) (protoreflect.Descriptor, error) {
	d, err := r.r.FindDescriptorByName(n)
	if err == protoregistry.NotFound {
		r.descs = append(r.descs, string(n))
	}
	return d, err
}

// Module returns a module with top-level definitions from some *.proto file.
//
// The descriptor of this proto file should be registered already via
// AddDescriptorSet. 'path' here is matched to what's in the descriptor, which
// is a path to *.proto EXACTLY as it was given to 'protoc'.
//
// The name of the module matches the proto package name (per 'package ...'
// statement in the proto file).
func (l *Loader) Module(path string) (*starlarkstruct.Module, error) {
	// Lookup in the cache under the reader lock.
	mod, desc, err := func() (*starlarkstruct.Module, protoreflect.FileDescriptor, error) {
		l.m.RLock()
		defer l.m.RUnlock()
		if mod := l.modules[path]; mod != nil {
			return mod, nil, nil
		}
		desc, err := l.files.FindFileByPath(path)
		if err != nil {
			return nil, nil, fmt.Errorf("loading %s: %s", path, err)
		}
		return nil, desc, nil
	}()
	if mod != nil || err != nil {
		return mod, err
	}

	l.m.Lock()
	defer l.m.Unlock()

	// Populate the module dict with top-level symbols in the file.
	mod = &starlarkstruct.Module{
		Name:    string(desc.Package()),
		Members: starlark.StringDict{},
	}
	l.injectMessageTypesLocked(mod.Members, desc.Messages())
	l.injectEnumValuesLocked(mod.Members, desc.Enums())

	l.modules[path] = mod
	return mod, nil
}

// MessageType creates new (or returns existing) MessageType.
//
// The return value can be used to instantiate Starlark values via Message() or
// MessageFromProto(m).
func (l *Loader) MessageType(desc protoreflect.MessageDescriptor) *MessageType {
	l.m.RLock()
	mt := l.mtypes[desc]
	l.m.RUnlock()
	if mt != nil {
		return mt
	}

	l.m.Lock()
	defer l.m.Unlock()
	return l.initMessageTypeLocked(desc)
}

// initMessageTypeLocked creates *MessageType if it didn't exist before.
func (l *Loader) initMessageTypeLocked(desc protoreflect.MessageDescriptor) *MessageType {
	if typ := l.mtypes[desc]; typ != nil {
		return typ
	}

	typ := &MessageType{
		loader: l,
		desc:   desc,
		attrs:  starlark.StringDict{},
	}
	typ.initLocked()

	// Constructor function that uses `typ` to instantiate messages.
	typ.Builtin = starlark.NewBuiltin(typ.Type(), func(_ *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
		if len(args) != 0 {
			return nil, fmt.Errorf("proto message constructors accept only keyword arguments")
		}
		msg := typ.Message()
		for _, kv := range kwargs {
			if err := msg.SetField(string(kv[0].(starlark.String)), kv[1]); err != nil {
				return nil, err
			}
		}
		return msg, nil
	})

	// Inject nested symbols.
	l.injectMessageTypesLocked(typ.attrs, desc.Messages())
	l.injectEnumValuesLocked(typ.attrs, desc.Enums())

	l.mtypes[desc] = typ
	return typ
}

// injectMessageTypesLocked instantiates constructors for messages in 'msgs' and
// adds them to the dict 'd'.
func (l *Loader) injectMessageTypesLocked(d starlark.StringDict, msgs protoreflect.MessageDescriptors) {
	for i := 0; i < msgs.Len(); i++ {
		desc := msgs.Get(i)
		// map<...> fields are represented by magical map message types. We do not
		// expose them on Starlark level and represent maps as dicts instead.
		if !desc.IsMapEntry() {
			d[string(desc.Name())] = l.initMessageTypeLocked(desc)
		}
	}
}

// injectEnumValuesLocked takes enum constants defined in 'enums' and puts them
// directly into the given dict as integers.
func (l *Loader) injectEnumValuesLocked(d starlark.StringDict, enums protoreflect.EnumDescriptors) {
	for i := 0; i < enums.Len(); i++ {
		vals := enums.Get(i).Values()
		for j := 0; j < vals.Len(); j++ {
			val := vals.Get(j)
			d[string(val.Name())] = starlark.MakeInt(int(val.Number()))
		}
	}
}

// Implementation of starlark.Value and starlark.HasAttrs.

// String returns str(...) representation of the loader.
func (l *Loader) String() string {
	return fmt.Sprintf("proto.Loader(0x%x)", l.hash)
}

// Type returns "proto.Loader".
func (l *Loader) Type() string {
	return "proto.Loader"
}

// Freeze is noop for now.
func (l *Loader) Freeze() {}

// Truth returns True.
func (l *Loader) Truth() starlark.Bool { return starlark.True }

// Hash returns an integer assigned to this loader when it was created.
func (l *Loader) Hash() (uint32, error) { return l.hash, nil }

// AtrrNames lists available attributes.
func (l *Loader) AttrNames() []string {
	return []string{
		"add_descriptor_set",
		"module",
	}
}

// Attr returns an attribute given its name (or nil if not present).
func (l *Loader) Attr(name string) (starlark.Value, error) {
	switch name {
	case "add_descriptor_set":
		return addDescSetBuiltin.BindReceiver(l), nil
	case "module":
		return moduleBuiltin.BindReceiver(l), nil
	default:
		return nil, nil
	}
}

// Shims for calling Loader methods from Starlark.

var addDescSetBuiltin = starlark.NewBuiltin("add_descriptor_set", func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var ds *DescriptorSet
	if err := starlark.UnpackPositionalArgs("add_descriptor_set", args, kwargs, 1, &ds); err != nil {
		return nil, err
	}
	return starlark.None, b.Receiver().(*Loader).AddDescriptorSet(ds)
})

var moduleBuiltin = starlark.NewBuiltin("module", func(_ *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path string
	if err := starlark.UnpackPositionalArgs("module", args, kwargs, 1, &path); err != nil {
		return nil, err
	}
	return b.Receiver().(*Loader).Module(path)
})
