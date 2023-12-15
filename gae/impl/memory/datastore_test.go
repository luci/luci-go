// Copyright 2015 The LUCI Authors.
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

package memory

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	ds "go.chromium.org/luci/gae/service/datastore"
	infoS "go.chromium.org/luci/gae/service/info"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type MetaGroup struct {
	_id    int64   `gae:"$id,1"`
	_kind  string  `gae:"$kind,__entity_group__"`
	Parent *ds.Key `gae:"$parent"`

	Version int64 `gae:"__version__"`
}

func testGetMeta(c context.Context, k *ds.Key) int64 {
	mg := &MetaGroup{Parent: k.Root()}
	if err := ds.Get(c, mg); err != nil {
		panic(err)
	}
	return mg.Version
}

type Nested struct {
	Inner int
}

type Foo struct {
	ID     int64   `gae:"$id"`
	Parent *ds.Key `gae:"$parent"`

	Val    int
	Name   string
	Multi  []string
	Key    *ds.Key
	Nested Nested `gae:",lsp"`

	Scatter []byte `gae:"__scatter__"` // this is normally invisible
}

func TestDatastoreSingleReadWriter(t *testing.T) {
	t.Parallel()

	Convey("Datastore single reads and writes", t, func() {
		c := Use(context.Background())
		So(ds.Raw(c), ShouldNotBeNil)

		Convey("getting objects that DNE is an error", func() {
			So(ds.Get(c, &Foo{ID: 1}), ShouldEqual, ds.ErrNoSuchEntity)
		})

		Convey("bad namespaces fail", func() {
			_, err := infoS.Namespace(c, "$$blzyall")
			So(err.Error(), ShouldContainSubstring, "namespace \"$$blzyall\" does not match")
		})

		Convey("Can Put stuff", func() {
			// with an incomplete key!
			f := &Foo{
				Val:   10,
				Multi: []string{"foo", "bar"},
				Key:   ds.MakeKey(c, "Bar", "Baz"),
				Nested: Nested{
					Inner: 456,
				},
			}
			So(ds.Put(c, f), ShouldBeNil)
			k := ds.KeyForObj(c, f)
			So(k.String(), ShouldEqual, "dev~app::/Foo,1")

			Convey("and Get it back", func() {
				newFoo := &Foo{ID: 1}
				So(ds.Get(c, newFoo), ShouldBeNil)
				So(newFoo, ShouldResemble, f)

				Convey("but it's hidden from a different namespace", func() {
					c, err := infoS.Namespace(c, "whombat")
					So(err, ShouldBeNil)
					So(ds.Get(c, f), ShouldEqual, ds.ErrNoSuchEntity)
				})

				Convey("and we can Delete it", func() {
					So(ds.Delete(c, k), ShouldBeNil)
					So(ds.Get(c, newFoo), ShouldEqual, ds.ErrNoSuchEntity)
				})

			})
			Convey("Can Get it back as a PropertyMap", func() {
				pmap := ds.PropertyMap{
					"$id":   propNI(1),
					"$kind": propNI("Foo"),
				}
				So(ds.Get(c, pmap), ShouldBeNil)
				So(pmap, ShouldResemble, ds.PropertyMap{
					"$id":   propNI(1),
					"$kind": propNI("Foo"),
					"Name":  prop(""),
					"Val":   prop(10),
					"Multi": ds.PropertySlice{prop("foo"), prop("bar")},
					"Key":   prop(ds.MkKeyContext("dev~app", "").MakeKey("Bar", "Baz")),
					"Nested": prop(ds.PropertyMap{
						"Inner": prop(456),
					}),
				})
			})
			Convey("Deleting with a bogus key is bad", func() {
				So(ds.IsErrInvalidKey(ds.Delete(c, ds.NewKey(c, "Foo", "wat", 100, nil))), ShouldBeTrue)
			})
			Convey("Deleting a DNE entity is fine", func() {
				So(ds.Delete(c, ds.NewKey(c, "Foo", "wat", 0, nil)), ShouldBeNil)
			})

			Convey("Deleting entities from a nonexistant namespace works", func(gctx C) {
				c := infoS.MustNamespace(c, "noexist")
				keys := make([]*ds.Key, 10)
				for i := range keys {
					keys[i] = ds.MakeKey(c, "Kind", i+1)
				}
				So(ds.Delete(c, keys), ShouldBeNil)
				count := 0
				So(ds.Raw(c).DeleteMulti(keys, func(idx int, err error) {
					gctx.So(idx, ShouldEqual, count)
					gctx.So(err, ShouldBeNil)

					count++
				}), ShouldBeNil)
				So(count, ShouldEqual, len(keys))
			})

			Convey("with multiple puts", func() {
				So(testGetMeta(c, k), ShouldEqual, 1)

				foos := make([]Foo, 10)
				for i := range foos {
					foos[i].Val = 10
					foos[i].Parent = k
				}
				So(ds.Put(c, foos), ShouldBeNil)
				So(testGetMeta(c, k), ShouldEqual, 11)

				keys := make([]*ds.Key, len(foos))
				for i, f := range foos {
					keys[i] = ds.KeyForObj(c, &f)
				}

				Convey("ensure that group versions persist across deletes", func() {
					So(ds.Delete(c, append(keys, k)), ShouldBeNil)

					ds.GetTestable(c).CatchupIndexes()

					count := 0
					So(ds.Run(c, ds.NewQuery(""), func(_ *ds.Key) {
						count++
					}), ShouldBeNil)
					So(count, ShouldEqual, 2)

					So(testGetMeta(c, k), ShouldEqual, 22)

					So(ds.Put(c, &Foo{ID: 1}), ShouldBeNil)
					So(testGetMeta(c, k), ShouldEqual, 23)
				})

				Convey("can Get", func() {
					vals := make([]ds.PropertyMap, len(keys))
					for i := range vals {
						vals[i] = ds.PropertyMap{}
						So(vals[i].SetMeta("key", keys[i]), ShouldBeTrue)
					}
					So(ds.Get(c, vals), ShouldBeNil)

					for i, val := range vals {
						So(val, ShouldResemble, ds.PropertyMap{
							"Val":  ds.MkProperty(10),
							"Name": ds.MkProperty(""),
							"$key": ds.MkPropertyNI(keys[i]),
							"Key":  ds.MkProperty(nil),
							"Nested": ds.MkProperty(ds.PropertyMap{
								"Inner": ds.MkProperty(0),
							}),
						})
					}
				})

			})

			Convey("allocating ids prevents their use", func() {
				keys := ds.NewIncompleteKeys(c, 100, "Foo", nil)
				So(ds.AllocateIDs(c, keys), ShouldBeNil)
				So(len(keys), ShouldEqual, 100)

				// Assert that none of our keys share the same ID.
				ids := make(map[int64]struct{})
				for _, k := range keys {
					ids[k.IntID()] = struct{}{}
				}
				So(len(ids), ShouldEqual, len(keys))

				// Put a new object and ensure that it is allocated an unused ID.
				f := &Foo{Val: 10}
				So(ds.Put(c, f), ShouldBeNil)
				k := ds.KeyForObj(c, f)
				So(k.String(), ShouldEqual, "dev~app::/Foo,102")

				_, ok := ids[k.IntID()]
				So(ok, ShouldBeFalse)
			})
		})

		Convey("implements DSTransactioner", func() {
			Convey("Put", func() {
				f := &Foo{Val: 10}
				So(ds.Put(c, f), ShouldBeNil)
				k := ds.KeyForObj(c, f)
				So(k.String(), ShouldEqual, "dev~app::/Foo,1")

				Convey("can describe its transaction state", func() {
					So(ds.CurrentTransaction(c), ShouldBeNil)

					err := ds.RunInTransaction(c, func(c context.Context) error {
						So(ds.CurrentTransaction(c), ShouldNotBeNil)

						// Can reset to nil.
						nc := ds.WithoutTransaction(c)
						So(ds.CurrentTransaction(nc), ShouldBeNil)
						return nil
					}, nil)
					So(err, ShouldBeNil)
				})

				Convey("can Put new entity groups", func() {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						f := &Foo{Val: 100}
						So(ds.Put(c, f), ShouldBeNil)
						So(f.ID, ShouldEqual, 2)

						f.ID = 0
						f.Val = 200
						So(ds.Put(c, f), ShouldBeNil)
						So(f.ID, ShouldEqual, 3)

						return nil
					}, nil)
					So(err, ShouldBeNil)

					f := &Foo{ID: 2}
					So(ds.Get(c, f), ShouldBeNil)
					So(f.Val, ShouldEqual, 100)

					f.ID = 3
					So(ds.Get(c, f), ShouldBeNil)
					So(f.Val, ShouldEqual, 200)
				})

				Convey("can Put new entities in a current group", func() {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						f := &Foo{Val: 100, Parent: k}
						So(ds.Put(c, f), ShouldBeNil)
						So(ds.KeyForObj(c, f).String(), ShouldEqual, "dev~app::/Foo,1/Foo,1")

						f.ID = 0
						f.Val = 200
						So(ds.Put(c, f), ShouldBeNil)
						So(ds.KeyForObj(c, f).String(), ShouldEqual, "dev~app::/Foo,1/Foo,2")

						return nil
					}, nil)
					So(err, ShouldBeNil)

					f := &Foo{ID: 1, Parent: k}
					So(ds.Get(c, f), ShouldBeNil)
					So(f.Val, ShouldEqual, 100)

					f.ID = 2
					So(ds.Get(c, f), ShouldBeNil)
					So(f.Val, ShouldEqual, 200)
				})

				Convey("Deletes work too", func() {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						return ds.Delete(c, k)
					}, nil)
					So(err, ShouldBeNil)
					So(ds.Get(c, &Foo{ID: 1}), ShouldEqual, ds.ErrNoSuchEntity)
				})

				Convey("Get takes a snapshot", func() {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						So(ds.Get(c, f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)

						// Don't ever do this in a real program unless you want to guarantee
						// a failed transaction :)
						f.Val = 11
						So(ds.Put(ds.WithoutTransaction(c), f), ShouldBeNil)

						So(ds.Get(c, f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)

						return nil
					}, nil)
					So(err, ShouldBeNil)

					f := &Foo{ID: 1}
					So(ds.Get(c, f), ShouldBeNil)
					So(f.Val, ShouldEqual, 11)
				})

				Convey("and snapshots are consistent even after Puts", func() {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						f := &Foo{ID: 1}
						So(ds.Get(c, f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)

						// Don't ever do this in a real program unless you want to guarantee
						// a failed transaction :)
						f.Val = 11
						So(ds.Put(ds.WithoutTransaction(c), f), ShouldBeNil)

						So(ds.Get(c, f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)

						f.Val = 20
						So(ds.Put(c, f), ShouldBeNil)

						So(ds.Get(c, f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10) // still gets 10

						return nil
					}, &ds.TransactionOptions{Attempts: 1})
					So(err.Error(), ShouldContainSubstring, "concurrent")

					f := &Foo{ID: 1}
					So(ds.Get(c, f), ShouldBeNil)
					So(f.Val, ShouldEqual, 11)
				})

				Convey("Reusing a transaction context is bad news", func() {
					var txnCtx context.Context
					err := ds.RunInTransaction(c, func(c context.Context) error {
						txnCtx = c
						So(ds.Get(c, f), ShouldBeNil)
						return nil
					}, nil)
					So(err, ShouldBeNil)
					So(ds.Get(txnCtx, f).Error(), ShouldContainSubstring, "expired")
				})

				Convey("Nested transactions are rejected", func() {
					err := ds.RunInTransaction(c, func(c context.Context) error {
						err := ds.RunInTransaction(c, func(c context.Context) error {
							panic("noooo")
						}, nil)
						So(err.Error(), ShouldContainSubstring, "nested transactions")
						return nil
					}, nil)
					So(err, ShouldBeNil)
				})

				Convey("Transactions can be escaped.", func() {
					testError := errors.New("test error")
					noTxnPM := ds.PropertyMap{
						"$kind": ds.MkProperty("Test"),
						"$id":   ds.MkProperty("no txn"),
					}

					err := ds.RunInTransaction(c, func(c context.Context) error {
						So(ds.CurrentTransaction(c), ShouldNotBeNil)

						pmap := ds.PropertyMap{
							"$kind": ds.MkProperty("Test"),
							"$id":   ds.MkProperty("quux"),
						}
						if err := ds.Put(c, pmap); err != nil {
							return err
						}

						// Put an entity outside of the transaction so we can confirm that
						// it was added even when the transaction fails.
						if err := ds.Put(ds.WithoutTransaction(c), noTxnPM); err != nil {
							return err
						}
						return testError
					}, nil)
					So(err, ShouldEqual, testError)

					// Confirm that noTxnPM was added.
					So(ds.CurrentTransaction(c), ShouldBeNil)
					So(ds.Get(c, noTxnPM), ShouldBeNil)
				})

				Convey("Concurrent transactions only accept one set of changes", func() {
					// Note: I think this implementation is actually /slightly/ wrong.
					// According to my read of the docs for appengine, when you open a
					// transaction it actually (essentially) holds a reference to the
					// entire datastore. Our implementation takes a snapshot of the
					// entity group as soon as something observes/affects it.
					//
					// That said... I'm not sure if there's really a semantic difference.
					err := ds.RunInTransaction(c, func(c context.Context) error {
						So(ds.Put(c, &Foo{ID: 1, Val: 21}), ShouldBeNil)

						err := ds.RunInTransaction(ds.WithoutTransaction(c), func(c context.Context) error {
							So(ds.Put(c, &Foo{ID: 1, Val: 27}), ShouldBeNil)
							return nil
						}, nil)
						So(err, ShouldBeNil)

						return nil
					}, nil)
					So(err.Error(), ShouldContainSubstring, "concurrent")

					f := &Foo{ID: 1}
					So(ds.Get(c, f), ShouldBeNil)
					So(f.Val, ShouldEqual, 27)
				})

				Convey("Errors and panics", func() {
					Convey("returning an error aborts", func() {
						err := ds.RunInTransaction(c, func(c context.Context) error {
							So(ds.Put(c, &Foo{ID: 1, Val: 200}), ShouldBeNil)
							return fmt.Errorf("thingy")
						}, nil)
						So(err.Error(), ShouldEqual, "thingy")

						f := &Foo{ID: 1}
						So(ds.Get(c, f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)
					})

					Convey("panicing aborts", func() {
						So(func() {
							So(ds.RunInTransaction(c, func(c context.Context) error {
								So(ds.Put(c, &Foo{Val: 200}), ShouldBeNil)
								panic("wheeeeee")
							}, nil), ShouldBeNil)
						}, ShouldPanic)

						f := &Foo{ID: 1}
						So(ds.Get(c, f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)
					})
				})

				Convey("Transaction retries", func() {
					tst := ds.GetTestable(c)
					Reset(func() { tst.SetTransactionRetryCount(0) })

					Convey("SetTransactionRetryCount set to zero", func() {
						tst.SetTransactionRetryCount(0)
						calls := 0
						So(ds.RunInTransaction(c, func(c context.Context) error {
							calls++
							return nil
						}, nil), ShouldBeNil)
						So(calls, ShouldEqual, 1)
					})

					Convey("default TransactionOptions is 3 attempts", func() {
						tst.SetTransactionRetryCount(100) // more than 3
						calls := 0
						So(ds.RunInTransaction(c, func(c context.Context) error {
							calls++
							return nil
						}, nil), ShouldEqual, ds.ErrConcurrentTransaction)
						So(calls, ShouldEqual, 3)
					})

					Convey("non-default TransactionOptions ", func() {
						tst.SetTransactionRetryCount(100) // more than 20
						calls := 0
						So(ds.RunInTransaction(c, func(c context.Context) error {
							calls++
							return nil
						}, &ds.TransactionOptions{Attempts: 20}), ShouldEqual, ds.ErrConcurrentTransaction)
						So(calls, ShouldEqual, 20)
					})

					Convey("SetTransactionRetryCount is respected", func() {
						tst.SetTransactionRetryCount(1) // less than 3
						calls := 0
						So(ds.RunInTransaction(c, func(c context.Context) error {
							calls++
							return nil
						}, nil), ShouldBeNil)
						So(calls, ShouldEqual, 2)
					})

					Convey("fatal errors are not retried", func() {
						tst.SetTransactionRetryCount(1)
						calls := 0
						So(ds.RunInTransaction(c, func(c context.Context) error {
							calls++
							return fmt.Errorf("omg")
						}, nil).Error(), ShouldEqual, "omg")
						So(calls, ShouldEqual, 1)
					})
				})

				Convey("Read-only transactions reject writes", func() {
					So(ds.Put(c, &Foo{ID: 1, Val: 100}), ShouldBeNil)
					var val int

					So(ds.RunInTransaction(c, func(c context.Context) error {
						foo := &Foo{ID: 1}
						So(ds.Get(c, foo), ShouldBeNil)
						val = foo.Val

						foo.Val = 1337
						return ds.Put(c, foo)
					}, &ds.TransactionOptions{ReadOnly: true}), ShouldErrLike, "Attempting to write")
					So(val, ShouldResemble, 100)
					foo := &Foo{ID: 1}
					So(ds.Get(c, foo), ShouldBeNil)
					So(foo.Val, ShouldResemble, 100)
				})

				Convey("Read-only transactions reject deletes", func() {
					So(ds.Put(c, &Foo{ID: 1, Val: 100}), ShouldBeNil)
					var val int

					So(ds.RunInTransaction(c, func(c context.Context) error {
						foo := &Foo{ID: 1}
						So(ds.Get(c, foo), ShouldBeNil)
						val = foo.Val

						return ds.Delete(c, foo)
					}, &ds.TransactionOptions{ReadOnly: true}), ShouldErrLike, "Attempting to delete")
					So(val, ShouldResemble, 100)
					So(ds.Get(c, &Foo{ID: 1}), ShouldBeNil)
				})
			})
		})

		Convey("Testable.Consistent", func() {
			Convey("false", func() {
				ds.GetTestable(c).Consistent(false) // the default
				for i := 0; i < 10; i++ {
					So(ds.Put(c, &Foo{ID: int64(i + 1), Val: i + 1}), ShouldBeNil)
				}
				q := ds.NewQuery("Foo").Gt("Val", 3)
				count, err := ds.Count(c, q)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 0)

				So(ds.Delete(c, ds.MakeKey(c, "Foo", 4)), ShouldBeNil)

				count, err = ds.Count(c, q)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 0)

				ds.GetTestable(c).Consistent(true)
				count, err = ds.Count(c, q)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 6)
			})

			Convey("true", func() {
				ds.GetTestable(c).Consistent(true)
				for i := 0; i < 10; i++ {
					So(ds.Put(c, &Foo{ID: int64(i + 1), Val: i + 1}), ShouldBeNil)
				}
				q := ds.NewQuery("Foo").Gt("Val", 3)
				count, err := ds.Count(c, q)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 7)

				So(ds.Delete(c, ds.MakeKey(c, "Foo", 4)), ShouldBeNil)

				count, err = ds.Count(c, q)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 6)
			})
		})

		Convey("Testable.DisableSpecialEntities", func() {
			ds.GetTestable(c).DisableSpecialEntities(true)

			So(ds.Put(c, &Foo{}), ShouldErrLike, "allocateIDs is disabled")

			So(ds.Put(c, &Foo{ID: 1}), ShouldBeNil)

			ds.GetTestable(c).CatchupIndexes()

			count, err := ds.Count(c, ds.NewQuery(""))
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1) // normally this would include __entity_group__
		})

		Convey("Datastore namespace interaction", func() {
			run := func(rc context.Context, txn bool) (putErr, getErr, queryErr, countErr error) {
				var foo Foo

				putFunc := func(doC context.Context) error {
					return ds.Put(doC, &foo)
				}

				doFunc := func(doC context.Context) {
					getErr = ds.Get(doC, &foo)

					q := ds.NewQuery("Foo").Ancestor(ds.KeyForObj(doC, &foo))
					queryErr = ds.Run(doC, q, func(f *Foo) error { return nil })
					_, countErr = ds.Count(doC, q)
				}

				if txn {
					putErr = ds.RunInTransaction(rc, func(ic context.Context) error {
						return putFunc(ic)
					}, nil)
					if putErr != nil {
						return
					}

					ds.GetTestable(rc).CatchupIndexes()
					ds.RunInTransaction(rc, func(ic context.Context) error {
						doFunc(ic)
						return nil
					}, nil)
				} else {
					putErr = putFunc(rc)
					if putErr != nil {
						return
					}
					ds.GetTestable(rc).CatchupIndexes()
					doFunc(rc)
				}
				return
			}

			for _, txn := range []bool{false, true} {
				Convey(fmt.Sprintf("In transaction? %v", txn), func() {
					Convey("With no namespace installed, can Put, Get, Query, and Count.", func() {
						So(infoS.GetNamespace(c), ShouldEqual, "")

						putErr, getErr, queryErr, countErr := run(c, txn)
						So(putErr, ShouldBeNil)
						So(getErr, ShouldBeNil)
						So(queryErr, ShouldBeNil)
						So(countErr, ShouldBeNil)
					})

					Convey("With a namespace installed, can Put, Get, Query, and Count.", func() {
						putErr, getErr, queryErr, countErr := run(infoS.MustNamespace(c, "foo"), txn)
						So(putErr, ShouldBeNil)
						So(getErr, ShouldBeNil)
						So(queryErr, ShouldBeNil)
						So(countErr, ShouldBeNil)
					})
				})
			}
		})

		Convey("Testable.ShowSpecialProperties", func() {
			ds.GetTestable(c).ShowSpecialProperties(true)

			var ents []Foo
			for i := 0; i < 10; i++ {
				ent := &Foo{}
				So(ds.Put(c, ent), ShouldBeNil)
				ents = append(ents, *ent)
			}
			So(ds.Get(c, ents), ShouldBeNil)

			// Some of these entities (~50%) should have __scatter__ property
			// populated. The algorithm is deterministic.
			scatter := make([]string, len(ents))
			for i, e := range ents {
				scatter[i] = hex.EncodeToString(e.Scatter)
			}
			So(scatter, ShouldResemble, []string{
				"d77e219d0669b1808f236ca5b25127bf8e865e3f0e68b792374526251c873c61",
				"",
				"",
				"",
				"",
				"",
				"b592c9de652ffc3f458910247fc16690ba2ceeef20a8566fda5dd989a5fc160e",
				"bcefad8a2212ee1cfa3636e94264b8c73c90eaded9f429e27c7384830c1e381c",
				"d2358c1d9e5951be7117e06eaec96a6a63090f181615e2c51afaf7f214e4d873",
				"b29a46a6c01adb88d7001fe399d6346d5d2725b190f4fb025c9cb7c73c4ffb15",
			})
		})

		Convey("Query by __scatter__", func() {
			for i := 0; i < 100; i++ {
				So(ds.Put(c, &Foo{}), ShouldBeNil)
			}
			ds.GetTestable(c).CatchupIndexes()

			var ids []int64
			So(ds.Run(c, ds.NewQuery("Foo").Order("__scatter__").Limit(5), func(f *Foo) {
				So(f.Scatter, ShouldBeNil) // it is "invisible"
				ids = append(ids, f.ID)
			}), ShouldBeNil)

			// Approximately "even" distribution within [1, 100] range.
			So(ids, ShouldResemble, []int64{43, 55, 99, 23, 17})
		})
	})
}

