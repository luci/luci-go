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
	"sort"

	"go.starlark.net/starlark"
)

// messageCtor represents a proto message constructor: it is a callable that
// produces instances of Message.
//
// Instances of messageCtor are created (and put into a module dict) in
// loader.go.
//
// Implements HasAttrs interface. Attributes represent constructors for nested
// messages and values of enums.
type messageCtor struct {
	*starlark.Builtin

	typ   *MessageType
	attrs starlark.StringDict
}

func (b *messageCtor) Type() string {
	return b.Name()
}

func (b *messageCtor) Attr(name string) (starlark.Value, error) {
	return b.attrs[name], nil // (nil, nil) means "not found"
}

func (b *messageCtor) AttrNames() []string {
	keys := make([]string, 0, len(b.attrs))
	for k := range b.attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
