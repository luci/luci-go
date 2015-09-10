// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package serialize

import (
	"bytes"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/luci/gae/service/blobstore"
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/dskey"
	"github.com/luci/luci-go/common/cmpbin"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	WritePropertyMapDeterministic = true
}

var (
	mp   = ds.MkProperty
	mpNI = ds.MkPropertyNI
)

type dspmapTC struct {
	name  string
	props ds.PropertyMap
}

// TODO(riannucci): dedup with datastore/key testing file.
func mkKey(aid, ns string, elems ...interface{}) ds.Key {
	if len(elems)%2 != 0 {
		panic("odd number of tokens")
	}
	toks := make([]ds.KeyTok, len(elems)/2)
	for i := 0; i < len(elems); i += 2 {
		toks[i/2].Kind = elems[i].(string)
		switch x := elems[i+1].(type) {
		case string:
			toks[i/2].StringID = x
		case int:
			toks[i/2].IntID = int64(x)
		default:
			panic("bad token id")
		}
	}
	return dskey.NewToks(aid, ns, toks)
}

func mkBuf(data []byte) Buffer {
	return Invertible(bytes.NewBuffer(data))
}

// TODO(riannucci): dedup with datastore/key testing file
func ShouldEqualKey(actual interface{}, expected ...interface{}) string {
	if len(expected) != 1 {
		return fmt.Sprintf("Assertion requires 1 expected value, got %d", len(expected))
	}
	if dskey.Equal(actual.(ds.Key), expected[0].(ds.Key)) {
		return ""
	}
	return fmt.Sprintf("Expected: %q\nActual: %q", actual, expected[0])
}

