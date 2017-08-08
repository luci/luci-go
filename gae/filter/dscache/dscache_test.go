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

package dscache

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math/rand"
	"testing"
	"time"

	"go.chromium.org/gae/filter/featureBreaker"
	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/datastore/serialize"
	mc "go.chromium.org/gae/service/memcache"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

type object struct {
	ID int64 `gae:"$id"`

	Value   string
	BigData []byte
}

type shardObj struct {
	ID int64 `gae:"$id"`

	Value string
}

func shardObjFn(k *ds.Key) (amt int, ok bool) {
	if last := k.LastTok(); last.Kind == "shardObj" {
		amt = int(last.IntID)
		ok = true
	}
	return
}

type noCacheObj struct {
	ID string `gae:"$id"`

	Value bool
}

func noCacheObjFn(k *ds.Key) (amt int, ok bool) {
	if k.Kind() == "noCacheObj" {
		ok = true
	}
	return
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

		underCtx := c

		numMemcacheItems := func() uint64 {
			stats, err := mc.Stats(c)
			So(err, ShouldBeNil)
			return stats.Items
		}

		Convey("enabled cases", func() {
			c = FilterRDS(c)
			c = AddShardFunctions(c, shardObjFn, noCacheObjFn)

			Convey("basically works", func() {
				pm := ds.PropertyMap{
					"BigData": ds.MkProperty([]byte("")),
					"Value":   ds.MkProperty("hi"),
				}
				encoded := append([]byte{0}, serialize.ToBytes(pm)...)

				o := object{ID: 1, Value: "hi"}
				So(ds.Put(c, &o), ShouldBeNil)

				o = object{ID: 1}
				So(ds.Get(underCtx, &o), ShouldBeNil)
				So(o.Value, ShouldEqual, "hi")

				itm, err := mc.GetKey(c, MakeMemcacheKey(0, ds.KeyForObj(c, &o)))
				So(err, ShouldEqual, mc.ErrCacheMiss)

				o = object{ID: 1}
				So(ds.Get(c, &o), ShouldBeNil)
				So(o.Value, ShouldEqual, "hi")

				itm, err = mc.GetKey(c, itm.Key())
				So(err, ShouldBeNil)
				So(itm.Value(), ShouldResemble, encoded)

				Convey("now we don't need the datastore!", func() {
					o := object{ID: 1}

					// delete it, bypassing the cache filter. Don't do this in production
					// unless you want a crappy cache.
					So(ds.Delete(underCtx, ds.KeyForObj(underCtx, &o)), ShouldBeNil)

					itm, err := mc.GetKey(c, MakeMemcacheKey(0, ds.KeyForObj(c, &o)))
					So(err, ShouldBeNil)
					So(itm.Value(), ShouldResemble, encoded)

					So(ds.Get(c, &o), ShouldBeNil)
					So(o.Value, ShouldEqual, "hi")
				})

				Convey("deleting it properly records that fact, however", func() {
					o := object{ID: 1}
					So(ds.Delete(c, ds.KeyForObj(c, &o)), ShouldBeNil)

					itm, err := mc.GetKey(c, MakeMemcacheKey(0, ds.KeyForObj(c, &o)))
					So(err, ShouldEqual, mc.ErrCacheMiss)
					So(ds.Get(c, &o), ShouldEqual, ds.ErrNoSuchEntity)

					itm, err = mc.GetKey(c, itm.Key())
					So(err, ShouldBeNil)
					So(itm.Value(), ShouldResemble, []byte{})

					// this one hits memcache
					So(ds.Get(c, &o), ShouldEqual, ds.ErrNoSuchEntity)
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

				So(ds.Put(c, &o), ShouldBeNil)
				So(ds.Get(c, &o), ShouldBeNil)

				itm, err := mc.GetKey(c, MakeMemcacheKey(0, ds.KeyForObj(c, &o)))
				So(err, ShouldBeNil)

				So(itm.Value()[0], ShouldEqual, ZlibCompression)
				So(len(itm.Value()), ShouldEqual, 653) // a bit smaller than 4k

				// ensure the next Get comes from the cache
				So(ds.Delete(underCtx, ds.KeyForObj(underCtx, &o)), ShouldBeNil)

				o = object{ID: 2}
				So(ds.Get(c, &o), ShouldBeNil)
				So(o.Value, ShouldEqual, `¯\_(ツ)_/¯`)
				So(o.BigData, ShouldResemble, data)
			})

			Convey("transactions", func() {
				Convey("work", func() {
					// populate an object @ ID1
					So(ds.Put(c, &object{ID: 1, Value: "something"}), ShouldBeNil)
					So(ds.Get(c, &object{ID: 1}), ShouldBeNil)

					So(ds.Put(c, &object{ID: 2, Value: "nurbs"}), ShouldBeNil)
					So(ds.Get(c, &object{ID: 2}), ShouldBeNil)

					// memcache now has the wrong value (simulated race)
					So(ds.Put(underCtx, &object{ID: 1, Value: "else"}), ShouldBeNil)
					So(ds.RunInTransaction(c, func(c context.Context) error {
						o := &object{ID: 1}
						So(ds.Get(c, o), ShouldBeNil)
						So(o.Value, ShouldEqual, "else")
						o.Value = "txn"
						So(ds.Put(c, o), ShouldBeNil)

						So(ds.Delete(c, ds.KeyForObj(c, &object{ID: 2})), ShouldBeNil)
						return nil
					}, &ds.TransactionOptions{XG: true}), ShouldBeNil)

					_, err := mc.GetKey(c, MakeMemcacheKey(0, ds.KeyForObj(c, &object{ID: 1})))
					So(err, ShouldEqual, mc.ErrCacheMiss)
					_, err = mc.GetKey(c, MakeMemcacheKey(0, ds.KeyForObj(c, &object{ID: 2})))
					So(err, ShouldEqual, mc.ErrCacheMiss)
					o := &object{ID: 1}
					So(ds.Get(c, o), ShouldBeNil)
					So(o.Value, ShouldEqual, "txn")
				})

				Convey("errors don't invalidate", func() {
					// populate an object @ ID1
					So(ds.Put(c, &object{ID: 1, Value: "something"}), ShouldBeNil)
					So(ds.Get(c, &object{ID: 1}), ShouldBeNil)
					So(numMemcacheItems(), ShouldEqual, 1)

					So(ds.RunInTransaction(c, func(c context.Context) error {
						o := &object{ID: 1}
						So(ds.Get(c, o), ShouldBeNil)
						So(o.Value, ShouldEqual, "something")
						o.Value = "txn"
						So(ds.Put(c, o), ShouldBeNil)
						return errors.New("OH NOES")
					}, nil).Error(), ShouldContainSubstring, "OH NOES")

					// memcache still has the original
					So(numMemcacheItems(), ShouldEqual, 1)
					So(ds.Delete(underCtx, ds.KeyForObj(underCtx, &object{ID: 1})), ShouldBeNil)
					o := &object{ID: 1}
					So(ds.Get(c, o), ShouldBeNil)
					So(o.Value, ShouldEqual, "something")
				})
			})

			Convey("control", func() {
				Convey("per-model bypass", func() {
					type model struct {
						ID         string    `gae:"$id"`
						UseDSCache ds.Toggle `gae:"$dscache.enable,false"`

						Value string
					}

					itms := []model{
						{ID: "hi", Value: "something"},
						{ID: "there", Value: "else", UseDSCache: ds.On},
					}

					So(ds.Put(c, itms), ShouldBeNil)
					So(ds.Get(c, itms), ShouldBeNil)

					So(numMemcacheItems(), ShouldEqual, 1)
				})

				Convey("per-key shard count", func() {
					s := &shardObj{ID: 4, Value: "hi"}
					So(ds.Put(c, s), ShouldBeNil)
					So(ds.Get(c, s), ShouldBeNil)

					So(numMemcacheItems(), ShouldEqual, 1)
					for i := 0; i < 20; i++ {
						So(ds.Get(c, s), ShouldBeNil)
					}
					So(numMemcacheItems(), ShouldEqual, 4)
				})

				Convey("per-key cache disablement", func() {
					n := &noCacheObj{ID: "nurbs", Value: true}
					So(ds.Put(c, n), ShouldBeNil)
					So(ds.Get(c, n), ShouldBeNil)
					So(numMemcacheItems(), ShouldEqual, 0)
				})

				Convey("per-model expiration", func() {
					type model struct {
						ID         int64 `gae:"$id"`
						DSCacheExp int64 `gae:"$dscache.expiration,7"`

						Value string
					}

					So(ds.Put(c, &model{ID: 1, Value: "mooo"}), ShouldBeNil)
					So(ds.Get(c, &model{ID: 1}), ShouldBeNil)

					itm, err := mc.GetKey(c, MakeMemcacheKey(0, ds.KeyForObj(c, &model{ID: 1})))
					So(err, ShouldBeNil)

					clk.Add(10 * time.Second)
					_, err = mc.GetKey(c, itm.Key())
					So(err, ShouldEqual, mc.ErrCacheMiss)
				})
			})

			Convey("screw cases", func() {
				Convey("memcache contains bogus value (simulated failed AddMulti)", func() {
					o := &object{ID: 1, Value: "spleen"}
					So(ds.Put(c, o), ShouldBeNil)

					sekret := []byte("I am a banana")
					itm := mc.NewItem(c, MakeMemcacheKey(0, ds.KeyForObj(c, o))).SetValue(sekret)
					So(mc.Set(c, itm), ShouldBeNil)

					o = &object{ID: 1}
					So(ds.Get(c, o), ShouldBeNil)
					So(o.Value, ShouldEqual, "spleen")

					itm, err := mc.GetKey(c, itm.Key())
					So(err, ShouldBeNil)
					So(itm.Flags(), ShouldEqual, ItemUKNONWN)
					So(itm.Value(), ShouldResemble, sekret)
				})

				Convey("memcache contains bogus value (corrupt entry)", func() {
					o := &object{ID: 1, Value: "spleen"}
					So(ds.Put(c, o), ShouldBeNil)

					sekret := []byte("I am a banana")
					itm := (mc.NewItem(c, MakeMemcacheKey(0, ds.KeyForObj(c, o))).
						SetValue(sekret).
						SetFlags(uint32(ItemHasData)))
					So(mc.Set(c, itm), ShouldBeNil)

					o = &object{ID: 1}
					So(ds.Get(c, o), ShouldBeNil)
					So(o.Value, ShouldEqual, "spleen")

					itm, err := mc.GetKey(c, itm.Key())
					So(err, ShouldBeNil)
					So(itm.Flags(), ShouldEqual, ItemHasData)
					So(itm.Value(), ShouldResemble, sekret)
				})

				Convey("other entity has the lock", func() {
					o := &object{ID: 1, Value: "spleen"}
					So(ds.Put(c, o), ShouldBeNil)

					sekret := []byte("r@vmarod!#)%9T")
					itm := (mc.NewItem(c, MakeMemcacheKey(0, ds.KeyForObj(c, o))).
						SetValue(sekret).
						SetFlags(uint32(ItemHasLock)))
					So(mc.Set(c, itm), ShouldBeNil)

					o = &object{ID: 1}
					So(ds.Get(c, o), ShouldBeNil)
					So(o.Value, ShouldEqual, "spleen")

					itm, err := mc.GetKey(c, itm.Key())
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
					So(ds.Put(c, o), ShouldBeNil)

					o.BigData = nil
					So(ds.Get(c, o), ShouldBeNil)

					itm, err := mc.GetKey(c, MakeMemcacheKey(0, ds.KeyForObj(c, o)))
					So(err, ShouldBeNil)

					// Is locked until the next put, forcing all access to the datastore.
					So(itm.Value(), ShouldResemble, []byte{})
					So(itm.Flags(), ShouldEqual, ItemHasLock)

					o.BigData = []byte("hi :)")
					So(ds.Put(c, o), ShouldBeNil)
					So(ds.Get(c, o), ShouldBeNil)

					itm, err = mc.GetKey(c, itm.Key())
					So(err, ShouldBeNil)
					So(itm.Flags(), ShouldEqual, ItemHasData)
				})

				Convey("failure on Setting memcache locks is a hard stop", func() {
					c, fb := featureBreaker.FilterMC(c, nil)
					fb.BreakFeatures(nil, "SetMulti")
					So(ds.Put(c, &object{ID: 1}).Error(), ShouldContainSubstring, "SetMulti")
				})

				Convey("failure on Setting memcache locks in a transaction is a hard stop", func() {
					c, fb := featureBreaker.FilterMC(c, nil)
					fb.BreakFeatures(nil, "SetMulti")
					So(ds.RunInTransaction(c, func(c context.Context) error {
						So(ds.Put(c, &object{ID: 1}), ShouldBeNil)
						// no problems here... memcache operations happen after the function
						// body quits.
						return nil
					}, nil).Error(), ShouldContainSubstring, "SetMulti")
				})

			})

			Convey("misc", func() {
				Convey("verify numShards caps at MaxShards", func() {
					sc := supportContext{shardsForKey: []ShardFunction{shardObjFn}}
					So(sc.numShards(ds.KeyForObj(c, &shardObj{ID: 9001})), ShouldEqual, MaxShards)
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

			So(mc.Set(c, mc.NewItem(c, "test").SetValue([]byte("hi"))), ShouldBeNil)
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
		newC := FilterRDS(c)
		So(newC, ShouldEqual, c)
	})
}