func TestCompoundIndexes(t *testing.T) {
	t.Parallel()

	idxKey := func(def ds.IndexDefinition) string {
		So(def, ShouldNotBeNil)
		return "idx::" + string(ds.Serialize.ToBytes(*def.PrepForIdxTable()))
	}

	Convey("Test Compound indexes", t, func() {
		type Model struct {
			ID int64 `gae:"$id"`

			Field1 []string
			Field2 []int64
		}

		c := Use(context.Background())
		t := ds.GetTestable(c).(*dsImpl)
		head := t.data.head

		So(ds.Put(c, &Model{1, []string{"hello", "world"}, []int64{10, 11}}), ShouldBeNil)

		idx := ds.IndexDefinition{
			Kind: "Model",
			SortBy: []ds.IndexColumn{
				{Property: "Field2"},
			},
		}

		coll := head.Snapshot().GetCollection(idxKey(idx))
		So(coll, ShouldNotBeNil)
		So(countItems(coll), ShouldEqual, 2)

		idx.SortBy[0].Property = "Field1"
		coll = head.Snapshot().GetCollection(idxKey(idx))
		So(coll, ShouldNotBeNil)
		So(countItems(coll), ShouldEqual, 2)

		idx.SortBy = append(idx.SortBy, ds.IndexColumn{Property: "Field1"})
		So(head.GetCollection(idxKey(idx)), ShouldBeNil)

		t.AddIndexes(&idx)
		coll = head.Snapshot().GetCollection(idxKey(idx))
		So(coll, ShouldNotBeNil)
		So(countItems(coll), ShouldEqual, 4)
	})
}

