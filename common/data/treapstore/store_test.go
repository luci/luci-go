// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package treapstore

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/luci/gtreap"

	. "github.com/smartystreets/goconvey/convey"
)

func stringCompare(a, b interface{}) int {
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

func shouldHaveKeys(actual interface{}, expected ...interface{}) string {
	c := actual.(*Collection)

	// expected can either be a single []string or a series of strings.
	var keys []string
	var ok bool
	if len(expected) == 1 {
		keys, ok = expected[0].([]string)
	}
	if !ok {
		keys = make([]string, len(expected))
		for i, v := range expected {
			keys[i] = v.(string)
		}
	}

	if err := ShouldResemble(iterAll(c.Iterator("")), keys); err != "" {
		return fmt.Sprintf("failed via iterator: %s", err)
	}
	if err := ShouldResemble(visitAll(c, ""), keys); err != "" {
		return fmt.Sprintf("failed via visit: %s", err)
	}
	return ""
}

func TestStore(t *testing.T) {
	t.Parallel()

	Convey(`Testing a string Store`, t, func() {
		st := New()
		coll := st.CreateCollection("test", stringCompare)

		Convey(`When empty`, func() {
			checkEmpty := func(c *Collection) {
				So(c.Get("foo"), ShouldBeNil)
				So(c, shouldHaveKeys)
			}

			// Check the basic Store.
			checkEmpty(coll)

			// Take a snapshot, then mutate the base Store. Assert that the snapshot
			// is still empty.
			snap := st.Snapshot()
			coll.Put("foo")
			checkEmpty(snap.GetCollection("test"))
		})

		Convey(`With keys`, func() {
			putMulti(coll, "x", "w", "b", "a")

			Convey(`Can iterate`, func() {
				checkKeys := func(coll *Collection, keys ...string) {
					for _, k := range keys {
						So(coll.Get(k), ShouldEqual, k)
					}

					So(coll, shouldHaveKeys, keys)
					for i, k := range keys {
						So(iterAll(coll.Iterator(k)), ShouldResemble, keys[i:])
						So(iterAll(coll.Iterator(k+"1")), ShouldResemble, keys[i+1:])
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

			Convey(`Modified after a snapshot`, func() {
				snap := st.Snapshot()
				snapColl := snap.GetCollection("test")
				putMulti(coll, "z")

				Convey(`A snapshot of a snapshot is itself.`, func() {
					So(snap.Snapshot(), ShouldEqual, snap)
				})

				Convey(`A snapshot is read-only, and cannot create collections.`, func() {
					So(func() { snap.CreateCollection("new", stringCompare) }, ShouldPanic)
				})

				Convey(`Can get its collection name`, func() {
					So(coll.Name(), ShouldEqual, "test")
					So(snapColl.Name(), ShouldEqual, "test")
				})

				Convey(`Can fetch the Min and Max`, func() {
					So(coll.Min(), ShouldResemble, "a")
					So(coll.Max(), ShouldResemble, "z")

					So(snapColl.Min(), ShouldResemble, "a")
					So(snapColl.Max(), ShouldResemble, "x")
				})

				Convey(`Cannot Put to a read-only snapshot.`, func() {
					So(func() { snapColl.Put("panic") }, ShouldPanic)
				})
			})
		})

		Convey(`Creating a Collection with a duplicate name will panic.`, func() {
			So(func() { st.CreateCollection("test", stringCompare) }, ShouldPanic)
		})

		Convey(`With multiple Collections`, func() {
			for _, v := range []string{"foo", "bar", "baz"} {
				st.CreateCollection(v, stringCompare)
			}
			So(st.GetCollectionNames(), ShouldResemble, []string{"bar", "baz", "foo", "test"})
			snap := st.Snapshot()
			So(snap.GetCollectionNames(), ShouldResemble, []string{"bar", "baz", "foo", "test"})

			Convey(`When new Collections are added, names remain sorted.`, func() {
				for _, v := range []string{"app", "cat", "bas", "qux"} {
					st.CreateCollection(v, stringCompare)
				}
				So(st.GetCollectionNames(), ShouldResemble,
					[]string{"app", "bar", "bas", "baz", "cat", "foo", "qux", "test"})
				So(st.Snapshot().GetCollectionNames(), ShouldResemble,
					[]string{"app", "bar", "bas", "baz", "cat", "foo", "qux", "test"})
				So(snap.GetCollectionNames(), ShouldResemble, []string{"bar", "baz", "foo", "test"})
			})
		})
	})
}

func TestStoreZeroValue(t *testing.T) {
	t.Parallel()

	Convey(`A Store's zero value is valid, empty, and read-only.`, t, func() {
		s := Store{}

		So(s.IsReadOnly(), ShouldBeTrue)
		So(s.GetCollectionNames(), ShouldBeNil)
		So(s.GetCollection("foo"), ShouldBeNil)
		So(s.Snapshot(), ShouldEqual, &s)
	})
}

func TestCollectionZeroValue(t *testing.T) {
	t.Parallel()

	Convey(`A Collection's zero value is valid, empty, and read-only.`, t, func() {
		c := Collection{}

		So(c.IsReadOnly(), ShouldBeTrue)
		So(c.Name(), ShouldEqual, "")
		So(c.Get("foo"), ShouldBeNil)
		So(c.Min(), ShouldBeNil)
		So(c.Max(), ShouldBeNil)

		it := c.Iterator(nil)
		So(it, ShouldNotBeNil)
		So(iterAll(it), ShouldHaveLength, 0)
	})
}

// TestStoreParallel performs several rounds of parallel accesses. Each round
// takes a snapshot of the "head" Store, then simultaneusly dispatches a round
// of parallel writes agains the "head" store, reads against the snapshot, and
// reads against the "head" store.
//
// This is meant to be run with "-race" to trigger on race conditions.
func TestStoreParallel(t *testing.T) {
	t.Parallel()

	Convey(`Testing a string Store for parallel access.`, t, func() {
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

			check := ShouldBeGreaterThan
			if i == 0 {
				// The first time around, we *could* read before anything has been
				// written. Every other time, something from the previous round will
				// have been written.
				check = ShouldBeGreaterThanOrEqualTo
			}
			So(<-readDoneC, check, 0)
			snaps = append(snaps, head.Snapshot())
		}
	})
}
