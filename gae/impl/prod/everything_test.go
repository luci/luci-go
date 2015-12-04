// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build appengine

package prod

import (
	"testing"
	"time"

	"github.com/luci/gae/service/blobstore"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/logging"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
	"google.golang.org/appengine/aetest"
)

var (
	mp   = datastore.MkProperty
	mpNI = datastore.MkPropertyNI
)

type TestStruct struct {
	ID int64 `gae:"$id"`

	ValueI  []int64
	ValueB  []bool
	ValueS  []string
	ValueF  []float64
	ValueBS [][]byte // "ByteString"
	ValueK  []*datastore.Key
	ValueBK []blobstore.Key
	ValueGP []datastore.GeoPoint
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
		ds := datastore.Get(ctx)
		inf := info.Get(ctx)

		// You have to visually confirm that this actually happens in the stdout
		// of the test... yeah I know.
		logging.Infof(ctx, "I am a banana")

		Convey("Can probe/change Namespace", func() {
			So(inf.GetNamespace(), ShouldEqual, "")
			ctx, err = inf.Namespace("wat")
			So(err, ShouldBeNil)
			inf = info.Get(ctx)
			So(inf.GetNamespace(), ShouldEqual, "wat")
			ds = datastore.Get(ctx)
			So(ds.MakeKey("Hello", "world").Namespace(), ShouldEqual, "wat")
		})

		Convey("Can get non-transactional context", func() {
			ctx, err := inf.Namespace("foo")
			So(err, ShouldBeNil)
			ds = datastore.Get(ctx)
			inf = info.Get(ctx)

			ds.RunInTransaction(func(ctx context.Context) error {
				So(ds.MakeKey("Foo", "bar").Namespace(), ShouldEqual, "foo")

				So(ds.Put(&TestStruct{ValueI: []int64{100}}), ShouldBeNil)

				err = datastore.GetNoTxn(ctx).RunInTransaction(func(ctx context.Context) error {
					ds = datastore.Get(ctx)
					So(ds.MakeKey("Foo", "bar").Namespace(), ShouldEqual, "foo")
					So(ds.Put(&TestStruct{ValueI: []int64{100}}), ShouldBeNil)
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
				ValueK: []*datastore.Key{
					ds.NewKey("Something", "Cool", 0, nil),
					ds.NewKey("Something", "", 1, nil),
					ds.NewKey("Something", "Recursive", 0,
						ds.NewKey("Parent", "", 2, nil)),
				},
				ValueBK: []blobstore.Key{"bellow", "hello"},
				ValueGP: []datastore.GeoPoint{
					{Lat: 120.7, Lng: 95.5},
				},
			}
			So(ds.Put(&orig), ShouldBeNil)

			ret := TestStruct{ID: orig.ID}
			So(ds.Get(&ret), ShouldBeNil)
			So(ret, ShouldResemble, orig)

			// can't be sure the indexes have caught up... so sleep
			time.Sleep(time.Second)

			Convey("Can query", func() {
				q := datastore.NewQuery("TestStruct")
				ds.Run(q, func(ts *TestStruct, _ datastore.CursorCB) bool {
					So(*ts, ShouldResemble, orig)
					return true
				})
				count, err := ds.Count(q)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 1)
			})

			Convey("Can project", func() {
				q := datastore.NewQuery("TestStruct").Project("ValueS")
				rslts := []datastore.PropertyMap{}
				So(ds.GetAll(q, &rslts), ShouldBeNil)
				So(rslts, ShouldResemble, []datastore.PropertyMap{
					{
						"$key":   {mpNI(ds.KeyForObj(&orig))},
						"ValueS": {mp("hello")},
					},
					{
						"$key":   {mpNI(ds.KeyForObj(&orig))},
						"ValueS": {mp("world")},
					},
				})

				q = datastore.NewQuery("TestStruct").Project("ValueBS")
				rslts = []datastore.PropertyMap{}
				So(ds.GetAll(q, &rslts), ShouldBeNil)
				So(rslts, ShouldResemble, []datastore.PropertyMap{
					{
						"$key":    {mpNI(ds.KeyForObj(&orig))},
						"ValueBS": {mp("allo")},
					},
					{
						"$key":    {mpNI(ds.KeyForObj(&orig))},
						"ValueBS": {mp("hello")},
					},
					{
						"$key":    {mpNI(ds.KeyForObj(&orig))},
						"ValueBS": {mp("world")},
					},
					{
						"$key":    {mpNI(ds.KeyForObj(&orig))},
						"ValueBS": {mp("zurple")},
					},
				})

				count, err := ds.Count(q)
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 4)
			})
		})

		Convey("Can Put/Get (time)", func() {
			// time comparisons in Go are wonky, so this is pulled out
			pm := datastore.PropertyMap{
				"$key": {mpNI(ds.NewKey("Something", "value", 0, nil))},
				"Time": {
					mp(time.Date(1938, time.January, 1, 1, 1, 1, 1, time.UTC)),
					mp(time.Time{}),
				},
			}
			So(ds.Put(&pm), ShouldBeNil)

			rslt := datastore.PropertyMap{}
			rslt.SetMeta("key", ds.KeyForObj(pm))
			So(ds.Get(&rslt), ShouldBeNil)

			So(pm["Time"][0].Value(), ShouldResemble, rslt["Time"][0].Value())

			q := datastore.NewQuery("Something").Project("Time")
			all := []datastore.PropertyMap{}
			So(ds.GetAll(q, &all), ShouldBeNil)
			So(len(all), ShouldEqual, 2)
			prop := all[0]["Time"][0]
			So(prop.Type(), ShouldEqual, datastore.PTInt)

			tval, err := prop.Project(datastore.PTTime)
			So(err, ShouldBeNil)
			So(tval, ShouldResemble, time.Time{})

			tval, err = all[1]["Time"][0].Project(datastore.PTTime)
			So(err, ShouldBeNil)
			So(tval, ShouldResemble, pm["Time"][0].Value())

			ent := datastore.PropertyMap{
				"$key": {mpNI(ds.MakeKey("Something", "value"))},
			}
			So(ds.Get(&ent), ShouldBeNil)
			So(ent["Time"], ShouldResemble, pm["Time"])
		})
	})
}
