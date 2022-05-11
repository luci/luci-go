// Copyright 2020 The LUCI Authors.
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

package module

import (
	"fmt"

	"go.chromium.org/luci/common/data/stringset"
)

var registered stringset.Set

// Name is a name of a registered module.
//
// Usually it is a full name of the go package that implements the module, but
// it can be arbitrary as long as it is unique within a server. Attempting to
// register two identical names results in a panic during the server startup.
//
// Names are usually registered during init-time via RegisterName and stored as
// global variables, so they can be referred to in Dependencies() implementation
// of other modules.
type Name struct {
	name string
}

// String returns the name string.
func (n Name) String() string { return n.name }

// RegisterName registers a module name and returns it.
//
// It's the only way to construct new Name instances. Panics if such name was
// already registered.
func RegisterName(name string) Name {
	if registered.Has(name) {
		panic(fmt.Sprintf("module name %q has already been registered", name))
	}
	if registered == nil {
		registered = stringset.New(1)
	}
	registered.Add(name)
	return Name{name}
}

// Dependency represents a dependency on a module.
//
// It can be either required or optional. If a module A declares a require
// dependency on another module B, then A cannot function without B at all. Also
// B will be initialized before A.
//
// If the dependency on B is optional, A will start even if B is not present.
// But if B is present, it will be initialized before A.
//
// Dependency cycles result in an undefined order of initialization.
type Dependency struct {
	name     Name
	required bool
}

// RequiredDependency declares a required dependency.
func RequiredDependency(dep Name) Dependency {
	return Dependency{name: dep, required: true}
}

// OptionalDependency declares an optional dependency.
func OptionalDependency(dep Name) Dependency {
	return Dependency{name: dep, required: false}
}

// Dependency is the name of a module to depend on.
func (d Dependency) Dependency() Name { return d.name }

// Required is true if this is a required dependency.
func (d Dependency) Required() bool { return d.required }
