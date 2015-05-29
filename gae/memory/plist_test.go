// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"testing"
	"time"

	. "github.com/smartystreets/goconvey/convey"

	"appengine"
	"appengine/datastore"
)

func TestPlistBinaryCodec(t *testing.T) {
	t.Parallel()

	Convey("Plist binary codec", t, func() {
		Convey("basic functionality", func() {
			Convey("empty", func() {
				pl := (*propertyList)(nil)
				data, err := pl.MarshalBinary()
				So(err, ShouldBeNil)
				So(data, ShouldBeEmpty)
			})

			Convey("one item", func() {
				pl := &propertyList{datastore.Property{
					Name: "Bob", Value: 301.23,
				}}
				data, err := pl.MarshalBinary()
				So(err, ShouldBeNil)
				pl2 := &propertyList{}

				err = pl2.UnmarshalBinary(data)
				So(err, ShouldBeNil)
				So(pl, ShouldResemble, pl2)
			})

			Convey("one of each", func() {
				k := newKey("coolspace", "nerd", "wat", 0,
					newKey("coolspace", "child", "", 20, nil))

				pl := &propertyList{
					datastore.Property{Name: "null"},
					datastore.Property{Name: "int", Value: int64(100)},
					datastore.Property{Name: "time", Value: time.Now().Round(time.Microsecond)},
					datastore.Property{Name: "float", Value: float64(301.23)},
					datastore.Property{Name: "bool", Value: true, Multiple: true},
					datastore.Property{Name: "bool", Value: false, Multiple: true},
					datastore.Property{Name: "bool", Value: "mixed types are allowed!", Multiple: true},
					datastore.Property{Name: "bool", Value: true, Multiple: true},
					datastore.Property{Name: "[]byte", Value: []byte("sup"), NoIndex: true},
					datastore.Property{Name: "ByteString", Value: datastore.ByteString("sup")},
					datastore.Property{Name: "BlobKey", Value: appengine.BlobKey("bkey")},
					datastore.Property{Name: "string", Value: "stringy"},
					datastore.Property{Name: "GeoPoint", Value: appengine.GeoPoint{Lat: 123.3, Lng: 456.6}},
					datastore.Property{Name: "*Key", Value: k},
				}
				data, err := pl.MarshalBinary()
				So(err, ShouldBeNil)
				pl2 := &propertyList{}

				err = pl2.UnmarshalBinary(data)
				So(err, ShouldBeNil)
				So(len(*pl), ShouldEqual, len(*pl2))

				for i, v := range *pl {
					v2 := (*pl2)[i]
					So(v2.Name, ShouldEqual, v.Name)
					So(v2.Multiple, ShouldEqual, v.Multiple)
					So(v2.NoIndex, ShouldEqual, v.NoIndex)
					switch v.Name {
					case "*Key":
						So(v2.Value, shouldEqualKey, v.Value)
					default:
						So(v2.Value, ShouldResemble, v.Value)
					}
				}
			})

		})
	})
}