// High level test for regression in how zero time is stored,
// see https://codereview.chromium.org/1334043003/
func TestDefaultTimeField(t *testing.T) {
	t.Parallel()

	Convey("Default time.Time{} can be stored", t, func() {
		type Model struct {
			ID   int64 `gae:"$id"`
			Time time.Time
		}
		c := Use(context.Background())
		m := Model{ID: 1}
		So(ds.Put(c, &m), ShouldBeNil)

		// Reset to something non zero to ensure zero is fetched.
		m.Time = time.Now().UTC()
		So(ds.Get(c, &m), ShouldBeNil)
		So(m.Time.IsZero(), ShouldBeTrue)
	})
}

func TestNewDatastore(t *testing.T) {
	t.Parallel()

	Convey("Can get and use a NewDatastore", t, func() {
		c := UseWithAppID(context.Background(), "dev~aid")
		c = infoS.MustNamespace(c, "ns")

		dsInst := NewDatastore(c, infoS.Raw(c))
		c = ds.SetRaw(c, dsInst)

		k := ds.MakeKey(c, "Something", 1)
		So(k.AppID(), ShouldEqual, "dev~aid")
		So(k.Namespace(), ShouldEqual, "ns")

		type Model struct {
			ID    int64 `gae:"$id"`
			Value []int64
		}
		So(ds.Put(c, &Model{ID: 1, Value: []int64{20, 30}}), ShouldBeNil)

		vals := []ds.PropertyMap{}
		So(ds.GetAll(c, ds.NewQuery("Model").Project("Value"), &vals), ShouldBeNil)
		So(len(vals), ShouldEqual, 2)

		So(vals[0].Slice("Value")[0].Value(), ShouldEqual, 20)
		So(vals[1].Slice("Value")[0].Value(), ShouldEqual, 30)
	})
}

