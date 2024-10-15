// Copyright 2016 The LUCI Authors.
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

package treapstore

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/luci/gtreap"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
	"go.chromium.org/luci/common/testing/truth/should"
)

func stringCompare(a, b any) int {
	return strings.Compare(a.(string), b.(string))
}

func putMulti(c *Collection, vs ...string) {
	for _, v := range vs {
		c.Put(v)
	}
}

func visitAll(c *Collection, pivot string) []string {
	res := []string{}
	c.VisitAscend(pivot, func(v gtreap.Item) bool {
		res = append(res, v.(string))
		return true
	})
	return res
}

func iterAll(it *gtreap.Iterator) []string {
	all := []string{}
	for {
		v, ok := it.Next()
		if !ok {
			return all
		}
		all = append(all, v.(string))
	}
}

func shouldHaveKeys(expected ...string) comparison.Func[*Collection] {
	if expected == nil {
		expected = []string{}
	}

	return func(c *Collection) *failure.Summary {
		if fail := should.Match(expected)(iterAll(c.Iterator(""))); fail != nil {
			fail.Findings = append(fail.Findings, &failure.Finding{
				Name:  "shouldHaveKeys.Method",
				Value: []string{"iterAll"},
			})
			return fail
		}
		if fail := should.Match(expected)(visitAll(c, "")); fail != nil {
			fail.Findings = append(fail.Findings, &failure.Finding{
				Name:  "shouldHaveKeys.Method",
				Value: []string{"visitAll"},
			})
			return fail
		}
		return nil
	}
}

func TestStore(t *testing.T) {
	t.Parallel()

	ftt.Run(`Testing a string Store`, t, func(t *ftt.Test) {
		st := New()
		coll := st.CreateCollection("test", stringCompare)

		t.Run(`When empty`, func(t *ftt.Test) {
			checkEmpty := func(c *Collection) {
				assert.Loosely(t, c.Get("foo"), should.BeNil)
				assert.Loosely(t, c, shouldHaveKeys())
			}

			// Check the basic Store.
			checkEmpty(coll)

			// Take a snapshot, then mutate the base Store. Assert that the snapshot
			// is still empty.
			snap := st.Snapshot()
			coll.Put("foo")
			checkEmpty(snap.GetCollection("test"))
		})

		t.Run(`With keys`, func(t *ftt.Test) {
			putMulti(coll, "x", "w", "b", "a")

			t.Run(`Can iterate`, func(t *ftt.Test) {
				checkKeys := func(coll *Collection, keys ...string) {
					for _, k := range keys {
						assert.Loosely(t, coll.Get(k), should.Equal(k))
					}

					assert.Loosely(t, coll, shouldHaveKeys(keys...))
					for i, k := range keys {
						assert.Loosely(t, iterAll(coll.Iterator(k)), should.Resemble(keys[i:]))
						assert.Loosely(t, iterAll(coll.Iterator(k+"1")), should.Resemble(keys[i+1:]))
					}
				}
				checkKeys(coll, "a", "b", "w", "x")

				// Take a snapshot, then mutate the base Store. Assert that the snapshot
				// is still empty.
				snap := st.Snapshot()
				snapColl := snap.GetCollection("test")
				putMulti(coll, "foo")
				coll.Delete("b")
				checkKeys(snapColl, "a", "b", "w", "x")
				checkKeys(coll, "a", "foo", "w", "x")
			})

			t.Run(`Modified after a snapshot`, func(t *ftt.Test) {
				snap := st.Snapshot()
				snapColl := snap.GetCollection("test")
				putMulti(coll, "z")

				t.Run(`A snapshot of a snapshot is itself.`, func(t *ftt.Test) {
					assert.Loosely(t, snap.Snapshot(), should.Equal(snap))
				})

				t.Run(`A snapshot is read-only, and cannot create collections.`, func(t *ftt.Test) {
					assert.Loosely(t, func() { snap.CreateCollection("new", stringCompare) }, should.Panic)
				})

				t.Run(`Can get its collection name`, func(t *ftt.Test) {
					assert.Loosely(t, coll.Name(), should.Equal("test"))
					assert.Loosely(t, snapColl.Name(), should.Equal("test"))
				})

				t.Run(`Can fetch the Min and Max`, func(t *ftt.Test) {
					assert.Loosely(t, coll.Min(), should.Match("a"))
					assert.Loosely(t, coll.Max(), should.Match("z"))

					assert.Loosely(t, snapColl.Min(), should.Match("a"))
					assert.Loosely(t, snapColl.Max(), should.Match("x"))
				})

				t.Run(`Cannot Put to a read-only snapshot.`, func(t *ftt.Test) {
					assert.Loosely(t, func() { snapColl.Put("panic") }, should.Panic)
				})
			})
		})

		t.Run(`Creating a Collection with a duplicate name will panic.`, func(t *ftt.Test) {
			assert.Loosely(t, func() { st.CreateCollection("test", stringCompare) }, should.Panic)
		})

		t.Run(`With multiple Collections`, func(t *ftt.Test) {
			for _, v := range []string{"foo", "bar", "baz"} {
				st.CreateCollection(v, stringCompare)
			}
			assert.Loosely(t, st.GetCollectionNames(), should.Resemble([]string{"bar", "baz", "foo", "test"}))
			snap := st.Snapshot()
			assert.Loosely(t, snap.GetCollectionNames(), should.Resemble([]string{"bar", "baz", "foo", "test"}))

			t.Run(`When new Collections are added, names remain sorted.`, func(t *ftt.Test) {
				for _, v := range []string{"app", "cat", "bas", "qux"} {
					st.CreateCollection(v, stringCompare)
				}
				assert.Loosely(t, st.GetCollectionNames(), should.Resemble(
					[]string{"app", "bar", "bas", "baz", "cat", "foo", "qux", "test"}))
				assert.Loosely(t, st.Snapshot().GetCollectionNames(), should.Resemble(
					[]string{"app", "bar", "bas", "baz", "cat", "foo", "qux", "test"}))
				assert.Loosely(t, snap.GetCollectionNames(), should.Resemble([]string{"bar", "baz", "foo", "test"}))
			})
		})
	})
}

