// Copyright 2024 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package registry provides a way to register options to cmp.Diff.
//
// This is known to be used by:
//
//   - go.chromium.org/luci/testing/truth/should.{Resemble,Match}
//   - go.chromium.org/luci/testing/typed.{Diff,Got}
//
// By default this includes:
//   - "google.golang.org/protobuf/testing/protocmp".Transform()
//   - A direct comparison of protoreflect.Descriptor types. These are
//     documented as being comparable with `==`, but by default `cmp` will
//     recurse into their guts.
//   - A direct comparison of reflect.Type interfaces.
//   - Functions will compare by their function pointer.
package registry

import (
	"reflect"
	"slices"
	"sync"

	"github.com/google/go-cmp/cmp"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/testing/protocmp"
)

// globalOptionsRegistryMutex is the mutex governing globalOptionsRegistry.
var globalOptionsRegistryMutex sync.Mutex

var comparableInterfaces = map[reflect.Type]bool{
	reflect.TypeFor[protoreflect.FileDescriptor]():      true,
	reflect.TypeFor[protoreflect.MessageDescriptor]():   true,
	reflect.TypeFor[protoreflect.FieldDescriptor]():     true,
	reflect.TypeFor[protoreflect.OneofDescriptor]():     true,
	reflect.TypeFor[protoreflect.EnumDescriptor]():      true,
	reflect.TypeFor[protoreflect.EnumValueDescriptor](): true,
	reflect.TypeFor[protoreflect.ServiceDescriptor]():   true,
	reflect.TypeFor[protoreflect.MethodDescriptor]():    true,

	reflect.TypeFor[reflect.Type](): true,
}

// globalOptionsRegistry is the registry of global options that will get passed to
// typed.Diff and other similar things that eventually call cmp.Diff.
var globalOptionsRegistry = []cmp.Option{
	protocmp.Transform(),
	// This incantation ensures that all protoreflect descriptor instances will be
	// compared with `==`, rather than being recursed into.
	cmp.FilterPath(func(p cmp.Path) bool {
		return comparableInterfaces[p.Last().Type()]
	}, cmp.Comparer(func(a, b any) bool {
		return a == b
	})),
	cmp.FilterPath(func(p cmp.Path) bool {
		return p.Last().Type().Kind() == reflect.Func
	}, cmp.Transformer("func.pointer", func(f any) uintptr {
		if f == nil {
			return 0
		}
		return reflect.ValueOf(f).Pointer()
	})),
}

// RegisterCmpOption registers an option to the registry.
//
// RegisterCmpOption will panic if the option is nil. This function does not
// check for duplicate options because I don't have a good way to do that.
func RegisterCmpOption(opt cmp.Option) {
	if opt == nil {
		panic("cannot register nil option")
	}
	globalOptionsRegistryMutex.Lock()
	defer globalOptionsRegistryMutex.Unlock()
	globalOptionsRegistry = append(globalOptionsRegistry, opt)
}

// GetCmpOptions gets a copy of the registry at the time it was called.
func GetCmpOptions() []cmp.Option {
	globalOptionsRegistryMutex.Lock()
	defer globalOptionsRegistryMutex.Unlock()
	return slices.Clone(globalOptionsRegistry)
}
