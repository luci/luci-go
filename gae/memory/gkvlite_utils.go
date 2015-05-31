// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"sync"

	"github.com/luci/gkvlite"
)

func gkvCollide(o, n *memCollection, f func(k, ov, nv []byte)) {
	oldItems, newItems := make(chan *gkvlite.Item), make(chan *gkvlite.Item)
	walker := func(c *memCollection, ch chan<- *gkvlite.Item, wg *sync.WaitGroup) {
		defer close(ch)
		defer wg.Done()
		if c != nil {
			c.VisitItemsAscend(nil, true, func(i *gkvlite.Item) bool {
				ch <- i
				return true
			})
		}
	}

	wg := &sync.WaitGroup{}
	wg.Add(2)
	go walker(o, oldItems, wg)
	go walker(n, newItems, wg)

	l, r := <-oldItems, <-newItems
	for {
		if l == nil && r == nil {
			break
		}

		if l == nil {
			f(r.Key, nil, r.Val)
			r = <-newItems
		} else if r == nil {
			f(l.Key, l.Val, nil)
			l = <-oldItems
		} else {
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
	wg.Wait()
}

// memStore is a gkvlite.Store which will panic for anything which might
// otherwise return an error.
//
// This is reasonable for in-memory Store objects, since the only errors that
// should occur happen with file IO on the underlying file (which of course
// doesn't exist).
type memStore gkvlite.Store

func newMemStore() *memStore {
	ret, err := gkvlite.NewStore(nil)
	if err != nil {
		panic(err)
	}
	return (*memStore)(ret)
}

func (ms *memStore) Snapshot() *memStore {
	return (*memStore)((*gkvlite.Store)(ms).Snapshot())
}

func (ms *memStore) MakePrivateCollection(cmp gkvlite.KeyCompare) *memCollection {
	return (*memCollection)((*gkvlite.Store)(ms).MakePrivateCollection(cmp))
}

func (ms *memStore) GetCollection(name string) *memCollection {
	return (*memCollection)((*gkvlite.Store)(ms).GetCollection(name))
}

func (ms *memStore) SetCollection(name string, cmp gkvlite.KeyCompare) *memCollection {
	return (*memCollection)((*gkvlite.Store)(ms).SetCollection(name, cmp))
}

func (ms *memStore) RemoveCollection(name string) {
	(*gkvlite.Store)(ms).RemoveCollection(name)
}

func (ms *memStore) GetCollectionNames() []string {
	return (*gkvlite.Store)(ms).GetCollectionNames()
}

// memCollection is a gkvlite.Collection which will panic for anything which
// might otherwise return an error.
//
// This is reasonable for in-memory Store objects, since the only errors that
// should occur happen with file IO on the underlying file (which of course
// doesn't exist.
type memCollection gkvlite.Collection

func (mc *memCollection) Get(k []byte) []byte {
	ret, err := (*gkvlite.Collection)(mc).Get(k)
	if err != nil {
		panic(err)
	}
	return ret
}

func (mc *memCollection) MinItem(withValue bool) *gkvlite.Item {
	ret, err := (*gkvlite.Collection)(mc).MinItem(withValue)
	if err != nil {
		panic(err)
	}
	return ret
}

func (mc *memCollection) Set(k, v []byte) {
	if err := (*gkvlite.Collection)(mc).Set(k, v); err != nil {
		panic(err)
	}
}

func (mc *memCollection) Delete(k []byte) bool {
	ret, err := (*gkvlite.Collection)(mc).Delete(k)
	if err != nil {
		panic(err)
	}
	return ret
}

func (mc *memCollection) VisitItemsAscend(target []byte, withValue bool, visitor gkvlite.ItemVisitor) {
	if err := (*gkvlite.Collection)(mc).VisitItemsAscend(target, withValue, visitor); err != nil {
		panic(err)
	}
}

func (mc *memCollection) GetTotals() (numItems, numBytes uint64) {
	numItems, numBytes, err := (*gkvlite.Collection)(mc).GetTotals()
	if err != nil {
		panic(err)
	}
	return
}
