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

// Name is a name of a Node.
// The name components are base names of nodes from the root to the node,
// e.g. ["a", "b", "c"] represents "a/b/c".
type Name []string

// ParseName parses a slash-separatd relative path.
// It is the opposite of (Name).String().
func ParseName(slashSeparated string) Name {
	return Name(strings.Split(slashSeparated, "/"))
}

// String returns a slash-separated name, e.g. ["a/b/c"] => "a/b/c".
// It is the opposite of ParseName().
func (n Name) String() string {
	return path.Join(n...)
}
