// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"math"
	"testing"
	"time"

	"github.com/luci/gae/service/blobstore"
	. "github.com/smartystreets/goconvey/convey"
)

type myint int
type mybool bool
type mystring string
type myfloat float32

func TestProperties(t *testing.T) {
	t.Parallel()

	Convey("Test Property", t, func() {
		Convey("Construction", func() {
			Convey("empty", func() {
				pv := Property{}
				So(pv.Value(), ShouldBeNil)
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "PTNull")
			})
			Convey("set", func() {
				pv := MkPropertyNI(100)
				So(pv.Value(), ShouldHaveSameTypeAs, int64(100))
				So(pv.Value(), ShouldEqual, 100)
				So(pv.IndexSetting(), ShouldEqual, NoIndex)
				So(pv.Type().String(), ShouldEqual, "PTInt")

				So(pv.SetValue(nil, ShouldIndex), ShouldBeNil)
				So(pv.Value(), ShouldBeNil)
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "PTNull")
			})
			Convey("derived types", func() {
				Convey("int", func() {
					pv := MkProperty(19)
					So(pv.Value(), ShouldHaveSameTypeAs, int64(19))
					So(pv.Value(), ShouldEqual, 19)
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "PTInt")
				})
				Convey("bool (true)", func() {
					pv := MkProperty(mybool(true))
					So(pv.Value(), ShouldBeTrue)
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "PTBool")
				})
				Convey("string", func() {
					pv := MkProperty(mystring("sup"))
					So(pv.Value(), ShouldEqual, "sup")
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "PTString")
				})
				Convey("blobstore.Key is distinquished", func() {
					pv := MkProperty(blobstore.Key("sup"))
					So(pv.Value(), ShouldEqual, blobstore.Key("sup"))
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "PTBlobKey")
				})
				Convey("float", func() {
					pv := Property{}
					So(pv.SetValue(myfloat(19.7), ShouldIndex), ShouldBeNil)
					So(pv.Value(), ShouldHaveSameTypeAs, float64(19.7))
					So(pv.Value(), ShouldEqual, float32(19.7))
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "PTFloat")
				})
			})
			Convey("bad type", func() {
				pv := Property{}
				err := pv.SetValue(complex(100, 29), ShouldIndex)
				So(err.Error(), ShouldContainSubstring, "has bad type complex")
				So(pv.Value(), ShouldBeNil)
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "PTNull")
			})
			Convey("invalid GeoPoint", func() {
				pv := Property{}
				err := pv.SetValue(GeoPoint{-1000, 0}, ShouldIndex)
				So(err.Error(), ShouldContainSubstring, "invalid GeoPoint value")
				So(pv.Value(), ShouldBeNil)
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "PTNull")
			})
			Convey("invalid time", func() {
				pv := Property{}
				loc, err := time.LoadLocation("America/Los_Angeles")
				So(err, ShouldBeNil)
				t := time.Date(1970, 1, 1, 0, 0, 0, 0, loc)

				err = pv.SetValue(t, ShouldIndex)
				So(err.Error(), ShouldContainSubstring, "time value has wrong Location")

				err = pv.SetValue(time.Unix(math.MaxInt64, 0).UTC(), ShouldIndex)
				So(err.Error(), ShouldContainSubstring, "time value out of range")
				So(pv.Value(), ShouldBeNil)
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "PTNull")
			})
			Convey("time gets rounded", func() {
				pv := Property{}
				now := time.Now().In(time.UTC)
				now = now.Round(time.Microsecond).Add(time.Nanosecond * 313)
				So(pv.SetValue(now, ShouldIndex), ShouldBeNil)
				So(pv.Value(), ShouldHappenBefore, now)
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "PTTime")
			})
			Convey("[]byte allows IndexSetting", func() {
				pv := Property{}
				So(pv.SetValue([]byte("hello"), ShouldIndex), ShouldBeNil)
				So(pv.Value(), ShouldResemble, []byte("hello"))
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "PTBytes")
			})
		})
	})
}

func TestDSPropertyMapImpl(t *testing.T) {
	t.Parallel()

	Convey("PropertyMap load/save err conditions", t, func() {
		Convey("empty", func() {
			pm := PropertyMap{}
			err := pm.Load(PropertyMap{"hello": {Property{}}})
			So(err, ShouldBeNil)
			So(pm, ShouldResemble, PropertyMap{"hello": {Property{}}})

			npm, _ := pm.Save(false)
			So(npm, ShouldResemble, pm)
		})
		Convey("meta", func() {
			Convey("working", func() {
				pm := PropertyMap{"": {MkProperty("trap!")}}
				_, ok := pm.GetMeta("foo")
				So(ok, ShouldBeFalse)

				So(pm.SetMeta("foo", 100), ShouldBeTrue)

				v, ok := pm.GetMeta("foo")
				So(ok, ShouldBeTrue)
				So(v, ShouldEqual, 100)

				So(GetMetaDefault(pm, "foo", 100), ShouldEqual, 100)

				So(GetMetaDefault(pm, "bar", 100), ShouldEqual, 100)

				npm, err := pm.Save(false)
				So(err, ShouldBeNil)
				So(len(npm), ShouldEqual, 0)
			})

			Convey("too many values picks the first one", func() {
				pm := PropertyMap{
					"$thing": {MkProperty(100), MkProperty(200)},
				}
				v, ok := pm.GetMeta("thing")
				So(ok, ShouldBeTrue)
				So(v, ShouldEqual, 100)
			})

			Convey("errors", func() {

				Convey("weird value", func() {
					pm := PropertyMap{}
					So(pm.SetMeta("sup", complex(100, 20)), ShouldBeFalse)
				})
			})
		})
	})
}
