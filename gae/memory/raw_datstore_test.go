// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"fmt"
	"infra/gae/libs/gae"
	"infra/gae/libs/gae/helper"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestDatastoreKinder(t *testing.T) {
	t.Parallel()

	Convey("Datastore keys", t, func() {
		c := Use(context.Background())
		rds := gae.GetRDS(c)
		So(rds, ShouldNotBeNil)

		Convey("implements DSNewKeyer", func() {
			Convey("NewKey", func() {
				key := rds.NewKey("nerd", "stringID", 0, nil)
				So(key, ShouldNotBeNil)
				So(key.Kind(), ShouldEqual, "nerd")
				So(key.StringID(), ShouldEqual, "stringID")
				So(key.IntID(), ShouldEqual, 0)
				So(key.Parent(), ShouldBeNil)
				So(key.AppID(), ShouldEqual, "dev~app")
				So(key.Namespace(), ShouldEqual, "")
				So(key.String(), ShouldEqual, "/nerd,stringID")
				So(helper.DSKeyIncomplete(key), ShouldBeFalse)
				So(helper.DSKeyValid(key, "", false), ShouldBeTrue)
			})
		})

	})
}

func testGetMeta(c context.Context, k gae.DSKey) int64 {
	rds := gae.GetRDS(c)
	k = rds.NewKey("__entity_group__", "", 1, helper.DSKeyRoot(k))
	pmap := gae.DSPropertyMap{}
	rds.Get(k, &pmap)
	return pmap["__version__"][0].Value().(int64)
}

