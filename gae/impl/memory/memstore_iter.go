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

package memory

import (
	"bytes"

	"go.chromium.org/gae/service/datastore/serialize"
)

type iterDefinition struct {
	// The collection to iterate over
	c memCollection

	// The prefix to always assert for every row. A nil prefix matches every row.
	prefix []byte

	// prefixLen is the number of prefix bytes that the caller cares about. It
	// may be <= len(prefix). When doing a multiIterator, this number will be used
	// to determine the amount of suffix to transfer accross iterators. This is
	// used specifically when using builtin indexes to service ancestor queries.
	// The builtin index represents the ancestor key with prefix bytes, but in a
	// multiIterator context, it wants the entire key to be included in the
	// suffix.
	prefixLen int

	// The start cursor. It's appended to prefix to find the first row.
	start []byte

	// The end cursor. It's appended to prefix to find the last row (which is not
	// included in the interation result). If this is nil, then there's no end
	// except the natural end of the collection.
	end []byte
}

func multiIterate(defs []*iterDefinition, cb func(suffix []byte) error) error {
	if len(defs) == 0 {
		return nil
	}

	ts := make([]*iterator, len(defs))
	prefixLens := make([]int, len(defs))
	for i, def := range defs {
		// bind i so that the defer below doesn't get goofed by the loop variable
		i := i
		ts[i] = def.mkIter()
		prefixLens[i] = def.prefixLen
	}

	suffix := []byte(nil)
	skip := -1

MainLoop:
	for {
		for idx, it := range ts {
			if skip >= 0 && skip == idx {
				continue
			}

			pfxLen := prefixLens[idx]
			it.skip(serialize.Join(it.def.prefix[:pfxLen], suffix))
			ent := it.next()
			if ent == nil {
				// we hit the end of an iterator, we're now done with the whole
				// query.
				return nil
			}
			sfxRO := ent.key[pfxLen:]

			if bytes.Compare(sfxRO, suffix) > 0 {
				// this row has a higher suffix than anything we've seen before. Set
				// ourself to be the skip, and resart this loop from the top.
				suffix = append(suffix[:0], sfxRO...)
				skip = idx
				if idx != 0 {
					// no point to restarting on the 0th index
					continue MainLoop
				}
			}
		}

		if err := cb(suffix); err != nil {
			return err
		}
		suffix = nil
		skip = -1
	}
}

type iterator struct {
	def  *iterDefinition
	base memIterator

	start   []byte
	end     []byte
	lastKey []byte
}

func (def *iterDefinition) mkIter() *iterator {
	if !def.c.IsReadOnly() {
		panic("attempting to make an iterator with r/w collection")
	}

	it := iterator{
		def: def,
	}

	// convert the suffixes from the iterDefinition into full rows for the
	// underlying storage.
	it.start = serialize.Join(def.prefix, def.start)
	if def.end != nil {
		it.end = serialize.Join(def.prefix, def.end)
	}
	return &it
}

func (it *iterator) skip(targ []byte) {
	if bytes.Compare(targ, it.start) < 0 {
		targ = it.start
	}
	if it.base == nil || bytes.Compare(targ, it.lastKey) > 0 {
		// If our skip target is >= our last key, then create a new Iterator
		// starting from that target.
		it.base = it.def.c.Iterator(targ)
	}
}

func (it *iterator) next() *storeEntry {
	if it.base == nil {
		it.skip(nil)
	}

	ent := it.base.Next()
	switch {
	case ent == nil:
		return nil

	case !bytes.HasPrefix(ent.key, it.def.prefix):
		// we're no longer in prefix, terminate
		return nil

	case it.end != nil && bytes.Compare(ent.key, it.end) >= 0:
		// we hit our cap, terminate.
		return nil

	default:
		it.lastKey = ent.key
		return ent
	}
}
