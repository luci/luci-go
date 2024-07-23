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
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/gae/service/info"
	mc "go.chromium.org/luci/gae/service/memcache"
)

func TestMemcache(t *testing.T) {
	t.Parallel()

	ftt.Run("memcache", t, func(t *ftt.Test) {
		now := time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC)
		c, tc := testclock.UseTime(context.Background(), now)
		c = Use(c)

		t.Run("implements MCSingleReadWriter", func(t *ftt.Test) {
			t.Run("Add", func(t *ftt.Test) {
				itm := (mc.NewItem(c, "sup").
					SetValue([]byte("cool")).
					SetExpiration(time.Second))
				assert.Loosely(t, mc.Add(c, itm), should.BeNil)
				t.Run("which rejects objects already there", func(t *ftt.Test) {
					assert.Loosely(t, mc.Add(c, itm), should.Equal(mc.ErrNotStored))
				})
			})

			t.Run("Get", func(t *ftt.Test) {
				itm := &mcItem{
					key:        "sup",
					value:      []byte("cool"),
					expiration: time.Second,
				}
				assert.Loosely(t, mc.Add(c, itm), should.BeNil)

				testItem := &mcItem{
					key:   "sup",
					value: []byte("cool"),
					CasID: 1,
				}
				getItm, err := mc.GetKey(c, "sup")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, getItm, should.Resemble(testItem))

				t.Run("which can expire", func(t *ftt.Test) {
					tc.Add(time.Second * 4)
					getItm, err := mc.GetKey(c, "sup")
					assert.Loosely(t, err, should.Equal(mc.ErrCacheMiss))
					assert.Loosely(t, getItm, should.Resemble(&mcItem{key: "sup"}))
				})
			})

			t.Run("Delete", func(t *ftt.Test) {
				t.Run("works if it's there", func(t *ftt.Test) {
					itm := &mcItem{
						key:        "sup",
						value:      []byte("cool"),
						expiration: time.Second,
					}
					assert.Loosely(t, mc.Add(c, itm), should.BeNil)

					assert.Loosely(t, mc.Delete(c, "sup"), should.BeNil)

					_, err := mc.GetKey(c, "sup")
					assert.Loosely(t, err, should.Equal(mc.ErrCacheMiss))
				})

				t.Run("but not if it's not there", func(t *ftt.Test) {
					assert.Loosely(t, mc.Delete(c, "sup"), should.Equal(mc.ErrCacheMiss))
				})
			})

			t.Run("Set", func(t *ftt.Test) {
				itm := &mcItem{
					key:        "sup",
					value:      []byte("cool"),
					expiration: time.Second,
				}
				assert.Loosely(t, mc.Add(c, itm), should.BeNil)

				itm.SetValue([]byte("newp"))
				assert.Loosely(t, mc.Set(c, itm), should.BeNil)

				testItem := &mcItem{
					key:   "sup",
					value: []byte("newp"),
					CasID: 2,
				}
				getItm, err := mc.GetKey(c, "sup")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, getItm, should.Resemble(testItem))

				t.Run("Flush works too", func(t *ftt.Test) {
					assert.Loosely(t, mc.Flush(c), should.BeNil)
					_, err := mc.GetKey(c, "sup")
					assert.Loosely(t, err, should.Equal(mc.ErrCacheMiss))
				})
			})

			t.Run("Set (nil) is equivalent to Set([]byte{})", func(t *ftt.Test) {
				assert.Loosely(t, mc.Set(c, mc.NewItem(c, "bob")), should.BeNil)

				bob, err := mc.GetKey(c, "bob")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, bob.Value(), should.Resemble([]byte{}))
			})

			t.Run("Increment", func(t *ftt.Test) {
				val, err := mc.Increment(c, "num", 7, 2)
				assert.Loosely(t, err, should.BeNil)
				assert.That(t, val, should.Equal[uint64](9))

				t.Run("Increment again", func(t *ftt.Test) {
					val, err = mc.Increment(c, "num", 7, 2)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, val, should.Equal[uint64](16))
				})

				t.Run("IncrementExisting", func(t *ftt.Test) {
					val, err := mc.IncrementExisting(c, "num", -2)
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, val, should.Equal[uint64](7))

					val, err = mc.IncrementExisting(c, "num", -100)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, val, should.BeZero)

					_, err = mc.IncrementExisting(c, "noexist", 2)
					assert.Loosely(t, err, should.Equal(mc.ErrCacheMiss))

					assert.Loosely(t, mc.Set(c, mc.NewItem(c, "text").SetValue([]byte("hello world, hooman!"))), should.BeNil)

					_, err = mc.IncrementExisting(c, "text", 2)
					assert.Loosely(t, err.Error(), should.ContainSubstring("got invalid current value"))
				})
			})

			t.Run("CompareAndSwap", func(t *ftt.Test) {
				itm := mc.Item(&mcItem{
					key:        "sup",
					value:      []byte("cool"),
					expiration: time.Second * 2,
				})
				assert.Loosely(t, mc.Add(c, itm), should.BeNil)

				t.Run("works after a Get", func(t *ftt.Test) {
					itm, err := mc.GetKey(c, "sup")
					assert.Loosely(t, err, should.BeNil)
					assert.That(t, itm.(*mcItem).CasID, should.Equal[uint64](1))

					itm.SetValue([]byte("newp"))
					assert.Loosely(t, mc.CompareAndSwap(c, itm), should.BeNil)
				})

				t.Run("but fails if you don't", func(t *ftt.Test) {
					itm.SetValue([]byte("newp"))
					assert.Loosely(t, mc.CompareAndSwap(c, itm), should.Equal(mc.ErrCASConflict))
				})

				t.Run("and fails if the item is expired/gone", func(t *ftt.Test) {
					tc.Add(3 * time.Second)
					itm.SetValue([]byte("newp"))
					assert.Loosely(t, mc.CompareAndSwap(c, itm), should.Equal(mc.ErrNotStored))
				})
			})
		})

		t.Run("check that the internal implementation is sane", func(t *ftt.Test) {
			curTime := now
			err := mc.Add(c, &mcItem{
				key:        "sup",
				value:      []byte("cool"),
				expiration: time.Second * 2,
			})

			for i := 0; i < 4; i++ {
				_, err := mc.GetKey(c, "sup")
				assert.Loosely(t, err, should.BeNil)
			}
			_, err = mc.GetKey(c, "wot")
			assert.Loosely(t, err, should.ErrLike(mc.ErrCacheMiss))

			mci := mc.Raw(c).(*memcacheImpl)

			stats, err := mc.Stats(c)
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, stats.Items, should.Equal[uint64](1))
			assert.That(t, stats.Bytes, should.Equal[uint64](4))
			assert.That(t, stats.Hits, should.Equal[uint64](4))
			assert.That(t, stats.Misses, should.Equal[uint64](1))
			assert.That(t, stats.ByteHits, should.Equal[uint64](4*4))
			assert.That(t, mci.data.casID, should.Equal[uint64](1))
			assert.Loosely(t, mci.data.items["sup"], should.Resemble(&mcDataItem{
				value:      []byte("cool"),
				expiration: curTime.Add(time.Second * 2).Truncate(time.Second),
				casID:      1,
			}))

			getItm, err := mc.GetKey(c, "sup")
			assert.Loosely(t, err, should.BeNil)
			assert.That(t, len(mci.data.items), should.Equal(1))
			assert.That(t, mci.data.casID, should.Equal[uint64](1))

			testItem := &mcItem{
				key:   "sup",
				value: []byte("cool"),
				CasID: 1,
			}
			assert.Loosely(t, getItm, should.Resemble(testItem))
		})

		t.Run("When adding an item to an unset namespace", func(t *ftt.Test) {
			assert.Loosely(t, info.GetNamespace(c), should.BeEmpty)

			item := mc.NewItem(c, "foo").SetValue([]byte("heya"))
			assert.Loosely(t, mc.Set(c, item), should.BeNil)

			t.Run("The item can be retrieved from the unset namespace.", func(t *ftt.Test) {
				got, err := mc.GetKey(c, "foo")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got.Value(), should.Resemble([]byte("heya")))
			})

			t.Run("The item can be retrieved from a set, empty namespace.", func(t *ftt.Test) {
				// Now test with empty namespace.
				c = info.MustNamespace(c, "")

				got, err := mc.GetKey(c, "foo")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, got.Value(), should.Resemble([]byte("heya")))
			})
		})
	})
}
