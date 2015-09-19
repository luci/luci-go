// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dscache

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/luci/gae/filter/featureBreaker"
	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/serialize"
	"github.com/luci/gae/service/memcache"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/mathrand"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

type object struct {
	ID int64 `gae:"$id"`

	Value   string
	BigData []byte
}

type shardObj struct { // see shardsForKey() at top
	ID int64 `gae:"$id"`

	Value string
}

type noCacheObj struct { // see shardsForKey() at top
	ID string `gae:"$id"`

	Value bool
}

func init() {
	serialize.WritePropertyMapDeterministic = true

	internalValueSizeLimit = 2048
}

func TestDSCache(t *testing.T) {
	t.Parallel()

	zeroTime, err := time.Parse("2006-01-02T15:04:05.999999999Z", "2006-01-02T15:04:05.999999999Z")
	if err != nil {
		panic(err)
	}

	Convey("Test dscache", t, func() {
		c := mathrand.Set(context.Background(), rand.New(rand.NewSource(1)))
		clk := testclock.New(zeroTime)
		c = clock.Set(c, clk)
		c = memory.Use(c)

		dsUnder := datastore.Get(c)
		mc := memcache.Get(c)

		shardsForKey := func(k *datastore.Key) int {
			last := k.Last()
			if last.Kind == "shardObj" {
				return int(last.IntID)
			}
			if last.Kind == "noCacheObj" {
				return 0
			}
			return DefaultShards
		}

		numMemcacheItems := func() uint64 {
			stats, err := mc.Stats()
			So(err, ShouldBeNil)
			return stats.Items
		}

		Convey("enabled cases", func() {
			c = FilterRDS(c, shardsForKey)
			ds := datastore.Get(c)
			So(dsUnder, ShouldNotBeNil)
			So(ds, ShouldNotBeNil)
			So(mc, ShouldNotBeNil)

			Convey("basically works", func() {
				pm := datastore.PropertyMap{
					"BigData": {datastore.MkProperty([]byte(""))},
					"Value":   {datastore.MkProperty("hi")},
				}
				encoded := append([]byte{0}, serialize.ToBytes(pm)...)

				o := object{ID: 1, Value: "hi"}
				So(ds.Put(&o), ShouldBeNil)

				o = object{ID: 1}
				So(dsUnder.Get(&o), ShouldBeNil)
				So(o.Value, ShouldEqual, "hi")

				itm, err := mc.Get(MakeMemcacheKey(0, ds.KeyForObj(&o)))
				So(err, ShouldEqual, memcache.ErrCacheMiss)

				o = object{ID: 1}
				So(ds.Get(&o), ShouldBeNil)
				So(o.Value, ShouldEqual, "hi")

				itm, err = mc.Get(itm.Key())
				So(err, ShouldBeNil)
				So(itm.Value(), ShouldResemble, encoded)

				Convey("now we don't need the datastore!", func() {
					o := object{ID: 1}

					// delete it, bypassing the cache filter. Don't do this in production
					// unless you want a crappy cache.
					So(dsUnder.Delete(ds.KeyForObj(&o)), ShouldBeNil)

					itm, err := mc.Get(MakeMemcacheKey(0, ds.KeyForObj(&o)))
					So(err, ShouldBeNil)
					So(itm.Value(), ShouldResemble, encoded)

					So(ds.Get(&o), ShouldBeNil)
					So(o.Value, ShouldEqual, "hi")
				})

				Convey("deleting it properly records that fact, however", func() {
					o := object{ID: 1}
					So(ds.Delete(ds.KeyForObj(&o)), ShouldBeNil)

					itm, err := mc.Get(MakeMemcacheKey(0, ds.KeyForObj(&o)))
					So(err, ShouldEqual, memcache.ErrCacheMiss)
					So(ds.Get(&o), ShouldEqual, datastore.ErrNoSuchEntity)

					itm, err = mc.Get(itm.Key())
					So(err, ShouldBeNil)
					So(itm.Value(), ShouldResemble, []byte{})

					// this one hits memcache
					So(ds.Get(&o), ShouldEqual, datastore.ErrNoSuchEntity)
				})
			})

			Convey("compression works", func() {
				o := object{ID: 2, Value: `¯\_(ツ)_/¯`}
				data := make([]byte, 4000)
				for i := range data {
					const alpha = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*()"
					data[i] = alpha[i%len(alpha)]
				}
				o.BigData = data

				So(ds.Put(&o), ShouldBeNil)
				So(ds.Get(&o), ShouldBeNil)

				itm, err := mc.Get(MakeMemcacheKey(0, ds.KeyForObj(&o)))
				So(err, ShouldBeNil)

				So(itm.Value()[0], ShouldEqual, ZlibCompression)
				So(len(itm.Value()), ShouldEqual, 653) // a bit smaller than 4k

				// ensure the next Get comes from the cache
				So(dsUnder.Delete(ds.KeyForObj(&o)), ShouldBeNil)

				o = object{ID: 2}
				So(ds.Get(&o), ShouldBeNil)
				So(o.Value, ShouldEqual, `¯\_(ツ)_/¯`)
				So(o.BigData, ShouldResemble, data)
			})

			Convey("transactions", func() {
				Convey("work", func() {
					// populate an object @ ID1
					So(ds.Put(&object{ID: 1, Value: "something"}), ShouldBeNil)
					So(ds.Get(&object{ID: 1}), ShouldBeNil)

					So(ds.Put(&object{ID: 2, Value: "nurbs"}), ShouldBeNil)
					So(ds.Get(&object{ID: 2}), ShouldBeNil)

					// memcache now has the wrong value (simulated race)
					So(dsUnder.Put(&object{ID: 1, Value: "else"}), ShouldBeNil)
					So(ds.RunInTransaction(func(c context.Context) error {
						ds := datastore.Get(c)
						o := &object{ID: 1}
						So(ds.Get(o), ShouldBeNil)
						So(o.Value, ShouldEqual, "else")
						o.Value = "txn"
						So(ds.Put(o), ShouldBeNil)

						So(ds.Delete(ds.KeyForObj(&object{ID: 2})), ShouldBeNil)
						return nil
					}, &datastore.TransactionOptions{XG: true}), ShouldBeNil)

					_, err := mc.Get(MakeMemcacheKey(0, ds.KeyForObj(&object{ID: 1})))
					So(err, ShouldEqual, memcache.ErrCacheMiss)
					_, err = mc.Get(MakeMemcacheKey(0, ds.KeyForObj(&object{ID: 2})))
					So(err, ShouldEqual, memcache.ErrCacheMiss)
					o := &object{ID: 1}
					So(ds.Get(o), ShouldBeNil)
					So(o.Value, ShouldEqual, "txn")
				})

				Convey("errors don't invalidate", func() {
					// populate an object @ ID1
					So(ds.Put(&object{ID: 1, Value: "something"}), ShouldBeNil)
					So(ds.Get(&object{ID: 1}), ShouldBeNil)
					So(numMemcacheItems(), ShouldEqual, 1)

					So(ds.RunInTransaction(func(c context.Context) error {
						ds := datastore.Get(c)
						o := &object{ID: 1}
						So(ds.Get(o), ShouldBeNil)
						So(o.Value, ShouldEqual, "something")
						o.Value = "txn"
						So(ds.Put(o), ShouldBeNil)
						return errors.New("OH NOES")
					}, nil).Error(), ShouldContainSubstring, "OH NOES")

					// memcache still has the original
					So(numMemcacheItems(), ShouldEqual, 1)
					So(dsUnder.Delete(ds.KeyForObj(&object{ID: 1})), ShouldBeNil)
					o := &object{ID: 1}
					So(ds.Get(o), ShouldBeNil)
					So(o.Value, ShouldEqual, "something")
				})
			})

			Convey("control", func() {
				Convey("per-model bypass", func() {
					type model struct {
						ID         string           `gae:"$id"`
						UseDSCache datastore.Toggle `gae:"$dscache.enable,false"`

						Value string
					}

					itms := []model{
						{ID: "hi", Value: "something"},
						{ID: "there", Value: "else", UseDSCache: datastore.On},
					}

					So(ds.PutMulti(itms), ShouldBeNil)
					So(ds.GetMulti(itms), ShouldBeNil)

					So(numMemcacheItems(), ShouldEqual, 1)
				})

				Convey("per-key shard count", func() {
					s := &shardObj{ID: 4, Value: "hi"}
					So(ds.Put(s), ShouldBeNil)
					So(ds.Get(s), ShouldBeNil)

					So(numMemcacheItems(), ShouldEqual, 1)
					for i := 0; i < 20; i++ {
						So(ds.Get(s), ShouldBeNil)
					}
					So(numMemcacheItems(), ShouldEqual, 4)
				})

				Convey("per-key cache disablement", func() {
					n := &noCacheObj{ID: "nurbs", Value: true}
					So(ds.Put(n), ShouldBeNil)
					So(ds.Get(n), ShouldBeNil)
					So(numMemcacheItems(), ShouldEqual, 0)
				})

				Convey("per-model expiration", func() {
					type model struct {
						ID         int64 `gae:"$id"`
						DSCacheExp int64 `gae:"$dscache.expiration,7"`

						Value string
					}

					So(ds.Put(&model{ID: 1, Value: "mooo"}), ShouldBeNil)
					So(ds.Get(&model{ID: 1}), ShouldBeNil)

					itm, err := mc.Get(MakeMemcacheKey(0, ds.KeyForObj(&model{ID: 1})))
					So(err, ShouldBeNil)

					clk.Add(10 * time.Second)
					_, err = mc.Get(itm.Key())
					So(err, ShouldEqual, memcache.ErrCacheMiss)
				})
			})

			Convey("screw cases", func() {
				Convey("memcache contains bogus value (simulated failed AddMulti)", func() {
					o := &object{ID: 1, Value: "spleen"}
					So(ds.Put(o), ShouldBeNil)

					sekret := []byte("I am a banana")
					itm := mc.NewItem(MakeMemcacheKey(0, ds.KeyForObj(o))).SetValue(sekret)
					So(mc.Set(itm), ShouldBeNil)

					o = &object{ID: 1}
					So(ds.Get(o), ShouldBeNil)
					So(o.Value, ShouldEqual, "spleen")

					itm, err := mc.Get(itm.Key())
					So(err, ShouldBeNil)
					So(itm.Flags(), ShouldEqual, ItemUKNONWN)
					So(itm.Value(), ShouldResemble, sekret)
				})

				Convey("memcache contains bogus value (corrupt entry)", func() {
					o := &object{ID: 1, Value: "spleen"}
					So(ds.Put(o), ShouldBeNil)

					sekret := []byte("I am a banana")
					itm := (mc.NewItem(MakeMemcacheKey(0, ds.KeyForObj(o))).
						SetValue(sekret).
						SetFlags(uint32(ItemHasData)))
					So(mc.Set(itm), ShouldBeNil)

					o = &object{ID: 1}
					So(ds.Get(o), ShouldBeNil)
					So(o.Value, ShouldEqual, "spleen")

					itm, err := mc.Get(itm.Key())
					So(err, ShouldBeNil)
					So(itm.Flags(), ShouldEqual, ItemHasData)
					So(itm.Value(), ShouldResemble, sekret)
				})

				Convey("other entity has the lock", func() {
					o := &object{ID: 1, Value: "spleen"}
					So(ds.Put(o), ShouldBeNil)

					sekret := []byte("r@vmarod!#)%9T")
					itm := (mc.NewItem(MakeMemcacheKey(0, ds.KeyForObj(o))).
						SetValue(sekret).
						SetFlags(uint32(ItemHasLock)))
					So(mc.Set(itm), ShouldBeNil)

					o = &object{ID: 1}
					So(ds.Get(o), ShouldBeNil)
					So(o.Value, ShouldEqual, "spleen")

					itm, err := mc.Get(itm.Key())
					So(err, ShouldBeNil)
					So(itm.Flags(), ShouldEqual, ItemHasLock)
					So(itm.Value(), ShouldResemble, sekret)
				})

				Convey("massive entities can't be cached", func() {
					o := &object{ID: 1, Value: "spleen"}
					mr := mathrand.Get(c)
					numRounds := (internalValueSizeLimit / 8) * 2
					buf := bytes.Buffer{}
					for i := 0; i < numRounds; i++ {
						So(binary.Write(&buf, binary.LittleEndian, mr.Int63()), ShouldBeNil)
					}
					o.BigData = buf.Bytes()
					So(ds.Put(o), ShouldBeNil)

					o.BigData = nil
					So(ds.Get(o), ShouldBeNil)

					itm, err := mc.Get(MakeMemcacheKey(0, ds.KeyForObj(o)))
					So(err, ShouldBeNil)

					// Is locked until the next put, forcing all access to the datastore.
					So(itm.Value(), ShouldResemble, []byte{})
					So(itm.Flags(), ShouldEqual, ItemHasLock)

					o.BigData = []byte("hi :)")
					So(ds.Put(o), ShouldBeNil)
					So(ds.Get(o), ShouldBeNil)

					itm, err = mc.Get(itm.Key())
					So(err, ShouldBeNil)
					So(itm.Flags(), ShouldEqual, ItemHasData)
				})

				Convey("failure on Setting memcache locks is a hard stop", func() {
					c, fb := featureBreaker.FilterMC(c, nil)
					fb.BreakFeatures(nil, "SetMulti")
					ds := datastore.Get(c)
					So(ds.Put(&object{ID: 1}).Error(), ShouldContainSubstring, "SetMulti")
				})

				Convey("failure on Setting memcache locks in a transaction is a hard stop", func() {
					c, fb := featureBreaker.FilterMC(c, nil)
					fb.BreakFeatures(nil, "SetMulti")
					ds := datastore.Get(c)
					So(ds.RunInTransaction(func(c context.Context) error {
						So(datastore.Get(c).Put(&object{ID: 1}), ShouldBeNil)
						// no problems here... memcache operations happen after the function
						// body quits.
						return nil
					}, nil).Error(), ShouldContainSubstring, "SetMulti")
				})

			})

			Convey("misc", func() {
				Convey("verify numShards caps at MaxShards", func() {
					sc := supportContext{shardsForKey: shardsForKey}
					So(sc.numShards(ds.KeyForObj(&shardObj{ID: 9001})), ShouldEqual, MaxShards)
				})

				Convey("CompressionType.String", func() {
					So(NoCompression.String(), ShouldEqual, "NoCompression")
					So(ZlibCompression.String(), ShouldEqual, "ZlibCompression")
					So(CompressionType(100).String(), ShouldEqual, "UNKNOWN_CompressionType(100)")
				})
			})
		})

		Convey("disabled cases", func() {
			defer func() {
				globalEnabled = true
			}()

			So(IsGloballyEnabled(c), ShouldBeTrue)

			So(SetGlobalEnable(c, false), ShouldBeNil)
			// twice is a nop
			So(SetGlobalEnable(c, false), ShouldBeNil)

			// but it takes 5 minutes to kick in
			So(IsGloballyEnabled(c), ShouldBeTrue)
			clk.Add(time.Minute*5 + time.Second)
			So(IsGloballyEnabled(c), ShouldBeFalse)

			So(mc.Set(mc.NewItem("test").SetValue([]byte("hi"))), ShouldBeNil)
			So(numMemcacheItems(), ShouldEqual, 1)
			So(SetGlobalEnable(c, true), ShouldBeNil)
			// memcache gets flushed as a side effect
			So(numMemcacheItems(), ShouldEqual, 0)

			// Still takes 5 minutes to kick in
			So(IsGloballyEnabled(c), ShouldBeFalse)
			clk.Add(time.Minute*5 + time.Second)
			So(IsGloballyEnabled(c), ShouldBeTrue)
		})
	})
}

func TestStaticEnable(t *testing.T) {
	// intentionally not parallel b/c deals with global variable
	// t.Parallel()

	Convey("Test InstanceEnabledStatic", t, func() {
		InstanceEnabledStatic = false
		defer func() {
			InstanceEnabledStatic = true
		}()

		c := context.Background()
		newC := FilterRDS(c, nil)
		So(newC, ShouldEqual, c)
	})
}