func TestStoreZeroValue(t *testing.T) {
	t.Parallel()

	ftt.Run(`A Store's zero value is valid, empty, and read-only.`, t, func(t *ftt.Test) {
		s := Store{}

		assert.Loosely(t, s.IsReadOnly(), should.BeTrue)
		assert.Loosely(t, s.GetCollectionNames(), should.BeNil)
		assert.Loosely(t, s.GetCollection("foo"), should.BeNil)
		assert.Loosely(t, s.Snapshot(), should.Equal(&s))
	})
}

func TestCollectionZeroValue(t *testing.T) {
	t.Parallel()

	ftt.Run(`A Collection's zero value is valid, empty, and read-only.`, t, func(t *ftt.Test) {
		c := Collection{}

		assert.Loosely(t, c.IsReadOnly(), should.BeTrue)
		assert.Loosely(t, c.Name(), should.BeEmpty)
		assert.Loosely(t, c.Get("foo"), should.BeNil)
		assert.Loosely(t, c.Min(), should.BeNil)
		assert.Loosely(t, c.Max(), should.BeNil)

		it := c.Iterator(nil)
		assert.Loosely(t, it, should.NotBeNil)
		assert.Loosely(t, iterAll(it), should.HaveLength(0))
	})
}

// TestStoreParallel performs several rounds of parallel accesses. Each round
// takes a snapshot of the "head" Store, then simultaneusly dispatches a round
// of parallel writes against the "head" store, reads against the snapshot, and
// reads against the "head" store.
//
// This is meant to be run with "-race" to trigger on race conditions.
func TestStoreParallel(t *testing.T) {
	t.Parallel()

	ftt.Run(`Testing a string Store for parallel access.`, t, func(t *ftt.Test) {
		const (
			readers = 128
			writers = 16
			rounds  = 8
		)

		head := New()
		head.CreateCollection("", stringCompare)
		var snaps []*Store

		// Dispatch readers.
		doReads := func() int {
			readDoneC := make(chan int, readers)
			for i := 0; i < readers; i++ {
				go func() {
					var (
						doneC = make(chan int, 1+len(snaps))
					)

					// "head"
					go func() {
						doneC <- len(iterAll(head.GetCollection("").Iterator("")))
					}()

					// "snap"
					for _, snap := range snaps {
						go func(snap *Store) {
							doneC <- len(iterAll(snap.GetCollection("").Iterator("")))
						}(snap)
					}

					total := 0
					for i := 0; i < 1+len(snaps); i++ {
						total += <-doneC
					}
					readDoneC <- total
				}()
			}

			total := 0
			for i := 0; i < readers; i++ {
				total += <-readDoneC
			}
			return total
		}

		// Dispatch writers.
		doWrite := func(base string) {
			writeDoneC := make(chan struct{}, writers)
			for i := 0; i < writers; i++ {
				go func(idx int) {
					head.GetCollection("").Put(fmt.Sprintf("%s.%d", base, idx))
					writeDoneC <- struct{}{}
				}(i)
			}

			for i := 0; i < writers; i++ {
				<-writeDoneC
			}
		}

		// Main loop.
		for i := 0; i < rounds; i++ {
			writeDoneC := make(chan struct{})
			readDoneC := make(chan int)
			go func() {
				doWrite(strconv.Itoa(i))
				close(writeDoneC)
			}()
			// The first round has to actually create the Collection.
			go func() {
				readDoneC <- doReads()
			}()

			<-writeDoneC

			var check func(int) comparison.Func[int] = should.BeGreaterThan[int]
			if i == 0 {
				// The first time around, we *could* read before anything has been
				// written. Every other time, something from the previous round will
				// have been written.
				check = should.BeGreaterThanOrEqual[int]
			}
			assert.Loosely(t, <-readDoneC, check(0))
			snaps = append(snaps, head.Snapshot())
		}
	})
}