func TestAddIndexes(t *testing.T) {
	t.Parallel()

	Convey("Test Testable.AddIndexes", t, func() {
		ctx := UseWithAppID(context.Background(), "aid")
		namespaces := []string{"", "good", "news", "everyone"}

		Convey("After adding datastore entries, can query against indexes in various namespaces", func() {
			foos := []*Foo{
				{ID: 1, Val: 1, Name: "foo"},
				{ID: 2, Val: 2, Name: "bar"},
				{ID: 3, Val: 2, Name: "baz"},
			}
			for _, ns := range namespaces {
				So(ds.Put(infoS.MustNamespace(ctx, ns), foos), ShouldBeNil)
			}

			// Initial query, no indexes, will fail.
			ds.GetTestable(ctx).CatchupIndexes()

			var results []*Foo
			q := ds.NewQuery("Foo").Eq("Val", 2).Gte("Name", "bar")
			So(ds.GetAll(ctx, q, &results), ShouldErrLike, "Insufficient indexes")

			// Add index for default namespace.
			ds.GetTestable(ctx).AddIndexes(&ds.IndexDefinition{
				Kind: "Foo",
				SortBy: []ds.IndexColumn{
					{Property: "Val"},
					{Property: "Name"},
				},
			})
			ds.GetTestable(ctx).CatchupIndexes()

			for _, ns := range namespaces {
				if ns == "" {
					// Skip query test for empty namespace, as this is invalid.
					continue
				}

				results = nil
				So(ds.GetAll(infoS.MustNamespace(ctx, ns), q, &results), ShouldBeNil)
				So(len(results), ShouldEqual, 2)
			}

			// Add "foos" to a new namespace, then confirm that it gets indexed.
			So(ds.Put(infoS.MustNamespace(ctx, "qux"), foos), ShouldBeNil)
			ds.GetTestable(ctx).CatchupIndexes()

			results = nil
			So(ds.GetAll(infoS.MustNamespace(ctx, "qux"), q, &results), ShouldBeNil)
			So(len(results), ShouldEqual, 2)
		})
	})
}

