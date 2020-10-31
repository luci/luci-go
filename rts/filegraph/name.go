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

package filegraph

import (
	"path"
	"strings"
)

// Name is a name of a graph node, which could be a file or a directory.
// The name components are base names of nodes from the root to the node,
// e.g. ["a", "b", "c"] represents "a/b/c".
type Name []string

// String returns a slash-separated name, e.g. ["a/b/c"] => "a/b/c".
func (n Name) String() string {
	return path.Join(n...)
}

// Split returns the parent's name and the base name.
// If n has only one component, returns nil and the component.
// If n is empty, panics.
func (n Name) Split() (parent Name, base string) {
	switch len(n) {
	case 0:
		panic("n is empty")
	case 1:
		return nil, n[0]
	default:
		last := len(n) - 1
		return Name(n[:last]), n[last]
	}
}

// ParseName parses a slash-separatd relative path.
func ParseName(slashSeparated string) Name {
	return Name(strings.Split(slashSeparated, "/"))
}
