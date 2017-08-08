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

// +build appengine

package prod

import (
	"testing"
	"time"

	"go.chromium.org/gae/service/blobstore"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"
	mc "go.chromium.org/gae/service/memcache"

	"go.chromium.org/luci/common/logging"

	"golang.org/x/net/context"
	"google.golang.org/appengine/aetest"

	. "github.com/smartystreets/goconvey/convey"
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

	Convey("basic", t, func() {
		inst, err := aetest.NewInstance(&aetest.Options{
			StronglyConsistentDatastore: true,
		})
		So(err, ShouldBeNil)
		defer inst.Close()

		req, err := inst.NewRequest("GET", "/", nil)
		So(err, ShouldBeNil)

		ctx := Use(context.Background(), req)

		Convey("logging allows you to tweak the level", func() {
			// You have to visually confirm that this actually happens in the stdout
			// of the test... yeah I know.
			logging.Debugf(ctx, "SHOULD NOT SEE")
			logging.Infof(ctx, "SHOULD SEE")

			ctx = logging.SetLevel(ctx, logging.Debug)
			logging.Debugf(ctx, "SHOULD SEE")
			logging.Infof(ctx, "SHOULD SEE (2)")
		})

		Convey("Can probe/change Namespace", func() {
			So(info.GetNamespace(ctx), ShouldEqual, "")

			ctx, err = info.Namespace(ctx, "wat")
			So(err, ShouldBeNil)

			So(info.GetNamespace(ctx), ShouldEqual, "wat")
			So(ds.MakeKey(ctx, "Hello", "world").Namespace(), ShouldEqual, "wat")
		})

		Convey("Can get non-transactional context", func() {
			ctx, err := info.Namespace(ctx, "foo")
			So(err, ShouldBeNil)

			So(ds.CurrentTransaction(ctx), ShouldBeNil)

			ds.RunInTransaction(ctx, func(ctx context.Context) error {
				So(ds.CurrentTransaction(ctx), ShouldNotBeNil)
				So(ds.MakeKey(ctx, "Foo", "bar").Namespace(), ShouldEqual, "foo")

				So(ds.Put(ctx, &TestStruct{ValueI: []int64{100}}), ShouldBeNil)

				noTxnCtx := ds.WithoutTransaction(ctx)
				So(ds.CurrentTransaction(noTxnCtx), ShouldBeNil)

				err = ds.RunInTransaction(noTxnCtx, func(ctx context.Context) error {
					So(ds.CurrentTransaction(ctx), ShouldNotBeNil)
					So(ds.MakeKey(ctx, "Foo", "bar").Namespace(), ShouldEqual, "foo")
					So(ds.Put(ctx, &TestStruct{ValueI: []int64{100}}), ShouldBeNil)
					return nil
				}, nil)
				So(err, ShouldBeNil)

				return nil
			}, nil)
		})

		Convey("Can Put/Get", func() {
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
			So(ds.Put(ctx, &orig), ShouldBeNil)

			ret := TestStruct{ID: orig.ID}
			So(ds.Get(ctx, &ret), ShouldBeNil)
			So(ret, ShouldResemble, orig)

			// make sure single- and multi- properties are preserved.
			pmap := ds.PropertyMap{
				"$id":   mpNI(orig.ID),
				"$kind": mpNI("TestStruct"),
			}
			So(ds.Get(ctx, pmap), ShouldBeNil)
			So(pmap["ValueSingle"], ShouldHaveSameTypeAs, ds.Property{})
			So(pmap["ValueSingleSlice"], ShouldHaveSameTypeAs, ds.PropertySlice(nil))

			// can't be sure the indexes have caught up... so sleep
			time.Sleep(time.Second)

			Convey("Can query", func() {
				q := ds.NewQuery("TestStruct")
				ds.Run(ctx, q, func(ts *TestStruct) {
					So(*ts, ShouldResemble, orig)
				})
				count, err := ds.Count(ctx, q)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 1)
			})

			Convey("Can query for bytes", func() {
				q := ds.NewQuery("TestStruct").Eq("ValueBS", []byte("allo"))
				ds.Run(ctx, q, func(ts *TestStruct) {
					So(*ts, ShouldResemble, orig)
				})
				count, err := ds.Count(ctx, q)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 1)
			})

			Convey("Can project", func() {
				q := ds.NewQuery("TestStruct").Project("ValueS")
				rslts := []ds.PropertyMap{}
				So(ds.GetAll(ctx, q, &rslts), ShouldBeNil)
				So(rslts, ShouldResemble, []ds.PropertyMap{
					{
						"$key":   mpNI(ds.KeyForObj(ctx, &orig)),
						"ValueS": mp("hello"),
					},
					{
						"$key":   mpNI(ds.KeyForObj(ctx, &orig)),
						"ValueS": mp("world"),
					},
				})

				q = ds.NewQuery("TestStruct").Project("ValueBS")
				rslts = []ds.PropertyMap{}
				So(ds.GetAll(ctx, q, &rslts), ShouldBeNil)
				So(rslts, ShouldResemble, []ds.PropertyMap{
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
				})

				count, err := ds.Count(ctx, q)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 4)

				q = ds.NewQuery("TestStruct").Lte("ValueI", 7).Project("ValueS").Distinct(true)
				rslts = []ds.PropertyMap{}
				So(ds.GetAll(ctx, q, &rslts), ShouldBeNil)
				So(rslts, ShouldResemble, []ds.PropertyMap{
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
				})

				count, err = ds.Count(ctx, q)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 4)
			})
		})

		Convey("Can Put/Get (time)", func() {
			// time comparisons in Go are wonky, so this is pulled out
			pm := ds.PropertyMap{
				"$key": mpNI(ds.NewKey(ctx, "Something", "value", 0, nil)),
				"Time": ds.PropertySlice{
					mp(time.Date(1938, time.January, 1, 1, 1, 1, 1, time.UTC)),
					mp(time.Time{}),
				},
			}
			So(ds.Put(ctx, &pm), ShouldBeNil)

			rslt := ds.PropertyMap{}
			rslt.SetMeta("key", ds.KeyForObj(ctx, pm))
			So(ds.Get(ctx, &rslt), ShouldBeNil)

			So(pm.Slice("Time")[0].Value(), ShouldResemble, rslt.Slice("Time")[0].Value())

			q := ds.NewQuery("Something").Project("Time")
			all := []ds.PropertyMap{}
			So(ds.GetAll(ctx, q, &all), ShouldBeNil)
			So(len(all), ShouldEqual, 2)
			prop := all[0].Slice("Time")[0]
			So(prop.Type(), ShouldEqual, ds.PTInt)

			tval, err := prop.Project(ds.PTTime)
			So(err, ShouldBeNil)
			So(tval, ShouldResemble, time.Time{}.UTC())

			tval, err = all[1].Slice("Time")[0].Project(ds.PTTime)
			So(err, ShouldBeNil)
			So(tval, ShouldResemble, pm.Slice("Time")[0].Value())

			ent := ds.PropertyMap{
				"$key": mpNI(ds.MakeKey(ctx, "Something", "value")),
			}
			So(ds.Get(ctx, &ent), ShouldBeNil)
			So(ent["Time"], ShouldResemble, pm["Time"])
		})

		Convey("memcache: Set (nil) is the same as Set ([]byte{})", func() {
			So(mc.Set(ctx, mc.NewItem(ctx, "bob")), ShouldBeNil) // normally would panic because Value is nil

			bob, err := mc.GetKey(ctx, "bob")
			So(err, ShouldBeNil)
			So(bob.Value(), ShouldResemble, []byte{})
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
