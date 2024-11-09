// Copyright 2017 The LUCI Authors.
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

package dsset

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func testingContext() context.Context {
	c := memory.Use(context.Background())
	c = clock.Set(c, testclock.New(time.Unix(1442270520, 0).UTC()))
	c = mathrand.Set(c, rand.New(rand.NewSource(1000)))
	return c
}

// pop pops a bunch of items from the set and returns items that were popped.
func pop(c context.Context, s *Set, listing *Listing, ids []string) (popped []string, tombs Garbage, err error) {
	op, err := s.BeginPop(c, listing)
	if err != nil {
		return nil, nil, err
	}
	for _, id := range ids {
		if op.Pop(id) {
			popped = append(popped, id)
		}
	}
	if tombs, err = FinishPop(c, op); err != nil {
		return nil, nil, err
	}
	return popped, tombs, nil
}

func TestSet(t *testing.T) {
	t.Parallel()

	ftt.Run("item one lifecycle", t, func(t *ftt.Test) {
		c := testingContext()

		set := Set{
			ID:              "test",
			ShardCount:      3,
			TombstonesRoot:  datastore.NewKey(c, "Root", "root", 0, nil),
			TombstonesDelay: time.Minute,
		}

		// Add one item.
		assert.Loosely(t, set.Add(c, []Item{{ID: "abc"}}), should.BeNil)

		// The item is returned by the listing.
		listing, err := set.List(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, listing.Items, should.Resemble([]Item{{ID: "abc"}}))
		assert.Loosely(t, listing.Garbage, should.BeNil)

		// Pop it!
		var cleanup Garbage
		err = datastore.RunInTransaction(c, func(c context.Context) error {
			popped, tombs, err := pop(c, &set, listing, []string{"abc"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, popped, should.Resemble([]string{"abc"}))
			assert.Loosely(t, len(tombs), should.Equal(1))
			assert.Loosely(t, tombs[0].id, should.Equal("abc"))
			assert.Loosely(t, len(tombs[0].storage), should.Equal(1))
			cleanup = tombs
			return nil
		}, nil)
		assert.Loosely(t, err, should.BeNil)

		// The listing no longer returns it, but we have a fresh tombstone that can
		// be cleaned up.
		listing, err = set.List(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, listing.Items, should.BeNil)
		assert.Loosely(t, len(listing.Garbage), should.Equal(1))
		assert.Loosely(t, listing.Garbage[0].id, should.Equal("abc"))

		// Cleaning up the storage using tombstones from Pop works.
		assert.Loosely(t, CleanupGarbage(c, cleanup), should.BeNil)

		// The listing no longer returns the item, and there's no tombstones to
		// cleanup.
		listing, err = set.List(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, listing.Items, should.BeNil)
		assert.Loosely(t, listing.Garbage, should.BeNil)

		// Attempt to add it back (should be ignored). Add a bunch of times to make
		// sure to fill in many shards (this is pseudo-random).
		for i := 0; i < 5; i++ {
			assert.Loosely(t, set.Add(c, []Item{{ID: "abc"}}), should.BeNil)
		}

		// The listing still doesn't returns it, but we now have a tombstone to
		// cleanup (again).
		listing, err = set.List(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, listing.Items, should.BeNil)
		assert.Loosely(t, len(listing.Garbage), should.Equal(1))
		assert.Loosely(t, listing.Garbage[0].old, should.BeFalse)
		assert.Loosely(t, len(listing.Garbage[0].storage), should.Equal(3)) // all shards

		// Popping it again doesn't work either.
		err = datastore.RunInTransaction(c, func(c context.Context) error {
			popped, tombs, err := pop(c, &set, listing, []string{"abc"})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, popped, should.BeNil)
			assert.Loosely(t, tombs, should.BeNil)
			return nil
		}, nil)
		assert.Loosely(t, err, should.BeNil)

		// Cleaning up the storage, again. This should make List stop returning
		// the tombstone (since it has no storage items associated with it and it's
		// not ready to be evicted yet).
		assert.Loosely(t, CleanupGarbage(c, listing.Garbage), should.BeNil)
		listing, err = set.List(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, listing.Items, should.BeNil)
		assert.Loosely(t, listing.Garbage, should.BeNil)

		// Time passes, tombstone expires.
		clock.Get(c).(testclock.TestClock).Add(2 * time.Minute)

		// Listing now returns expired tombstone.
		listing, err = set.List(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, listing.Items, should.BeNil)
		assert.Loosely(t, len(listing.Garbage), should.Equal(1))
		assert.Loosely(t, len(listing.Garbage[0].storage), should.BeZero) // cleaned already

		// Cleanup storage keys.
		assert.Loosely(t, CleanupGarbage(c, listing.Garbage), should.BeNil)

		// Cleanup the tombstones themselves.
		err = datastore.RunInTransaction(c, func(c context.Context) error {
			popped, tombs, err := pop(c, &set, listing, nil)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, popped, should.BeNil)
			assert.Loosely(t, tombs, should.BeNil)
			return nil
		}, nil)
		assert.Loosely(t, err, should.BeNil)

		// No tombstones returned any longer.
		listing, err = set.List(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, listing.Items, should.BeNil)
		assert.Loosely(t, listing.Garbage, should.BeNil)

		// And the item can be added back now, since no trace of it is left.
		assert.Loosely(t, set.Add(c, []Item{{ID: "abc"}}), should.BeNil)

		// Yep, it is there.
		listing, err = set.List(c)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, listing.Items, should.Resemble([]Item{{ID: "abc"}}))
		assert.Loosely(t, listing.Garbage, should.BeNil)
	})

	ftt.Run("stress", t, func(t *ftt.Test) {
		// Add 1000 items in parallel from N goroutines, and (also in parallel),
		// run N instances of "List and pop all", collecting the result in single
		// list. There should be no duplicates in the final list!
		c := testingContext()

		set := Set{
			ID:              "test",
			ShardCount:      3,
			TombstonesRoot:  datastore.NewKey(c, "Root", "root", 0, nil),
			TombstonesDelay: time.Minute,
		}

		producers := 3
		consumers := 5
		items := 100

		wakeups := make(chan string)

		lock := sync.Mutex{}
		var consumed []string

		for i := 0; i < producers; i++ {
			go func() {
				for j := 0; j < items; j++ {
					set.Add(c, []Item{{ID: fmt.Sprintf("%d", j)}})
					// Wake up 3 consumers, so they "fight".
					wakeups <- "wake"
					wakeups <- "wake"
					wakeups <- "wake"
				}
				for i := 0; i < consumers; i++ {
					wakeups <- "done"
				}
			}()
		}

		consume := func() {
			listing, err := set.List(c)
			if err != nil || len(listing.Items) == 0 {
				return
			}

			keys := make([]string, len(listing.Items))
			for i, itm := range listing.Items {
				keys[i] = itm.ID
			}

			// Try to pop all.
			var popped []string
			var tombs Garbage
			err = datastore.RunInTransaction(c, func(c context.Context) error {
				var err error
				popped, tombs, err = pop(c, &set, listing, keys)
				return err
			}, nil)
			// Best-effort storage cleanup on success.
			if err == nil {
				CleanupGarbage(c, tombs)
			}

			// Consider items consumed only if transaction has landed.
			if err == nil && len(popped) != 0 {
				lock.Lock()
				consumed = append(consumed, popped...)
				lock.Unlock()
			}
		}

		wg := sync.WaitGroup{}
		wg.Add(consumers)
		for i := 0; i < consumers; i++ {
			go func() {
				defer wg.Done()
				done := false
				for !done {
					done = (<-wakeups) == "done"
					consume()
				}
			}()
		}

		wg.Wait() // this waits for completion of the entire pipeline

		// Make sure 'consumed' is the initially produced set.
		dedup := stringset.New(len(consumed))
		for _, itm := range consumed {
			dedup.Add(itm)
		}
		assert.Loosely(t, dedup.Len(), should.Equal(len(consumed))) // no dups
		assert.Loosely(t, len(consumed), should.Equal(items))       // all are accounted for
	})
}
