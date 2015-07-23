// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package rawdatastore

import (
	"bytes"
	"io"
	"testing"
	"time"

	"github.com/luci/gae/service/blobstore"
	"github.com/luci/luci-go/common/cmpbin"
	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	WritePropertyMapDeterministic = true
}

type dspmapTC struct {
	name  string
	props PropertyMap
}

func TestPropertyMapSerialization(t *testing.T) {
	t.Parallel()

	tests := []dspmapTC{
		{
			"basic",
			PropertyMap{
				"R": {mp(false), mp(2.1), mpNI(3)},
				"S": {mp("hello"), mp("world")},
			},
		},
		{
			"keys",
			PropertyMap{
				"DS":        {mp(mkKey("appy", "ns", "Foo", 7)), mp(mkKey("other", "", "Yot", "wheeep"))},
				"blobstore": {mp(blobstore.Key("sup")), mp(blobstore.Key("nerds"))},
			},
		},
		{
			"geo",
			PropertyMap{
				"G": {mp(GeoPoint{Lat: 1, Lng: 2})},
			},
		},
		{
			"data",
			PropertyMap{
				"S":          {mp("sup"), mp("fool"), mp("nerd")},
				"D.Foo.Nerd": {mp([]byte("sup")), mp([]byte("fool"))},
				"B":          {mp(ByteString("sup")), mp(ByteString("fool"))},
			},
		},
		{
			"time",
			PropertyMap{
				"T": {
					mp(time.Now().UTC()),
					mp(time.Now().Add(time.Second).UTC())},
			},
		},
		{
			"empty vals",
			PropertyMap{
				"T": {mp(true), mp(true)},
				"F": {mp(false), mp(false)},
				"N": {mp(nil), mp(nil)},
				"E": {},
			},
		},
	}

	Convey("PropertyMap serialization", t, func() {
		Convey("round trip", func() {
			for _, tc := range tests {
				tc := tc
				Convey(tc.name, func() {
					buf := &bytes.Buffer{}
					WritePropertyMap(buf, tc.props, WithContext)
					dec, err := ReadPropertyMap(buf, WithContext, "", "")
					So(err, ShouldBeNil)
					So(dec, ShouldResemble, tc.props)
				})
			}
		})
	})
}

