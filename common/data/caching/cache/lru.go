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

package cache

import (
	"container/list"
	"crypto"
	"encoding/json"
	"fmt"
	"time"

	"go.chromium.org/luci/common/data/text/units"
)

// entry is an entry in the orderedDict.
type entry struct {
	key        HexDigest
	value      units.Size
	lastAccess int64 // UTC time
}

func (e *entry) MarshalJSON() ([]byte, error) {
	// encode as a tuple.
	return json.Marshal([]any{e.key, []any{e.value, e.lastAccess}})
}

func (e *entry) UnmarshalJSON(data []byte) error {
	// decode from tuple.
	var elems []any
	if err := json.Unmarshal(data, &elems); err != nil {
		return fmt.Errorf("invalid entry: %w: %s", err, string(data))
	}
	if len(elems) != 2 {
		return fmt.Errorf("invalid entry: expected 2 items: %s", string(data))
	}
	if key, ok := elems[0].(string); ok {
		e.key = HexDigest(key)
		values, ok := elems[1].([]any)
		if !ok {
			return fmt.Errorf("invalid entry: expected array for second element: %s", string(data))
		}

		if len(values) != 2 {
			return fmt.Errorf("invalid entry: expected 2 items: %v", values)
		}

		if value, ok := values[0].(float64); ok {
			e.value = units.Size(value)
		} else {
			return fmt.Errorf("invalid entry: expected value to be number: %#v", values[0])
		}

		if value, ok := values[1].(float64); ok {
			e.lastAccess = int64(value)
		} else {
			return fmt.Errorf("invalid entry: expected value to be number: %#v", values[1])
		}
	} else {
		return fmt.Errorf("invalid entry: expected key to be string: %s", string(data))
	}
	return nil
}

// orderedDict implements a dict that keeps ordering.
type orderedDict struct {
	ll      *list.List
	entries map[HexDigest]*list.Element
}

func makeOrderedDict() orderedDict {
	return orderedDict{
		ll:      list.New(),
		entries: map[HexDigest]*list.Element{},
	}
}

// keys returns the keys in order.
func (o *orderedDict) keys() HexDigests {
	out := make(HexDigests, 0, o.length())
	for e := o.ll.Front(); e != nil; e = e.Next() {
		out = append(out, e.Value.(*entry).key)
	}
	return out
}

func (o *orderedDict) pop(key HexDigest) (units.Size, bool) {
	if e, hit := o.entries[key]; hit {
		return o.removeElement(e).value, true
	}
	return 0, false
}

func (o *orderedDict) popOldest() (HexDigest, units.Size) {
	if e := o.ll.Back(); e != nil {
		entry := o.removeElement(e)
		return entry.key, entry.value
	}
	return "", 0
}

func (o *orderedDict) removeElement(e *list.Element) *entry {
	o.ll.Remove(e)
	kv := e.Value.(*entry)
	delete(o.entries, kv.key)
	return kv
}

func (o *orderedDict) length() int {
	return o.ll.Len()
}

func (o *orderedDict) pushFront(key HexDigest, value units.Size) {
	if e, ok := o.entries[key]; ok {
		o.ll.MoveToFront(e)
		e.Value.(*entry).value = value
		e.Value.(*entry).lastAccess = time.Now().Unix()
		return
	}
	o.entries[key] = o.ll.PushFront(&entry{key, value, time.Now().Unix()})
}

func (o *orderedDict) pushBack(key HexDigest, value units.Size, lastAccess int64) {
	if e, ok := o.entries[key]; ok {
		o.ll.MoveToBack(e)
		e.Value.(*entry).value = value
		e.Value.(*entry).lastAccess = lastAccess
		return
	}
	o.entries[key] = o.ll.PushBack(&entry{key, value, lastAccess})
}

// serialized returns all the items in order.
func (o *orderedDict) serialized() []entry {
	out := make([]entry, 0, o.length())
	for e := o.ll.Front(); e != nil; e = e.Next() {
		out = append(out, *e.Value.(*entry))
	}
	return out
}

// lruDict is a dictionary that evicts least recently used items. It is a
// higher level than orderedDict.
//
// Designed to be serialized as JSON on disk.
type lruDict struct {
	h     crypto.Hash
	items orderedDict // ordered key -> value mapping, newest items at the bottom.
	dirty bool        // true if was modified after loading until it is marshaled.
	sum   units.Size  // sum of all the values.
}

func makeLRUDict(h crypto.Hash) lruDict {
	return lruDict{
		h:     h,
		items: makeOrderedDict(),
	}
}

func (l *lruDict) IsDirty() bool {
	return l.dirty
}

func (l *lruDict) keys() HexDigests {
	return l.items.keys()
}

func (l *lruDict) length() int {
	return l.items.length()
}

func (l *lruDict) pop(key HexDigest) (units.Size, bool) {
	out, b := l.items.pop(key)
	l.sum -= out
	l.dirty = true
	return out, b
}

func (l *lruDict) popOldest() (HexDigest, units.Size) {
	k, v := l.items.popOldest()
	l.sum -= v
	if k != "" {
		l.dirty = true
	}
	return k, v
}

func (l *lruDict) pushFront(key HexDigest, value units.Size) {
	l.items.pushFront(key, value)
	l.sum += value
	l.dirty = true
}

func (l *lruDict) touch(key HexDigest) bool {
	l.dirty = true
	if value, b := l.items.pop(key); b {
		l.items.pushFront(key, value)
		return true
	}
	return false
}

type serializedLRUDict struct {
	Version int     `json:"version"` // 1.
	Items   []entry `json:"items"`   // ordered key -> value mapping in order.
}

const currentVersion = 3

func (l *lruDict) MarshalJSON() ([]byte, error) {
	s := &serializedLRUDict{
		Version: currentVersion,
		Items:   l.items.serialized(),
	}
	// Not strictly true but #closeneough.
	content, err := json.Marshal(s)
	if err == nil {
		l.dirty = false
	}
	return content, err
}

func (l *lruDict) UnmarshalJSON(data []byte) error {
	s := &serializedLRUDict{}
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}

	if s.Version != currentVersion {
		return fmt.Errorf("invalid lru dict version %d instead of current version %d", s.Version, currentVersion)
	}
	l.sum = 0
	for _, e := range s.Items {
		if !e.key.Validate(l.h) {
			return fmt.Errorf("invalid entry: %s", e.key)
		}
		l.items.pushBack(e.key, e.value, e.lastAccess)
		l.sum += e.value
	}
	l.dirty = false
	return nil
}
