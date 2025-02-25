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

//go:build appengine
// +build appengine

package prod

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/gae/service/blobstore"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	mc "go.chromium.org/luci/gae/service/memcache"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
	"google.golang.org/appengine/aetest"
)

var (
	mp   = ds.MkProperty
	mpNI = ds.MkPropertyNI
)

type TestStruct struct {
	ID int64 `gae:"$id"`

	ValueI           []int64
	ValueB           []bool
	ValueS           []string
	ValueF           []float64
	ValueBS          [][]byte // "ByteString"
	ValueK           []*ds.Key
	ValueBK          []blobstore.Key
	ValueGP          []ds.GeoPoint
	ValueSingle      string
	ValueSingleSlice []string
}

func TestBasicDatastore(t *testing.T) {
	t.Parallel()

	ftt.Run("basic", t, func(t *ftt.Test) {
		inst, err := aetest.NewInstance(&aetest.Options{
			StronglyConsistentDatastore: true,
		})
		assert.Loosely(t, err, should.BeNil)
		defer inst.Close()

		req, err := inst.NewRequest("GET", "/", nil)
		assert.Loosely(t, err, should.BeNil)

		ctx := Use(context.Background(), req)

		t.Run("logging allows you to tweak the level", func(t *ftt.Test) {
			// You have to visually confirm that this actually happens in the stdout
			// of the test... yeah I know.
			logging.Debugf(ctx, "SHOULD NOT SEE")
			logging.Infof(ctx, "SHOULD SEE")

			ctx = logging.SetLevel(ctx, logging.Debug)
			logging.Debugf(ctx, "SHOULD SEE")
			logging.Infof(ctx, "SHOULD SEE (2)")
		})

		t.Run("Can probe/change Namespace", func(t *ftt.Test) {
			assert.Loosely(t, info.GetNamespace(ctx), should.BeEmpty)

			ctx, err = info.Namespace(ctx, "wat")
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, info.GetNamespace(ctx), should.Equal("wat"))
			assert.Loosely(t, ds.MakeKey(ctx, "Hello", "world").Namespace(), should.Equal("wat"))
		})

		t.Run("Can get non-transactional context", func(t *ftt.Test) {
			ctx, err := info.Namespace(ctx, "foo")
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, ds.CurrentTransaction(ctx), should.BeNil)

			ds.RunInTransaction(ctx, func(ctx context.Context) error {
				assert.Loosely(t, ds.CurrentTransaction(ctx), should.NotBeNil)
				assert.Loosely(t, ds.MakeKey(ctx, "Foo", "bar").Namespace(), should.Equal("foo"))

				assert.Loosely(t, ds.Put(ctx, &TestStruct{ValueI: []int64{100}}), should.BeNil)

				noTxnCtx := ds.WithoutTransaction(ctx)
				assert.Loosely(t, ds.CurrentTransaction(noTxnCtx), should.BeNil)

				err = ds.RunInTransaction(noTxnCtx, func(ctx context.Context) error {
					assert.Loosely(t, ds.CurrentTransaction(ctx), should.NotBeNil)
					assert.Loosely(t, ds.MakeKey(ctx, "Foo", "bar").Namespace(), should.Equal("foo"))
					assert.Loosely(t, ds.Put(ctx, &TestStruct{ValueI: []int64{100}}), should.BeNil)
					return nil
				}, nil)
				assert.Loosely(t, err, should.BeNil)

				return nil
			}, nil)
		})

		t.Run("Can Put/Get", func(t *ftt.Test) {
			orig := TestStruct{
				ValueI: []int64{1, 7, 946688461000000, 996688461000000},
				ValueB: []bool{true, false},
				ValueS: []string{"hello", "world"},
				ValueF: []float64{1.0, 7.0, 946688461000000.0, 996688461000000.0},
				ValueBS: [][]byte{
					[]byte("allo"),
					[]byte("hello"),
					[]byte("world"),
					[]byte("zurple"),
				},
				ValueK: []*ds.Key{
					ds.NewKey(ctx, "Something", "Cool", 0, nil),
					ds.NewKey(ctx, "Something", "", 1, nil),
					ds.NewKey(ctx, "Something", "Recursive", 0,
						ds.NewKey(ctx, "Parent", "", 2, nil)),
				},
				ValueBK: []blobstore.Key{"bellow", "hello"},
				ValueGP: []ds.GeoPoint{
					{Lat: 120.7, Lng: 95.5},
				},
				ValueSingle:      "ohai",
				ValueSingleSlice: []string{"kthxbye"},
			}
			assert.Loosely(t, ds.Put(ctx, &orig), should.BeNil)

			ret := TestStruct{ID: orig.ID}
			assert.Loosely(t, ds.Get(ctx, &ret), should.BeNil)
			assert.Loosely(t, ret, should.Match(orig))

			// make sure single- and multi- properties are preserved.
			pmap := ds.PropertyMap{
				"$id":   mpNI(orig.ID),
				"$kind": mpNI("TestStruct"),
			}
			assert.Loosely(t, ds.Get(ctx, pmap), should.BeNil)
			assert.Loosely(t, pmap["ValueSingle"], convey.Adapt(ShouldHaveSameTypeAs)(ds.Property{}))
			assert.Loosely(t, pmap["ValueSingleSlice"], convey.Adapt(ShouldHaveSameTypeAs)(ds.PropertySlice(nil)))

			// can't be sure the indexes have caught up... so sleep
			time.Sleep(time.Second)

			t.Run("Can query", func(t *ftt.Test) {
				q := ds.NewQuery("TestStruct")
				ds.Run(ctx, q, func(ts *TestStruct) {
					assert.Loosely(t, *ts, should.Match(orig))
				})
				count, err := ds.Count(ctx, q)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.Equal(1))
			})

			t.Run("Can query for bytes", func(t *ftt.Test) {
				q := ds.NewQuery("TestStruct").Eq("ValueBS", []byte("allo"))
				ds.Run(ctx, q, func(ts *TestStruct) {
					assert.Loosely(t, *ts, should.Match(orig))
				})
				count, err := ds.Count(ctx, q)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.Equal(1))
			})

			t.Run("Can project", func(t *ftt.Test) {
				q := ds.NewQuery("TestStruct").Project("ValueS")
				rslts := []ds.PropertyMap{}
				assert.Loosely(t, ds.GetAll(ctx, q, &rslts), should.BeNil)
				assert.Loosely(t, rslts, should.Match([]ds.PropertyMap{
					{
						"$key":   mpNI(ds.KeyForObj(ctx, &orig)),
						"ValueS": mp("hello"),
					},
					{
						"$key":   mpNI(ds.KeyForObj(ctx, &orig)),
						"ValueS": mp("world"),
					},
				}))

				q = ds.NewQuery("TestStruct").Project("ValueBS")
				rslts = []ds.PropertyMap{}
				assert.Loosely(t, ds.GetAll(ctx, q, &rslts), should.BeNil)
				assert.Loosely(t, rslts, should.Match([]ds.PropertyMap{
					{
						"$key":    mpNI(ds.KeyForObj(ctx, &orig)),
						"ValueBS": mp("allo"),
					},
					{
						"$key":    mpNI(ds.KeyForObj(ctx, &orig)),
						"ValueBS": mp("hello"),
					},
					{
						"$key":    mpNI(ds.KeyForObj(ctx, &orig)),
						"ValueBS": mp("world"),
					},
					{
						"$key":    mpNI(ds.KeyForObj(ctx, &orig)),
						"ValueBS": mp("zurple"),
					},
				}))

				count, err := ds.Count(ctx, q)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.Equal(4))

				q = ds.NewQuery("TestStruct").Lte("ValueI", 7).Project("ValueS").Distinct(true)
				rslts = []ds.PropertyMap{}
				assert.Loosely(t, ds.GetAll(ctx, q, &rslts), should.BeNil)
				assert.Loosely(t, rslts, should.Match([]ds.PropertyMap{
					{
						"$key":   mpNI(ds.KeyForObj(ctx, &orig)),
						"ValueI": mp(1),
						"ValueS": mp("hello"),
					},
					{
						"$key":   mpNI(ds.KeyForObj(ctx, &orig)),
						"ValueI": mp(1),
						"ValueS": mp("world"),
					},
					{
						"$key":   mpNI(ds.KeyForObj(ctx, &orig)),
						"ValueI": mp(7),
						"ValueS": mp("hello"),
					},
					{
						"$key":   mpNI(ds.KeyForObj(ctx, &orig)),
						"ValueI": mp(7),
						"ValueS": mp("world"),
					},
				}))

				count, err = ds.Count(ctx, q)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.Equal(4))
			})
		})

		t.Run("Can Put/Get (time)", func(t *ftt.Test) {
			// time comparisons in Go are wonky, so this is pulled out
			pm := ds.PropertyMap{
				"$key": mpNI(ds.NewKey(ctx, "Something", "value", 0, nil)),
				"Time": ds.PropertySlice{
					mp(time.Date(1938, time.January, 1, 1, 1, 1, 1, time.UTC)),
					mp(time.Time{}),
				},
			}
			assert.Loosely(t, ds.Put(ctx, &pm), should.BeNil)

			rslt := ds.PropertyMap{}
			rslt.SetMeta("key", ds.KeyForObj(ctx, pm))
			assert.Loosely(t, ds.Get(ctx, &rslt), should.BeNil)

			assert.Loosely(t, pm.Slice("Time")[0].Value(), should.Match(rslt.Slice("Time")[0].Value()))

			q := ds.NewQuery("Something").Project("Time")
			all := []ds.PropertyMap{}
			assert.Loosely(t, ds.GetAll(ctx, q, &all), should.BeNil)
			assert.Loosely(t, len(all), should.Equal(2))
			prop := all[0].Slice("Time")[0]
			assert.Loosely(t, prop.Type(), should.Equal(ds.PTInt))

			tval, err := prop.Project(ds.PTTime)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tval, should.Match(time.Time{}.UTC()))

			tval, err = all[1].Slice("Time")[0].Project(ds.PTTime)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, tval, should.Match(pm.Slice("Time")[0].Value()))

			ent := ds.PropertyMap{
				"$key": mpNI(ds.MakeKey(ctx, "Something", "value")),
			}
			assert.Loosely(t, ds.Get(ctx, &ent), should.BeNil)
			assert.Loosely(t, ent["Time"], should.Match(pm["Time"]))
		})

		t.Run(`Can Get empty []byte slice as nil`, func(t *ftt.Test) {
			put := ds.PropertyMap{
				"$id":   mpNI("foo"),
				"$kind": mpNI("FooType"),
				"Empty": mp([]byte(nil)),
				"Nilly": mp([]byte{}),
			}
			get := ds.PropertyMap{
				"$id":   put["$id"],
				"$kind": put["$kind"],
			}
			exp := put.Clone()
			exp["Nilly"] = mp([]byte(nil))

			assert.Loosely(t, ds.Put(ctx, put), should.BeNil)
			assert.Loosely(t, ds.Get(ctx, get), should.BeNil)
			assert.Loosely(t, get, should.Match(exp))
		})

		t.Run("memcache: Set (nil) is the same as Set ([]byte{})", func(t *ftt.Test) {
			assert.Loosely(t, mc.Set(ctx, mc.NewItem(ctx, "bob")), should.BeNil) // normally would panic because Value is nil

			bob, err := mc.GetKey(ctx, "bob")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, bob.Value(), should.Match([]byte{}))
		})
	})
}

func BenchmarkTransactionsParallel(b *testing.B) {
	type Counter struct {
		ID    int `gae:"$id"`
		Count int
	}

	inst, err := aetest.NewInstance(&aetest.Options{
		StronglyConsistentDatastore: true,
	})
	if err != nil {
		b.Fatalf("failed to initialize aetest: %v", err)
	}
	defer inst.Close()

	req, err := inst.NewRequest("GET", "/", nil)
	if err != nil {
		b.Fatalf("failed to create GET request: %v", err)
	}

	ctx := Use(context.Background(), req)

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			ctr := Counter{ID: 1}

			err := ds.RunInTransaction(ctx, func(ctx context.Context) error {
				switch err := ds.Get(ctx, &ctr); err {
				case nil, ds.ErrNoSuchEntity:
					ctr.Count++
					return ds.Put(ctx, &ctr)

				default:
					return err
				}
			}, &ds.TransactionOptions{Attempts: 9999999})
			if err != nil {
				b.Fatalf("failed to run transaction: %v", err)
			}
		}
	})
}
