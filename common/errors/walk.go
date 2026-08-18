// Copyright 2015 The LUCI Authors.
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

package errors

import (
	"fmt"
	"strings"

	"go.chromium.org/luci/common/errors/errtag"
)

// Walk performs a depth-first traversal of the supplied error, unfolding it and
// invoke the supplied callback for each layered error recursively. If the
// callback returns true, Walk will continue its traversal.
//
//   - If walk encounters a MultiError, the callback is called once for the
//     outer MultiError, then once for each inner error.
//   - If walk encounters a Wrapped error, the callback is called for the outer
//     and inner error.
//   - If an inner error is, itself, a container, Walk will recurse into it.
//
// If err is nil, the callback will not be invoked.
func Walk(err error, fn func(error) bool) {
	_ = walkVisit(err, fn, false)
}

// WalkLeaves is like Walk, but only calls fn on leaf nodes.
func WalkLeaves(err error, fn func(error) bool) {
	_ = walkVisit(err, fn, true)
}

func walkVisit(err error, fn func(error) bool, leavesOnly bool) bool {
	if err == nil {
		return true
	}

	// Call fn if we are not in leavesOnly mode.
	if !(leavesOnly || fn(err)) {
		return false
	}

	switch t := err.(type) {
	case interface{ Unwrap() []error }:
		for _, e := range t.Unwrap() {
			if !walkVisit(e, fn, leavesOnly) {
				return false
			}
		}

	case Wrapped:
		unwrapped := t.Unwrap()
		if unwrapped == nil {
			// This was a leaf after all.
			if leavesOnly {
				return fn(err)
			}
		} else {
			return walkVisit(unwrapped, fn, leavesOnly)
		}

	default:
		if leavesOnly {
			return fn(err)
		}
	}

	return true
}

// Any performs a Walk traversal of an error, returning true (and
// short-circuiting) if the supplied filter function returns true for any
// visited error.
//
// If err is nil, Any will return false.
func Any(err error, fn func(error) bool) (any bool) {
	Walk(err, func(err error) bool {
		any = fn(err)
		return !any
	})
	return
}

// Contains performs a Walk traversal of |outer|, returning true if any visited
// error is equal to |inner|.
func Contains(outer error, inner error) bool {
	return Any(outer, func(item error) bool {
		if item == inner {
			return true
		}
		if is, ok := item.(interface{ Is(error) bool }); ok {
			return is.Is(inner)
		}
		return false
	})
}

// Tree represents a parsed error as a tree.
//
// The top of the tree represents the most recent error wrapping, the leaves
// of the tree represent the root/original errors.
//
// An error wrapping a singular other error (e.g. `Unwrap() error`) represents
// a single chain link, and an error wrapping multiple other errors (e.g.
// `Unwrap() []error`) represents multiple branches.
type Tree struct {
	// The error at this node.
	Err error

	// The unique keys of any tags this err contains.
	Tags []errtag.TagKey

	// All immediate non-tag errors this error contains.
	Wraps []*Tree
}

const (
	treeMore = "├"
	treeLast = "└"

	treeContMore = "│"
	treeContLast = " "
)

// String formats this tree like:
//
//	err
//	 ├ [Tag]
//	 ├╔inner err 1 line 1
//	 │║inner err 1 line 2
//	 │╚inner err 1 line 3
//	 │ ├ [Tag]
//	 │ └ [Tag]
//	 └ inner err N
//	   └ [Tag]
func (t *Tree) String() string {
	var bld strings.Builder
	t.stringInner(&bld, "", false)
	return bld.String()
}

func (t *Tree) stringInner(bld *strings.Builder, prefix string, isLast bool) {
	var childPrefix string
	if prefix != "" {
		bld.WriteByte('\n')
		bld.WriteString(prefix)
		if isLast {
			bld.WriteString(treeLast)
			childPrefix = prefix + treeContLast
		} else {
			bld.WriteString(treeMore)
			childPrefix = prefix + treeContMore
		}
	}

	if t.Err != nil {
		lines := strings.Split(t.Err.Error(), "\n")
		if len(lines) == 1 {
			if prefix != "" {
				bld.WriteRune(' ')
			}
			bld.WriteString(t.Err.Error())
		} else {
			pfx := "╔"
			for i, line := range lines {
				fmt.Fprintf(bld, "%s%s", pfx, line)
				if i == len(lines)-1 {
					break
				}
				bld.WriteString("\n")
				if i < len(lines)-2 {
					pfx = childPrefix + "║"
				} else {
					pfx = childPrefix + "╚"
				}
			}
		}
	} else {
		if prefix != "" {
			bld.WriteRune(' ')
		}
		bld.WriteString("<nil>")
	}

	childPrefix += " "

	total := len(t.Tags) + len(t.Wraps)
	for i, tag := range t.Tags {
		tree := treeMore
		if i == total-1 {
			tree = treeLast
		}
		fmt.Fprintf(bld, "\n%s%s [%s]", childPrefix, tree, tag.String())
	}

	for i, wrap := range t.Wraps {
		idx := len(t.Tags) + i
		wrap.stringInner(bld, childPrefix, idx == total-1)
	}
}

// ParseTree parses the `err` into a *Tree.
//
// If `err` contains errtags, they are represented as errtag.TagKey's in the
// Tree of where they were attached. Use errtag.Collect to actually get the
// merged values back for `err`.
func ParseTree(err error) *Tree {
	tree := &Tree{Err: err}
	if err == nil {
		return tree
	}

	// Unwrap all error tags to get to the root/original error.
	for {
		tk, ok := errtag.IsWrapper(err)
		if !ok {
			break
		}
		err = err.(Wrapped).Unwrap()
		tree.Tags = append(tree.Tags, tk)
	}

	// Now unwrap and explore the root/original error.
	switch t := err.(type) {
	case interface{ Unwrap() []error }:
		inners := t.Unwrap()
		if len(inners) > 0 {
			subtrees := make([]*Tree, len(inners))
			for i, inner := range inners {
				subtrees[i] = ParseTree(inner)
			}
			tree.Wraps = subtrees
		}

	case Wrapped:
		tree.Wraps = []*Tree{ParseTree(t.Unwrap())}
	}

	return tree
}