func TestConcurrentTxn(t *testing.T) {
	t.Parallel()

	// Stress test for concurrent transactions. It transactionally increments a
	// counter in an entity and counts how many transactions succeeded. The final
	// counter value and the number of committed transactions should match.

	Convey("Concurrent transactions work", t, func() {
		c := Use(context.Background())

		var successes int64

		for round := 0; round < 1000; round++ {
			barrier := make(chan struct{})
			wg := sync.WaitGroup{}

			for track := 0; track < 5; track++ {
				wg.Add(1)
				go func(round, track int) {
					defer wg.Done()
					<-barrier

					err := ds.RunInTransaction(c, func(c context.Context) error {
						ent := Foo{ID: 1}
						switch err := ds.Get(c, &ent); {
						case err == ds.ErrNoSuchEntity:
							// new entity
						case err != nil:
							return err
						}
						ent.Val++
						return ds.Put(c, &ent)
					}, nil)
					if err == nil {
						atomic.AddInt64(&successes, 1)
					}

				}(round, track)
			}

			// Run one round of the test.
			close(barrier)
			wg.Wait()

			// Verify that everything is still ok.
			ent := Foo{ID: 1}
			ds.Get(c, &ent)
			counter := atomic.LoadInt64(&successes)
			if int64(ent.Val) != counter { // don't spam convey assertions
				So(ent.Val, ShouldEqual, counter)
			}
		}
	})
}
