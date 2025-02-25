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
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
)

func testingContext() context.Context {
	c := txndefer.FilterRDS(memory.Use(context.Background()))
	datastore.GetTestable(c).AutoIndex(true)
	datastore.GetTestable(c).Consistent(true)
	c = clock.Set(c, testclock.New(time.Unix(1442270520, 0).UTC()))
	c = mathrand.Set(c, rand.New(rand.NewSource(1000)))
	return c
}

// pop pops a bunch of items from the set and returns items that were popped.
func pop(c context.Context, s *Set, listing *Listing, ids []string) (popped []string, err error) {
	op, err := s.BeginPop(c, listing)
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if op.Pop(id) {
			popped = append(popped, id)
		}
	}
	if err = FinishPop(c, op); err != nil {
		return nil, err
	}
	return popped, nil
}

func TestSet(t *testing.T) {
	t.Parallel()

	ftt.Run("item one lifecycle", t, func(t *ftt.Test) {
		c := testingContext()

		set := Set{
			Parent:          datastore.NewKey(c, "Parent", "parent", 0, nil),
			TombstonesDelay: time.Minute,
		}
		const limit = 10

		// Add one item.
		assert.Loosely(t, set.Add(c, []Item{{ID: "abc"}}), should.BeNil)

		// The item is returned by the listing.
		listing, err := set.List(c, limit)
		assert.NoErr(t, err)
		assert.Loosely(t, listing.Items, should.Match([]Item{{ID: "abc"}}))
		assert.Loosely(t, listing.Garbage, should.BeNil)

		// Pop it!
		err = datastore.RunInTransaction(c, func(c context.Context) error {
			popped, err := pop(c, &set, listing, []string{"abc"})
			assert.NoErr(t, err)
			assert.Loosely(t, popped, should.Match([]string{"abc"}))
			return nil
		}, nil)
		assert.NoErr(t, err)

		// The listing no longer returns it.
		listing, err = set.List(c, limit)
		assert.NoErr(t, err)
		assert.Loosely(t, listing.Items, should.BeNil)

		// The listing no longer returns the item, and there's no tombstones to
		// cleanup.
		listing, err = set.List(c, limit)
		assert.NoErr(t, err)
		assert.Loosely(t, listing.Items, should.BeNil)
		assert.Loosely(t, listing.Garbage, should.BeNil)

		// Attempt to add it back (should be ignored).
		assert.Loosely(t, set.Add(c, []Item{{ID: "abc"}}), should.BeNil)

		// The listing still doesn't return it, but we now have a tombstone to
		// cleanup (again).
		listing, err = set.List(c, limit)
		assert.NoErr(t, err)
		assert.Loosely(t, listing.Items, should.BeNil)
		assert.Loosely(t, len(listing.Garbage), should.Equal(1))
		assert.Loosely(t, listing.Garbage[0].old, should.BeFalse)
		assert.Loosely(t, listing.Garbage[0].storage, should.NotBeNil)

		// Popping it again doesn't work either.
		err = datastore.RunInTransaction(c, func(c context.Context) error {
			popped, err := pop(c, &set, listing, []string{"abc"})
			assert.NoErr(t, err)
			assert.Loosely(t, popped, should.BeNil)
			return nil
		}, nil)
		assert.NoErr(t, err)

		// Cleaning up the storage, again. This should make List stop returning
		// the tombstone (since it has no storage items associated with it and it's
		// not ready to be evicted yet).
		assert.Loosely(t, CleanupGarbage(c, listing.Garbage), should.BeNil)
		listing, err = set.List(c, limit)
		assert.NoErr(t, err)
		assert.Loosely(t, listing.Items, should.BeNil)
		assert.Loosely(t, listing.Garbage, should.BeNil)

		// Time passes, tombstone expires.
		clock.Get(c).(testclock.TestClock).Add(2 * time.Minute)

		// Listing now returns expired tombstone.
		listing, err = set.List(c, limit)
		assert.NoErr(t, err)
		assert.Loosely(t, listing.Items, should.BeNil)
		assert.Loosely(t, len(listing.Garbage), should.Equal(1))
		assert.Loosely(t, listing.Garbage[0].storage, should.BeNil) // cleaned already

		// Cleanup storage keys.
		assert.Loosely(t, CleanupGarbage(c, listing.Garbage), should.BeNil)

		// Cleanup the tombstones themselves.
		err = datastore.RunInTransaction(c, func(c context.Context) error {
			popped, err := pop(c, &set, listing, nil)
			assert.NoErr(t, err)
			assert.Loosely(t, popped, should.BeNil)
			return nil
		}, nil)
		assert.NoErr(t, err)

		// No tombstones returned any longer.
		listing, err = set.List(c, limit)
		assert.NoErr(t, err)
		assert.Loosely(t, listing.Items, should.BeNil)
		assert.Loosely(t, listing.Garbage, should.BeNil)

		// And the item can be added back now, since no trace of it is left.
		assert.Loosely(t, set.Add(c, []Item{{ID: "abc"}}), should.BeNil)

		// Yep, it is there.
		listing, err = set.List(c, limit)
		assert.NoErr(t, err)
		assert.Loosely(t, listing.Items, should.Match([]Item{{ID: "abc"}}))
		assert.Loosely(t, listing.Garbage, should.BeNil)
	})

	ftt.Run("List obeys limit", t, func(t *ftt.Test) {
		c := testingContext()
		set := Set{
			Parent:          datastore.MakeKey(c, "Parent", "parent"),
			TombstonesDelay: time.Minute,
		}
		assert.Loosely(t, set.Add(c, []Item{{ID: "abc"}}), should.BeNil)
		assert.Loosely(t, set.Add(c, []Item{{ID: "def"}}), should.BeNil)
		assert.Loosely(t, set.Add(c, []Item{{ID: "ghi"}}), should.BeNil)

		l, err := set.List(c, 2)
		assert.NoErr(t, err)
		assert.Loosely(t, l.Items, should.HaveLength(2))
	})

	ftt.Run("delete items non-transactionally", t, func(t *ftt.Test) {
		c := testingContext()

		set := Set{
			Parent:          datastore.MakeKey(c, "Parent", "parent"),
			TombstonesDelay: time.Minute,
		}

		// Add 3 items.
		assert.Loosely(t, set.Add(c, []Item{{ID: "abc"}}), should.BeNil)
		assert.Loosely(t, set.Add(c, []Item{{ID: "def"}}), should.BeNil)
		assert.Loosely(t, set.Add(c, []Item{{ID: "ghi"}}), should.BeNil)

		l, err := set.List(c, 10)
		assert.NoErr(t, err)
		assert.Loosely(t, l.Items, should.HaveLength(3))

		// Delete 2 items before transacting.
		i := 0
		err = set.Delete(c, func() string {
			switch i = i + 1; i {
			case 1:
				return "def"
			case 2:
				return "abc"
			default:
				return ""
			}
		})
		assert.NoErr(t, err)

		l2, err := set.List(c, 10)
		assert.NoErr(t, err)
		assert.Loosely(t, l2.Items, should.Match([]Item{{ID: "ghi"}}))
	})
}

func TestStress(t *testing.T) {
	t.Parallel()

	ftt.Run("stress", t, func(t *ftt.Test) {
		// Add 1000 items in parallel from N goroutines, and (also in parallel),
		// run N instances of "List and pop all", collecting the result in single
		// list. There should be no duplicates in the final list!
		c := testingContext()

		set := Set{
			Parent:          datastore.MakeKey(c, "Parent", "parent"),
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
			listing, err := set.List(c, 100000)
			if err != nil || len(listing.Items) == 0 {
				return
			}

			keys := make([]string, len(listing.Items))
			for i, itm := range listing.Items {
				keys[i] = itm.ID
			}

			// Try to pop all.
			var popped []string
			err = datastore.RunInTransaction(c, func(c context.Context) error {
				var err error
				popped, err = pop(c, &set, listing, keys)
				return err
			}, nil)

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
