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

package directoryocclusion

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	// "golang.org/x/exp/maps"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
)

// Checker is a very limited filesystem hierarchy checker.
//
// This forms a tree, where each node is a directory. Nodes in the tree may have
// a mapping from owner claiming this directory to a series of notes
// (descriptions of /why/ this owner claims this directory).
//
// Paths may only ever have one owner; After adding all paths to the Checker,
// call Conflicts to populate a validation.Context with any conflicts
// discovered.
//
// Practically, this is used to ensure that Cache directories do not overlap with
// CIPD package directives; CIPD packages may not be installed as subdirs of
// caches, and caches may not be installed as subdirs of CIPD package
// directories. Similarly, multiple caches cannot be mapped to the same
// directory.
type Checker struct {
	fullPath   string
	ownerNotes map[string]stringset.Set
	subdirs    map[string]*Checker
}

// NewChecker creates a new Checker.
func NewChecker(fullPath string) *Checker {
	return &Checker{
		fullPath:   fullPath,
		ownerNotes: make(map[string]stringset.Set),
		subdirs:    make(map[string]*Checker),
	}
}

// Add adds the path to the tree with the owner.
func (c *Checker) Add(path, owner, note string) {
	tokens := strings.Split(path, "/")
	node := c
	for i, subdir := range tokens {
		if node.subdirs[subdir] == nil {
			node.subdirs[subdir] = NewChecker(strings.Join(tokens[:i+1], "/"))
		}
		node = node.subdirs[subdir]
	}
	if node.ownerNotes[owner] == nil {
		node.ownerNotes[owner] = stringset.New(1)
	}
	node.ownerNotes[owner].Add(note)
}

// Conflicts populates `merr` with all violations found in this Checker.
//
// This will walk the Checker depth-first, pruning branches at the first conflict.
func (c *Checker) Conflicts() errors.MultiError {
	var merr errors.MultiError
	c.conflicts(nil, &merr)
	return merr
}

func (c *Checker) conflicts(parentOwnedNode *Checker, merr *errors.MultiError) {
	myOwners := slices.Collect(maps.Keys(c.ownerNotes))

	// Multiple owners tried to claim this directory. In this case there's no
	// discernible owner for subdirectories, so return immediately.
	if len(myOwners) > 1 {
		merr.MaybeAdd(
			errors.Reason("%q: directory has conflicting owners: %s",
				c.fullPath, strings.Join(c.descriptions(), " and ")).Err())
		return
	}

	// Something (singular) claimed this directory
	if len(myOwners) == 1 {
		myOwner := myOwners[0]

		// Some directory above us also has an owner set, check for conflicts.
		if parentOwnedNode != nil {
			if myOwner != parentOwnedNode.owner() {
				// We found a conflict; there's no discernible owner for
				// subdirectories, so return immediately.
				merr.MaybeAdd(
					errors.Reason("%s uses %q, which conflicts with %s using %q",
						c.describeOne(), c.fullPath, parentOwnedNode.describeOne(),
						parentOwnedNode.fullPath).Err())
				return
			}
		} else {
			// We're the first owner down this leg of the tree, so parentOwnedNode
			// is now us for all subdirectories.
			parentOwnedNode = c
		}
	}

	sortedSubdirs := slices.Collect(maps.Keys(c.subdirs))
	sort.Strings(sortedSubdirs)
	for _, subdir := range sortedSubdirs {
		c.subdirs[subdir].conflicts(parentOwnedNode, merr)
	}
}

func (c *Checker) descriptions() []string {
	var ret []string
	owners := slices.Collect(maps.Keys(c.ownerNotes))
	sort.Strings(owners)

	for _, owner := range owners {
		notes := c.ownerNotes[owner].ToSlice()
		if len(notes) > 0 {
			sort.Strings(notes)
			ret = append(ret, fmt.Sprintf("%s[%s]", owner, strings.Join(notes, ", ")))
		} else {
			ret = append(ret, owner)
		}
	}
	return ret
}

func (c *Checker) describeOne() string {
	ret := c.descriptions()
	if len(ret) != 1 {
		panic("describeOne called with multiple owners")
	}
	return ret[0]
}

func (c *Checker) owner() string {
	if len(c.ownerNotes) != 1 {
		panic("owner called with multiple owners")
	}
	for k := range c.ownerNotes {
		return k
	}
	return ""
}
