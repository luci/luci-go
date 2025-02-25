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
	"context"
	"encoding/binary"
	"errors"
	"math/rand"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/impl/memory"
	ds "go.chromium.org/luci/gae/service/datastore"
	mc "go.chromium.org/luci/gae/service/memcache"
)

type object struct {
	ID int64 `gae:"$id"`

	Value   string
	BigData []byte

	Nested nested `gae:",lsp"`
}

type nested struct {
	Kind  string `gae:"$kind,NestedKind"`
	ID    string `gae:"$id"`
	Value int64
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
	ds.WritePropertyMapDeterministic = true

	internalValueSizeLimit = CompressionThreshold + 2048
}

func TestDSCache(t *testing.T) {
	t.Parallel()

	zeroTime, err := time.Parse("2006-01-02T15:04:05.999999999Z", "2006-01-02T15:04:05.999999999Z")
	if err != nil {
		panic(err)
	}

	ftt.Run("Test dscache", t, func(t *ftt.Test) {
		c := mathrand.Set(context.Background(), rand.New(rand.NewSource(1)))
		clk := testclock.New(zeroTime)
		c = clock.Set(c, clk)
		c = memory.Use(c)

		underCtx := c

		numMemcacheItems := func() uint64 {
			stats, err := mc.Stats(c)
			assert.Loosely(t, err, should.BeNil)
			return stats.Items
		}

		c = FilterRDS(c, nil)
		c = AddShardFunctions(c, shardObjFn, noCacheObjFn)

		t.Run("basically works", func(t *ftt.Test) {
			pm := ds.PropertyMap{
				"BigData": ds.MkProperty([]byte("")),
				"Value":   ds.MkProperty("hi"),
				"Nested": ds.MkProperty(ds.PropertyMap{
					"$key":  ds.MkProperty(ds.NewKey(c, "NestedKind", "ho", 0, nil)),
					"Value": ds.MkProperty(123),
				}),
			}
			encoded := append([]byte{0}, ds.Serialize.ToBytes(pm)...)

			o := object{ID: 1, Value: "hi", Nested: nested{ID: "ho", Value: 123}}
			assert.Loosely(t, ds.Put(c, &o), should.BeNil)

			expected := o
			expected.Nested.Kind = "NestedKind"

			o = object{ID: 1}
			assert.Loosely(t, ds.Get(underCtx, &o), should.BeNil)
			assert.Loosely(t, o, should.Match(expected))

			itm, err := mc.GetKey(c, makeMemcacheKey(0, ds.KeyForObj(c, &o)))
			assert.Loosely(t, err, should.Equal(mc.ErrCacheMiss))

			o = object{ID: 1}
			assert.Loosely(t, ds.Get(c, &o), should.BeNil)
			assert.Loosely(t, o, should.Match(expected))

			itm, err = mc.GetKey(c, itm.Key())
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, itm.Value(), should.Match(encoded))

			t.Run("now we don't need the datastore!", func(t *ftt.Test) {
				o := object{ID: 1}

				// delete it, bypassing the cache filter. Don't do this in production
				// unless you want a crappy cache.
				assert.Loosely(t, ds.Delete(underCtx, ds.KeyForObj(underCtx, &o)), should.BeNil)

				itm, err := mc.GetKey(c, makeMemcacheKey(0, ds.KeyForObj(c, &o)))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, itm.Value(), should.Match(encoded))

				assert.Loosely(t, ds.Get(c, &o), should.BeNil)
				assert.Loosely(t, o, should.Match(expected))
			})

			t.Run("deleting it properly records that fact, however", func(t *ftt.Test) {
				o := object{ID: 1}
				assert.Loosely(t, ds.Delete(c, ds.KeyForObj(c, &o)), should.BeNil)

				itm, err := mc.GetKey(c, makeMemcacheKey(0, ds.KeyForObj(c, &o)))
				assert.Loosely(t, err, should.Equal(mc.ErrCacheMiss))
				assert.Loosely(t, ds.Get(c, &o), should.Equal(ds.ErrNoSuchEntity))

				itm, err = mc.GetKey(c, itm.Key())
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, itm.Value(), should.Match([]byte{}))

				// this one hits memcache
				assert.Loosely(t, ds.Get(c, &o), should.Equal(ds.ErrNoSuchEntity))
			})
		})

		t.Run("compression works", func(t *ftt.Test) {
			o := object{ID: 2, Value: `¯\_(ツ)_/¯`}
			data := make([]byte, CompressionThreshold+1)
			for i := range data {
				const alpha = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*()"
				data[i] = alpha[i%len(alpha)]
			}
			o.BigData = data

			assert.Loosely(t, ds.Put(c, &o), should.BeNil)
			assert.Loosely(t, ds.Get(c, &o), should.BeNil)

			itm, err := mc.GetKey(c, makeMemcacheKey(0, ds.KeyForObj(c, &o)))
			assert.Loosely(t, err, should.BeNil)

			algo := compressionZlib
			if UseZstd {
				algo = compressionZstd
			}
			assert.Loosely(t, itm.Value()[0], should.Equal(algo))
			assert.Loosely(t, len(itm.Value()), should.BeLessThan(len(data)))

			t.Run("uses compressed cache entry", func(t *ftt.Test) {
				// ensure the next Get comes from the cache
				assert.Loosely(t, ds.Delete(underCtx, ds.KeyForObj(underCtx, &o)), should.BeNil)

				o = object{ID: 2}
				assert.Loosely(t, ds.Get(c, &o), should.BeNil)
				assert.Loosely(t, o.Value, should.Equal(`¯\_(ツ)_/¯`))
				assert.Loosely(t, o.BigData, should.Match(data))
			})

			t.Run("skips unknown compression algo", func(t *ftt.Test) {
				blob := append([]byte(nil), itm.Value()...)
				blob[0] = 123 // unknown algo
				itm.SetValue(blob)

				assert.Loosely(t, mc.Set(c, itm), should.BeNil)

				// Should fallback to fetching from datastore.
				o = object{ID: 2}
				assert.Loosely(t, ds.Get(c, &o), should.BeNil)
				assert.Loosely(t, o.Value, should.Equal(`¯\_(ツ)_/¯`))
				assert.Loosely(t, o.BigData, should.Match(data))
			})
		})

		t.Run("transactions", func(t *ftt.Test) {
			t.Run("work", func(t *ftt.Test) {
				// populate an object @ ID1
				assert.Loosely(t, ds.Put(c, &object{ID: 1, Value: "something"}), should.BeNil)
				assert.Loosely(t, ds.Get(c, &object{ID: 1}), should.BeNil)

				assert.Loosely(t, ds.Put(c, &object{ID: 2, Value: "nurbs"}), should.BeNil)
				assert.Loosely(t, ds.Get(c, &object{ID: 2}), should.BeNil)

				// memcache now has the wrong value (simulated race)
				assert.Loosely(t, ds.Put(underCtx, &object{ID: 1, Value: "else"}), should.BeNil)
				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					o := &object{ID: 1}
					assert.Loosely(t, ds.Get(c, o), should.BeNil)
					assert.Loosely(t, o.Value, should.Equal("else"))
					o.Value = "txn"
					assert.Loosely(t, ds.Put(c, o), should.BeNil)

					assert.Loosely(t, ds.Delete(c, ds.KeyForObj(c, &object{ID: 2})), should.BeNil)
					return nil
				}, nil), should.BeNil)

				_, err := mc.GetKey(c, makeMemcacheKey(0, ds.KeyForObj(c, &object{ID: 1})))
				assert.Loosely(t, err, should.Equal(mc.ErrCacheMiss))
				_, err = mc.GetKey(c, makeMemcacheKey(0, ds.KeyForObj(c, &object{ID: 2})))
				assert.Loosely(t, err, should.Equal(mc.ErrCacheMiss))
				o := &object{ID: 1}
				assert.Loosely(t, ds.Get(c, o), should.BeNil)
				assert.Loosely(t, o.Value, should.Equal("txn"))
			})

			t.Run("errors don't invalidate", func(t *ftt.Test) {
				// populate an object @ ID1
				assert.Loosely(t, ds.Put(c, &object{ID: 1, Value: "something"}), should.BeNil)
				assert.Loosely(t, ds.Get(c, &object{ID: 1}), should.BeNil)
				assert.That(t, numMemcacheItems(), should.Equal[uint64](1))

				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					o := &object{ID: 1}
					assert.Loosely(t, ds.Get(c, o), should.BeNil)
					assert.Loosely(t, o.Value, should.Equal("something"))
					o.Value = "txn"
					assert.Loosely(t, ds.Put(c, o), should.BeNil)
					return errors.New("OH NOES")
				}, nil).Error(), should.ContainSubstring("OH NOES"))

				// memcache still has the original
				assert.That(t, numMemcacheItems(), should.Equal[uint64](1))
				assert.Loosely(t, ds.Delete(underCtx, ds.KeyForObj(underCtx, &object{ID: 1})), should.BeNil)
				o := &object{ID: 1}
				assert.Loosely(t, ds.Get(c, o), should.BeNil)
				assert.Loosely(t, o.Value, should.Equal("something"))
			})
		})

		t.Run("control", func(t *ftt.Test) {
			t.Run("per-model bypass", func(t *ftt.Test) {
				type model struct {
					ID         string    `gae:"$id"`
					UseDSCache ds.Toggle `gae:"$dscache.enable,false"`

					Value string
				}

				itms := []model{
					{ID: "hi", Value: "something"},
					{ID: "there", Value: "else", UseDSCache: ds.On},
				}

				assert.Loosely(t, ds.Put(c, itms), should.BeNil)
				assert.Loosely(t, ds.Get(c, itms), should.BeNil)

				assert.That(t, numMemcacheItems(), should.Equal[uint64](1))
			})

			t.Run("per-key shard count", func(t *ftt.Test) {
				s := &shardObj{ID: 4, Value: "hi"}
				assert.Loosely(t, ds.Put(c, s), should.BeNil)
				assert.Loosely(t, ds.Get(c, s), should.BeNil)

				assert.That(t, numMemcacheItems(), should.Equal[uint64](1))
				for i := 0; i < 20; i++ {
					assert.Loosely(t, ds.Get(c, s), should.BeNil)
				}
				assert.That(t, numMemcacheItems(), should.Equal[uint64](4))
			})

			t.Run("per-key cache disablement", func(t *ftt.Test) {
				n := &noCacheObj{ID: "nurbs", Value: true}
				assert.Loosely(t, ds.Put(c, n), should.BeNil)
				assert.Loosely(t, ds.Get(c, n), should.BeNil)
				assert.Loosely(t, numMemcacheItems(), should.BeZero)
			})

			t.Run("per-model expiration", func(t *ftt.Test) {
				type model struct {
					ID         int64 `gae:"$id"`
					DSCacheExp int64 `gae:"$dscache.expiration,7"`

					Value string
				}

				assert.Loosely(t, ds.Put(c, &model{ID: 1, Value: "mooo"}), should.BeNil)
				assert.Loosely(t, ds.Get(c, &model{ID: 1}), should.BeNil)

				itm, err := mc.GetKey(c, makeMemcacheKey(0, ds.KeyForObj(c, &model{ID: 1})))
				assert.Loosely(t, err, should.BeNil)

				clk.Add(10 * time.Second)
				_, err = mc.GetKey(c, itm.Key())
				assert.Loosely(t, err, should.Equal(mc.ErrCacheMiss))
			})
		})

		t.Run("screw cases", func(t *ftt.Test) {
			t.Run("memcache contains bogus value (simulated failed AddMulti)", func(t *ftt.Test) {
				o := &object{ID: 1, Value: "spleen"}
				assert.Loosely(t, ds.Put(c, o), should.BeNil)

				sekret := []byte("I am a banana")
				itm := mc.NewItem(c, makeMemcacheKey(0, ds.KeyForObj(c, o))).SetValue(sekret)
				assert.Loosely(t, mc.Set(c, itm), should.BeNil)

				o = &object{ID: 1}
				assert.Loosely(t, ds.Get(c, o), should.BeNil)
				assert.Loosely(t, o.Value, should.Equal("spleen"))

				itm, err := mc.GetKey(c, itm.Key())
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, itm.Flags(), should.Equal(itemFlagUnknown))
				assert.Loosely(t, itm.Value(), should.Match(sekret))
			})

			t.Run("memcache contains bogus value (corrupt entry)", func(t *ftt.Test) {
				o := &object{ID: 1, Value: "spleen"}
				assert.Loosely(t, ds.Put(c, o), should.BeNil)

				sekret := []byte("I am a banana")
				itm := (mc.NewItem(c, makeMemcacheKey(0, ds.KeyForObj(c, o))).
					SetValue(sekret).
					SetFlags(itemFlagHasData))
				assert.Loosely(t, mc.Set(c, itm), should.BeNil)

				o = &object{ID: 1}
				assert.Loosely(t, ds.Get(c, o), should.BeNil)
				assert.Loosely(t, o.Value, should.Equal("spleen"))

				itm, err := mc.GetKey(c, itm.Key())
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, itm.Flags(), should.Equal(itemFlagHasData))
				assert.Loosely(t, itm.Value(), should.Match(sekret))
			})

			t.Run("other entity has the lock", func(t *ftt.Test) {
				o := &object{ID: 1, Value: "spleen"}
				assert.Loosely(t, ds.Put(c, o), should.BeNil)

				sekret := []byte("r@vmarod!#)%9T")
				itm := (mc.NewItem(c, makeMemcacheKey(0, ds.KeyForObj(c, o))).
					SetValue(sekret).
					SetFlags(itemFlagHasLock))
				assert.Loosely(t, mc.Set(c, itm), should.BeNil)

				o = &object{ID: 1}
				assert.Loosely(t, ds.Get(c, o), should.BeNil)
				assert.Loosely(t, o.Value, should.Equal("spleen"))

				itm, err := mc.GetKey(c, itm.Key())
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, itm.Flags(), should.Equal(itemFlagHasLock))
				assert.Loosely(t, itm.Value(), should.Match(sekret))
			})

			t.Run("massive entities can't be cached", func(t *ftt.Test) {
				o := &object{ID: 1, Value: "spleen"}
				mr := mathrand.Get(c)
				numRounds := (internalValueSizeLimit / 8) * 2
				buf := bytes.Buffer{}
				for i := 0; i < numRounds; i++ {
					assert.Loosely(t, binary.Write(&buf, binary.LittleEndian, mr.Int63()), should.BeNil)
				}
				o.BigData = buf.Bytes()
				assert.Loosely(t, ds.Put(c, o), should.BeNil)

				o.BigData = nil
				assert.Loosely(t, ds.Get(c, o), should.BeNil)

				itm, err := mc.GetKey(c, makeMemcacheKey(0, ds.KeyForObj(c, o)))
				assert.Loosely(t, err, should.BeNil)

				// Is locked until the next put, forcing all access to the datastore.
				assert.Loosely(t, itm.Value(), should.Match([]byte{}))
				assert.Loosely(t, itm.Flags(), should.Equal(itemFlagHasLock))

				o.BigData = []byte("hi :)")
				assert.Loosely(t, ds.Put(c, o), should.BeNil)
				assert.Loosely(t, ds.Get(c, o), should.BeNil)

				itm, err = mc.GetKey(c, itm.Key())
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, itm.Flags(), should.Equal(itemFlagHasData))
			})

			t.Run("failure on Setting memcache locks is a hard stop", func(t *ftt.Test) {
				c, fb := featureBreaker.FilterMC(c, nil)
				fb.BreakFeatures(nil, "SetMulti")
				assert.Loosely(t, ds.Put(c, &object{ID: 1}).Error(), should.ContainSubstring("SetMulti"))
			})

			t.Run("failure on Setting memcache locks in a transaction is a hard stop", func(t *ftt.Test) {
				c, fb := featureBreaker.FilterMC(c, nil)
				fb.BreakFeatures(nil, "SetMulti")
				assert.Loosely(t, ds.RunInTransaction(c, func(c context.Context) error {
					assert.Loosely(t, ds.Put(c, &object{ID: 1}), should.BeNil)
					// no problems here... memcache operations happen after the function
					// body quits.
					return nil
				}, nil).Error(), should.ContainSubstring("SetMulti"))
			})

			t.Run("verify numShards caps at MaxShards", func(t *ftt.Test) {
				sc := supportContext{shardsForKey: []ShardFunction{shardObjFn}}
				assert.Loosely(t, sc.numShards(ds.KeyForObj(c, &shardObj{ID: 9001})), should.Equal(MaxShards))
			})
		})
	})
}