func TestSerializationReadMisc(t *testing.T) {
	t.Parallel()

	Convey("Misc Serialization tests", t, func() {
		buf := &bytes.Buffer{}
		Convey("ReadKey", func() {
			Convey("good cases", func() {
				Convey("w/ ctx decodes normally w/ ctx", func() {
					k := mkKey("aid", "ns", "knd", "yo", "other", 10)
					WriteKey(buf, WithContext, k)
					dk, err := ReadKey(buf, WithContext, "", "")
					So(err, ShouldBeNil)
					So(dk, ShouldEqualKey, k)
				})
				Convey("w/ ctx decodes normally w/o ctx", func() {
					k := mkKey("aid", "ns", "knd", "yo", "other", 10)
					WriteKey(buf, WithContext, k)
					dk, err := ReadKey(buf, WithoutContext, "spam", "nerd")
					So(err, ShouldBeNil)
					So(dk, ShouldEqualKey, mkKey("spam", "nerd", "knd", "yo", "other", 10))
				})
				Convey("w/o ctx decodes normally w/ ctx", func() {
					k := mkKey("aid", "ns", "knd", "yo", "other", 10)
					WriteKey(buf, WithoutContext, k)
					dk, err := ReadKey(buf, WithContext, "spam", "nerd")
					So(err, ShouldBeNil)
					So(dk, ShouldEqualKey, mkKey("", "", "knd", "yo", "other", 10))
				})
				Convey("w/o ctx decodes normally w/o ctx", func() {
					k := mkKey("aid", "ns", "knd", "yo", "other", 10)
					WriteKey(buf, WithoutContext, k)
					dk, err := ReadKey(buf, WithoutContext, "spam", "nerd")
					So(err, ShouldBeNil)
					So(dk, ShouldEqualKey, mkKey("spam", "nerd", "knd", "yo", "other", 10))
				})
				Convey("IntIDs always sort before StringIDs", func() {
					// -1 writes as almost all 1's in the first byte under cmpbin, even
					// though it's technically not a valid key.
					k := mkKey("aid", "ns", "knd", -1)
					WriteKey(buf, WithoutContext, k)

					k = mkKey("aid", "ns", "knd", "hat")
					buf2 := &bytes.Buffer{}
					WriteKey(buf2, WithoutContext, k)

					So(bytes.Compare(buf.Bytes(), buf2.Bytes()), ShouldBeLessThan, 0)
				})
			})

			Convey("err cases", func() {
				Convey("nil", func() {
					_, err := ReadKey(buf, WithContext, "", "")
					So(err, ShouldEqual, io.EOF)
				})
				Convey("str", func() {
					buf.WriteString("sup")
					_, err := ReadKey(buf, WithContext, "", "")
					So(err, ShouldErrLike, "expected actualCtx")
				})
				Convey("truncated 1", func() {
					buf.WriteByte(1) // actualCtx == 1
					_, err := ReadKey(buf, WithContext, "", "")
					So(err, ShouldEqual, io.EOF)
				})
				Convey("truncated 2", func() {
					buf.WriteByte(1) // actualCtx == 1
					cmpbin.WriteString(buf, "aid")
					_, err := ReadKey(buf, WithContext, "", "")
					So(err, ShouldEqual, io.EOF)
				})
				Convey("truncated 3", func() {
					buf.WriteByte(1) // actualCtx == 1
					cmpbin.WriteString(buf, "aid")
					cmpbin.WriteString(buf, "ns")
					_, err := ReadKey(buf, WithContext, "", "")
					So(err, ShouldEqual, io.EOF)
				})
				Convey("huge key", func() {
					buf.WriteByte(1) // actualCtx == 1
					cmpbin.WriteString(buf, "aid")
					cmpbin.WriteString(buf, "ns")
					cmpbin.WriteUint(buf, 1000)
					_, err := ReadKey(buf, WithContext, "", "")
					So(err, ShouldErrLike, "huge key")
				})
				Convey("insufficient tokens", func() {
					buf.WriteByte(1) // actualCtx == 1
					cmpbin.WriteString(buf, "aid")
					cmpbin.WriteString(buf, "ns")
					cmpbin.WriteUint(buf, 2)
					_, err := ReadKey(buf, WithContext, "", "")
					So(err, ShouldEqual, io.EOF)
				})
				Convey("partial token 1", func() {
					buf.WriteByte(1) // actualCtx == 1
					cmpbin.WriteString(buf, "aid")
					cmpbin.WriteString(buf, "ns")
					cmpbin.WriteUint(buf, 2)
					cmpbin.WriteString(buf, "hi")
					_, err := ReadKey(buf, WithContext, "", "")
					So(err, ShouldEqual, io.EOF)
				})
				Convey("partial token 2", func() {
					buf.WriteByte(1) // actualCtx == 1
					cmpbin.WriteString(buf, "aid")
					cmpbin.WriteString(buf, "ns")
					cmpbin.WriteUint(buf, 2)
					cmpbin.WriteString(buf, "hi")
					buf.WriteByte(byte(PTString))
					_, err := ReadKey(buf, WithContext, "", "")
					So(err, ShouldEqual, io.EOF)
				})
				Convey("bad token (invalid type)", func() {
					buf.WriteByte(1) // actualCtx == 1
					cmpbin.WriteString(buf, "aid")
					cmpbin.WriteString(buf, "ns")
					cmpbin.WriteUint(buf, 2)
					cmpbin.WriteString(buf, "hi")
					buf.WriteByte(byte(PTBlobKey))
					_, err := ReadKey(buf, WithContext, "", "")
					So(err, ShouldErrLike, "invalid type PTBlobKey")
				})
				Convey("bad token (invalid IntID)", func() {
					buf.WriteByte(1) // actualCtx == 1
					cmpbin.WriteString(buf, "aid")
					cmpbin.WriteString(buf, "ns")
					cmpbin.WriteUint(buf, 2)
					cmpbin.WriteString(buf, "hi")
					buf.WriteByte(byte(PTInt))
					cmpbin.WriteInt(buf, -2)
					_, err := ReadKey(buf, WithContext, "", "")
					So(err, ShouldErrLike, "zero/negative")
				})
			})
		})

		Convey("ReadGeoPoint", func() {
			Convey("trunc 1", func() {
				_, err := ReadGeoPoint(buf)
				So(err, ShouldEqual, io.EOF)
			})
			Convey("trunc 2", func() {
				cmpbin.WriteFloat64(buf, 100)
				_, err := ReadGeoPoint(buf)
				So(err, ShouldEqual, io.EOF)
			})
			Convey("invalid", func() {
				cmpbin.WriteFloat64(buf, 100)
				cmpbin.WriteFloat64(buf, 1000)
				_, err := ReadGeoPoint(buf)
				So(err, ShouldErrLike, "invalid GeoPoint")
			})
		})

		Convey("WriteTime", func() {
			Convey("in non-UTC!", func() {
				pst, err := time.LoadLocation("America/Los_Angeles")
				So(err, ShouldBeNil)
				So(func() {
					WriteTime(buf, time.Now().In(pst))
				}, ShouldPanic)
			})
		})

		Convey("ReadTime", func() {
			Convey("trunc 1", func() {
				_, err := ReadTime(buf)
				So(err, ShouldEqual, io.EOF)
			})
		})

		Convey("ReadProperty", func() {
			Convey("trunc 1", func() {
				_, err := ReadProperty(buf, WithContext, "", "")
				So(err, ShouldEqual, io.EOF)
			})
			Convey("trunc (PTBytes)", func() {
				buf.WriteByte(byte(PTBytes))
				_, err := ReadProperty(buf, WithContext, "", "")
				So(err, ShouldEqual, io.EOF)
			})
			Convey("trunc (PTBlobKey)", func() {
				buf.WriteByte(byte(PTBlobKey))
				_, err := ReadProperty(buf, WithContext, "", "")
				So(err, ShouldEqual, io.EOF)
			})
			Convey("invalid type", func() {
				buf.WriteByte(byte(PTUnknown + 1))
				_, err := ReadProperty(buf, WithContext, "", "")
				So(err, ShouldErrLike, "unknown type!")
			})
		})

		Convey("ReadPropertyMap", func() {
			Convey("trunc 1", func() {
				_, err := ReadPropertyMap(buf, WithContext, "", "")
				So(err, ShouldEqual, io.EOF)
			})
			Convey("too many rows", func() {
				cmpbin.WriteUint(buf, 1000000)
				_, err := ReadPropertyMap(buf, WithContext, "", "")
				So(err, ShouldErrLike, "huge number of rows")
			})
			Convey("trunc 2", func() {
				cmpbin.WriteUint(buf, 10)
				_, err := ReadPropertyMap(buf, WithContext, "", "")
				So(err, ShouldEqual, io.EOF)
			})
			Convey("trunc 3", func() {
				cmpbin.WriteUint(buf, 10)
				cmpbin.WriteString(buf, "ohai")
				_, err := ReadPropertyMap(buf, WithContext, "", "")
				So(err, ShouldEqual, io.EOF)
			})
			Convey("too many values", func() {
				cmpbin.WriteUint(buf, 10)
				cmpbin.WriteString(buf, "ohai")
				cmpbin.WriteUint(buf, 100000)
				_, err := ReadPropertyMap(buf, WithContext, "", "")
				So(err, ShouldErrLike, "huge number of properties")
			})
			Convey("trunc 4", func() {
				cmpbin.WriteUint(buf, 10)
				cmpbin.WriteString(buf, "ohai")
				cmpbin.WriteUint(buf, 10)
				_, err := ReadPropertyMap(buf, WithContext, "", "")
				So(err, ShouldEqual, io.EOF)
			})
		})
	})
}
