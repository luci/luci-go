// Copyright 2024 The LUCI Authors.
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

package ftt

import (
	"sync"
)

// stringTree is a simple tree of strings based on a recursive map type.
type stringTree map[string]stringTree

// Adds a path into the tree.
//
// Given:
//
//	root {
//	  a {}
//	  b {
//	    c {}
//	  }
//	}
//
// Adding `a b nice d` would result in a tree state like:
//
//	root {
//	  a {}
//	  b {
//	    c {}
//	    nice {
//	      d {}
//	    }
//	  }
//	}
//
// This returns true iff the path already exists (as a leaf or a branch).
func (s stringTree) add(path []string) (exists bool) {
	if len(path) == 0 {
		panic("stringTree.add called with empty s")
	}

	cur, exists := s[path[0]]
	if len(path) == 1 {
		if !exists {
			s[path[0]] = nil
		}
		return
	}

	if cur == nil {
		cur = stringTree{}
		s[path[0]] = cur
	}

	return cur.add(path[1:])
}

// syncStringTree is a goroutine-safe tree of strings.
//
// This is used to represent the full name of all Run/Parallel suites discovered
// during the execution of a given ftt root.
type syncStringTree struct {
	mu   sync.Mutex
	tree stringTree
}

// Adds a path to this tree while holding the mutex.
//
// See stringTree.add.
func (s *syncStringTree) add(path []string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.tree == nil {
		s.tree = stringTree{}
	}
	return s.tree.add(path)
}

// state is shared between all subtests suites which stem from the same root
// Parallel or Run invocation.
type state struct {
	// rootCb is the callback to the root of this ftt tree.
	rootCb func(*Test)

	// rootName is the name of the root ftt.Run/ftt.Parallel call.
	rootName string

	// testNames is the full tree of all subtests discovered in this ftt tree.
	//
	// The top level of testNames will be the tests discovered immediately under
	// rootCb (that is, excluding rootName).
	testNames syncStringTree
}
