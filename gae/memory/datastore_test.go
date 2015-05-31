// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"fmt"
	"infra/gae/libs/meta"
	"infra/gae/libs/wrapper"
	"math"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"

	"appengine/datastore"
)

func TestDatastoreKinder(t *testing.T) {
	t.Parallel()

	Convey("Datastore kinds and keys", t, func() {
		c := Use(context.Background())
		ds := wrapper.GetDS(c)
		So(ds, ShouldNotBeNil)

		Convey("implements DSKinder", func() {
			type Foo struct{}
			So(ds.Kind(&Foo{}), ShouldEqual, "Foo")

			Convey("which can be tweaked by DSKindSetter", func() {
				ds.SetKindNameResolver(func(interface{}) string { return "spam" })
				So(ds.Kind(&Foo{}), ShouldEqual, "spam")

				Convey("and it retains the function so you can stack them", func() {
					cur := ds.KindNameResolver()
					ds.SetKindNameResolver(func(o interface{}) string { return "wat" + cur(o) })
					So(ds.Kind(&Foo{}), ShouldEqual, "watspam")
				})
			})
		})

		Convey("implements DSNewKeyer", func() {
			Convey("NewKey", func() {
				key := ds.NewKey("nerd", "stringID", 0, nil)
				So(key, ShouldNotBeNil)
				So(key.Kind(), ShouldEqual, "nerd")
				So(key.StringID(), ShouldEqual, "stringID")
				So(key.IntID(), ShouldEqual, 0)
				So(key.Parent(), ShouldBeNil)
				So(key.AppID(), ShouldEqual, "dev~my~app")
				So(key.Namespace(), ShouldEqual, "")
				So(key.String(), ShouldEqual, "/nerd,stringID")
				So(key.Incomplete(), ShouldBeFalse)
				So(keyValid("", key, userKeyOnly), ShouldBeTrue)

				chkey := ds.NewKey("wat", "", 100, key)
				So(chkey, ShouldNotBeNil)
				So(chkey.Kind(), ShouldEqual, "wat")
				So(chkey.StringID(), ShouldEqual, "")
				So(chkey.IntID(), ShouldEqual, 100)
				So(chkey.Parent(), ShouldEqual, key)
				So(chkey.AppID(), ShouldEqual, "dev~my~app")
				So(chkey.Namespace(), ShouldEqual, "")
				So(chkey.String(), ShouldEqual, "/nerd,stringID/wat,100")
				So(key.Incomplete(), ShouldBeFalse)
				So(keyValid("", chkey, userKeyOnly), ShouldBeTrue)

				incompl := ds.NewKey("sup", "", 0, key)
				So(incompl, ShouldNotBeNil)
				So(incompl.Incomplete(), ShouldBeTrue)
				So(keyValid("", incompl, userKeyOnly), ShouldBeTrue)
				So(incompl.String(), ShouldEqual, "/nerd,stringID/sup,0")

				bad := ds.NewKey("nooo", "", 10, incompl)
				So(bad, ShouldNotBeNil)
				So(bad.Incomplete(), ShouldBeFalse)
				So(keyValid("", bad, userKeyOnly), ShouldBeFalse)
				So(bad.String(), ShouldEqual, "/nerd,stringID/sup,0/nooo,10")

				So(rootKey(bad), ShouldEqual, key)

				Convey("other key validation", func() {
					So(keyValid("", nil, userKeyOnly), ShouldBeFalse)

					key := ds.NewKey("", "", 0, nil)
					So(key, ShouldNotBeNil)

					So(keyValid("", key, userKeyOnly), ShouldBeFalse)

					key = ds.NewKey("noop", "power level", 9000, nil)
					So(key, ShouldNotBeNil)

					So(keyValid("", key, userKeyOnly), ShouldBeFalse)
				})
			})

			Convey("NewKeyObj", func() {
				type Foo struct {
					_knd   string         `goon:"kind,coool"`
					ID     int64          `goon:"id"`
					Parent *datastore.Key `goon:"parent"`
				}
				f := &Foo{ID: 100}
				k := ds.NewKeyObj(f)
				So(k.String(), ShouldEqual, "/coool,100")

				f.Parent = k
				f._knd = "weevils"
				f.ID = 19
				k = ds.NewKeyObj(f)
				So(k.String(), ShouldEqual, "/coool,100/weevils,19")

				Convey("panics when you do a dumb thing", func() {
					type Foo struct {
						ID []byte `goon:"id"`
					}
					So(func() { ds.NewKeyObj(&Foo{}) }, ShouldPanic)
				})
			})

			Convey("NewKeyObjError", func() {
				type Foo struct {
					ID []byte `goon:"id"`
				}
				_, err := ds.NewKeyObjError(&Foo{})
				So(err.Error(), ShouldContainSubstring, "must be int64 or string")
			})
		})

	})
}

