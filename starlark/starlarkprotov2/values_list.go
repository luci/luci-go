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

package starlarkprotov2

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
func toStarlarkList(l *Loader, fd protoreflect.FieldDescriptor, list protoreflect.List) (*typed.List, error) {
	vals := make([]starlark.Value, list.Len())
	for i := 0; i < list.Len(); i++ {
		var err error
		if vals[i], err = toStarlarkSingular(l, fd, list.Get(i)); err != nil {
			return nil, fmt.Errorf("list item #%d: %s", i, err)
		}
	}
	return typed.NewList(converter(l, fd), vals)
}

// assignProtoList does m.fd = list, where fd is a repeated field.
//
// Assumes type checks have been done already (this is responsibility of
// prepareRHS). Panics on type mismatch.
func assignProtoList(m protoreflect.Message, fd protoreflect.FieldDescriptor, list *typed.List) {
	protoList := m.Mutable(fd).List()
	if protoList.Len() != 0 {
		panic("the proto list is not empty")
	}
	for i := 0; i < list.Len(); i++ {
		protoList.Append(toProtoSingular(fd, list.Index(i)))
	}
}
