// Copyright 2026 The LUCI Authors.
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

package prefix

import (
	"strings"
)

// Track binds a typed [Tree] cursor into a single [Walk] invocation.
//
// It allows retrieving the effective value of the tree it is currently bound
// to, as well as switching to track a different tree, or ignoring all values
// from the currently bound tree at, or under, the current path in the walk.
//
// A zero-value Track is valid and starts unbound (equivalent to
// [NewTrack][V](nil)).
//
// A Track is single-use: passing it to [Walk] more than once (including
// multiple times to the same [Walk]) panics.
type Track[V any] struct {
	// The tree used for root. Note that `stack` frames may contain a different
	// tree than this in the event that [Track.SetTree] was called.
	rootTree *Tree[V]

	// used flips to true in [Walk], and reusing the Track with `used==true`
	// panics.
	used bool

	// curPath is the normalized ("" or trailing '/') path of the most recently
	// pushed [Walk] step.
	curPath string

	// stack is the active ancestor chain from Walk's prefix (stack[0]) down to
	// `curPath` (stack[len(stack)-1]).
	stack []trackFrame[V]
}

// NewTrack creates a [Track] which will start by tracking `t`.
//
// Passing `nil` creates an empty Track which will produce only zero values
// until [Track.SetTree] is called.
func NewTrack[V any](t *Tree[V]) *Track[V] {
	return &Track[V]{rootTree: t}
}

// Value returns the effective value of this track for the currently bound
// tree, at the current [Walk] path (or the zero value of `V` if the track is
// unbound).
//
// Only valid during an active [Walk] iteration (or this will panic).
func (tr *Track[V]) Value() V {
	return tr.top().val
}

// SetTree switches this track to `t` for the current path in [Walk].
//
// `t` may be nil; this will make [Track.Value] produce zero value Vs until
// this path is popped or SetTree is used with some non-nil [Tree].
//
// If `t` is already the active [Tree] on this track, SetTree is a no-op.
//
// Otherwise, `Value` immediately reflects `t` at the current path, and
// traversal of the current subtree uses `t`.
//
// When [Walk] finishes the current subtree and backtracks to sibling branches,
// the previous tree and values are restored.
//
// If called before [Walk] starts, SetTree (re)sets the track's initial tree.
func (tr *Track[V]) SetTree(t *Tree[V]) {
	if !tr.used {
		// Not yet used, rebind initial tree.
		tr.rootTree = t
		return
	}

	top := tr.top()
	if t == top.tree {
		// No-op - same tree as before.
		return
	}

	// Lookup our current path in the new tree and reset the top of our stack
	// (or clear the frame if t == nil).
	*top = makeTrackFrame(t, tr.curPath)
}

// Prune stops this [Track] from descending into child nodes below the current
// [Walk] path, while retaining its current effective [Track.Value] for any
// deeper paths visited in the walk.
//
// When [Walk] backtracks out of the current subtree to sibling branches,
// traversal of this track resumes.
//
// Only valid during an active [Walk] iteration (or this will panic).
func (tr *Track[V]) Prune() {
	tr.top().children = nil
}

// GenericTrack is a type-erased handle to a [Track] passed to [Walk].
type GenericTrack interface {
	// push advances the track to `nextPrefix`.
	//
	// When `init` is true, initializes the track at `nextPrefix` (panicking if
	// the track was already used) and reports whether the track has an initial
	// value there.
	//
	// When `init` is false, steps from the current stack top and reports whether
	// the track settled onto a node with a value at `nextPrefix`.
	push(nextPrefix string, init bool) (hasValue bool)

	// peek returns the full normalized prefix of the next unvisited child node
	// from the current stack top, if any.
	peek() (nodePrefix string, ok bool)

	// pop removes the top frame from the ancestor stack.
	pop()
}

type trackFrame[V any] struct {
	tree *Tree[V]

	// children is the slice of unvisited child nodes from this frame (when
	// settled on a node), or a 1-element slice holding the target node at the
	// end of an incoming compressed edge (when partway along an edge).
	//
	// If empty, this frame has no outgoing edges to visit (leaf, exhausted,
	// [Track.Prune]'d, [Track.SetTree](nil), or diverged out of `tree`), and
	// `val` will not change under this subtree unless [Track.SetTree] is called.
	children []node[V]

	// effective value at this path.
	val V
}

// makeTrackFrame makes a new frame around `t`.
//
// If `t` is nil, returns an empty frame.
//
// Otherwise, looks up `path` in `t` and returns a frame with cursor state for
// this.
func makeTrackFrame[V any](t *Tree[V], path string) trackFrame[V] {
	if t == nil {
		return trackFrame[V]{}
	}
	val, children := t.locate(path)
	return trackFrame[V]{
		tree:     t,
		children: children,
		val:      val,
	}
}

// makeChildFrame makes a new frame carrying forward the same tree and value
// as `tf`, advancing `tf` if its next child lies within `nextPrefix` and
// reporting whether the new frame settled onto a node with a value.
func (tf *trackFrame[V]) makeChildFrame(nextPrefix string) (ret trackFrame[V], hasValue bool) {
	ret = trackFrame[V]{
		tree: tf.tree,
		val:  tf.val,
	}
	if len(tf.children) == 0 || !strings.HasPrefix(tf.children[0].prefix, nextPrefix) {
		return ret, false
	}

	next := &tf.children[0]
	if len(next.prefix) > len(nextPrefix) {
		// `nextPrefix` is partway along the incoming edge to `next`.
		ret.children = tf.children[:1:1]
		tf.children = tf.children[1:]
		return ret, false
	}

	// `next.prefix == nextPrefix`: settled directly onto `next`.
	tf.children = tf.children[1:]
	ret.children = next.children
	if next.hasVal {
		ret.val = next.val
	}
	return ret, next.hasVal
}

// top returns the current top of the stack.
//
// Only valid during an active [Walk] iteration (or this will panic).
func (tr *Track[V]) top() *trackFrame[V] {
	return &tr.stack[len(tr.stack)-1]
}

// peek implements [GenericTrack].
func (tr *Track[V]) peek() (string, bool) {
	top := tr.top()
	if len(top.children) == 0 {
		return "", false
	}
	return top.children[0].prefix, true
}

// push implements [GenericTrack].
func (tr *Track[V]) push(nextPrefix string, init bool) bool {
	if init {
		if tr.used {
			panic("prefix.Track cannot be used more than once")
		}
		tr.used = true
		tr.curPath = nextPrefix
		tr.stack = append(tr.stack, makeTrackFrame(tr.rootTree, nextPrefix))
		return tr.rootTree != nil && len(tr.rootTree.nodes) > 0
	}

	tr.curPath = nextPrefix
	child, hasValue := tr.top().makeChildFrame(nextPrefix)
	tr.stack = append(tr.stack, child)
	return hasValue
}

// pop implements [GenericTrack].
func (tr *Track[V]) pop() {
	last := len(tr.stack) - 1
	tr.stack[last] = trackFrame[V]{}
	tr.stack = tr.stack[:last]
}
