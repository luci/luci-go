// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package mutexpool implements P, a pool of keyed mutexes. These mutexes are
// created on-demand and deleted when no longer referenced, so the pool's
// maximum size is a function of the maximum number of concurrent mutexes
// held at any given time.
//
// Package mutexpool is useful when coordinating access to resources that are
// not managed by the accessor such as remote resource accesses.
package mutexpool

import (
	"fmt"
	"sync"
)

// P is a pool of keyed mutexes. The zero value is a valid empty pool.
//
// A user can grab an arbitrary Mutex's lock by calling WithMutex with a key.
// If something else currently holds that Mutex's lock, WithMutex will block
// until it can claim the lock. When a key is no longer in use, it will be
// removed from P.
type P struct {
	mutexesLock sync.Mutex
	mutexes     map[interface{}]*mutexEntry
}

func (pc *P) getConfigLock(key interface{}) *mutexEntry {
	// Does the lock already exist?
	pc.mutexesLock.Lock()
	defer pc.mutexesLock.Unlock()

	if me := pc.mutexes[key]; me != nil {
		me.count++
		if me.count == 0 {
			panic(fmt.Errorf("mutex refrerence counter overflow"))
		}
		return me
	}

	if pc.mutexes == nil {
		pc.mutexes = make(map[interface{}]*mutexEntry)
	}
	me := &mutexEntry{
		count: 1, // Start with one ref.
	}
	pc.mutexes[key] = me
	return me
}

func (pc *P) decRef(me *mutexEntry, key interface{}) {
	pc.mutexesLock.Lock()
	defer pc.mutexesLock.Unlock()

	me.count--
	if me.count == 0 {
		delete(pc.mutexes, key)
	}
}

// WithMutex locks the Mutex matching the specified key and executes fn while
// holding its lock.
//
// If a mutex for key doesn't exist, one will be created, and will be
// automatically cleaned up when no longer referenced.
func (pc *P) WithMutex(key interface{}, fn func()) {
	// Get a lock for this config key, and increment its reference.
	me := pc.getConfigLock(key)
	defer pc.decRef(me, key)

	// Hold this lock's mutex and call "fn".
	me.Lock()
	defer me.Unlock()

	fn()
}

type mutexEntry struct {
	sync.Mutex

	// count is the number of references to this mutexEntry. It is protected
	// by P's lock.
	count uint64
}
