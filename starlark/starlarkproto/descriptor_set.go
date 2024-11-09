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
	"sync/atomic"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/types/descriptorpb"
)

// DescriptorSet contains FileDescriptorProto of zero or more *.proto files,
// along with pointers to DescriptorSets with their imports.
//
// A descriptor set can be registered in a Loader. Doing so transitively
// registers all its dependencies. See Loader.AddDescriptorSet for more details.
//
// Implements starlark.Value and starlark.HasAttrs interfaces. Usage from
// Starlark side may look like this:
//
//	load(".../wellknown_descpb.star", "wellknown_descpb")
//	myprotos_descpb = proto.new_descriptor_set(
//	    name = "myprotos",
//	    deps = [wellknown_descpb],
//	    blob = io.read_file("myprotos.descpb"),
//	)
//	myprotos_descpb.register()
//
// By default register() registers the descriptor set in the default loader,
// i.e. ds.register() is same as ds.register(loader=proto.default_loader()).
// Also note that ds.register(loader=l) is a sugar for l.add_descriptor_set(ds),
// so ds.register() is same as proto.default_loader().add_descriptor_set(ds),
// just much shorter.
type DescriptorSet struct {
	name string // for debug messages and errors, can be anything
	hash uint32 // unique (within the process) value, used by Hash()

	fdps []*descriptorpb.FileDescriptorProto // files in this set
	deps []*DescriptorSet                    // direct dependencies of this set
}

// descSetHash is used to give each instance of *DescriptorSet its own unique
// non-reused hash value for Hash() method.
var descSetHash uint32 = 10000

// NewDescriptorSet evaluates given file descriptors and their dependencies and
// produces new DescriptorSet if there are no unresolved imports and no
// duplicated files.
//
// 'fdps' should be ordered topologically (i.e. if file A imports file B, then
// B should precede A in 'fdps' or be somewhere among 'deps'). This is always
// the case when generating sets via 'protoc --descriptor_set_out=...'.
//
// Note that dependencies can only be specified when creating the descriptor
// set and can't be changed later. Cycles thus are impossible.
func NewDescriptorSet(name string, fdps []*descriptorpb.FileDescriptorProto, deps []*DescriptorSet) (*DescriptorSet, error) {
	ds := &DescriptorSet{
		name: name,
		hash: atomic.AddUint32(&descSetHash, 1),
		fdps: append([]*descriptorpb.FileDescriptorProto(nil), fdps...),
		deps: append([]*DescriptorSet(nil), deps...),
	}

	// Collect ALL dependencies (most nested first). Note that diamond dependency
	// diagram is fine, but the same *.proto file registered in different sets is
	// not fine: this will be checked below.

	topo := make([]*DescriptorSet, 0, len(deps))
	seen := make(map[*DescriptorSet]struct{}, len(deps))

	var visitDep func(d *DescriptorSet)
	visitDep = func(d *DescriptorSet) {
		if _, ok := seen[d]; !ok {
			seen[d] = struct{}{}
			for _, dep := range d.deps {
				visitDep(dep)
			}
			topo = append(topo, d)
		}
	}
	visitDep(ds)

	// Verify each *.proto file is described by exactly one set, and at the moment
	// it is added (based on topological order), all its dependencies are already
	// present.

	files := map[string]*DescriptorSet{} // *.proto path => DescriptoSet it is defined in
	for _, dep := range topo {
		for _, fd := range dep.fdps {
			pname := fd.GetName()
			if ds := files[pname]; ds != nil {
				return nil, fmt.Errorf("conflict between descriptor sets %q and %q: both define %q", dep.name, ds.name, pname)
			}
			for _, imp := range fd.GetDependency() {
				if files[imp] == nil {
					return nil, fmt.Errorf("in descriptor set %q: %q imports unknown %q", dep.name, pname, imp)
				}
			}
			files[pname] = dep
		}
	}

	return ds, nil
}

// Implementation of starlark.Value and starlark.HasAttrs.

// String returns str(...) representation of the set, for debug messages.
func (ds *DescriptorSet) String() string { return fmt.Sprintf("proto.DescriptorSet(%q)", ds.name) }

// Type returns a short string describing the value's type.
func (ds *DescriptorSet) Type() string { return "proto.DescriptorSet" }

// Freeze does nothing since DescriptorSet is already immutable.
func (ds *DescriptorSet) Freeze() {}

// Truth returns the truth value of an object.
func (ds *DescriptorSet) Truth() starlark.Bool { return starlark.True }

// Hash returns unique value associated with this set.
func (ds *DescriptorSet) Hash() (uint32, error) { return ds.hash, nil }

// AtrrNames lists available attributes.
func (ds *DescriptorSet) AttrNames() []string {
	return []string{
		"register",
	}
}

// Attr returns an attribute given its name (or nil if not present).
func (ds *DescriptorSet) Attr(name string) (starlark.Value, error) {
	switch name {
	case "register":
		return dsRegisterBuiltin.BindReceiver(ds), nil
	default:
		return nil, nil
	}
}

// Implementation of DescriptorSet Starlark methods.

var dsRegisterBuiltin = starlark.NewBuiltin("register", func(th *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var loader starlark.Value
	if err := starlark.UnpackArgs("register", args, kwargs, "loader?", &loader); err != nil {
		return nil, err
	}
	if loader == nil || loader == starlark.None {
		loader = DefaultLoader(th)
		if loader == nil {
			return nil, fmt.Errorf("register: loader is None and there's no default loader")
		}
	}
	l, ok := loader.(*Loader)
	if !ok {
		return nil, fmt.Errorf("register: for parameter \"loader\": got %s, want proto.Loader", l.Type())
	}
	return starlark.None, l.AddDescriptorSet(b.Receiver().(*DescriptorSet))
})
