// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package memory

import (
	"bytes"
	"runtime"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/gkvlite"
)

func gkvCollide(o, n memCollection, f func(k, ov, nv []byte)) {
	if o != nil && !o.IsReadOnly() {
		panic("old collection is r/w")
	}
	if n != nil && !n.IsReadOnly() {
		panic("new collection is r/w")
	}

	// TODO(riannucci): reimplement in terms of *iterator.
	oldItems, newItems := make(chan *gkvlite.Item), make(chan *gkvlite.Item)
	walker := func(c memCollection, ch chan<- *gkvlite.Item) {
		defer close(ch)
		if c != nil {
			c.VisitItemsAscend(nil, true, func(i *gkvlite.Item) bool {
				ch <- i
				return true
			})
		}
	}

	go walker(o, oldItems)
	go walker(n, newItems)

	l, r := <-oldItems, <-newItems
	for {
		switch {
		case l == nil && r == nil:
			return

		case l == nil:
			f(r.Key, nil, r.Val)
			r = <-newItems

		case r == nil:
			f(l.Key, l.Val, nil)
			l = <-oldItems

		default:
			switch bytes.Compare(l.Key, r.Key) {
			case -1: // l < r
				f(l.Key, l.Val, nil)
				l = <-oldItems
			case 0: // l == r
				f(l.Key, l.Val, r.Val)
				l, r = <-oldItems, <-newItems
			case 1: // l > r
				f(r.Key, nil, r.Val)
				r = <-newItems
			}
		}
	}
}

// memStore is a gkvlite.Store which will panic for anything which might
// otherwise return an error.
//
// This is reasonable for in-memory Store objects, since the only errors that
// should occur happen with file IO on the underlying file (which of course
// doesn't exist).
type memStore interface {
	datastore.TestingSnapshot

	GetCollection(name string) memCollection
	GetCollectionNames() []string
	GetOrCreateCollection(name string) memCollection
	Snapshot() memStore

	IsReadOnly() bool
}

// memCollection is a gkvlite.Collection which will panic for anything which
// might otherwise return an error.
//
// This is reasonable for in-memory Store objects, since the only errors that
// should occur happen with file IO on the underlying file (which of course
// doesn't exist.
type memCollection interface {
	Name() string
	Delete(k []byte) bool
	Get(k []byte) []byte
	GetTotals() (numItems, numBytes uint64)
	MinItem(withValue bool) *gkvlite.Item
	Set(k, v []byte)
	VisitItemsAscend(target []byte, withValue bool, visitor gkvlite.ItemVisitor)

	IsReadOnly() bool
}

type memStoreImpl struct {
	s  *gkvlite.Store
	ro bool
}

var _ memStore = (*memStoreImpl)(nil)

func (*memStoreImpl) ImATestingSnapshot() {}

func (ms *memStoreImpl) IsReadOnly() bool { return ms.ro }

func newMemStore() memStore {
	store, err := gkvlite.NewStore(nil)
	memoryCorruption(err)
	ret := memStore(&memStoreImpl{store, false})
	if *logMemCollectionFolder != "" {
		ret = wrapTracingMemStore(ret)
	}
	return ret
}

func (ms *memStoreImpl) Snapshot() memStore {
	if ms.ro {
		return ms
	}
	ret := ms.s.Snapshot()
	runtime.SetFinalizer(ret, func(s *gkvlite.Store) { go s.Close() })
	return &memStoreImpl{ret, true}
}

func (ms *memStoreImpl) GetCollection(name string) memCollection {
	coll := ms.s.GetCollection(name)
	if coll == nil {
		return nil
	}
	return &memCollectionImpl{coll, ms.ro}
}

func (ms *memStoreImpl) GetOrCreateCollection(name string) memCollection {
	coll := ms.GetCollection(name)
	if coll == nil {
		coll = &memCollectionImpl{(ms.s.SetCollection(name, nil)), ms.ro}
	}
	return coll
}

func (ms *memStoreImpl) GetCollectionNames() []string {
	return ms.s.GetCollectionNames()
}

type memCollectionImpl struct {
	c  *gkvlite.Collection
	ro bool
}

var _ memCollection = (*memCollectionImpl)(nil)

func (mc *memCollectionImpl) Name() string     { return mc.c.Name() }
func (mc *memCollectionImpl) IsReadOnly() bool { return mc.ro }

func (mc *memCollectionImpl) Get(k []byte) []byte {
	ret, err := mc.c.Get(k)
	memoryCorruption(err)
	return ret
}

func (mc *memCollectionImpl) MinItem(withValue bool) *gkvlite.Item {
	ret, err := mc.c.MinItem(withValue)
	memoryCorruption(err)
	return ret
}

func (mc *memCollectionImpl) Set(k, v []byte) {
	err := mc.c.Set(k, v)
	memoryCorruption(err)
}

func (mc *memCollectionImpl) Delete(k []byte) bool {
	ret, err := mc.c.Delete(k)
	memoryCorruption(err)
	return ret
}

func (mc *memCollectionImpl) VisitItemsAscend(target []byte, withValue bool, visitor gkvlite.ItemVisitor) {
	if !mc.ro {
		panic("attempting to VisitItemsAscend from r/w memCollection")
	}
	err := mc.c.VisitItemsAscend(target, withValue, visitor)
	memoryCorruption(err)
}

func (mc *memCollectionImpl) GetTotals() (numItems, numBytes uint64) {
	numItems, numBytes, err := mc.c.GetTotals()
	memoryCorruption(err)
	return
}