func TestDatastoreSingleReadWriter(t *testing.T) {
	t.Parallel()

	Convey("Datastore single reads and writes", t, func() {
		c := Use(context.Background())
		rds := gae.GetRDS(c)
		So(rds, ShouldNotBeNil)

		Convey("implements DSSingleReadWriter", func() {
			type Foo struct {
				Val int
			}

			Convey("invalid keys break", func() {
				k := rds.NewKey("Foo", "", 0, nil)
				So(rds.Get(k, nil), ShouldEqual, gae.ErrDSInvalidKey)

				_, err := rds.Put(rds.NewKey("Foo", "", 0, k), &Foo{})
				So(err, ShouldEqual, gae.ErrDSInvalidKey)
			})

			Convey("getting objects that DNE is an error", func() {
				k := rds.NewKey("Foo", "", 1, nil)
				So(rds.Get(k, nil), ShouldEqual, gae.ErrDSNoSuchEntity)
			})

			Convey("Can Put stuff", func() {
				// with an incomplete key!
				k := rds.NewKey("Foo", "", 0, nil)
				f := &Foo{Val: 10}
				k, err := rds.Put(k, f)
				So(err, ShouldBeNil)
				So(k.String(), ShouldEqual, "/Foo,1")

				Convey("and Get it back", func() {
					newFoo := &Foo{}
					err := rds.Get(k, newFoo)
					So(err, ShouldBeNil)
					So(newFoo, ShouldResemble, f)

					Convey("and we can Delete it", func() {
						err := rds.Delete(k)
						So(err, ShouldBeNil)

						err = rds.Get(k, newFoo)
						So(err, ShouldEqual, gae.ErrDSNoSuchEntity)
					})
				})
				Convey("Deleteing with a bogus key is bad", func() {
					err := rds.Delete(rds.NewKey("Foo", "wat", 100, nil))
					So(err, ShouldEqual, gae.ErrDSInvalidKey)
				})
				Convey("Deleteing a DNE entity is fine", func() {
					err := rds.Delete(rds.NewKey("Foo", "wat", 0, nil))
					So(err, ShouldBeNil)
				})

				Convey("serialization breaks in the normal ways", func() {
					type BadFoo struct {
						Val uint8
					}
					_, err := rds.Put(k, &BadFoo{})
					So(err.Error(), ShouldContainSubstring, "invalid type: uint8")

					err = rds.Get(k, &BadFoo{})
					So(err.Error(), ShouldContainSubstring, "invalid type: uint8")
				})

				Convey("check that metadata works", func() {
					So(testGetMeta(c, k), ShouldEqual, 1)

					pkey := k
					for i := 0; i < 10; i++ {
						k := rds.NewKey("Foo", "", 0, pkey)
						_, err = rds.Put(k, &Foo{Val: 10})
						So(err, ShouldBeNil)
					}
					So(testGetMeta(c, k), ShouldEqual, 11)

					Convey("ensure that group versions persist across deletes", func() {
						So(rds.Delete(k), ShouldBeNil)
						for i := int64(1); i < 11; i++ {
							So(rds.Delete(rds.NewKey("Foo", "", i, k)), ShouldBeNil)
						}
						// TODO(riannucci): replace with a Count query instead of this cast
						ents := rds.(*dsImpl).data.store.GetCollection("ents:")
						num, _ := ents.GetTotals()
						// /__entity_root_ids__,Foo
						// /Foo,1/__entity_group__,1
						// /Foo,1/__entity_group_ids__,1
						So(num, ShouldEqual, 3)

						version, err := curVersion(ents, groupMetaKey(k))
						So(err, ShouldBeNil)
						So(version, ShouldEqual, 22)

						k, err := rds.Put(k, f)
						So(err, ShouldBeNil)
						So(testGetMeta(c, k), ShouldEqual, 23)
					})
				})
			})
		})

		Convey("implements DSTransactioner", func() {
			type Foo struct {
				Val int
			}
			Convey("Put", func() {
				k := rds.NewKey("Foo", "", 0, nil)
				f := &Foo{Val: 10}
				k, err := rds.Put(k, f)
				So(err, ShouldBeNil)
				So(k.String(), ShouldEqual, "/Foo,1")

				Convey("can Put new entity groups", func() {
					err := rds.RunInTransaction(func(c context.Context) error {
						rds := gae.GetRDS(c)
						So(rds, ShouldNotBeNil)

						f1 := &Foo{Val: 100}
						k, err := rds.Put(rds.NewKey("Foo", "", 0, nil), f1)
						So(err, ShouldBeNil)
						So(k.String(), ShouldEqual, "/Foo,2")

						f2 := &Foo{Val: 200}
						k, err = rds.Put(rds.NewKey("Foo", "", 0, nil), f2)
						So(err, ShouldBeNil)
						So(k.String(), ShouldEqual, "/Foo,3")

						return nil
					}, &gae.DSTransactionOptions{XG: true})
					So(err, ShouldBeNil)

					f := &Foo{}
					So(rds.Get(rds.NewKey("Foo", "", 2, nil), f), ShouldBeNil)
					So(f.Val, ShouldEqual, 100)

					f = &Foo{}
					So(rds.Get(rds.NewKey("Foo", "", 3, nil), f), ShouldBeNil)
					So(f.Val, ShouldEqual, 200)
				})

				Convey("can Put new entities in a current group", func() {
					err := rds.RunInTransaction(func(c context.Context) error {
						rds := gae.GetRDS(c)
						So(rds, ShouldNotBeNil)

						par := k

						f1 := &Foo{Val: 100}
						k, err := rds.Put(rds.NewKey("Foo", "", 0, par), f1)
						So(err, ShouldBeNil)
						So(k.String(), ShouldEqual, "/Foo,1/Foo,1")

						f2 := &Foo{Val: 200}
						k, err = rds.Put(rds.NewKey("Foo", "", 0, par), f2)
						So(err, ShouldBeNil)
						So(k.String(), ShouldEqual, "/Foo,1/Foo,2")

						return nil
					}, nil)
					So(err, ShouldBeNil)

					f1 := &Foo{}
					So(rds.Get(rds.NewKey("Foo", "", 1, k), f1), ShouldBeNil)
					So(f1.Val, ShouldEqual, 100)

					f2 := &Foo{}
					So(rds.Get(rds.NewKey("Foo", "", 2, k), f2), ShouldBeNil)
					So(f2.Val, ShouldEqual, 200)
				})

				Convey("Deletes work too", func() {
					err := rds.RunInTransaction(func(c context.Context) error {
						rds := gae.GetRDS(c)
						So(rds, ShouldNotBeNil)
						So(rds.Delete(k), ShouldBeNil)
						return nil
					}, nil)
					So(err, ShouldBeNil)
					So(rds.Get(k, f), ShouldEqual, gae.ErrDSNoSuchEntity)
				})

				Convey("A Get counts against your group count", func() {
					err := rds.RunInTransaction(func(c context.Context) error {
						rds := gae.GetRDS(c)
						f := &Foo{}
						So(rds.Get(rds.NewKey("Foo", "", 20, nil), f), ShouldEqual, gae.ErrDSNoSuchEntity)

						So(rds.Get(k, f).Error(), ShouldContainSubstring, "cross-group")
						return nil
					}, nil)
					So(err, ShouldBeNil)
				})

				Convey("Get takes a snapshot", func() {
					err := rds.RunInTransaction(func(c context.Context) error {
						txnDS := gae.GetRDS(c)
						So(txnDS, ShouldNotBeNil)

						So(txnDS.Get(k, f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)

						// Don't ever do this in a real program unless you want to guarantee
						// a failed transaction :)
						f.Val = 11
						_, err := rds.Put(k, f)
						So(err, ShouldBeNil)

						So(txnDS.Get(k, f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)

						return nil
					}, nil)
					So(err, ShouldBeNil)

					f := &Foo{}
					So(rds.Get(k, f), ShouldBeNil)
					So(f.Val, ShouldEqual, 11)
				})

				Convey("and snapshots are consistent even after Puts", func() {
					err := rds.RunInTransaction(func(c context.Context) error {
						txnDS := gae.GetRDS(c)
						So(txnDS, ShouldNotBeNil)

						f := &Foo{}
						So(txnDS.Get(k, f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)

						// Don't ever do this in a real program unless you want to guarantee
						// a failed transaction :)
						f.Val = 11
						_, err := rds.Put(k, f)
						So(err, ShouldBeNil)

						So(txnDS.Get(k, f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)

						f.Val = 20
						_, err = txnDS.Put(k, f)
						So(err, ShouldBeNil)

						So(txnDS.Get(k, f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10) // still gets 10

						return nil
					}, nil)
					So(err.Error(), ShouldContainSubstring, "concurrent")

					f := &Foo{}
					So(rds.Get(k, f), ShouldBeNil)
					So(f.Val, ShouldEqual, 11)
				})

				Convey("Reusing a transaction context is bad news", func() {
					k := rds.NewKey("Foo", "", 1, nil)
					txnDS := gae.RawDatastore(nil)
					err := rds.RunInTransaction(func(c context.Context) error {
						txnDS = gae.GetRDS(c)
						So(txnDS.Get(k, &Foo{}), ShouldBeNil)
						return nil
					}, nil)
					So(err, ShouldBeNil)
					So(txnDS.Get(k, &Foo{}).Error(), ShouldContainSubstring, "expired")
				})

				Convey("Nested transactions are rejected", func() {
					err := rds.RunInTransaction(func(c context.Context) error {
						err := gae.GetRDS(c).RunInTransaction(func(c context.Context) error {
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
					err := rds.RunInTransaction(func(c context.Context) error {
						txnDS := gae.GetRDS(c)
						f := &Foo{Val: 21}
						_, err = txnDS.Put(k, f)
						So(err, ShouldBeNil)

						err := rds.RunInTransaction(func(c context.Context) error {
							txnDS := gae.GetRDS(c)
							f := &Foo{Val: 27}
							_, err := txnDS.Put(k, f)
							So(err, ShouldBeNil)
							return nil
						}, nil)
						So(err, ShouldBeNil)

						return nil
					}, nil)
					So(err.Error(), ShouldContainSubstring, "concurrent")

					f := &Foo{}
					So(rds.Get(k, f), ShouldBeNil)
					So(f.Val, ShouldEqual, 27)
				})

				Convey("XG", func() {
					Convey("Modifying two groups with XG=false is invalid", func() {
						err := rds.RunInTransaction(func(c context.Context) error {
							rds := gae.GetRDS(c)
							f := &Foo{Val: 200}
							_, err := rds.Put(k, f)
							So(err, ShouldBeNil)

							_, err = rds.Put(rds.NewKey("Foo", "", 2, nil), f)
							So(err.Error(), ShouldContainSubstring, "cross-group")
							return err
						}, nil)
						So(err.Error(), ShouldContainSubstring, "cross-group")
					})

					Convey("Modifying >25 groups with XG=true is invald", func() {
						err := rds.RunInTransaction(func(c context.Context) error {
							rds := gae.GetRDS(c)
							for i := int64(1); i < 26; i++ {
								k := rds.NewKey("Foo", "", i, nil)
								f := &Foo{Val: 200}
								_, err := rds.Put(k, f)
								So(err, ShouldBeNil)
							}
							f := &Foo{Val: 200}
							_, err := rds.Put(rds.NewKey("Foo", "", 27, nil), f)
							So(err.Error(), ShouldContainSubstring, "too many entity groups")
							return err
						}, &gae.DSTransactionOptions{XG: true})
						So(err.Error(), ShouldContainSubstring, "too many entity groups")
					})
				})

				Convey("Errors and panics", func() {
					Convey("returning an error aborts", func() {
						err := rds.RunInTransaction(func(c context.Context) error {
							rds := gae.GetRDS(c)
							f := &Foo{Val: 200}
							_, err := rds.Put(k, f)
							So(err, ShouldBeNil)

							return fmt.Errorf("thingy")
						}, nil)
						So(err.Error(), ShouldEqual, "thingy")

						f := &Foo{}
						So(rds.Get(k, f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)
					})

					Convey("panicing aborts", func() {
						So(func() {
							rds.RunInTransaction(func(c context.Context) error {
								rds := gae.GetRDS(c)
								f := &Foo{Val: 200}
								_, err := rds.Put(k, f)
								So(err, ShouldBeNil)
								panic("wheeeeee")
							}, nil)
						}, ShouldPanic)

						f := &Foo{}
						So(rds.Get(k, f), ShouldBeNil)
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
		rds := gae.GetRDS(c)
		So(rds, ShouldNotBeNil)

		Convey("can create good queries", func() {
			q := rds.NewQuery("Foo").KeysOnly().Limit(10).Offset(39)
			q = q.Start(queryCursor("kosmik")).End(queryCursor("krabs"))
			So(q, ShouldNotBeNil)
			So(q.(*queryImpl).err, ShouldBeNil)
			qi := q.(*queryImpl).checkCorrectness("", false)
			So(qi.err, ShouldBeNil)
		})

		Convey("normalize ensures orders make sense", func() {
			q := rds.NewQuery("Cool")
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
				q := q.Filter("__key__ =", rds.NewKey("Foo", "wat", 0, nil))
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
			q := rds.NewQuery("Foo")

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
				q := q.Ancestor(rds.NewKey("Goop", "wat", 10, nil))
				So(q, ShouldNotBeNil)
				qi := q.(*queryImpl).checkCorrectness("", false)
				So(qi.err, ShouldEqual, gae.ErrDSInvalidKey)
			})
			Convey("nil ancestors", func() {
				qi := q.Ancestor(nil).(*queryImpl).checkCorrectness("", false)
				So(qi.err.Error(), ShouldContainSubstring, "nil query ancestor")
			})
			Convey("Bad key filters", func() {
				q := q.Filter("__key__ =", rds.NewKey("Goop", "wat", 10, nil))
				qi := q.(*queryImpl).checkCorrectness("", false)
				So(qi.err, ShouldEqual, gae.ErrDSInvalidKey)
			})
			Convey("non-ancestor queries in a transaction", func() {
				qi := q.(*queryImpl).checkCorrectness("", true)
				So(qi.err.Error(), ShouldContainSubstring, "Only ancestor queries")
			})
			Convey("absurd numbers of filters are prohibited", func() {
				q := q.Ancestor(rds.NewKey("thing", "wat", 0, nil))
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
				q := rds.NewQuery("").Filter("face <", 25.3)
				qi := q.(*queryImpl).checkCorrectness("", false)
				So(qi.err.Error(), ShouldContainSubstring, "kind is required for non-__key__")
			})
			Convey("kindless with non-__key__ orders", func() {
				q := rds.NewQuery("").Order("face")
				qi := q.(*queryImpl).checkCorrectness("", false)
				So(qi.err.Error(), ShouldContainSubstring, "kind is required for all orders")
			})
			Convey("kindless with decending-__key__ orders", func() {
				q := rds.NewQuery("").Order("-__key__")
				qi := q.(*queryImpl).checkCorrectness("", false)
				So(qi.err.Error(), ShouldContainSubstring, "kind is required for all orders")
			})
		})

	})
}
