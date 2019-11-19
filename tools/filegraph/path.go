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

package main

import (
	"path"
	"strings"
)

// Path is a path to a file with a repository.
// It is an ordered list of path components relative to the repository root.
type Path []string

// String returns string representation of the path, where components are
// separated with forward slashes.
func (p Path) String() string {
	return path.Join(p...)
}

// ParsePath parses a slash-separatd path.
func ParsePath(slashSeparated string) Path {
	return Path(strings.Split(slashSeparated, "/"))
}
