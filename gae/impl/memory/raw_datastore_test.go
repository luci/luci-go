// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"fmt"
	"math"
	"testing"

	dsS "github.com/luci/gae/service/datastore"
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
				So(dsS.KeyIncomplete(key), ShouldBeFalse)
				So(dsS.KeyValid(key, false, "dev~app", ""), ShouldBeTrue)
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
	mg := &MetaGroup{Parent: dsS.KeyRoot(k)}
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
						ents := ds.(*dsImpl).data.store.GetCollection("ents:")
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
					}, nil)
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
					// Accorting to my read of the docs for appengine, when you open a
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
			})
		})

	})
}

const MaxUint = ^uint(0)
const MaxInt = int(MaxUint >> 1)
const IntIs32Bits = int64(MaxInt) < math.MaxInt64

func TestDatastoreQueryer(t *testing.T) {
	Convey("Datastore Query suport", t, func() {
		c := Use(context.Background())
		ds := dsS.Get(c)
		So(ds, ShouldNotBeNil)

		Convey("can create good queries", func() {
			q := ds.NewQuery("Foo").KeysOnly().Limit(10).Offset(39)
			q = q.Start(queryCursor("kosmik")).End(queryCursor("krabs"))
			So(q, ShouldNotBeNil)
			So(q.(*queryImpl).err, ShouldBeNil)
			qi := q.(*queryImpl).checkCorrectness("", false)
			So(qi.err, ShouldBeNil)
		})

		Convey("normalize ensures orders make sense", func() {
			q := ds.NewQuery("Cool")
			q = q.Filter("cat =", 19).Filter("bob =", 10).Order("bob").Order("bob")

			Convey("removes dups and equality orders", func() {
				q = q.Order("wat")
				qi := q.(*queryImpl).normalize().checkCorrectness("", false)
				So(qi.err, ShouldBeNil)
				So(qi.order, ShouldResemble, []queryOrder{{"wat", qASC}})
			})

			Convey("keeps inequality orders", func() {
				q = q.Order("wat")
				q := q.Filter("bob >", 10).Filter("wat <", 29)
				qi := q.(*queryImpl).normalize().checkCorrectness("", false)
				So(qi.order, ShouldResemble, []queryOrder{{"bob", qASC}, {"wat", qASC}})
				So(qi.err.Error(), ShouldContainSubstring, "Only one inequality")
			})

			Convey("if we equality-filter on __key__, order is ditched", func() {
				q = q.Order("wat")
				q := q.Filter("__key__ =", ds.NewKey("Foo", "wat", 0, nil))
				qi := q.(*queryImpl).normalize().checkCorrectness("", false)
				So(qi.order, ShouldResemble, []queryOrder(nil))
				So(qi.err, ShouldBeNil)
			})

			Convey("if we order by key and something else, key dominates", func() {
				q := q.Order("__key__").Order("wat")
				qi := q.(*queryImpl).normalize().checkCorrectness("", false)
				So(qi.order, ShouldResemble, []queryOrder{{"__key__", qASC}})
				So(qi.err, ShouldBeNil)
			})
		})

		Convey("can create bad queries", func() {
			q := ds.NewQuery("Foo")

			Convey("bad filter ops", func() {
				q := q.Filter("Bob !", "value")
				So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "invalid operator \"!\"")
			})
			Convey("bad filter", func() {
				q := q.Filter("Bob", "value")
				So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "invalid filter")
			})
			Convey("bad order", func() {
				q := q.Order("+Bob")
				So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "invalid order")
			})
			Convey("empty", func() {
				q := q.Order("")
				So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "empty order")
			})
			Convey("OOB limit", func() {
				// this is supremely stupid. The SDK uses 'int' which measn we have to
				// use it too, but then THEY BOUNDS CHECK IT FOR 32 BITS... *sigh*
				if !IntIs32Bits {
					q := q.Limit(MaxInt)
					So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "query limit overflow")
				}
			})
			Convey("underflow offset", func() {
				q := q.Offset(-29)
				So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "negative query offset")
			})
			Convey("OOB offset", func() {
				if !IntIs32Bits {
					q := q.Offset(MaxInt)
					So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "query offset overflow")
				}
			})
			Convey("Bad cursors", func() {
				q := q.Start(queryCursor("")).End(queryCursor(""))
				So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "invalid cursor")
			})
			Convey("Bad ancestors", func() {
				q := q.Ancestor(ds.NewKey("Goop", "wat", 10, nil))
				So(q, ShouldNotBeNil)
				qi := q.(*queryImpl).checkCorrectness("", false)
				So(qi.err, ShouldEqual, dsS.ErrInvalidKey)
			})
			Convey("nil ancestors", func() {
				qi := q.Ancestor(nil).(*queryImpl).checkCorrectness("", false)
				So(qi.err.Error(), ShouldContainSubstring, "nil query ancestor")
			})
			Convey("Bad key filters", func() {
				q := q.Filter("__key__ =", ds.NewKey("Goop", "wat", 10, nil))
				qi := q.(*queryImpl).checkCorrectness("", false)
				So(qi.err, ShouldEqual, dsS.ErrInvalidKey)
			})
			Convey("non-ancestor queries in a transaction", func() {
				qi := q.(*queryImpl).checkCorrectness("", true)
				So(qi.err.Error(), ShouldContainSubstring, "Only ancestor queries")
			})
			Convey("absurd numbers of filters are prohibited", func() {
				q := q.Ancestor(ds.NewKey("thing", "wat", 0, nil))
				for i := 0; i < 100; i++ {
					q = q.Filter("something =", 10)
				}
				qi := q.(*queryImpl).checkCorrectness("", false)
				So(qi.err.Error(), ShouldContainSubstring, "query is too large")
			})
			Convey("filters for __key__ that aren't keys", func() {
				q := q.Filter("__key__ = ", 10)
				qi := q.(*queryImpl).checkCorrectness("", false)
				So(qi.err.Error(), ShouldContainSubstring, "must be a Key")
			})
			Convey("multiple inequalities", func() {
				q := q.Filter("bob > ", 19).Filter("charlie < ", 20)
				qi := q.(*queryImpl).checkCorrectness("", false)
				So(qi.err.Error(), ShouldContainSubstring, "one inequality filter")
			})
			Convey("bad sort orders", func() {
				q := q.Filter("bob > ", 19).Order("-charlie")
				qi := q.(*queryImpl).checkCorrectness("", false)
				So(qi.err.Error(), ShouldContainSubstring, "first sort property")
			})
			Convey("kindless with non-__key__ filters", func() {
				q := ds.NewQuery("").Filter("face <", 25.3)
				qi := q.(*queryImpl).checkCorrectness("", false)
				So(qi.err.Error(), ShouldContainSubstring, "kind is required for non-__key__")
			})
			Convey("kindless with non-__key__ orders", func() {
				q := ds.NewQuery("").Order("face")
				qi := q.(*queryImpl).checkCorrectness("", false)
				So(qi.err.Error(), ShouldContainSubstring, "kind is required for all orders")
			})
			Convey("kindless with decending-__key__ orders", func() {
				q := ds.NewQuery("").Order("-__key__")
				qi := q.(*queryImpl).checkCorrectness("", false)
				So(qi.err.Error(), ShouldContainSubstring, "kind is required for all orders")
			})
		})

	})
}
