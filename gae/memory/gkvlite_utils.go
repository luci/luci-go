// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"github.com/luci/gkvlite"
)

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

func (mc *memCollection) VisitItemsDescend(target []byte, withValue bool, visitor gkvlite.ItemVisitor) {
	if err := (*gkvlite.Collection)(mc).VisitItemsDescend(target, withValue, visitor); err != nil {
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
