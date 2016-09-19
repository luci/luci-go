// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package dscache

import (
	"sync"

	"github.com/luci/gae/service/datastore"
	mc "github.com/luci/gae/service/memcache"

	"github.com/luci/luci-go/common/errors"
	log "github.com/luci/luci-go/common/logging"
)

type dsTxnState struct {
	sync.Mutex

	toLock   []mc.Item
	toDelete map[string]struct{}
}

// reset sets the transaction state back to its 0 state. This is used so that
// when a transaction retries the function, we don't accidentally leak state
// from one function to the next.
func (s *dsTxnState) reset() {
	s.Lock()
	defer s.Unlock()
	// reduce capacity back to 0, but keep the allocated array. If the transaction
	// body retries, it'll probably end up re-allocating the same amount of space
	// anyway.
	s.toLock = s.toLock[:0]
	s.toDelete = make(map[string]struct{}, len(s.toDelete))
}

// apply is called right before the trasnaction is about to commit. It's job
// is to lock all the to-be-changed memcache keys.
func (s *dsTxnState) apply(sc *supportContext) error {
	s.Lock()
	defer s.Unlock()

	// this is a hard failure. No mutation can occur if we're unable to set
	// locks out. See "DANGER ZONE" in the docs.
	err := mc.Set(sc.c, s.toLock...)
	if err != nil {
		(log.Fields{log.ErrorKey: err}).Errorf(
			sc.c, "dscache: HARD FAILURE: dsTxnState.apply(): mc.Set")
	}
	return err
}

// release is called right after a successful transaction completion. It's job
// is to clear out all the locks, if possible (but if not, no worries,
// they'll expire soon).
func (s *dsTxnState) release(sc *supportContext) {
	s.Lock()
	defer s.Unlock()

	delKeys := make([]string, 0, len(s.toDelete))
	for k := range s.toDelete {
		delKeys = append(delKeys, k)
	}

	if err := errors.Filter(mc.Delete(sc.c, delKeys...), mc.ErrCacheMiss); err != nil {
		(log.Fields{log.ErrorKey: err}).Warningf(
			sc.c, "dscache: txn.release: memcache.Delete")
	}
}

func (s *dsTxnState) add(sc *supportContext, keys []*datastore.Key) {
	lockItems, lockKeys := sc.mkAllLockItems(keys)
	if lockItems == nil {
		return
	}

	s.Lock()
	defer s.Unlock()

	for i, li := range lockItems {
		k := lockKeys[i]
		if _, ok := s.toDelete[k]; !ok {
			s.toLock = append(s.toLock, li)
			s.toDelete[k] = struct{}{}
		}
	}
}
