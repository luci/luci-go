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

	"go.chromium.org/gae/service/datastore"

	"github.com/luci/gtreap"
	"go.chromium.org/luci/common/data/treapstore"
)

type storeEntry struct {
	key   []byte
	value []byte
}

// storeEntryCompare is a gtreap.Compare function for *storeEntry.
func storeEntryCompare(a, b interface{}) int {
	// TODO(dnj): Investigate optimizing this by removing the type assertions,
	// either by explicitly tailoring gtreap / treapstore to use []byte or by
	// optimizing via special-case interface.
	return bytes.Compare(a.(*storeEntry).key, b.(*storeEntry).key)
}

func memStoreCollide(o, n memCollection, f func(k, ov, nv []byte)) {
	var oldIter, newIter memIterator
	if o != nil {
		if !o.IsReadOnly() {
			panic("old collection is r/w")
		}
		oldIter = o.Iterator(nil)
	} else {
		oldIter = nilIterator{}
	}

	if n != nil {
		if !n.IsReadOnly() {
			panic("new collection is r/w")
		}
		newIter = n.Iterator(nil)
	} else {
		newIter = nilIterator{}
	}

	l, r := oldIter.Next(), newIter.Next()
	for {
		switch {
		case l == nil:
			// No more "old" items, use up the rest of "new" and finish.
			for r != nil {
				f(r.key, nil, r.value)
				r = newIter.Next()
			}
			return

		case r == nil:
			// No more "new" items, use up the rest of "old" and finish.
			for l != nil {
				f(l.key, l.value, nil)
				l = oldIter.Next()
			}
			return

		default:
			switch bytes.Compare(l.key, r.key) {
			case -1: // l < r
				f(l.key, l.value, nil)
				l = oldIter.Next()
			case 0: // l == r
				f(l.key, l.value, r.value)
				l, r = oldIter.Next(), newIter.Next()
			case 1: // l > r
				f(r.key, nil, r.value)
				r = newIter.Next()
			}
		}
	}
}

// memStore is a generic interface modeled after treapstore.Store.
type memStore interface {
	datastore.TestingSnapshot

	GetCollection(name string) memCollection
	GetCollectionNames() []string
	GetOrCreateCollection(name string) memCollection
	Snapshot() memStore

	IsReadOnly() bool
}

// memIterator is an iterator over a memStore's contents.
type memIterator interface {
	Next() *storeEntry
}

// memVisitor is a callback for ForEachItem.
type memVisitor func(k, v []byte) bool

// memCollection is a generic interface modeled after treapstore.Collection.
type memCollection interface {
	Name() string
	Delete(k []byte)
	Get(k []byte) []byte
	MinItem() *storeEntry
	Set(k, v []byte)
	Iterator(pivot []byte) memIterator
	ForEachItem(memVisitor)

	IsReadOnly() bool
}

type memStoreImpl struct {
	s *treapstore.Store
}

var _ memStore = (*memStoreImpl)(nil)

func (*memStoreImpl) ImATestingSnapshot() {}

func (ms *memStoreImpl) IsReadOnly() bool { return ms.s.IsReadOnly() }

func newMemStore() memStore {
	ret := memStore(&memStoreImpl{treapstore.New()})
	if *logMemCollectionFolder != "" {
		ret = wrapTracingMemStore(ret)
	}
	return ret
}

func (ms *memStoreImpl) Snapshot() memStore {
	if ms.s.IsReadOnly() {
		return ms
	}
	return &memStoreImpl{ms.s.Snapshot()}
}

func (ms *memStoreImpl) GetCollection(name string) memCollection {
	coll := ms.s.GetCollection(name)
	if coll == nil {
		return nil
	}
	return &memCollectionImpl{coll}
}

func (ms *memStoreImpl) GetOrCreateCollection(name string) memCollection {
	coll := ms.s.GetCollection(name)
	if coll == nil {
		coll = ms.s.CreateCollection(name, storeEntryCompare)
	}
	return &memCollectionImpl{coll}
}

func (ms *memStoreImpl) GetCollectionNames() []string { return ms.s.GetCollectionNames() }

type memIteratorImpl struct {
	base *gtreap.Iterator
}

func (it *memIteratorImpl) Next() *storeEntry {
	v, ok := it.base.Next()
	if !ok {
		return nil
	}
	return v.(*storeEntry)
}

type memCollectionImpl struct {
	c *treapstore.Collection
}

var _ memCollection = (*memCollectionImpl)(nil)

func (mc *memCollectionImpl) Name() string     { return mc.c.Name() }
func (mc *memCollectionImpl) IsReadOnly() bool { return mc.c.IsReadOnly() }

func (mc *memCollectionImpl) Get(k []byte) []byte {
	if ent := mc.c.Get(storeKey(k)); ent != nil {
		return ent.(*storeEntry).value
	}
	return nil
}

func (mc *memCollectionImpl) MinItem() *storeEntry {
	ent, _ := mc.c.Min().(*storeEntry)
	return ent
}

func (mc *memCollectionImpl) Set(k, v []byte) { mc.c.Put(&storeEntry{k, v}) }
func (mc *memCollectionImpl) Delete(k []byte) { mc.c.Delete(storeKey(k)) }

func (mc *memCollectionImpl) Iterator(target []byte) memIterator {
	if !mc.c.IsReadOnly() {
		// We prevent this to ensure our internal logic, not because it's actually
		// an invalid operation.
		panic("attempting to get Iterator from r/w memCollection")
	}
	return &memIteratorImpl{mc.c.Iterator(storeKey(target))}
}

func (mc *memCollectionImpl) ForEachItem(fn memVisitor) {
	mc.c.VisitAscend(storeKey(nil), func(it gtreap.Item) bool {
		ent := it.(*storeEntry)
		return fn(ent.key, ent.value)
	})
}

func storeKey(k []byte) *storeEntry { return &storeEntry{k, nil} }

// nilIterator is a memIterator that begins in a depleted state.
type nilIterator struct{}

func (it nilIterator) Next() *storeEntry { return nil }
