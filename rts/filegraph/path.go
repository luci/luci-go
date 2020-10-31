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

// Path is a file path.
type Path []string

func (p Path) String() string {
	return path.Join(p...)
}

// Split returns path to the parent and the base name.
// If p has only one component, returns nil and the component.
// If p is empty, panics.
func (p Path) Split() (parent Path, base string) {
	switch len(p) {
	case 0:
		panic("p is empty")
	case 1:
		return nil, p[0]
	default:
		last := len(p) - 1
		return Path(p[:last]), p[last]
	}
}

// ParsePath parses a slash-separatd relative path.
func ParsePath(slashSeparated string) Path {
	return Path(strings.Split(slashSeparated, "/"))
}
