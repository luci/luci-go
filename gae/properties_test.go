// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gae

import (
	"math"
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"
)

type myint int
type mybool bool
type mystring string
type myfloat float32

func TestProperties(t *testing.T) {
	t.Parallel()

	Convey("Test DSProperty", t, func() {
		Convey("Construction", func() {
			Convey("empty", func() {
				pv := DSProperty{}
				So(pv.Value(), ShouldBeNil)
				So(pv.NoIndex(), ShouldBeFalse)
				So(pv.Type().String(), ShouldEqual, "DSPTNull")
			})
			Convey("set", func() {
				pv := DSProperty{}
				pv.SetValue(100, true)
				So(pv.Value(), ShouldHaveSameTypeAs, int64(100))
				So(pv.Value(), ShouldEqual, 100)
				So(pv.NoIndex(), ShouldBeTrue)
				So(pv.Type().String(), ShouldEqual, "DSPTInt")

				pv.SetValue(nil, false)
				So(pv.Value(), ShouldBeNil)
				So(pv.NoIndex(), ShouldBeFalse)
				So(pv.Type().String(), ShouldEqual, "DSPTNull")
			})
			Convey("derived types", func() {
				Convey("int", func() {
					pv := DSProperty{}
					pv.SetValue(myint(19), false)
					So(pv.Value(), ShouldHaveSameTypeAs, int64(19))
					So(pv.Value(), ShouldEqual, 19)
					So(pv.NoIndex(), ShouldBeFalse)
					So(pv.Type().String(), ShouldEqual, "DSPTInt")
				})
				Convey("bool (true)", func() {
					pv := DSProperty{}
					pv.SetValue(mybool(true), false)
					So(pv.Value(), ShouldBeTrue)
					So(pv.NoIndex(), ShouldBeFalse)
					So(pv.Type().String(), ShouldEqual, "DSPTBoolTrue")
				})
				Convey("string", func() {
					pv := DSProperty{}
					pv.SetValue(mystring("sup"), false)
					So(pv.Value(), ShouldEqual, "sup")
					So(pv.NoIndex(), ShouldBeFalse)
					So(pv.Type().String(), ShouldEqual, "DSPTString")
				})
				Convey("BSKey is distinquished", func() {
					pv := DSProperty{}
					pv.SetValue(BSKey("sup"), false)
					So(pv.Value(), ShouldEqual, BSKey("sup"))
					So(pv.NoIndex(), ShouldBeFalse)
					So(pv.Type().String(), ShouldEqual, "DSPTBlobKey")
				})
				Convey("float", func() {
					pv := DSProperty{}
					pv.SetValue(myfloat(19.7), false)
					So(pv.Value(), ShouldHaveSameTypeAs, float64(19.7))
					So(pv.Value(), ShouldEqual, float32(19.7))
					So(pv.NoIndex(), ShouldBeFalse)
					So(pv.Type().String(), ShouldEqual, "DSPTFloat")
				})
			})
			Convey("bad type", func() {
				pv := DSProperty{}
				err := pv.SetValue(complex(100, 29), false)
				So(err.Error(), ShouldContainSubstring, "has bad type complex")
				So(pv.Value(), ShouldBeNil)
				So(pv.NoIndex(), ShouldBeFalse)
				So(pv.Type().String(), ShouldEqual, "DSPTNull")
			})
			Convey("invalid GeoPoint", func() {
				pv := DSProperty{}
				err := pv.SetValue(DSGeoPoint{-1000, 0}, false)
				So(err.Error(), ShouldContainSubstring, "invalid GeoPoint value")
				So(pv.Value(), ShouldBeNil)
				So(pv.NoIndex(), ShouldBeFalse)
				So(pv.Type().String(), ShouldEqual, "DSPTNull")
			})
			Convey("invalid time", func() {
				pv := DSProperty{}
				err := pv.SetValue(time.Now(), false)
				So(err.Error(), ShouldContainSubstring, "time value has wrong Location")

				err = pv.SetValue(time.Unix(math.MaxInt64, 0).UTC(), false)
				So(err.Error(), ShouldContainSubstring, "time value out of range")
				So(pv.Value(), ShouldBeNil)
				So(pv.NoIndex(), ShouldBeFalse)
				So(pv.Type().String(), ShouldEqual, "DSPTNull")
			})
			Convey("time gets rounded", func() {
				pv := DSProperty{}
				now := time.Now().In(time.UTC)
				now = now.Round(time.Microsecond).Add(time.Nanosecond * 313)
				pv.SetValue(now, false)
				So(pv.Value(), ShouldHappenBefore, now)
				So(pv.NoIndex(), ShouldBeFalse)
				So(pv.Type().String(), ShouldEqual, "DSPTTime")
			})
			Convey("[]byte coerces NoIndex", func() {
				pv := DSProperty{}
				pv.SetValue([]byte("hello"), false)
				So(pv.Value(), ShouldResemble, []byte("hello"))
				So(pv.NoIndex(), ShouldBeTrue)
				So(pv.Type().String(), ShouldEqual, "DSPTBytes")
			})
			Convey("ByteString allows !NoIndex", func() {
				pv := DSProperty{}
				pv.SetValue(DSByteString("hello"), false)
				So(pv.Value(), ShouldResemble, DSByteString("hello"))
				So(pv.NoIndex(), ShouldBeFalse)
				So(pv.Type().String(), ShouldEqual, "DSPTBytes")
			})
		})
	})
}

func TestDSPropertyMapImpl(t *testing.T) {
	t.Parallel()

	Convey("DSPropertyMap load/save err conditions", t, func() {
		Convey("nil", func() {
			pm := (*DSPropertyMap)(nil)
			_, err := pm.Load(nil)
			So(err.Error(), ShouldContainSubstring, "nil DSPropertyMap")

			_, err = pm.Save()
			So(err.Error(), ShouldContainSubstring, "nil DSPropertyMap")
		})
		Convey("empty", func() {
			pm := DSPropertyMap{}
			_, err := pm.Load(DSPropertyMap{"hello": {DSProperty{}}})
			So(err, ShouldBeNil)
			So(pm, ShouldResemble, DSPropertyMap{"hello": {DSProperty{}}})

			// it can also self-initialize
			pm = DSPropertyMap(nil)
			_, err = pm.Load(DSPropertyMap{"hello": {DSProperty{}}})
			So(err, ShouldBeNil)
			So(pm, ShouldResemble, DSPropertyMap{"hello": {DSProperty{}}})

			npm, _ := pm.Save()
			So(npm, ShouldResemble, pm)
		})
	})
}
