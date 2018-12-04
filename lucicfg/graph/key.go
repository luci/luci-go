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
	"fmt"
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
	pairs []string // original list of (typ1, id1, typ2, id2, ...) strings
	idx   int      // index of this key in the KeySet
}

// String is part of starlark.Value interface.
//
// Returns [typ1("id1"), typ2("id2"), ...]. Must not be parsed, only for
// logging.
func (k *Key) String() string {
	if len(k.pairs)%2 != 0 {
		panic("odd") // never happens, validated by KeySet.Key(...) constructor
	}
	sb := strings.Builder{}
	sb.WriteRune('[')
	for i := 0; i < len(k.pairs)/2; i++ {
		typ, id := k.pairs[i*2], k.pairs[i*2+1]
		if i != 0 {
			sb.WriteString(", ")
		}
		sb.WriteString(typ)
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

// KeySet is a set of all keys ever defined in a graph.
//
// Each key is a singleton object: asking for the same key twice returns exact
// same *Key object. This simplifies comparison of keys and using keys as, well,
// keys in a map.
type KeySet struct {
	keys map[string]*Key // compact representation of the key path -> *Key
}

// Key returns a *Key given a list of (type, id) pairs.
//
// Assumes strings don't have zero bytes. No other restrictions.
func (k *KeySet) Key(pairs ...string) (*Key, error) {
	switch {
	case len(pairs) == 0:
		return nil, fmt.Errorf("empty key path")
	case len(pairs)%2 != 0:
		return nil, fmt.Errorf("key path %q has odd number of components", pairs)
	}

	for _, s := range pairs {
		if strings.IndexByte(s, 0) != -1 {
			return nil, fmt.Errorf("bad key path element %q, has zero byte inside", s)
		}
	}

	keyID := strings.Join(pairs, "\x00")
	if key := k.keys[keyID]; key != nil {
		return key, nil
	}

	if k.keys == nil {
		k.keys = make(map[string]*Key, 1)
	}

	key := &Key{set: k, pairs: pairs, idx: len(k.keys)}
	k.keys[keyID] = key
	return key, nil
}