func TestPropertyMapSerialization(t *testing.T) {
	t.Parallel()

	tests := []dspmapTC{
		{
			"basic",
			ds.PropertyMap{
				"R": {mp(false), mp(2.1), mpNI(3)},
				"S": {mp("hello"), mp("world")},
			},
		},
		{
			"keys",
			ds.PropertyMap{
				"DS":        {mp(mkKey("appy", "ns", "Foo", 7)), mp(mkKey("other", "", "Yot", "wheeep"))},
				"blobstore": {mp(blobstore.Key("sup")), mp(blobstore.Key("nerds"))},
			},
		},
		{
			"geo",
			ds.PropertyMap{
				"G": {mp(ds.GeoPoint{Lat: 1, Lng: 2})},
			},
		},
		{
			"data",
			ds.PropertyMap{
				"S":          {mp("sup"), mp("fool"), mp("nerd")},
				"D.Foo.Nerd": {mp([]byte("sup")), mp([]byte("fool"))},
			},
		},
		{
			"time",
			ds.PropertyMap{
				"T": {
					mp(time.Now().UTC()),
					mp(time.Now().Add(time.Second).UTC())},
			},
		},
		{
			"empty vals",
			ds.PropertyMap{
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
					data := ToBytesWithContext(tc.props)
					dec, err := ReadPropertyMap(mkBuf(data), WithContext, "", "")
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
		Convey("GeoPoint", func() {
			buf := mkBuf(nil)
			cmpbin.WriteFloat64(buf, 10)
			cmpbin.WriteFloat64(buf, 20)
			So(string(ToBytes(ds.GeoPoint{Lat: 10, Lng: 20})), ShouldEqual, buf.String())
		})

		Convey("IndexColumn", func() {
			buf := mkBuf(nil)
			buf.WriteByte(1)
			cmpbin.WriteString(buf, "hi")
			So(string(ToBytes(ds.IndexColumn{Property: "hi", Direction: ds.DESCENDING})),
				ShouldEqual, buf.String())
		})

		Convey("KeyTok", func() {
			buf := mkBuf(nil)
			cmpbin.WriteString(buf, "foo")
			buf.WriteByte(byte(ds.PTInt))
			cmpbin.WriteInt(buf, 20)
			So(string(ToBytes(ds.KeyTok{Kind: "foo", IntID: 20})),
				ShouldEqual, buf.String())
		})

		Convey("Property", func() {
			buf := mkBuf(nil)
			buf.WriteByte(0x80 | byte(ds.PTString))
			cmpbin.WriteString(buf, "nerp")
			So(string(ToBytes(mp("nerp"))),
				ShouldEqual, buf.String())
		})

		Convey("Time", func() {
			tp := mp(time.Now().UTC())
			So(string(ToBytes(tp.Value())), ShouldEqual, string(ToBytes(tp)[1:]))
		})

		Convey("Bad ToBytes", func() {
			So(func() { ToBytes(100.7) }, ShouldPanic)
			So(func() { ToBytesWithContext(100.7) }, ShouldPanic)
		})

		Convey("ReadKey", func() {
			Convey("good cases", func() {
				Convey("w/ ctx decodes normally w/ ctx", func() {
					k := mkKey("aid", "ns", "knd", "yo", "other", 10)
					data := ToBytesWithContext(k)
					dk, err := ReadKey(mkBuf(data), WithContext, "", "")
					So(err, ShouldBeNil)
					So(dk, ShouldEqualKey, k)
				})
				Convey("w/ ctx decodes normally w/o ctx", func() {
					k := mkKey("aid", "ns", "knd", "yo", "other", 10)
					data := ToBytesWithContext(k)
					dk, err := ReadKey(mkBuf(data), WithoutContext, "spam", "nerd")
					So(err, ShouldBeNil)
					So(dk, ShouldEqualKey, mkKey("spam", "nerd", "knd", "yo", "other", 10))
				})
				Convey("w/o ctx decodes normally w/ ctx", func() {
					k := mkKey("aid", "ns", "knd", "yo", "other", 10)
					data := ToBytes(k)
					dk, err := ReadKey(mkBuf(data), WithContext, "spam", "nerd")
					So(err, ShouldBeNil)
					So(dk, ShouldEqualKey, mkKey("", "", "knd", "yo", "other", 10))
				})
				Convey("w/o ctx decodes normally w/o ctx", func() {
					k := mkKey("aid", "ns", "knd", "yo", "other", 10)
					data := ToBytes(k)
					dk, err := ReadKey(mkBuf(data), WithoutContext, "spam", "nerd")
					So(err, ShouldBeNil)
					So(dk, ShouldEqualKey, mkKey("spam", "nerd", "knd", "yo", "other", 10))
				})
				Convey("IntIDs always sort before StringIDs", func() {
					// -1 writes as almost all 1's in the first byte under cmpbin, even
					// though it's technically not a valid key.
					k := mkKey("aid", "ns", "knd", -1)
					data := ToBytes(k)

					k = mkKey("aid", "ns", "knd", "hat")
					data2 := ToBytes(k)

					So(string(data), ShouldBeLessThan, string(data2))
				})
			})

			Convey("err cases", func() {
				buf := mkBuf(nil)
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
					for i := 1; i < 60; i++ {
						buf.WriteByte(1)
						WriteKeyTok(buf, ds.KeyTok{Kind: "sup", IntID: int64(i)})
					}
					buf.WriteByte(0)
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
					buf.WriteByte(1)
					cmpbin.WriteString(buf, "hi")
					_, err := ReadKey(buf, WithContext, "", "")
					So(err, ShouldEqual, io.EOF)
				})
				Convey("partial token 2", func() {
					buf.WriteByte(1) // actualCtx == 1
					cmpbin.WriteString(buf, "aid")
					cmpbin.WriteString(buf, "ns")
					buf.WriteByte(1)
					cmpbin.WriteString(buf, "hi")
					buf.WriteByte(byte(ds.PTString))
					_, err := ReadKey(buf, WithContext, "", "")
					So(err, ShouldEqual, io.EOF)
				})
				Convey("bad token (invalid type)", func() {
					buf.WriteByte(1) // actualCtx == 1
					cmpbin.WriteString(buf, "aid")
					cmpbin.WriteString(buf, "ns")
					buf.WriteByte(1)
					cmpbin.WriteString(buf, "hi")
					buf.WriteByte(byte(ds.PTBlobKey))
					_, err := ReadKey(buf, WithContext, "", "")
					So(err, ShouldErrLike, "invalid type PTBlobKey")
				})
				Convey("bad token (invalid IntID)", func() {
					buf.WriteByte(1) // actualCtx == 1
					cmpbin.WriteString(buf, "aid")
					cmpbin.WriteString(buf, "ns")
					buf.WriteByte(1)
					cmpbin.WriteString(buf, "hi")
					buf.WriteByte(byte(ds.PTInt))
					cmpbin.WriteInt(buf, -2)
					_, err := ReadKey(buf, WithContext, "", "")
					So(err, ShouldErrLike, "zero/negative")
				})
			})
		})

		Convey("ReadGeoPoint", func() {
			buf := mkBuf(nil)
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
					WriteTime(mkBuf(nil), time.Now().In(pst))
				}, ShouldPanic)
			})
		})

		Convey("ReadTime", func() {
			Convey("trunc 1", func() {
				_, err := ReadTime(mkBuf(nil))
				So(err, ShouldEqual, io.EOF)
			})
		})

		Convey("ReadProperty", func() {
			buf := mkBuf(nil)
			Convey("trunc 1", func() {
				p, err := ReadProperty(buf, WithContext, "", "")
				So(err, ShouldEqual, io.EOF)
				So(p.Type(), ShouldEqual, ds.PTNull)
				So(p.Value(), ShouldBeNil)
			})
			Convey("trunc (PTBytes)", func() {
				buf.WriteByte(byte(ds.PTBytes))
				_, err := ReadProperty(buf, WithContext, "", "")
				So(err, ShouldEqual, io.EOF)
			})
			Convey("trunc (PTBlobKey)", func() {
				buf.WriteByte(byte(ds.PTBlobKey))
				_, err := ReadProperty(buf, WithContext, "", "")
				So(err, ShouldEqual, io.EOF)
			})
			Convey("invalid type", func() {
				buf.WriteByte(byte(ds.PTUnknown + 1))
				_, err := ReadProperty(buf, WithContext, "", "")
				So(err, ShouldErrLike, "unknown type!")
			})
		})

		Convey("ReadPropertyMap", func() {
			buf := mkBuf(nil)
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

		Convey("IndexDefinition", func() {
			id := ds.IndexDefinition{Kind: "kind"}
			data := ToBytes(*id.PrepForIdxTable())
			newID, err := ReadIndexDefinition(mkBuf(data))
			So(err, ShouldBeNil)
			So(newID.Flip(), ShouldResemble, id.Normalize())

			id.SortBy = append(id.SortBy, ds.IndexColumn{Property: "prop"})
			data = ToBytes(*id.PrepForIdxTable())
			newID, err = ReadIndexDefinition(mkBuf(data))
			So(err, ShouldBeNil)
			So(newID.Flip(), ShouldResemble, id.Normalize())

			id.SortBy = append(id.SortBy, ds.IndexColumn{Property: "other", Direction: ds.DESCENDING})
			id.Ancestor = true
			data = ToBytes(*id.PrepForIdxTable())
			newID, err = ReadIndexDefinition(mkBuf(data))
			So(err, ShouldBeNil)
			So(newID.Flip(), ShouldResemble, id.Normalize())

			// invalid
			id.SortBy = append(id.SortBy, ds.IndexColumn{Property: "", Direction: ds.DESCENDING})
			data = ToBytes(*id.PrepForIdxTable())
			newID, err = ReadIndexDefinition(mkBuf(data))
			So(err, ShouldBeNil)
			So(newID.Flip(), ShouldResemble, id.Normalize())

			Convey("too many", func() {
				id := ds.IndexDefinition{Kind: "wat"}
				for i := 0; i < MaxIndexColumns+1; i++ {
					id.SortBy = append(id.SortBy, ds.IndexColumn{Property: "Hi", Direction: ds.ASCENDING})
				}
				data := ToBytes(*id.PrepForIdxTable())
				newID, err = ReadIndexDefinition(mkBuf(data))
				So(err, ShouldErrLike, "over 64 sort orders")
			})
		})
	})
}
