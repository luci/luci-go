// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"sync"

	"github.com/luci/gkvlite"
)

// TODO(riannucci): Add a multi-iterator which allows:
//   multiple collections
//     sort smallest collection -> largest collection
//   constant prefixes for each collection
//   start + end partial suffixes
//     * these are appended to the prefix for the collection to narrow the scan
//       range. The collection scans will start at >= prefix+start suffix, and
//       will scan until > prefix+end suffix (excluding this larger row).
//   produces hits (a suffix+value) when all collections contain the same prefix
//     and the same suffix
//     * a hit value will have
//       - a []byte suffix which is the matching suffix of every collection.
//         The caller already knows the prefix for each collection. The caller
//         may need to parse the suffix to separate the sort order(s) from the
//         Key.
//   dedup
//     on each hit, the row values are concatenated and compared to the
//		   concatenated prefixes+startsuffix. If it's <=, it's skipped.

type cmd struct {
	targ []byte
	cb   func(*gkvlite.Item)
}

type iterator struct {
	stopper sync.Once

	stopped bool
	prev    []byte
	ch      chan<- *cmd
}

func newIterable(coll *memCollection, end []byte) *iterator {
	cmdChan := make(chan *cmd)
	ret := &iterator{
		ch: cmdChan,
	}

	go func() {
		defer ret.stop()
		c := (*cmd)(nil)
		ensureCmd := func() bool {
			if c == nil {
				c = <-cmdChan
				if c == nil { // stop()
					return false
				}
			}
			return true
		}
		for {
			if !ensureCmd() {
				return
			}
			previous := c.targ
			needCallback := true
			coll.VisitItemsAscend(c.targ, true, func(i *gkvlite.Item) bool {
				if !ensureCmd() {
					return false
				}
				if !bytes.Equal(previous, c.targ) {
					// we need to start a new ascention function
					needCallback = false
					return false
				}
				if end != nil && bytes.Compare(i.Key, end) >= 0 {
					// we hit our cap
					ret.stop()
					return false
				}
				c.cb(i)
				previous = i.Key
				c = nil
				return true
			})
			if c != nil && needCallback {
				c.cb(nil)
				c = nil
			}
		}
	}()

	return ret
}

func (t *iterator) stop() {
	t.stopper.Do(func() {
		t.stopped = true
		close(t.ch)
	})
}

func (t *iterator) next(targ []byte, cb func(*gkvlite.Item)) {
	if t.stopped {
		cb(nil)
		return
	}

	if targ == nil {
		targ = t.prev
		if targ == nil {
			targ = []byte{}
		}
	}

	waiter := make(chan struct{})
	t.ch <- &cmd{targ, func(i *gkvlite.Item) {
		defer close(waiter)

		cb(i)
		if i == nil {
			t.stop()
		} else {
			t.prev = i.Key
		}
	}}
	<-waiter
}
