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
	"sort"

	"github.com/google/skylark"
)

// builtinWithAttrs decorates a Builtin by adding attributes to it and changing
// its type name to match the builtin name.
//
// Implements HasAttrs interface.
type builtinWithAttrs struct {
	*skylark.Builtin
	attrs skylark.StringDict
}

func (b *builtinWithAttrs) Type() string {
	return b.Name()
}

func (b *builtinWithAttrs) Attr(name string) (skylark.Value, error) {
	return b.attrs[name], nil // (nil, nil) means "not found"
}

func (b *builtinWithAttrs) AttrNames() []string {
	keys := make([]string, 0, len(b.attrs))
	for k := range b.attrs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
