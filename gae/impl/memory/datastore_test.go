// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"fmt"
	"testing"

	dsS "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/dskey"
	"github.com/luci/gae/service/datastore/serialize"
	infoS "github.com/luci/gae/service/info"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestDatastoreKinder(t *testing.T) {
	t.Parallel()

	Convey("Datastore keys", t, func() {
		c := Use(context.Background())
		ds := dsS.Get(c)
		So(ds, ShouldNotBeNil)

		Convey("implements DSNewKeyer", func() {
			Convey("NewKey", func() {
				key := ds.NewKey("nerd", "stringID", 0, nil)
				So(key, ShouldNotBeNil)
				So(key.Kind(), ShouldEqual, "nerd")
				So(key.StringID(), ShouldEqual, "stringID")
				So(key.IntID(), ShouldEqual, 0)
				So(key.Parent(), ShouldBeNil)
				So(key.AppID(), ShouldEqual, "dev~app")
				So(key.Namespace(), ShouldEqual, "")
				So(key.String(), ShouldEqual, "/nerd,stringID")
				So(key.Incomplete(), ShouldBeFalse)
				So(key.Valid(false, "dev~app", ""), ShouldBeTrue)
			})
		})

	})
}

type MetaGroup struct {
	_id    int64   `gae:"$id,1"`
	_kind  string  `gae:"$kind,__entity_group__"`
	Parent dsS.Key `gae:"$parent"`

	Version int64 `gae:"__version__"`
}

func testGetMeta(c context.Context, k dsS.Key) int64 {
	ds := dsS.Get(c)
	mg := &MetaGroup{Parent: dskey.Root(k)}
	if err := ds.Get(mg); err != nil {
		panic(err)
	}
	return mg.Version
}

var pls = dsS.GetPLS

type Foo struct {
	Id     int64   `gae:"$id"`
	Parent dsS.Key `gae:"$parent"`

	Val int
}

