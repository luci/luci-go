// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"sync"

	"github.com/luci/gae/service/datastore/serialize"
	"github.com/luci/gkvlite"
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
		defer ts[i].stop()
	}

	suffix := []byte(nil)
	skip := -1

	for {
		stop := false
		restart := false

		for idx, it := range ts {
			if skip >= 0 && skip == idx {
				continue
			}
			def := defs[idx]

			pfxLen := prefixLens[idx]
			it.next(serialize.Join(def.prefix[:pfxLen], suffix), func(itm *gkvlite.Item) {
				if itm == nil {
					// we hit the end of an iterator, we're now done with the whole
					// query.
					stop = true
					return
				}

				sfxRO := itm.Key[pfxLen:]

				if bytes.Compare(sfxRO, suffix) > 0 {
					// this row has a higher suffix than anything we've seen before. Set
					// ourself to be the skip, and resart this loop from the top.
					suffix = append(suffix[:0], sfxRO...)
					skip = idx
					if idx != 0 {
						// no point to restarting on the 0th index
						restart = true
					}
				}
			})
			if stop || restart {
				break
			}
		}
		if stop {
			return nil
		}
		if restart {
			continue
		}

		if err := cb(suffix); err != nil {
			return err
		}
		suffix = nil
		skip = -1
	}
}

type cmd struct {
	targ []byte
	cb   func(*gkvlite.Item)
}

type iterator struct {
	stopper sync.Once

	stopped bool
	ch      chan<- *cmd
}

func (def *iterDefinition) mkIter() *iterator {
	if !def.c.IsReadOnly() {
		panic("attempting to make an iterator with r/w collection")
	}

	cmdChan := make(chan *cmd)
	ret := &iterator{
		ch: cmdChan,
	}

	prefix := def.prefix
	collection := def.c

	// convert the suffixes from the iterDefinition into full rows for the
	// underlying storage.
	start := serialize.Join(prefix, def.start)

	end := []byte(nil)
	if def.end != nil {
		end = serialize.Join(prefix, def.end)
	}

	go func() {
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
		if ensureCmd() {
			if bytes.Compare(c.targ, start) < 0 {
				c.targ = start
			}
		}

		defer ret.stop()
		for {
			if !ensureCmd() {
				return
			}
			terminalCallback := true
			collection.VisitItemsAscend(c.targ, true, func(i *gkvlite.Item) bool {
				if !ensureCmd() {
					return false
				}
				if bytes.Compare(i.Key, c.targ) < 0 {
					// we need to start a new ascension function
					terminalCallback = false
					return false
				}
				if !bytes.HasPrefix(i.Key, prefix) {
					// we're no longer in prefix, terminate
					return false
				}
				if end != nil && bytes.Compare(i.Key, end) >= 0 {
					// we hit our cap, terminate.
					return false
				}
				c.cb(i)
				c = nil
				return true
			})
			if terminalCallback && ensureCmd() {
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

	waiter := make(chan struct{})
	t.ch <- &cmd{targ, func(i *gkvlite.Item) {
		defer close(waiter)

		if i == nil {
			t.stop()
		}
		cb(i)
	}}
	<-waiter
}
