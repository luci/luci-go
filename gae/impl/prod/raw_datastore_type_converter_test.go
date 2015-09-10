// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build appengine

package prod

import (
	"testing"
	"time"

	"github.com/luci/gae/service/blobstore"
	dstore "github.com/luci/gae/service/datastore"
	. "github.com/smartystreets/goconvey/convey"
	"google.golang.org/appengine/aetest"
)

var (
	mp   = dstore.MkProperty
	mpNI = dstore.MkPropertyNI
)

type TestStruct struct {
	ID int64 `gae:"$id"`

	ValueI  []int64
	ValueB  []bool
	ValueS  []string
	ValueF  []float64
	ValueBS [][]byte // "ByteString"
	ValueK  []dstore.Key
	ValueBK []blobstore.Key
	ValueGP []dstore.GeoPoint
}

func TestBasicDatastore(t *testing.T) {
	t.Parallel()

	Convey("basic", t, func() {
		ctx, closer, err := aetest.NewContext()
		So(err, ShouldBeNil)
		defer closer()

		ctx = Use(ctx)
		ds := dstore.Get(ctx)

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
				ValueK: []dstore.Key{
					ds.NewKey("Something", "Cool", 0, nil),
					ds.NewKey("Something", "", 1, nil),
					ds.NewKey("Something", "Recursive", 0,
						ds.NewKey("Parent", "", 2, nil)),
				},
				ValueBK: []blobstore.Key{"bellow", "hello"},
				ValueGP: []dstore.GeoPoint{
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
				ds.Run(ds.NewQuery("TestStruct"), func(ts *TestStruct, _ dstore.CursorCB) bool {
					So(*ts, ShouldResemble, orig)
					return true
				})
			})

			Convey("Can project", func() {
				q := ds.NewQuery("TestStruct").Project("ValueS")
				rslts := []dstore.PropertyMap{}
				So(ds.GetAll(q, &rslts), ShouldBeNil)
				So(rslts, ShouldResemble, []dstore.PropertyMap{
					{
						"$key":   {mpNI(ds.KeyForObj(&orig))},
						"ValueS": {mp("hello")},
					},
					{
						"$key":   {mpNI(ds.KeyForObj(&orig))},
						"ValueS": {mp("world")},
					},
				})

				q = ds.NewQuery("TestStruct").Project("ValueBS")
				rslts = []dstore.PropertyMap{}
				So(ds.GetAll(q, &rslts), ShouldBeNil)
				So(rslts, ShouldResemble, []dstore.PropertyMap{
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
			})
		})

		Convey("Can Put/Get (time)", func() {
			// time comparisons in Go are wonky, so this is pulled out
			pm := dstore.PropertyMap{
				"$key": {mpNI(ds.NewKey("Something", "value", 0, nil))},
				"Time": {mp(time.Date(1938, time.January, 1, 1, 1, 1, 1, time.UTC))},
			}
			So(ds.Put(&pm), ShouldBeNil)

			rslt := dstore.PropertyMap{}
			rslt.SetMeta("key", ds.KeyForObj(pm))
			So(ds.Get(&rslt), ShouldBeNil)

			So(pm["Time"][0].Value(), ShouldResemble, rslt["Time"][0].Value())

			q := ds.NewQuery("Something").Project("Time")
			all := []dstore.PropertyMap{}
			So(ds.GetAll(q, &all), ShouldBeNil)
			So(len(all), ShouldEqual, 1)
			prop := all[0]["Time"][0]
			So(prop.Type(), ShouldEqual, dstore.PTInt)

			tval, err := prop.Project(dstore.PTTime)
			So(err, ShouldBeNil)
			So(tval, ShouldResemble, pm["Time"][0].Value())
		})
	})
}
