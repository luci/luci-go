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

package datastore

import (
	"bytes"
	"io"
	"testing"
	"time"

	"go.chromium.org/luci/common/data/cmpbin"

	"go.chromium.org/luci/gae/service/blobstore"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func init() {
	WritePropertyMapDeterministic = true
}

type dspmapTC struct {
	name  string
	props PropertyMap
}

func mkKeyCtx(appID, namespace string, elems ...any) *Key {
	return MkKeyContext(appID, namespace).MakeKey(elems...)
}

func mkBuf(data []byte) cmpbin.WriteableBytesBuffer {
	return cmpbin.Invertible(bytes.NewBuffer(data))
}

func TestPropertyMapSerialization(t *testing.T) {
	t.Parallel()

	now := time.Now().UTC()
	tests := []dspmapTC{
		{
			"basic",
			PropertyMap{
				"R": PropertySlice{mp(false), mp(2.1), mpNI(3)},
				"S": PropertySlice{mp("hello"), mp("world")},
			},
		},
		{
			"keys",
			PropertyMap{
				"DS": PropertySlice{
					mp(mkKeyCtx("appy", "ns", "Foo", 7)),
					mp(mkKeyCtx("other", "", "Yot", "wheeep")),
					mp((*Key)(nil)),
				},
				"blobstore": PropertySlice{mp(blobstore.Key("sup")), mp(blobstore.Key("nerds"))},
			},
		},
		{
			"property map",
			PropertyMap{
				"pm": PropertySlice{
					mp(PropertyMap{
						"$key":    mpNI(mkKeyCtx("app", "ns", "entity", "id")),
						"$kind":   mpNI("entity"),
						"$id":     mpNI("id"),
						"$parent": mpNI(nil),
						"indexed": mp("indexed"),
						"map": mpNI(PropertyMap{
							"b": mpNI([]byte("byte")),
						}),
						"str": mpNI("string")},
					),
				},
			},
		},
		{
			"geo",
			PropertyMap{
				"G": mp(GeoPoint{Lat: 1, Lng: 2}),
			},
		},
		{
			"data",
			PropertyMap{
				"S":                    PropertySlice{mp("sup"), mp("fool"), mp("nerd")},
				"Deserialize.Foo.Nerd": PropertySlice{mp([]byte("sup")), mp([]byte("fool"))},
			},
		},
		{
			"time",
			PropertyMap{
				"T": PropertySlice{
					mp(now),
					mp(now.Add(time.Second)),
				},
			},
		},
		{
			"empty vals",
			PropertyMap{
				"T": PropertySlice{mp(true), mp(true)},
				"F": PropertySlice{mp(false), mp(false)},
				"N": PropertySlice{mp(nil), mp(nil)},
				"E": PropertySlice{},
			},
		},
	}

	Convey("PropertyMap serialization", t, func() {
		Convey("round trip", func() {
			for _, tc := range tests {
				tc := tc
				Convey(tc.name, func() {
					data := SerializeKC.ToBytes(tc.props)
					dec, err := Deserialize.PropertyMap(mkBuf(data))
					So(err, ShouldBeNil)
					So(dec, ShouldResemble, tc.props)
				})
			}
		})
	})
}

func die(err error) {
	if err != nil {
		panic(err)
	}
}

func wf(w io.Writer, v float64) int {
	ret, err := cmpbin.WriteFloat64(w, v)
	die(err)
	return ret
}

func ws(w io.ByteWriter, s string) int {
	ret, err := cmpbin.WriteString(w, s)
	die(err)
	return ret
}

func wi(w io.ByteWriter, i int64) int {
	ret, err := cmpbin.WriteInt(w, i)
	die(err)
	return ret
}

func wui(w io.ByteWriter, i uint64) int {
	ret, err := cmpbin.WriteUint(w, i)
	die(err)
	return ret
}

func TestSerializationReadMisc(t *testing.T) {
	t.Parallel()

	Convey("Misc Serialization tests", t, func() {
		Convey("GeoPoint", func() {
			buf := mkBuf(nil)
			wf(buf, 10)
			wf(buf, 20)
			So(string(Serialize.ToBytes(GeoPoint{Lat: 10, Lng: 20})), ShouldEqual, buf.String())
		})

		Convey("IndexColumn", func() {
			buf := mkBuf(nil)
			die(buf.WriteByte(1))
			ws(buf, "hi")
			So(string(Serialize.ToBytes(IndexColumn{Property: "hi", Descending: true})),
				ShouldEqual, buf.String())
		})

		Convey("KeyTok", func() {
			buf := mkBuf(nil)
			ws(buf, "foo")
			die(buf.WriteByte(byte(PTInt)))
			wi(buf, 20)
			So(string(Serialize.ToBytes(KeyTok{Kind: "foo", IntID: 20})),
				ShouldEqual, buf.String())
		})

		Convey("Property", func() {
			buf := mkBuf(nil)
			die(buf.WriteByte(0x80 | byte(PTString)))
			ws(buf, "nerp")
			So(string(Serialize.ToBytes(mp("nerp"))),
				ShouldEqual, buf.String())
		})

		Convey("Time", func() {
			tp := mp(time.Now().UTC())
			So(string(Serialize.ToBytes(tp.Value())), ShouldEqual, string(Serialize.ToBytes(tp)[1:]))
		})

		Convey("Zero time", func() {
			buf := mkBuf(nil)
			So(Serialize.Time(buf, time.Time{}), ShouldBeNil)
			t, err := Deserialize.Time(mkBuf(buf.Bytes()))
			So(err, ShouldBeNil)
			So(t.Equal(time.Time{}), ShouldBeTrue)
		})

		Convey("ReadKey", func() {
			Convey("good cases", func() {
				dwc := Deserializer{MkKeyContext("spam", "nerd")}

				Convey("w/ ctx decodes normally w/ ctx", func() {
					k := mkKeyCtx("aid", "ns", "knd", "yo", "other", 10)
					data := SerializeKC.ToBytes(k)
					dk, err := Deserialize.Key(mkBuf(data))
					So(err, ShouldBeNil)
					So(dk, ShouldEqualKey, k)
				})
				Convey("w/ ctx decodes normally w/o ctx", func() {
					k := mkKeyCtx("aid", "ns", "knd", "yo", "other", 10)
					data := SerializeKC.ToBytes(k)
					dk, err := dwc.Key(mkBuf(data))
					So(err, ShouldBeNil)
					So(dk, ShouldEqualKey, mkKeyCtx("spam", "nerd", "knd", "yo", "other", 10))
				})
				Convey("w/o ctx decodes normally w/ ctx", func() {
					k := mkKeyCtx("aid", "ns", "knd", "yo", "other", 10)
					data := Serialize.ToBytes(k)
					dk, err := Deserialize.Key(mkBuf(data))
					So(err, ShouldBeNil)
					So(dk, ShouldEqualKey, mkKeyCtx("", "", "knd", "yo", "other", 10))
				})
				Convey("w/o ctx decodes normally w/o ctx", func() {
					k := mkKeyCtx("aid", "ns", "knd", "yo", "other", 10)
					data := Serialize.ToBytes(k)
					dk, err := dwc.Key(mkBuf(data))
					So(err, ShouldBeNil)
					So(dk, ShouldEqualKey, mkKeyCtx("spam", "nerd", "knd", "yo", "other", 10))
				})
				Convey("IntIDs always sort before StringIDs", func() {
					// -1 writes as almost all 1's in the first byte under cmpbin, even
					// though it's technically not a valid key.
					k := mkKeyCtx("aid", "ns", "knd", -1)
					data := Serialize.ToBytes(k)

					k = mkKeyCtx("aid", "ns", "knd", "hat")
					data2 := Serialize.ToBytes(k)

					So(string(data), ShouldBeLessThan, string(data2))
				})
			})

			Convey("err cases", func() {
				buf := mkBuf(nil)

				Convey("nil", func() {
					_, err := Deserialize.Key(buf)
					So(err, ShouldEqual, io.EOF)
				})
				Convey("str", func() {
					_, err := buf.WriteString("sup")
					die(err)
					_, err = Deserialize.Key(buf)
					So(err, ShouldErrLike, "expected actualCtx")
				})
				Convey("truncated 1", func() {
					die(buf.WriteByte(1)) // actualCtx == 1
					_, err := Deserialize.Key(buf)
					So(err, ShouldEqual, io.EOF)
				})
				Convey("truncated 2", func() {
					die(buf.WriteByte(1)) // actualCtx == 1
					ws(buf, "aid")
					_, err := Deserialize.Key(buf)
					So(err, ShouldEqual, io.EOF)
				})
				Convey("truncated 3", func() {
					die(buf.WriteByte(1)) // actualCtx == 1
					ws(buf, "aid")
					ws(buf, "ns")
					_, err := Deserialize.Key(buf)
					So(err, ShouldEqual, io.EOF)
				})
				Convey("huge key", func() {
					die(buf.WriteByte(1)) // actualCtx == 1
					ws(buf, "aid")
					ws(buf, "ns")
					for i := 1; i < 60; i++ {
						die(buf.WriteByte(1))
						die(Serialize.KeyTok(buf, KeyTok{Kind: "sup", IntID: int64(i)}))
					}
					die(buf.WriteByte(0))
					_, err := Deserialize.Key(buf)
					So(err, ShouldErrLike, "huge key")
				})
				Convey("insufficient tokens", func() {
					die(buf.WriteByte(1)) // actualCtx == 1
					ws(buf, "aid")
					ws(buf, "ns")
					wui(buf, 2)
					_, err := Deserialize.Key(buf)
					So(err, ShouldEqual, io.EOF)
				})
				Convey("partial token 1", func() {
					die(buf.WriteByte(1)) // actualCtx == 1
					ws(buf, "aid")
					ws(buf, "ns")
					die(buf.WriteByte(1))
					ws(buf, "hi")
					_, err := Deserialize.Key(buf)
					So(err, ShouldEqual, io.EOF)
				})
				Convey("partial token 2", func() {
					die(buf.WriteByte(1)) // actualCtx == 1
					ws(buf, "aid")
					ws(buf, "ns")
					die(buf.WriteByte(1))
					ws(buf, "hi")
					die(buf.WriteByte(byte(PTString)))
					_, err := Deserialize.Key(buf)
					So(err, ShouldEqual, io.EOF)
				})
				Convey("bad token (invalid type)", func() {
					die(buf.WriteByte(1)) // actualCtx == 1
					ws(buf, "aid")
					ws(buf, "ns")
					die(buf.WriteByte(1))
					ws(buf, "hi")
					die(buf.WriteByte(byte(PTBlobKey)))
					_, err := Deserialize.Key(buf)
					So(err, ShouldErrLike, "invalid type PTBlobKey")
				})
				Convey("bad token (invalid IntID)", func() {
					die(buf.WriteByte(1)) // actualCtx == 1
					ws(buf, "aid")
					ws(buf, "ns")
					die(buf.WriteByte(1))
					ws(buf, "hi")
					die(buf.WriteByte(byte(PTInt)))
					wi(buf, -2)
					_, err := Deserialize.Key(buf)
					So(err, ShouldErrLike, "zero/negative")
				})
			})
		})

		Convey("ReadGeoPoint", func() {
			buf := mkBuf(nil)
			Convey("trunc 1", func() {
				_, err := Deserialize.GeoPoint(buf)
				So(err, ShouldEqual, io.EOF)
			})
			Convey("trunc 2", func() {
				wf(buf, 100)
				_, err := Deserialize.GeoPoint(buf)
				So(err, ShouldEqual, io.EOF)
			})
			Convey("invalid", func() {
				wf(buf, 100)
				wf(buf, 1000)
				_, err := Deserialize.GeoPoint(buf)
				So(err, ShouldErrLike, "invalid GeoPoint")
			})
		})

		Convey("WriteTime", func() {
			Convey("in non-UTC!", func() {
				pst, err := time.LoadLocation("America/Los_Angeles")
				So(err, ShouldBeNil)
				So(func() {
					die(Serialize.Time(mkBuf(nil), time.Now().In(pst)))
				}, ShouldPanic)
			})
		})

		Convey("ReadTime", func() {
			Convey("trunc 1", func() {
				_, err := Deserialize.Time(mkBuf(nil))
				So(err, ShouldEqual, io.EOF)
			})
		})

		Convey("ReadProperty", func() {
			buf := mkBuf(nil)
			Convey("trunc 1", func() {
				p, err := Deserialize.Property(buf)
				So(err, ShouldEqual, io.EOF)
				So(p.Type(), ShouldEqual, PTNull)
				So(p.Value(), ShouldBeNil)
			})
			Convey("trunc (PTBytes)", func() {
				die(buf.WriteByte(byte(PTBytes)))
				_, err := Deserialize.Property(buf)
				So(err, ShouldEqual, io.EOF)
			})
			Convey("trunc (PTBlobKey)", func() {
				die(buf.WriteByte(byte(PTBlobKey)))
				_, err := Deserialize.Property(buf)
				So(err, ShouldEqual, io.EOF)
			})
			Convey("invalid type", func() {
				die(buf.WriteByte(byte(PTUnknown + 1)))
				_, err := Deserialize.Property(buf)
				So(err, ShouldErrLike, "unknown type!")
			})
		})

		Convey("ReadPropertyMap", func() {
			buf := mkBuf(nil)
			Convey("trunc 1", func() {
				_, err := Deserialize.PropertyMap(buf)
				So(err, ShouldEqual, io.EOF)
			})
			Convey("too many rows", func() {
				wui(buf, 1000000)
				_, err := Deserialize.PropertyMap(buf)
				So(err, ShouldErrLike, "huge number of rows")
			})
			Convey("trunc 2", func() {
				wui(buf, 10)
				_, err := Deserialize.PropertyMap(buf)
				So(err, ShouldEqual, io.EOF)
			})
			Convey("trunc 3", func() {
				wui(buf, 10)
				ws(buf, "ohai")
				_, err := Deserialize.PropertyMap(buf)
				So(err, ShouldEqual, io.EOF)
			})
			Convey("too many values", func() {
				wui(buf, 10)
				ws(buf, "ohai")
				wui(buf, 100000)
				_, err := Deserialize.PropertyMap(buf)
				So(err, ShouldErrLike, "huge number of properties")
			})
			Convey("trunc 4", func() {
				wui(buf, 10)
				ws(buf, "ohai")
				wui(buf, 10)
				_, err := Deserialize.PropertyMap(buf)
				So(err, ShouldEqual, io.EOF)
			})
		})

		Convey("IndexDefinition", func() {
			id := IndexDefinition{Kind: "kind"}
			data := Serialize.ToBytes(*id.PrepForIdxTable())
			newID, err := Deserialize.IndexDefinition(mkBuf(data))
			So(err, ShouldBeNil)
			So(newID.Flip(), ShouldResemble, id.Normalize())

			id.SortBy = append(id.SortBy, IndexColumn{Property: "prop"})
			data = Serialize.ToBytes(*id.PrepForIdxTable())
			newID, err = Deserialize.IndexDefinition(mkBuf(data))
			So(err, ShouldBeNil)
			So(newID.Flip(), ShouldResemble, id.Normalize())

			id.SortBy = append(id.SortBy, IndexColumn{Property: "other", Descending: true})
			id.Ancestor = true
			data = Serialize.ToBytes(*id.PrepForIdxTable())
			newID, err = Deserialize.IndexDefinition(mkBuf(data))
			So(err, ShouldBeNil)
			So(newID.Flip(), ShouldResemble, id.Normalize())

			// invalid
			id.SortBy = append(id.SortBy, IndexColumn{Property: "", Descending: true})
			data = Serialize.ToBytes(*id.PrepForIdxTable())
			newID, err = Deserialize.IndexDefinition(mkBuf(data))
			So(err, ShouldBeNil)
			So(newID.Flip(), ShouldResemble, id.Normalize())

			Convey("too many", func() {
				id := IndexDefinition{Kind: "wat"}
				for i := 0; i < maxIndexColumns+1; i++ {
					id.SortBy = append(id.SortBy, IndexColumn{Property: "Hi", Descending: true})
				}
				data := Serialize.ToBytes(*id.PrepForIdxTable())
				newID, err = Deserialize.IndexDefinition(mkBuf(data))
				So(err, ShouldErrLike, "over 64 sort orders")
			})
		})
	})
}

func TestIndexedProperties(t *testing.T) {
	t.Parallel()

	fakeKey := mkKeyCtx("dev~app", "ns", "parentKind", "sid", "knd", 10)

	innerKey1 := mkKeyCtx("dev~app", "ns", "parentKind", "sid", "innerKey", 1)
	innerKey2 := mkKeyCtx("dev~app", "ns", "parentKind", "sid", "innerKey", 2)
	innerKey3 := mkKeyCtx("dev~app", "ns", "parentKind", "sid", "innerKey", 3)

	Convey("TestIndexedProperties", t, func() {
		pm := PropertyMap{
			"wat":  PropertySlice{mpNI("thing"), mp("hat"), mp(100)},
			"nerd": mp(103.7),
			"spaz": mpNI(false),
			"nested": PropertySlice{
				mp(PropertyMap{
					"$key":  mpNI(innerKey1),
					"prop":  mp(123),
					"slice": PropertySlice{mp(123), mp(456)},
					"ni":    mpNI(666),
				}),
				mpNI(PropertyMap{ // will be skipped, not indexed
					"$key":  mpNI(innerKey2),
					"prop":  mp(123),
					"slice": PropertySlice{mp(123), mp(456)},
				}),
				mp(PropertyMap{
					"$kind":   mpNI("innerKey"),
					"$id":     mpNI(3),
					"$parent": mpNI(innerKey3.Parent()),
					"prop":    mp(456),
					"slice":   PropertySlice{mp(456), mp(789)},
					"ni":      mpNI(666),
				}),
				mp(PropertyMap{ // key is optional
					"prop": mp(777),
					"deeper": mp(PropertyMap{
						"deep": mp(888),
					}),
				}),
			},
		}

		Convey("IndexedProperties", func() {
			sip := Serialize.IndexedProperties(fakeKey, pm)
			So(len(sip), ShouldEqual, 8)
			sip.Sort()

			So(sip, ShouldResemble, IndexedProperties{
				"wat": {
					Serialize.ToBytes(mp(100)),
					Serialize.ToBytes(mp("hat")),
					// 'thing' is skipped, because it's not NoIndex
				},
				"nerd": {
					Serialize.ToBytes(mp(103.7)),
				},
				"nested.__key__": {
					Serialize.ToBytes(mp(innerKey1)),
					Serialize.ToBytes(mp(innerKey3)),
				},
				"nested.deeper.deep": {
					Serialize.ToBytes(mp(888)),
				},
				"nested.prop": {
					Serialize.ToBytes(mp(123)),
					Serialize.ToBytes(mp(456)),
					Serialize.ToBytes(mp(777)),
				},
				"nested.slice": {
					Serialize.ToBytes(mp(123)),
					Serialize.ToBytes(mp(456)),
					Serialize.ToBytes(mp(789)),
				},
				"__key__": {
					Serialize.ToBytes(mp(fakeKey)),
				},
				"__ancestor__": {
					Serialize.ToBytes(mp(fakeKey.Parent())),
					Serialize.ToBytes(mp(fakeKey)),
				},
			})
		})

		Convey("IndexedPropertiesForIndicies", func() {
			sip := Serialize.IndexedPropertiesForIndicies(fakeKey, pm, []IndexColumn{
				{Property: "wat"},
				{Property: "wat", Descending: true},
				{Property: "unknown"},
				{Property: "nested.__key__"},
			})
			So(len(sip), ShouldEqual, 4)
			sip.Sort()

			So(sip, ShouldResemble, IndexedProperties{
				"wat": {
					Serialize.ToBytes(mp(100)),
					Serialize.ToBytes(mp("hat")),
				},
				"nested.__key__": {
					Serialize.ToBytes(mp(innerKey1)),
					Serialize.ToBytes(mp(innerKey3)),
				},
				"__key__": {
					Serialize.ToBytes(mp(fakeKey)),
				},
				"__ancestor__": {
					Serialize.ToBytes(mp(fakeKey.Parent())),
					Serialize.ToBytes(mp(fakeKey)),
				},
			})
		})
	})
}
