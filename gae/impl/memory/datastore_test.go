// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"fmt"
	"testing"
	"time"

	dsS "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/serialize"
	infoS "github.com/luci/gae/service/info"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

type MetaGroup struct {
	_id    int64    `gae:"$id,1"`
	_kind  string   `gae:"$kind,__entity_group__"`
	Parent *dsS.Key `gae:"$parent"`

	Version int64 `gae:"__version__"`
}

func testGetMeta(c context.Context, k *dsS.Key) int64 {
	ds := dsS.Get(c)
	mg := &MetaGroup{Parent: k.Root()}
	if err := ds.Get(mg); err != nil {
		panic(err)
	}
	return mg.Version
}

var pls = dsS.GetPLS

type Foo struct {
	ID     int64    `gae:"$id"`
	Parent *dsS.Key `gae:"$parent"`

	Val int
}

func TestDatastoreSingleReadWriter(t *testing.T) {
	t.Parallel()

	Convey("Datastore single reads and writes", t, func() {
		c := Use(context.Background())
		ds := dsS.Get(c)
		So(ds, ShouldNotBeNil)

		Convey("getting objects that DNE is an error", func() {
			So(ds.Get(&Foo{ID: 1}), ShouldEqual, dsS.ErrNoSuchEntity)
		})

		Convey("bad namespaces fail", func() {
			_, err := infoS.Get(c).Namespace("$$blzyall")
			So(err.Error(), ShouldContainSubstring, "namespace \"$$blzyall\" does not match")
		})

		Convey("Can Put stuff", func() {
			// with an incomplete key!
			f := &Foo{Val: 10}
			So(ds.Put(f), ShouldBeNil)
			k := ds.KeyForObj(f)
			So(k.String(), ShouldEqual, "dev~app::/Foo,1")

			Convey("and Get it back", func() {
				newFoo := &Foo{ID: 1}
				So(ds.Get(newFoo), ShouldBeNil)
				So(newFoo, ShouldResemble, f)

				Convey("but it's hidden from a different namespace", func() {
					c, err := infoS.Get(c).Namespace("whombat")
					So(err, ShouldBeNil)
					ds = dsS.Get(c)
					So(ds.Get(f), ShouldEqual, dsS.ErrNoSuchEntity)
				})

				Convey("and we can Delete it", func() {
					So(ds.Delete(k), ShouldBeNil)
					So(ds.Get(newFoo), ShouldEqual, dsS.ErrNoSuchEntity)
				})

			})
			Convey("Deleteing with a bogus key is bad", func() {
				So(ds.Delete(ds.NewKey("Foo", "wat", 100, nil)), ShouldEqual, dsS.ErrInvalidKey)
			})
			Convey("Deleteing a DNE entity is fine", func() {
				So(ds.Delete(ds.NewKey("Foo", "wat", 0, nil)), ShouldBeNil)
			})

			Convey("Deleting entities from a nonexistant namespace works", func() {
				aid := infoS.Get(c).FullyQualifiedAppID()
				keys := make([]*dsS.Key, 10)
				for i := range keys {
					keys[i] = ds.MakeKey(aid, "noexist", "Kind", i+1)
				}
				So(ds.DeleteMulti(keys), ShouldBeNil)
				count := 0
				So(ds.Raw().DeleteMulti(keys, func(err error) error {
					count++
					So(err, ShouldBeNil)
					return nil
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
				So(ds.PutMulti(foos), ShouldBeNil)
				So(testGetMeta(c, k), ShouldEqual, 11)

				keys := make([]*dsS.Key, len(foos))
				for i, f := range foos {
					keys[i] = ds.KeyForObj(&f)
				}

				Convey("ensure that group versions persist across deletes", func() {
					So(ds.DeleteMulti(append(keys, k)), ShouldBeNil)

					ds.Testable().CatchupIndexes()

					count := 0
					So(ds.Run(dsS.NewQuery(""), func(_ *dsS.Key) {
						count++
					}), ShouldBeNil)
					So(count, ShouldEqual, 3)

					So(testGetMeta(c, k), ShouldEqual, 22)

					So(ds.Put(&Foo{ID: 1}), ShouldBeNil)
					So(testGetMeta(c, k), ShouldEqual, 23)
				})

				Convey("can Get", func() {
					vals := make([]dsS.PropertyMap, len(keys))
					for i := range vals {
						vals[i] = dsS.PropertyMap{}
						So(vals[i].SetMeta("key", keys[i]), ShouldBeTrue)
					}
					So(ds.GetMulti(vals), ShouldBeNil)

					for i, val := range vals {
						So(val, ShouldResemble, dsS.PropertyMap{
							"Val":  {dsS.MkProperty(10)},
							"$key": {dsS.MkPropertyNI(keys[i])},
						})
					}
				})

			})

			Convey("allocating ids prevents their use", func() {
				start, err := ds.AllocateIDs(ds.MakeKey("Foo", 0), 100)
				So(err, ShouldBeNil)
				So(start, ShouldEqual, 2)

				f := &Foo{Val: 10}
				So(ds.Put(f), ShouldBeNil)
				k := ds.KeyForObj(f)
				So(k.String(), ShouldEqual, "dev~app::/Foo,102")
			})
		})

		Convey("implements DSTransactioner", func() {
			Convey("Put", func() {
				f := &Foo{Val: 10}
				So(ds.Put(f), ShouldBeNil)
				k := ds.KeyForObj(f)
				So(k.String(), ShouldEqual, "dev~app::/Foo,1")

				Convey("can Put new entity groups", func() {
					err := ds.RunInTransaction(func(c context.Context) error {
						ds := dsS.Get(c)

						f := &Foo{Val: 100}
						So(ds.Put(f), ShouldBeNil)
						So(f.ID, ShouldEqual, 2)

						f.ID = 0
						f.Val = 200
						So(ds.Put(f), ShouldBeNil)
						So(f.ID, ShouldEqual, 3)

						return nil
					}, &dsS.TransactionOptions{XG: true})
					So(err, ShouldBeNil)

					f := &Foo{ID: 2}
					So(ds.Get(f), ShouldBeNil)
					So(f.Val, ShouldEqual, 100)

					f.ID = 3
					So(ds.Get(f), ShouldBeNil)
					So(f.Val, ShouldEqual, 200)
				})

				Convey("can Put new entities in a current group", func() {
					err := ds.RunInTransaction(func(c context.Context) error {
						ds := dsS.Get(c)

						f := &Foo{Val: 100, Parent: k}
						So(ds.Put(f), ShouldBeNil)
						So(ds.KeyForObj(f).String(), ShouldEqual, "dev~app::/Foo,1/Foo,1")

						f.ID = 0
						f.Val = 200
						So(ds.Put(f), ShouldBeNil)
						So(ds.KeyForObj(f).String(), ShouldEqual, "dev~app::/Foo,1/Foo,2")

						return nil
					}, nil)
					So(err, ShouldBeNil)

					f := &Foo{ID: 1, Parent: k}
					So(ds.Get(f), ShouldBeNil)
					So(f.Val, ShouldEqual, 100)

					f.ID = 2
					So(ds.Get(f), ShouldBeNil)
					So(f.Val, ShouldEqual, 200)
				})

				Convey("Deletes work too", func() {
					err := ds.RunInTransaction(func(c context.Context) error {
						return dsS.Get(c).Delete(k)
					}, nil)
					So(err, ShouldBeNil)
					So(ds.Get(&Foo{ID: 1}), ShouldEqual, dsS.ErrNoSuchEntity)
				})

				Convey("A Get counts against your group count", func() {
					err := ds.RunInTransaction(func(c context.Context) error {
						ds := dsS.Get(c)

						pm := dsS.PropertyMap{}
						So(pm.SetMeta("key", ds.NewKey("Foo", "", 20, nil)), ShouldBeTrue)
						So(ds.Get(pm), ShouldEqual, dsS.ErrNoSuchEntity)

						So(pm.SetMeta("key", k), ShouldBeTrue)
						So(ds.Get(pm).Error(), ShouldContainSubstring, "cross-group")
						return nil
					}, nil)
					So(err, ShouldBeNil)
				})

				Convey("Get takes a snapshot", func() {
					err := ds.RunInTransaction(func(c context.Context) error {
						ds := dsS.Get(c)

						So(ds.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)

						// Don't ever do this in a real program unless you want to guarantee
						// a failed transaction :)
						f.Val = 11
						So(dsS.GetNoTxn(c).Put(f), ShouldBeNil)

						So(ds.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)

						return nil
					}, nil)
					So(err, ShouldBeNil)

					f := &Foo{ID: 1}
					So(ds.Get(f), ShouldBeNil)
					So(f.Val, ShouldEqual, 11)
				})

				Convey("and snapshots are consistent even after Puts", func() {
					err := ds.RunInTransaction(func(c context.Context) error {
						ds := dsS.Get(c)

						f := &Foo{ID: 1}
						So(ds.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)

						// Don't ever do this in a real program unless you want to guarantee
						// a failed transaction :)
						f.Val = 11
						So(dsS.GetNoTxn(c).Put(f), ShouldBeNil)

						So(ds.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)

						f.Val = 20
						So(ds.Put(f), ShouldBeNil)

						So(ds.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10) // still gets 10

						return nil
					}, &dsS.TransactionOptions{Attempts: 1})
					So(err.Error(), ShouldContainSubstring, "concurrent")

					f := &Foo{ID: 1}
					So(ds.Get(f), ShouldBeNil)
					So(f.Val, ShouldEqual, 11)
				})

				Convey("Reusing a transaction context is bad news", func() {
					txnDS := dsS.Interface(nil)
					err := ds.RunInTransaction(func(c context.Context) error {
						txnDS = dsS.Get(c)
						So(txnDS.Get(f), ShouldBeNil)
						return nil
					}, nil)
					So(err, ShouldBeNil)
					So(txnDS.Get(f).Error(), ShouldContainSubstring, "expired")
				})

				Convey("Nested transactions are rejected", func() {
					err := ds.RunInTransaction(func(c context.Context) error {
						err := dsS.Get(c).RunInTransaction(func(c context.Context) error {
							panic("noooo")
						}, nil)
						So(err.Error(), ShouldContainSubstring, "nested transactions")
						return nil
					}, nil)
					So(err, ShouldBeNil)
				})

				Convey("Concurrent transactions only accept one set of changes", func() {
					// Note: I think this implementation is actually /slightly/ wrong.
					// According to my read of the docs for appengine, when you open a
					// transaction it actually (essentially) holds a reference to the
					// entire datastore. Our implementation takes a snapshot of the
					// entity group as soon as something observes/affects it.
					//
					// That said... I'm not sure if there's really a semantic difference.
					err := ds.RunInTransaction(func(c context.Context) error {
						So(dsS.Get(c).Put(&Foo{ID: 1, Val: 21}), ShouldBeNil)

						err := dsS.GetNoTxn(c).RunInTransaction(func(c context.Context) error {
							So(dsS.Get(c).Put(&Foo{ID: 1, Val: 27}), ShouldBeNil)
							return nil
						}, nil)
						So(err, ShouldBeNil)

						return nil
					}, nil)
					So(err.Error(), ShouldContainSubstring, "concurrent")

					f := &Foo{ID: 1}
					So(ds.Get(f), ShouldBeNil)
					So(f.Val, ShouldEqual, 27)
				})

				Convey("XG", func() {
					Convey("Modifying two groups with XG=false is invalid", func() {
						err := ds.RunInTransaction(func(c context.Context) error {
							ds := dsS.Get(c)
							f := &Foo{ID: 1, Val: 200}
							So(ds.Put(f), ShouldBeNil)

							f.ID = 2
							err := ds.Put(f)
							So(err.Error(), ShouldContainSubstring, "cross-group")
							return err
						}, nil)
						So(err.Error(), ShouldContainSubstring, "cross-group")
					})

					Convey("Modifying >25 groups with XG=true is invald", func() {
						err := ds.RunInTransaction(func(c context.Context) error {
							ds := dsS.Get(c)
							foos := make([]Foo, 25)
							for i := int64(1); i < 26; i++ {
								foos[i-1].ID = i
								foos[i-1].Val = 200
							}
							So(ds.PutMulti(foos), ShouldBeNil)
							err := ds.Put(&Foo{ID: 26})
							So(err.Error(), ShouldContainSubstring, "too many entity groups")
							return err
						}, &dsS.TransactionOptions{XG: true})
						So(err.Error(), ShouldContainSubstring, "too many entity groups")
					})
				})

				Convey("Errors and panics", func() {
					Convey("returning an error aborts", func() {
						err := ds.RunInTransaction(func(c context.Context) error {
							ds := dsS.Get(c)
							So(ds.Put(&Foo{ID: 1, Val: 200}), ShouldBeNil)
							return fmt.Errorf("thingy")
						}, nil)
						So(err.Error(), ShouldEqual, "thingy")

						f := &Foo{ID: 1}
						So(ds.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)
					})

					Convey("panicing aborts", func() {
						So(func() {
							So(ds.RunInTransaction(func(c context.Context) error {
								ds := dsS.Get(c)
								So(ds.Put(&Foo{Val: 200}), ShouldBeNil)
								panic("wheeeeee")
							}, nil), ShouldBeNil)
						}, ShouldPanic)

						f := &Foo{ID: 1}
						So(ds.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)
					})
				})

				Convey("Transaction retries", func() {
					tst := ds.Testable()
					Reset(func() { tst.SetTransactionRetryCount(0) })

					Convey("SetTransactionRetryCount set to zero", func() {
						tst.SetTransactionRetryCount(0)
						calls := 0
						So(ds.RunInTransaction(func(c context.Context) error {
							calls++
							return nil
						}, nil), ShouldBeNil)
						So(calls, ShouldEqual, 1)
					})

					Convey("default TransactionOptions is 3 attempts", func() {
						tst.SetTransactionRetryCount(100) // more than 3
						calls := 0
						So(ds.RunInTransaction(func(c context.Context) error {
							calls++
							return nil
						}, nil), ShouldEqual, dsS.ErrConcurrentTransaction)
						So(calls, ShouldEqual, 3)
					})

					Convey("non-default TransactionOptions ", func() {
						tst.SetTransactionRetryCount(100) // more than 20
						calls := 0
						So(ds.RunInTransaction(func(c context.Context) error {
							calls++
							return nil
						}, &dsS.TransactionOptions{Attempts: 20}), ShouldEqual, dsS.ErrConcurrentTransaction)
						So(calls, ShouldEqual, 20)
					})

					Convey("SetTransactionRetryCount is respected", func() {
						tst.SetTransactionRetryCount(1) // less than 3
						calls := 0
						So(ds.RunInTransaction(func(c context.Context) error {
							calls++
							return nil
						}, nil), ShouldBeNil)
						So(calls, ShouldEqual, 2)
					})

					Convey("fatal errors are not retried", func() {
						tst.SetTransactionRetryCount(1)
						calls := 0
						So(ds.RunInTransaction(func(c context.Context) error {
							calls++
							return fmt.Errorf("omg")
						}, nil).Error(), ShouldEqual, "omg")
						So(calls, ShouldEqual, 1)
					})
				})
			})
		})

		Convey("Testable.Consistent", func() {
			Convey("false", func() {
				ds.Testable().Consistent(false) // the default
				for i := 0; i < 10; i++ {
					So(ds.Put(&Foo{ID: int64(i + 1), Val: i + 1}), ShouldBeNil)
				}
				q := dsS.NewQuery("Foo").Gt("Val", 3)
				count, err := ds.Count(q)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 0)

				So(ds.Delete(ds.MakeKey("Foo", 4)), ShouldBeNil)

				count, err = ds.Count(q)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 0)

				ds.Testable().Consistent(true)
				count, err = ds.Count(q)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 6)
			})

			Convey("true", func() {
				ds.Testable().Consistent(true)
				for i := 0; i < 10; i++ {
					So(ds.Put(&Foo{ID: int64(i + 1), Val: i + 1}), ShouldBeNil)
				}
				q := dsS.NewQuery("Foo").Gt("Val", 3)
				count, err := ds.Count(q)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 7)

				So(ds.Delete(ds.MakeKey("Foo", 4)), ShouldBeNil)

				count, err = ds.Count(q)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 6)
			})
		})

		Convey("Testable.DisableSpecialEntities", func() {
			ds.Testable().DisableSpecialEntities(true)

			So(ds.Put(&Foo{}), ShouldErrLike, "allocateIDs is disabled")

			So(ds.Put(&Foo{ID: 1}), ShouldBeNil)

			ds.Testable().CatchupIndexes()

			count, err := ds.Count(dsS.NewQuery(""))
			So(err, ShouldBeNil)
			So(count, ShouldEqual, 1) // normally this would include __entity_group__
		})
	})
}

