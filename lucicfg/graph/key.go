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

package graph

import (
	"strconv"
	"strings"

	"go.starlark.net/starlark"
)

// Key is a unique identifier of a node in the graph.
//
// It is constructed from a series of (kind: string, id: string) pairs, and once
// constructed it acts as an opaque label, not examinable through Starlark.
//
// From the Starlark side it looks like a ref-like hashable object: keys can be
// compared to each other via == and !=, and can be used in dicts and sets.
//
// Keys live in a KeySet, which represents a universe of all keys and it is also
// responsible for their construction. Keys from different key sets (even if
// they represent same path) are considered not equal and shouldn't be used
// together.
type Key struct {
	set   *KeySet  // the parent key set that created this key
	pairs []string // original list of (kind1, id1, kind2, id2, ...) strings
	idx   int      // index of this key in the KeySet
	cmp   string   // composite string to use to compare keys lexicographically
}

// Container returns a key with all (kind, id) pairs of this key, except the
// last one, or nil if this key has only one (kind, id) pair.
//
// Usually it represents a container that holds an object represented by the
// this key.
func (k *Key) Container() *Key {
	if len(k.pairs) == 2 {
		return nil
	}
	cont, err := k.set.Key(k.pairs[:len(k.pairs)-2]...)
	if err != nil {
		panic(err) // 'k' has been validated, all its components are thus also valid
	}
	return cont
}

// ID returns id of the last (kind, id) pair in the key, which usually holds
// a user-friendly name of an object this key represents.
func (k *Key) ID() string {
	return k.pairs[len(k.pairs)-1]
}

// Kind returns kind of the last (kind, id) pair in the key, which usually
// defines what sort of an object this key represents.
func (k *Key) Kind() string {
	return k.pairs[len(k.pairs)-2]
}

// Root returns a key with the first (kind, id) pair of this key.
func (k *Key) Root() *Key {
	r, err := k.set.Key(k.pairs[:2]...)
	if err != nil {
		panic(err) // 'k' has been validated, all its components are thus also valid
	}
	return r
}

// Less returns true if this key is lexicographically before another key.
func (k *Key) Less(an *Key) bool {
	return k.cmp < an.cmp
}

// String is part of starlark.Value interface.
//
// Returns [kind1("id1"), kind2("id2"), ...]. Must not be parsed, only for
// logging.
func (k *Key) String() string {
	if len(k.pairs)%2 != 0 {
		panic("odd") // never happens, validated by KeySet.Key(...) constructor
	}
	sb := strings.Builder{}
	sb.WriteRune('[')
	for i := 0; i < len(k.pairs)/2; i++ {
		kind, id := k.pairs[i*2], k.pairs[i*2+1]
		if i != 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(kind)
		sb.WriteRune('(')
		sb.WriteString(strconv.Quote(id))
		sb.WriteRune(')')
	}
	sb.WriteRune(']')
	return sb.String()
}

// Type is a part of starlark.Value interface.
func (k *Key) Type() string { return "graph.key" }

// Freeze is a part of starlark.Value interface.
func (k *Key) Freeze() {}

// Truth is a part of starlark.Value interface.
func (k *Key) Truth() starlark.Bool { return starlark.True }

// Hash is a part of starlark.Value interface.
func (k *Key) Hash() (uint32, error) { return uint32(k.idx), nil }

// AttrNames is a part of starlark.HasAttrs interface.
func (k *Key) AttrNames() []string {
	return []string{
		"container", // graph.key(...) of the owning container or None
		"id",        // key ID string
		"kind",      // key Kind string
		"root",      // graph.key(...) of the outermost owning container (or self)
	}
}

// Attr is a part of starlark.HasAttrs interface.
func (k *Key) Attr(name string) (starlark.Value, error) {
	switch name {
	case "container":
		if c := k.Container(); c != nil {
			return c, nil
		}
		return starlark.None, nil
	case "id":
		return starlark.String(k.ID()), nil
	case "kind":
		return starlark.String(k.Kind()), nil
	case "root":
		return k.Root(), nil
	default:
		return nil, nil // per Attr(...) contract
	}
}