func TestDatastoreSingleReadWriter(t *testing.T) {
	t.Parallel()

	Convey("Datastore single reads and writes", t, func() {
		c := Use(context.Background())
		ds := wrapper.GetDS(c)
		So(ds, ShouldNotBeNil)

		Convey("implements DSSingleReadWriter", func() {
			type Foo struct {
				ID     int64          `goon:"id" datastore:"-"`
				Parent *datastore.Key `goon:"parent" datastore:"-"`
				Val    int
			}

			Convey("invalid keys break", func() {
				k := ds.NewKeyObj(&Foo{})
				f := &Foo{Parent: k}
				So(ds.Get(f), ShouldEqual, datastore.ErrInvalidKey)

				_, err := ds.Put(f)
				So(err, ShouldEqual, datastore.ErrInvalidKey)
			})

			Convey("getting objects that DNE is an error", func() {
				So(ds.Get(&Foo{ID: 1}), ShouldEqual, datastore.ErrNoSuchEntity)
			})

			Convey("Can Put stuff", func() {
				// with an incomplete key!
				f := &Foo{Val: 10}
				k, err := ds.Put(f)
				So(err, ShouldBeNil)
				So(k.String(), ShouldEqual, "/Foo,1")
				So(ds.NewKeyObj(f), ShouldResemble, k)

				Convey("and Get it back", func() {
					newFoo := &Foo{ID: 1}
					err := ds.Get(newFoo)
					So(err, ShouldBeNil)
					So(newFoo, ShouldResemble, f)

					Convey("and we can Delete it", func() {
						err := ds.Delete(ds.NewKey("Foo", "", 1, nil))
						So(err, ShouldBeNil)

						err = ds.Get(newFoo)
						So(err, ShouldEqual, datastore.ErrNoSuchEntity)
					})
				})
				Convey("Deleteing with a bogus key is bad", func() {
					err := ds.Delete(ds.NewKey("Foo", "wat", 100, nil))
					So(err, ShouldEqual, datastore.ErrInvalidKey)
				})
				Convey("Deleteing a DNE entity is fine", func() {
					err := ds.Delete(ds.NewKey("Foo", "wat", 0, nil))
					So(err, ShouldBeNil)
				})

				Convey("serialization breaks in the normal ways", func() {
					type BadFoo struct {
						_kind string `goon:"kind,Foo"`
						ID    int64  `goon:"id" datastore:"-"`
						Val   uint8
					}
					_, err := ds.Put(&BadFoo{})
					So(err.Error(), ShouldContainSubstring,
						"unsupported struct field type: uint8")

					err = ds.Get(&BadFoo{ID: 1})
					So(err.Error(), ShouldContainSubstring,
						"type mismatch: int versus uint8")
				})

				Convey("check that metadata works", func() {
					val, _ := meta.GetEntityGroupVersion(c, k)
					So(val, ShouldEqual, 1)

					for i := 0; i < 10; i++ {
						_, err = ds.Put(&Foo{Val: 10, Parent: k})
						So(err, ShouldBeNil)
					}
					val, _ = meta.GetEntityGroupVersion(c, k)
					So(val, ShouldEqual, 11)

					Convey("ensure that group versions persist across deletes", func() {
						So(ds.Delete(k), ShouldBeNil)
						for i := int64(1); i < 11; i++ {
							So(ds.Delete(ds.NewKey("Foo", "", i, k)), ShouldBeNil)
						}
						// TODO(riannucci): replace with a Count query instead of this cast
						ents := ds.(*dsImpl).data.store.GetCollection("ents:")
						num, _ := ents.GetTotals()
						// /__entity_root_ids__,Foo
						// /Foo,1/__entity_group__,1
						// /Foo,1/__entity_group_ids__,1
						So(num, ShouldEqual, 3)

						version, err := curVersion(ents, groupMetaKey(k))
						So(err, ShouldBeNil)
						So(version, ShouldEqual, 22)

						k, err := ds.Put(f)
						So(err, ShouldBeNil)
						val, _ := meta.GetEntityGroupVersion(c, k)
						So(val, ShouldEqual, 23)
					})
				})
			})
		})

		Convey("implements DSTransactioner", func() {
			type Foo struct {
				ID     int64          `goon:"id" datastore:"-"`
				Parent *datastore.Key `goon:"parent" datastore:"-"`
				Val    int
			}
			Convey("Put", func() {
				f := &Foo{Val: 10}
				k, err := ds.Put(f)
				So(err, ShouldBeNil)
				So(k.String(), ShouldEqual, "/Foo,1")
				So(ds.NewKeyObj(f), ShouldResemble, k)

				Convey("can Put new entity groups", func() {
					err := ds.RunInTransaction(func(c context.Context) error {
						ds := wrapper.GetDS(c)
						So(ds, ShouldNotBeNil)

						f1 := &Foo{Val: 100}
						k, err := ds.Put(f1)
						So(err, ShouldBeNil)
						So(k.String(), ShouldEqual, "/Foo,2")

						f2 := &Foo{Val: 200}
						k, err = ds.Put(f2)
						So(err, ShouldBeNil)
						So(k.String(), ShouldEqual, "/Foo,3")

						return nil
					}, &datastore.TransactionOptions{XG: true})
					So(err, ShouldBeNil)

					f := &Foo{ID: 2}
					So(ds.Get(f), ShouldBeNil)
					So(f.Val, ShouldEqual, 100)

					f = &Foo{ID: 3}
					So(ds.Get(f), ShouldBeNil)
					So(f.Val, ShouldEqual, 200)
				})

				Convey("can Put new entities in a current group", func() {
					err := ds.RunInTransaction(func(c context.Context) error {
						ds := wrapper.GetDS(c)
						So(ds, ShouldNotBeNil)

						f1 := &Foo{Val: 100, Parent: ds.NewKeyObj(f)}
						k, err := ds.Put(f1)
						So(err, ShouldBeNil)
						So(k.String(), ShouldEqual, "/Foo,1/Foo,1")

						f2 := &Foo{Val: 200, Parent: ds.NewKeyObj(f)}
						k, err = ds.Put(f2)
						So(err, ShouldBeNil)
						So(k.String(), ShouldEqual, "/Foo,1/Foo,2")

						return nil
					}, nil)
					So(err, ShouldBeNil)

					f1 := &Foo{ID: 1, Parent: ds.NewKeyObj(&Foo{ID: 1})}
					So(ds.Get(f1), ShouldBeNil)
					So(f1.Val, ShouldEqual, 100)

					f2 := &Foo{ID: 2, Parent: f1.Parent}
					So(ds.Get(f2), ShouldBeNil)
					So(f2.Val, ShouldEqual, 200)
				})

				Convey("Deletes work too", func() {
					err := ds.RunInTransaction(func(c context.Context) error {
						ds := wrapper.GetDS(c)
						So(ds, ShouldNotBeNil)
						So(ds.Delete(ds.NewKeyObj(f)), ShouldBeNil)
						return nil
					}, nil)
					So(err, ShouldBeNil)
					So(ds.Get(f), ShouldEqual, datastore.ErrNoSuchEntity)
				})

				Convey("A Get counts against your group count", func() {
					err := ds.RunInTransaction(func(c context.Context) error {
						ds := wrapper.GetDS(c)
						f := &Foo{ID: 20}
						So(ds.Get(f), ShouldEqual, datastore.ErrNoSuchEntity)

						f.ID = 1
						So(ds.Get(f).Error(), ShouldContainSubstring, "cross-group")
						return nil
					}, nil)
					So(err, ShouldBeNil)
				})

				Convey("Get takes a snapshot", func() {
					err := ds.RunInTransaction(func(c context.Context) error {
						txnDS := wrapper.GetDS(c)
						So(txnDS, ShouldNotBeNil)

						f := &Foo{ID: 1}
						So(txnDS.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)

						// Don't ever do this in a real program unless you want to guarantee
						// a failed transaction :)
						f.Val = 11
						_, err := ds.Put(f)
						So(err, ShouldBeNil)

						So(txnDS.Get(f), ShouldBeNil)
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
						txnDS := wrapper.GetDS(c)
						So(txnDS, ShouldNotBeNil)

						f := &Foo{ID: 1}
						So(txnDS.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)

						// Don't ever do this in a real program unless you want to guarantee
						// a failed transaction :)
						f.Val = 11
						_, err := ds.Put(f)
						So(err, ShouldBeNil)

						So(txnDS.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)

						f.Val = 20
						_, err = txnDS.Put(f)
						So(err, ShouldBeNil)

						So(txnDS.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10) // still gets 10

						return nil
					}, nil)
					So(err.Error(), ShouldContainSubstring, "concurrent")

					f := &Foo{ID: 1}
					So(ds.Get(f), ShouldBeNil)
					So(f.Val, ShouldEqual, 11)
				})

				Convey("Reusing a transaction context is bad news", func() {
					txnDS := wrapper.Datastore(nil)
					err := ds.RunInTransaction(func(c context.Context) error {
						txnDS = wrapper.GetDS(c)
						So(txnDS.Get(&Foo{ID: 1}), ShouldBeNil)
						return nil
					}, nil)
					So(err, ShouldBeNil)
					So(txnDS.Get(&Foo{ID: 1}).Error(), ShouldContainSubstring, "expired")
				})

				Convey("Nested transactions are rejected", func() {
					err := ds.RunInTransaction(func(c context.Context) error {
						err := wrapper.GetDS(c).RunInTransaction(func(c context.Context) error {
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
						txnDS := wrapper.GetDS(c)
						f := &Foo{ID: 1, Val: 21}
						_, err = txnDS.Put(f)
						So(err, ShouldBeNil)

						err := ds.RunInTransaction(func(c context.Context) error {
							txnDS := wrapper.GetDS(c)
							f := &Foo{ID: 1, Val: 27}
							_, err := txnDS.Put(f)
							So(err, ShouldBeNil)
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
							ds := wrapper.GetDS(c)
							f := &Foo{ID: 1, Val: 200}
							_, err := ds.Put(f)
							So(err, ShouldBeNil)

							f.ID = 2
							_, err = ds.Put(f)
							So(err.Error(), ShouldContainSubstring, "cross-group")
							return err
						}, nil)
						So(err.Error(), ShouldContainSubstring, "cross-group")
					})

					Convey("Modifying >25 groups with XG=true is invald", func() {
						err := ds.RunInTransaction(func(c context.Context) error {
							ds := wrapper.GetDS(c)
							for i := int64(1); i < 26; i++ {
								f := &Foo{ID: i, Val: 200}
								_, err := ds.Put(f)
								So(err, ShouldBeNil)
							}
							f := &Foo{ID: 27, Val: 200}
							_, err := ds.Put(f)
							So(err.Error(), ShouldContainSubstring, "too many entity groups")
							return err
						}, &datastore.TransactionOptions{XG: true})
						So(err.Error(), ShouldContainSubstring, "too many entity groups")
					})
				})

				Convey("Errors and panics", func() {
					Convey("returning an error aborts", func() {
						err := ds.RunInTransaction(func(c context.Context) error {
							ds := wrapper.GetDS(c)
							f := &Foo{ID: 1, Val: 200}
							_, err := ds.Put(f)
							So(err, ShouldBeNil)

							return fmt.Errorf("thingy")
						}, nil)
						So(err.Error(), ShouldEqual, "thingy")

						f := &Foo{ID: 1}
						So(ds.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)
					})

					Convey("panicing aborts", func() {
						So(func() {
							ds.RunInTransaction(func(c context.Context) error {
								ds := wrapper.GetDS(c)
								f := &Foo{ID: 1, Val: 200}
								_, err := ds.Put(f)
								So(err, ShouldBeNil)
								panic("wheeeeee")
							}, nil)
						}, ShouldPanic)

						f := &Foo{ID: 1}
						So(ds.Get(f), ShouldBeNil)
						So(f.Val, ShouldEqual, 10)
					})
				})
			})
		})

	})
}

func TestDatastoreQueryer(t *testing.T) {
	Convey("Datastore Query suport", t, func() {
		c := Use(context.Background())
		ds := wrapper.GetDS(c)
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
				q := q.Limit(math.MaxInt64)
				So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "query limit overflow")
			})
			Convey("underflow offset", func() {
				q := q.Offset(-29)
				So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "negative query offset")
			})
			Convey("OOB offset", func() {
				q := q.Offset(math.MaxInt64)
				So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "query offset overflow")
			})
			Convey("Bad cursors", func() {
				q := q.Start(queryCursor("")).End(queryCursor(""))
				So(q.(*queryImpl).err.Error(), ShouldContainSubstring, "invalid cursor")
			})
			Convey("Bad ancestors", func() {
				q := q.Ancestor(ds.NewKey("Goop", "wat", 10, nil))
				So(q, ShouldNotBeNil)
				qi := q.(*queryImpl).checkCorrectness("", false)
				So(qi.err, ShouldEqual, datastore.ErrInvalidKey)
			})
			Convey("nil ancestors", func() {
				qi := q.Ancestor(nil).(*queryImpl).checkCorrectness("", false)
				So(qi.err.Error(), ShouldContainSubstring, "nil query ancestor")
			})
			Convey("Bad key filters", func() {
				q := q.Filter("__key__ =", ds.NewKey("Goop", "wat", 10, nil))
				qi := q.(*queryImpl).checkCorrectness("", false)
				So(qi.err, ShouldEqual, datastore.ErrInvalidKey)
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