func TestCompoundIndexes(t *testing.T) {
	t.Parallel()

	idxKey := func(def dsS.IndexDefinition) string {
		So(def, ShouldNotBeNil)
		return "idx::" + string(serialize.ToBytes(*def.PrepForIdxTable()))
	}

	numItms := func(c *memCollection) uint64 {
		ret, _ := c.GetTotals()
		return ret
	}

	Convey("Test Compound indexes", t, func() {
		type Model struct {
			ID int64 `gae:"$id"`

			Field1 []string
			Field2 []int64
		}

		c := Use(context.Background())
		ds := dsS.Get(c)
		t := ds.Testable().(*dsImpl)
		head := t.data.head

		So(ds.Put(&Model{1, []string{"hello", "world"}, []int64{10, 11}}), ShouldBeNil)

		idx := dsS.IndexDefinition{
			Kind: "Model",
			SortBy: []dsS.IndexColumn{
				{Property: "Field2"},
			},
		}

		coll := head.GetCollection(idxKey(idx))
		So(coll, ShouldNotBeNil)
		So(numItms(coll), ShouldEqual, 2)

		idx.SortBy[0].Property = "Field1"
		coll = head.GetCollection(idxKey(idx))
		So(coll, ShouldNotBeNil)
		So(numItms(coll), ShouldEqual, 2)

		idx.SortBy = append(idx.SortBy, dsS.IndexColumn{Property: "Field1"})
		So(head.GetCollection(idxKey(idx)), ShouldBeNil)

		t.AddIndexes(&idx)
		coll = head.GetCollection(idxKey(idx))
		So(coll, ShouldNotBeNil)
		So(numItms(coll), ShouldEqual, 4)
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
		ds := dsS.Get(Use(context.Background()))
		m := Model{ID: 1}
		So(ds.Put(&m), ShouldBeNil)

		// Reset to something non zero to ensure zero is fetched.
		m.Time = time.Now().UTC()
		So(ds.Get(&m), ShouldBeNil)
		So(m.Time.IsZero(), ShouldBeTrue)
	})
}

func TestNewDatastore(t *testing.T) {
	t.Parallel()

	Convey("Can get and use a NewDatastore", t, func() {
		ds, err := NewDatastore("aid", "ns")
		So(err, ShouldBeNil)

		k := ds.MakeKey("Something", 1)
		So(k.AppID(), ShouldEqual, "aid")
		So(k.Namespace(), ShouldEqual, "ns")

		type Model struct {
			ID    int64 `gae:"$id"`
			Value []int64
		}
		So(ds.Put(&Model{ID: 1, Value: []int64{20, 30}}), ShouldBeNil)

		vals := []dsS.PropertyMap{}
		So(ds.GetAll(dsS.NewQuery("Model").Project("Value"), &vals), ShouldBeNil)
		So(len(vals), ShouldEqual, 2)

		So(vals[0]["Value"][0].Value(), ShouldEqual, 20)
		So(vals[1]["Value"][0].Value(), ShouldEqual, 30)
	})
}
