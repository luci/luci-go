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
)

// toStarlarkList does protoreflect.List => starlark.List conversion.
func toStarlarkList(l *Loader, fd protoreflect.FieldDescriptor, list protoreflect.List) (*starlark.List, error) {
	vals := make([]starlark.Value, list.Len())
	for i := 0; i < list.Len(); i++ {
		var err error
		if vals[i], err = toStarlarkSingular(l, fd, list.Get(i)); err != nil {
			return nil, fmt.Errorf("list item #%d: %s", i, err)
		}
	}
	return starlark.NewList(vals), nil
}

// assignProtoList does m.fd = iter, where fd is a repeated field.
func assignProtoList(m protoreflect.Message, fd protoreflect.FieldDescriptor, iter starlark.Iterable) error {
	l := m.Mutable(fd).List()
	if l.Len() != 0 {
		panic("the list is not empty")
	}

	it := iter.Iterate()
	defer it.Done()

	var x starlark.Value
	for it.Next(&x) {
		val, err := toProtoSingular(fd, x)
		if err != nil {
			return fmt.Errorf("list item #%d: %s", l.Len(), err)
		}
		l.Append(val)
	}

	return nil
}
