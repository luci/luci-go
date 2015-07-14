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
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "DSPTNull")
			})
			Convey("set", func() {
				pv := MkDSPropertyNI(100)
				So(pv.Value(), ShouldHaveSameTypeAs, int64(100))
				So(pv.Value(), ShouldEqual, 100)
				So(pv.IndexSetting(), ShouldEqual, NoIndex)
				So(pv.Type().String(), ShouldEqual, "DSPTInt")

				pv.SetValue(nil, ShouldIndex)
				So(pv.Value(), ShouldBeNil)
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "DSPTNull")
			})
			Convey("derived types", func() {
				Convey("int", func() {
					pv := MkDSProperty(19)
					So(pv.Value(), ShouldHaveSameTypeAs, int64(19))
					So(pv.Value(), ShouldEqual, 19)
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "DSPTInt")
				})
				Convey("bool (true)", func() {
					pv := MkDSProperty(mybool(true))
					So(pv.Value(), ShouldBeTrue)
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "DSPTBoolTrue")
				})
				Convey("string", func() {
					pv := MkDSProperty(mystring("sup"))
					So(pv.Value(), ShouldEqual, "sup")
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "DSPTString")
				})
				Convey("BSKey is distinquished", func() {
					pv := MkDSProperty(BSKey("sup"))
					So(pv.Value(), ShouldEqual, BSKey("sup"))
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "DSPTBlobKey")
				})
				Convey("float", func() {
					pv := DSProperty{}
					pv.SetValue(myfloat(19.7), ShouldIndex)
					So(pv.Value(), ShouldHaveSameTypeAs, float64(19.7))
					So(pv.Value(), ShouldEqual, float32(19.7))
					So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
					So(pv.Type().String(), ShouldEqual, "DSPTFloat")
				})
			})
			Convey("bad type", func() {
				pv := DSProperty{}
				err := pv.SetValue(complex(100, 29), ShouldIndex)
				So(err.Error(), ShouldContainSubstring, "has bad type complex")
				So(pv.Value(), ShouldBeNil)
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "DSPTNull")
			})
			Convey("invalid GeoPoint", func() {
				pv := DSProperty{}
				err := pv.SetValue(DSGeoPoint{-1000, 0}, ShouldIndex)
				So(err.Error(), ShouldContainSubstring, "invalid GeoPoint value")
				So(pv.Value(), ShouldBeNil)
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "DSPTNull")
			})
			Convey("invalid time", func() {
				pv := DSProperty{}
				err := pv.SetValue(time.Now(), ShouldIndex)
				So(err.Error(), ShouldContainSubstring, "time value has wrong Location")

				err = pv.SetValue(time.Unix(math.MaxInt64, 0).UTC(), ShouldIndex)
				So(err.Error(), ShouldContainSubstring, "time value out of range")
				So(pv.Value(), ShouldBeNil)
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "DSPTNull")
			})
			Convey("time gets rounded", func() {
				pv := DSProperty{}
				now := time.Now().In(time.UTC)
				now = now.Round(time.Microsecond).Add(time.Nanosecond * 313)
				pv.SetValue(now, ShouldIndex)
				So(pv.Value(), ShouldHappenBefore, now)
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "DSPTTime")
			})
			Convey("[]byte coerces IndexSetting", func() {
				pv := DSProperty{}
				pv.SetValue([]byte("hello"), ShouldIndex)
				So(pv.Value(), ShouldResemble, []byte("hello"))
				So(pv.IndexSetting(), ShouldEqual, NoIndex)
				So(pv.Type().String(), ShouldEqual, "DSPTBytes")
			})
			Convey("ByteString allows !IndexSetting", func() {
				pv := DSProperty{}
				pv.SetValue(DSByteString("hello"), ShouldIndex)
				So(pv.Value(), ShouldResemble, DSByteString("hello"))
				So(pv.IndexSetting(), ShouldEqual, ShouldIndex)
				So(pv.Type().String(), ShouldEqual, "DSPTBytes")
			})
		})
	})
}

func TestDSPropertyMapImpl(t *testing.T) {
	t.Parallel()

	Convey("DSPropertyMap load/save err conditions", t, func() {
		Convey("empty", func() {
			pm := DSPropertyMap{}
			err := pm.Load(DSPropertyMap{"hello": {DSProperty{}}})
			So(err, ShouldBeNil)
			So(pm, ShouldResemble, DSPropertyMap{"hello": {DSProperty{}}})

			npm, _ := pm.Save(false)
			So(npm, ShouldResemble, pm)
		})
		Convey("meta", func() {
			Convey("working", func() {
				pm := DSPropertyMap{"": {MkDSProperty("trap!")}}
				_, err := pm.GetMeta("foo")
				So(err, ShouldEqual, ErrDSMetaFieldUnset)

				err = pm.SetMeta("foo", 100)
				So(err, ShouldBeNil)

				v, err := pm.GetMeta("foo")
				So(err, ShouldBeNil)
				So(v, ShouldEqual, 100)

				npm, err := pm.Save(false)
				So(err, ShouldBeNil)
				So(len(npm), ShouldEqual, 1)
			})

			Convey("errors", func() {
				Convey("too many values", func() {
					pm := DSPropertyMap{
						"$bad": {MkDSProperty(100), MkDSProperty(200)},
					}
					_, err := pm.GetMeta("bad")
					So(err.Error(), ShouldContainSubstring, "too many values")
				})

				Convey("weird value", func() {
					pm := DSPropertyMap{}
					err := pm.SetMeta("sup", complex(100, 20))
					So(err.Error(), ShouldContainSubstring, "bad type")
				})
			})
		})
	})
}
