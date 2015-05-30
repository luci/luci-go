// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"fmt"
	"infra/gae/libs/meta"
	"infra/gae/libs/wrapper"
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