func TestDatastoreSingleReadWriter(t *testing.T) {
	t.Parallel()

	Convey("Datastore single reads and writes", t, func() {
		c := Use(context.Background())
		ds := dsS.Get(c)
		So(ds, ShouldNotBeNil)

		Convey("getting objects that DNE is an error", func() {
			So(ds.Get(&Foo{Id: 1}), ShouldEqual, dsS.ErrNoSuchEntity)
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
			So(k.String(), ShouldEqual, "/Foo,1")

			Convey("and Get it back", func() {
				newFoo := &Foo{Id: 1}
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

			Convey("with multiple puts", func() {
				So(testGetMeta(c, k), ShouldEqual, 1)

				foos := make([]Foo, 10)
				for i := range foos {
					foos[i].Val = 10
					foos[i].Parent = k
				}
				So(ds.PutMulti(foos), ShouldBeNil)
				So(testGetMeta(c, k), ShouldEqual, 11)

				keys := make([]dsS.Key, len(foos))
				for i, f := range foos {
					keys[i] = ds.KeyForObj(&f)
				}

				Convey("ensure that group versions persist across deletes", func() {
					So(ds.DeleteMulti(append(keys, k)), ShouldBeNil)

					// TODO(riannucci): replace with a Count query instead of this cast
					/*
						ents := ds.(*dsImpl).data.head.GetCollection("ents:")
						num, _ := ents.GetTotals()
						// /__entity_root_ids__,Foo
						// /Foo,1/__entity_group__,1
						// /Foo,1/__entity_group_ids__,1
						So(num, ShouldEqual, 3)
					*/

					So(testGetMeta(c, k), ShouldEqual, 22)

					So(ds.Put(&Foo{Id: 1}), ShouldBeNil)
					So(testGetMeta(c, k), ShouldEqual, 23)
				})

				Convey("can Get", func() {
					vals := make([]dsS.PropertyMap, len(keys))
					for i := range vals {
						vals[i] = dsS.PropertyMap{}
						vals[i].SetMeta("key", keys[i])
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
		})

		Convey("implements DSTransactioner", func() {
			Convey("Put", func() {
				f := &Foo{Val: 10}
				So(ds.Put(f), ShouldBeNil)
				k := ds.KeyForObj(f)
				So(k.String(), ShouldEqual, "/Foo,1")

				Convey("can Put new entity groups", func() {
					err := ds.RunInTransaction(func(c context.Context) error {
						ds := dsS.Get(c)

						f := &Foo{Val: 100}
						So(ds.Put(f), ShouldBeNil)
						So(f.Id, ShouldEqual, 2)

						f.Id = 0
						f.Val = 200
						So(ds.Put(f), ShouldBeNil)
						So(f.Id, ShouldEqual, 3)

						return nil
					}, &dsS.TransactionOptions{XG: true})
					So(err, ShouldBeNil)

					f := &Foo{Id: 2}
					So(ds.Get(f), ShouldBeNil)
					So(f.Val, ShouldEqual, 100)

					f.Id = 3
					So(ds.Get(f), ShouldBeNil)
					So(f.Val, ShouldEqual, 200)
				})

				Convey("can Put new entities in a current group", func() {
					err := ds.RunInTransaction(func(c context.Context) error {
						ds := dsS.Get(c)

						f := &Foo{Val: 100, Parent: k}
						So(ds.Put(f), ShouldBeNil)
						So(ds.KeyForObj(f).String(), ShouldEqual, "/Foo,1/Foo,1")

						f.Id = 0
						f.Val = 200
						So(ds.Put(f), ShouldBeNil)
						So(ds.KeyForObj(f).String(), ShouldEqual, "/Foo,1/Foo,2")

						return nil
					}, nil)
					So(err, ShouldBeNil)

					f := &Foo{Id: 1, Parent: k}
					So(ds.Get(f), ShouldBeNil)
					So(f.Val, ShouldEqual, 100)

					f.Id = 2
					So(ds.Get(f), ShouldBeNil)
					So(f.Val, ShouldEqual, 200)
				})

				Convey("Deletes work too", func() {
					err := ds.RunInTransaction(func(c context.Context) error {
						return dsS.Get(c).Delete(k)
					}, nil)
					So(err, ShouldBeNil)
					So(ds.Get(&Foo{Id: 1}), ShouldEqual, dsS.ErrNoSuchEntity)
				})

				Convey("A Get counts against your group count", func() {
					err := ds.RunInTransaction(func(c context.Context) error {
						ds := dsS.Get(c)

						pm := dsS.PropertyMap{}
						pm.SetMeta("key", ds.NewKey("Foo", "", 20, nil))
						So(ds.Get(pm), ShouldEqual, dsS.ErrNoSuchEntity)

						pm.SetMeta("key", k)
						So(ds.Get(pm).Error(), ShouldContainSubstring, "cross-group")
						return nil
					}, nil)
					So(err, ShouldBeNil)
				})

				Convey("Get takes a snapshot", func() {
					err := ds.RunInTransaction(func(c context.Context) error {
						txnDS := dsS.Get(c)

						So(txnDS.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)

						// Don't ever do this in a real program unless you want to guarantee
						// a failed transaction :)
						f.Val = 11
						So(ds.Put(f), ShouldBeNil)

						So(txnDS.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)

						return nil
					}, nil)
					So(err, ShouldBeNil)

					f := &Foo{Id: 1}
					So(ds.Get(f), ShouldBeNil)
					So(f.Val, ShouldEqual, 11)
				})

				Convey("and snapshots are consistent even after Puts", func() {
					err := ds.RunInTransaction(func(c context.Context) error {
						txnDS := dsS.Get(c)

						f := &Foo{Id: 1}
						So(txnDS.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)

						// Don't ever do this in a real program unless you want to guarantee
						// a failed transaction :)
						f.Val = 11
						So(ds.Put(f), ShouldBeNil)

						So(txnDS.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)

						f.Val = 20
						So(txnDS.Put(f), ShouldBeNil)

						So(txnDS.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10) // still gets 10

						return nil
					}, &dsS.TransactionOptions{Attempts: 1})
					So(err.Error(), ShouldContainSubstring, "concurrent")

					f := &Foo{Id: 1}
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
						So(dsS.Get(c).Put(&Foo{Id: 1, Val: 21}), ShouldBeNil)

						err := ds.RunInTransaction(func(c context.Context) error {
							So(dsS.Get(c).Put(&Foo{Id: 1, Val: 27}), ShouldBeNil)
							return nil
						}, nil)
						So(err, ShouldBeNil)

						return nil
					}, nil)
					So(err.Error(), ShouldContainSubstring, "concurrent")

					f := &Foo{Id: 1}
					So(ds.Get(f), ShouldBeNil)
					So(f.Val, ShouldEqual, 27)
				})

				Convey("XG", func() {
					Convey("Modifying two groups with XG=false is invalid", func() {
						err := ds.RunInTransaction(func(c context.Context) error {
							ds := dsS.Get(c)
							f := &Foo{Id: 1, Val: 200}
							So(ds.Put(f), ShouldBeNil)

							f.Id = 2
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
								foos[i-1].Id = i
								foos[i-1].Val = 200
							}
							So(ds.PutMulti(foos), ShouldBeNil)
							err := ds.Put(&Foo{Id: 26})
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
							So(ds.Put(&Foo{Id: 1, Val: 200}), ShouldBeNil)
							return fmt.Errorf("thingy")
						}, nil)
						So(err.Error(), ShouldEqual, "thingy")

						f := &Foo{Id: 1}
						So(ds.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)
					})

					Convey("panicing aborts", func() {
						So(func() {
							ds.RunInTransaction(func(c context.Context) error {
								ds := dsS.Get(c)
								So(ds.Put(&Foo{Val: 200}), ShouldBeNil)
								panic("wheeeeee")
							}, nil)
						}, ShouldPanic)

						f := &Foo{Id: 1}
						So(ds.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)
					})
				})

				Convey("Transaction retries", func() {
					tst := ds.Testable()
					Reset(func() { tst.SetTransactionRetryCount(0) })

					Convey("SetTransactionRetryCount set to zere", func() {
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
