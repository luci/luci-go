// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package helper

import (
	"bytes"
	"io"
	"testing"
	"time"

	"infra/gae/libs/gae"

	"github.com/luci/luci-go/common/cmpbin"
	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	WriteDSPropertyMapDeterministic = true
}

type dspmapTC struct {
	name  string
	props gae.DSPropertyMap
}

func TestDSPropertyMapSerialization(t *testing.T) {
	t.Parallel()

	tests := []dspmapTC{
		{
			"basic",
			gae.DSPropertyMap{
				"R": {mp(false), mp(2.1), mpNI(3)},
				"S": {mp("hello"), mp("world")},
			},
		},
		{
			"keys",
			gae.DSPropertyMap{
				"DS": {mp(mkKey("appy", "ns", "Foo", 7)), mp(mkKey("other", "", "Yot", "wheeep"))},
				"BS": {mp(gae.BSKey("sup")), mp(gae.BSKey("nerds"))},
			},
		},
		{
			"geo",
			gae.DSPropertyMap{
				"G": {mp(gae.DSGeoPoint{Lat: 1, Lng: 2})},
			},
		},
		{
			"data",
			gae.DSPropertyMap{
				"S":          {mp("sup"), mp("fool"), mp("nerd")},
				"D.Foo.Nerd": {mp([]byte("sup")), mp([]byte("fool"))},
				"B":          {mp(gae.DSByteString("sup")), mp(gae.DSByteString("fool"))},
			},
		},
		{
			"time",
			gae.DSPropertyMap{
				"T": {
					mp(time.Now().UTC()),
					mp(time.Now().Add(time.Second).UTC())},
			},
		},
		{
			"empty vals",
			gae.DSPropertyMap{
				"T": {mp(true), mp(true)},
				"F": {mp(false), mp(false)},
				"N": {mp(nil), mp(nil)},
				"E": {},
			},
		},
	}

	Convey("DSPropertyMap serialization", t, func() {
		Convey("round trip", func() {
			for _, tc := range tests {
				tc := tc
				Convey(tc.name, func() {
					buf := &bytes.Buffer{}
					WriteDSPropertyMap(buf, tc.props, WithContext)
					dec, err := ReadDSPropertyMap(buf, WithContext, "", "")
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
		Convey("ReadDSKey", func() {
			Convey("good cases", func() {
				Convey("w/ ctx decodes normally w/ ctx", func() {
					k := mkKey("aid", "ns", "knd", "yo", "other", 10)
					buf := &bytes.Buffer{}
					WriteDSKey(buf, WithContext, k)
					dk, err := ReadDSKey(buf, WithContext, "", "")
					So(err, ShouldBeNil)
					So(dk, ShouldEqualKey, k)
				})
				Convey("w/ ctx decodes normally w/o ctx", func() {
					k := mkKey("aid", "ns", "knd", "yo", "other", 10)
					buf := &bytes.Buffer{}
					WriteDSKey(buf, WithContext, k)
					dk, err := ReadDSKey(buf, WithoutContext, "spam", "nerd")
					So(err, ShouldBeNil)
					So(dk, ShouldEqualKey, mkKey("spam", "nerd", "knd", "yo", "other", 10))
				})
				Convey("w/o ctx decodes normally w/ ctx", func() {
					k := mkKey("aid", "ns", "knd", "yo", "other", 10)
					buf := &bytes.Buffer{}
					WriteDSKey(buf, WithoutContext, k)
					dk, err := ReadDSKey(buf, WithContext, "spam", "nerd")
					So(err, ShouldBeNil)
					So(dk, ShouldEqualKey, mkKey("", "", "knd", "yo", "other", 10))
				})
				Convey("w/o ctx decodes normally w/o ctx", func() {
					k := mkKey("aid", "ns", "knd", "yo", "other", 10)
					buf := &bytes.Buffer{}
					WriteDSKey(buf, WithoutContext, k)
					dk, err := ReadDSKey(buf, WithoutContext, "spam", "nerd")
					So(err, ShouldBeNil)
					So(dk, ShouldEqualKey, mkKey("spam", "nerd", "knd", "yo", "other", 10))
				})
			})

			Convey("err cases", func() {
				Convey("nil", func() {
					buf := &bytes.Buffer{}
					_, err := ReadDSKey(buf, WithContext, "", "")
					So(err, ShouldEqual, io.EOF)
				})
				Convey("str", func() {
					buf := &bytes.Buffer{}
					buf.WriteString("sup")
					_, err := ReadDSKey(buf, WithContext, "", "")
					So(err, ShouldErrLike, "expected actualCtx")
				})
				Convey("truncated 1", func() {
					buf := &bytes.Buffer{}
					buf.WriteByte(1) // actualCtx == 1
					_, err := ReadDSKey(buf, WithContext, "", "")
					So(err, ShouldEqual, io.EOF)
				})
				Convey("truncated 2", func() {
					buf := &bytes.Buffer{}
					buf.WriteByte(1) // actualCtx == 1
					cmpbin.WriteString(buf, "aid")
					_, err := ReadDSKey(buf, WithContext, "", "")
					So(err, ShouldEqual, io.EOF)
				})
				Convey("truncated 3", func() {
					buf := &bytes.Buffer{}
					buf.WriteByte(1) // actualCtx == 1
					cmpbin.WriteString(buf, "aid")
					cmpbin.WriteString(buf, "ns")
					_, err := ReadDSKey(buf, WithContext, "", "")
					So(err, ShouldEqual, io.EOF)
				})
				Convey("huge key", func() {
					buf := &bytes.Buffer{}
					buf.WriteByte(1) // actualCtx == 1
					cmpbin.WriteString(buf, "aid")
					cmpbin.WriteString(buf, "ns")
					cmpbin.WriteUint(buf, 1000)
					_, err := ReadDSKey(buf, WithContext, "", "")
					So(err, ShouldErrLike, "huge key")
				})
				Convey("insufficient tokens", func() {
					buf := &bytes.Buffer{}
					buf.WriteByte(1) // actualCtx == 1
					cmpbin.WriteString(buf, "aid")
					cmpbin.WriteString(buf, "ns")
					cmpbin.WriteUint(buf, 2)
					_, err := ReadDSKey(buf, WithContext, "", "")
					So(err, ShouldEqual, io.EOF)
				})
				Convey("partial token 1", func() {
					buf := &bytes.Buffer{}
					buf.WriteByte(1) // actualCtx == 1
					cmpbin.WriteString(buf, "aid")
					cmpbin.WriteString(buf, "ns")
					cmpbin.WriteUint(buf, 2)
					cmpbin.WriteString(buf, "hi")
					_, err := ReadDSKey(buf, WithContext, "", "")
					So(err, ShouldEqual, io.EOF)
				})
				Convey("partial token 2", func() {
					buf := &bytes.Buffer{}
					buf.WriteByte(1) // actualCtx == 1
					cmpbin.WriteString(buf, "aid")
					cmpbin.WriteString(buf, "ns")
					cmpbin.WriteUint(buf, 2)
					cmpbin.WriteString(buf, "hi")
					cmpbin.WriteString(buf, "")
					_, err := ReadDSKey(buf, WithContext, "", "")
					So(err, ShouldEqual, io.EOF)
				})
				Convey("bad token", func() {
					buf := &bytes.Buffer{}
					buf.WriteByte(1) // actualCtx == 1
					cmpbin.WriteString(buf, "aid")
					cmpbin.WriteString(buf, "ns")
					cmpbin.WriteUint(buf, 2)
					cmpbin.WriteString(buf, "hi")
					cmpbin.WriteString(buf, "")
					cmpbin.WriteInt(buf, -2)
					_, err := ReadDSKey(buf, WithContext, "", "")
					So(err, ShouldErrLike, "zero/negative")
				})
			})
		})

		Convey("ReadDSGeoPoint", func() {
			Convey("trunc 1", func() {
				buf := &bytes.Buffer{}
				_, err := ReadDSGeoPoint(buf)
				So(err, ShouldEqual, io.EOF)
			})
			Convey("trunc 2", func() {
				buf := &bytes.Buffer{}
				cmpbin.WriteFloat64(buf, 100)
				_, err := ReadDSGeoPoint(buf)
				So(err, ShouldEqual, io.EOF)
			})
			Convey("invalid", func() {
				buf := &bytes.Buffer{}
				cmpbin.WriteFloat64(buf, 100)
				cmpbin.WriteFloat64(buf, 1000)
				_, err := ReadDSGeoPoint(buf)
				So(err, ShouldErrLike, "invalid DSGeoPoint")
			})
		})

		Convey("WriteTime", func() {
			Convey("in non-UTC!", func() {
				buf := &bytes.Buffer{}
				So(func() {
					WriteTime(buf, time.Now())
				}, ShouldPanic)
			})
		})

		Convey("ReadTime", func() {
			Convey("trunc 1", func() {
				buf := &bytes.Buffer{}
				_, err := ReadTime(buf)
				So(err, ShouldEqual, io.EOF)
			})
		})

		Convey("ReadDSProperty", func() {
			Convey("trunc 1", func() {
				buf := &bytes.Buffer{}
				_, err := ReadDSProperty(buf, WithContext, "", "")
				So(err, ShouldEqual, io.EOF)
			})
			Convey("trunc (DSPTBytes)", func() {
				buf := &bytes.Buffer{}
				buf.WriteByte(byte(gae.DSPTBytes))
				_, err := ReadDSProperty(buf, WithContext, "", "")
				So(err, ShouldEqual, io.EOF)
			})
			Convey("trunc (DSPTBlobKey)", func() {
				buf := &bytes.Buffer{}
				buf.WriteByte(byte(gae.DSPTBlobKey))
				_, err := ReadDSProperty(buf, WithContext, "", "")
				So(err, ShouldEqual, io.EOF)
			})
			Convey("invalid type", func() {
				buf := &bytes.Buffer{}
				buf.WriteByte(byte(gae.DSPTUnknown + 1))
				_, err := ReadDSProperty(buf, WithContext, "", "")
				So(err, ShouldErrLike, "unknown type!")
			})
		})

		Convey("ReadDSPropertyMap", func() {
			Convey("trunc 1", func() {
				buf := &bytes.Buffer{}
				_, err := ReadDSPropertyMap(buf, WithContext, "", "")
				So(err, ShouldEqual, io.EOF)
			})
			Convey("too many rows", func() {
				buf := &bytes.Buffer{}
				cmpbin.WriteUint(buf, 1000000)
				_, err := ReadDSPropertyMap(buf, WithContext, "", "")
				So(err, ShouldErrLike, "huge number of rows")
			})
			Convey("trunc 2", func() {
				buf := &bytes.Buffer{}
				cmpbin.WriteUint(buf, 10)
				_, err := ReadDSPropertyMap(buf, WithContext, "", "")
				So(err, ShouldEqual, io.EOF)
			})
			Convey("trunc 3", func() {
				buf := &bytes.Buffer{}
				cmpbin.WriteUint(buf, 10)
				cmpbin.WriteString(buf, "ohai")
				_, err := ReadDSPropertyMap(buf, WithContext, "", "")
				So(err, ShouldEqual, io.EOF)
			})
			Convey("too many values", func() {
				buf := &bytes.Buffer{}
				cmpbin.WriteUint(buf, 10)
				cmpbin.WriteString(buf, "ohai")
				cmpbin.WriteUint(buf, 100000)
				_, err := ReadDSPropertyMap(buf, WithContext, "", "")
				So(err, ShouldErrLike, "huge number of properties")
			})
			Convey("trunc 4", func() {
				buf := &bytes.Buffer{}
				cmpbin.WriteUint(buf, 10)
				cmpbin.WriteString(buf, "ohai")
				cmpbin.WriteUint(buf, 10)
				_, err := ReadDSPropertyMap(buf, WithContext, "", "")
				So(err, ShouldEqual, io.EOF)
			})
		})
	})
}
