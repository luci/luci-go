// Copyright 2022 The LUCI Authors.
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

package model

import (
	"context"

	"go.chromium.org/luci/bisection/util"
)

// Compile Failure Signal represents signal extracted from compile failure log.
type CompileFailureSignal struct {
	Nodes []string
	Edges []*CompileFailureEdge
	// A map of {<file_path>:[lines]} represents failure positions in source file
	Files map[string][]int
	// A map of {<dependency_file_name>:[<list of dependencies>]}. Used to improve
	// the speed when we do dependency analysis
	DependencyMap map[string][]string
}

// CompileFailureEdge represents a failed edge in ninja failure log
type CompileFailureEdge struct {
	Rule         string // Rule is like CXX, CC...
	OutputNodes  []string
	Dependencies []string
}

func (c *CompileFailureSignal) AddLine(filePath string, line int) {
	c.AddFilePath(filePath)
	for _, l := range c.Files[filePath] {
		if l == line {
			return
		}
	}
	c.Files[filePath] = append(c.Files[filePath], line)
}

func (c *CompileFailureSignal) AddFilePath(filePath string) {
	if c.Files == nil {
		c.Files = map[string][]int{}
	}
	_, exist := c.Files[filePath]
	if !exist {
		c.Files[filePath] = []int{}
	}
}

// Put all the dependencies in a map with the form
// {<dependency_file_name>:[<list of dependencies>]}
func (cfs *CompileFailureSignal) CalculateDependencyMap(c context.Context) {
	cfs.DependencyMap = map[string][]string{}
	for _, edge := range cfs.Edges {
		for _, dependency := range edge.Dependencies {
			fileName := util.GetCanonicalFileName(dependency)
			_, ok := cfs.DependencyMap[fileName]
			if !ok {
				cfs.DependencyMap[fileName] = []string{}
			}
			// Check if the dependency already exists
			// Do a for loop, the length is short. So it should be ok.
			exist := false
			for _, d := range cfs.DependencyMap[fileName] {
				if d == dependency {
					exist = true
					break
				}
			}
			if !exist {
				cfs.DependencyMap[fileName] = append(cfs.DependencyMap[fileName], dependency)
			}
		}
	}
}
