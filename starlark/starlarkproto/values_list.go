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

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/reflect/protoreflect"

	"go.chromium.org/luci/starlark/typed"
)

// newStarlarkList returns a new typed.List of an appropriate type.
func newStarlarkList(l *Loader, fd protoreflect.FieldDescriptor) *typed.List {
	list, _ := typed.NewList(converter(l, fd), nil)
	return list
}

// toStarlarkList does protoreflect.List => typed.List conversion.
//
// Panics if type of 'list' (or some of its items) doesn't match 'fd'.
func toStarlarkList(l *Loader, fd protoreflect.FieldDescriptor, list protoreflect.List) *typed.List {
	vals := make([]starlark.Value, list.Len())
	for i := range list.Len() {
		vals[i] = toStarlarkSingular(l, fd, list.Get(i))
	}
	tl, err := typed.NewList(converter(l, fd), vals)
	if err != nil {
		panic(fmt.Errorf("internal error: unexpectedly wrong protoreflect.List %s", list))
	}
	return tl
}

// assignProtoList does m.fd = list, where fd is a repeated field.
//
// Assumes type checks have been done already (this is responsibility of
// prepareRHS). Panics on type mismatch.
func assignProtoList(m protoreflect.Message, fd protoreflect.FieldDescriptor, list *typed.List) {
	protoList := m.Mutable(fd).List()
	if protoList.Len() != 0 {
		panic(fmt.Errorf("internal error: the proto list is not empty"))
	}
	for i := range list.Len() {
		protoList.Append(toProtoSingular(fd, list.Index(i)))
	}
}
