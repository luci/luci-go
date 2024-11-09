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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/gae/service/blobstore"
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

	ftt.Run("PropertyMap serialization", t, func(t *ftt.Test) {
		t.Run("round trip", func(t *ftt.Test) {
			for _, tc := range tests {
				tc := tc
				t.Run(tc.name, func(t *ftt.Test) {
					data := SerializeKC.ToBytes(tc.props)
					dec, err := Deserialize.PropertyMap(mkBuf(data))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, dec, should.Resemble(tc.props))
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

	ftt.Run("Misc Serialization tests", t, func(t *ftt.Test) {
		t.Run("GeoPoint", func(t *ftt.Test) {
			buf := mkBuf(nil)
			wf(buf, 10)
			wf(buf, 20)
			assert.Loosely(t, string(Serialize.ToBytes(GeoPoint{Lat: 10, Lng: 20})), should.Equal(buf.String()))
		})

		t.Run("IndexColumn", func(t *ftt.Test) {
			buf := mkBuf(nil)
			die(buf.WriteByte(1))
			ws(buf, "hi")
			assert.Loosely(t, string(Serialize.ToBytes(IndexColumn{Property: "hi", Descending: true})),
				should.Equal(buf.String()))
		})

		t.Run("KeyTok", func(t *ftt.Test) {
			buf := mkBuf(nil)
			ws(buf, "foo")
			die(buf.WriteByte(byte(PTInt)))
			wi(buf, 20)
			assert.Loosely(t, string(Serialize.ToBytes(KeyTok{Kind: "foo", IntID: 20})),
				should.Equal(buf.String()))
		})

		t.Run("Property", func(t *ftt.Test) {
			buf := mkBuf(nil)
			die(buf.WriteByte(0x80 | byte(PTString)))
			ws(buf, "nerp")
			assert.Loosely(t, string(Serialize.ToBytes(mp("nerp"))),
				should.Equal(buf.String()))
		})

		t.Run("Time", func(t *ftt.Test) {
			tp := mp(time.Now().UTC())
			assert.Loosely(t, string(Serialize.ToBytes(tp.Value())), should.Equal(string(Serialize.ToBytes(tp)[1:])))
		})

		t.Run("Zero time", func(t *ftt.Test) {
			buf := mkBuf(nil)
			assert.Loosely(t, Serialize.Time(buf, time.Time{}), should.BeNil)
			ts, err := Deserialize.Time(mkBuf(buf.Bytes()))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, ts.Equal(time.Time{}), should.BeTrue)
		})

		t.Run("ReadKey", func(t *ftt.Test) {
			t.Run("good cases", func(t *ftt.Test) {
				dwc := Deserializer{MkKeyContext("spam", "nerd")}

				t.Run("w/ ctx decodes normally w/ ctx", func(t *ftt.Test) {
					k := mkKeyCtx("aid", "ns", "knd", "yo", "other", 10)
					data := SerializeKC.ToBytes(k)
					dk, err := Deserialize.Key(mkBuf(data))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, dk, convey.Adapt(ShouldEqualKey)(k))
				})
				t.Run("w/ ctx decodes normally w/o ctx", func(t *ftt.Test) {
					k := mkKeyCtx("aid", "ns", "knd", "yo", "other", 10)
					data := SerializeKC.ToBytes(k)
					dk, err := dwc.Key(mkBuf(data))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, dk, convey.Adapt(ShouldEqualKey)(mkKeyCtx("spam", "nerd", "knd", "yo", "other", 10)))
				})
				t.Run("w/o ctx decodes normally w/ ctx", func(t *ftt.Test) {
					k := mkKeyCtx("aid", "ns", "knd", "yo", "other", 10)
					data := Serialize.ToBytes(k)
					dk, err := Deserialize.Key(mkBuf(data))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, dk, convey.Adapt(ShouldEqualKey)(mkKeyCtx("", "", "knd", "yo", "other", 10)))
				})
				t.Run("w/o ctx decodes normally w/o ctx", func(t *ftt.Test) {
					k := mkKeyCtx("aid", "ns", "knd", "yo", "other", 10)
					data := Serialize.ToBytes(k)
					dk, err := dwc.Key(mkBuf(data))
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, dk, convey.Adapt(ShouldEqualKey)(mkKeyCtx("spam", "nerd", "knd", "yo", "other", 10)))
				})
				t.Run("IntIDs always sort before StringIDs", func(t *ftt.Test) {
					// -1 writes as almost all 1's in the first byte under cmpbin, even
					// though it's technically not a valid key.
					k := mkKeyCtx("aid", "ns", "knd", -1)
					data := Serialize.ToBytes(k)

					k = mkKeyCtx("aid", "ns", "knd", "hat")
					data2 := Serialize.ToBytes(k)

					assert.Loosely(t, string(data), should.BeLessThan(string(data2)))
				})
			})

			t.Run("err cases", func(t *ftt.Test) {
				buf := mkBuf(nil)

				t.Run("nil", func(t *ftt.Test) {
					_, err := Deserialize.Key(buf)
					assert.Loosely(t, err, should.Equal(io.EOF))
				})
				t.Run("str", func(t *ftt.Test) {
					_, err := buf.WriteString("sup")
					die(err)
					_, err = Deserialize.Key(buf)
					assert.Loosely(t, err, should.ErrLike("expected actualCtx"))
				})
				t.Run("truncated 1", func(t *ftt.Test) {
					die(buf.WriteByte(1)) // actualCtx == 1
					_, err := Deserialize.Key(buf)
					assert.Loosely(t, err, should.Equal(io.EOF))
				})
				t.Run("truncated 2", func(t *ftt.Test) {
					die(buf.WriteByte(1)) // actualCtx == 1
					ws(buf, "aid")
					_, err := Deserialize.Key(buf)
					assert.Loosely(t, err, should.Equal(io.EOF))
				})
				t.Run("truncated 3", func(t *ftt.Test) {
					die(buf.WriteByte(1)) // actualCtx == 1
					ws(buf, "aid")
					ws(buf, "ns")
					_, err := Deserialize.Key(buf)
					assert.Loosely(t, err, should.Equal(io.EOF))
				})
				t.Run("huge key", func(t *ftt.Test) {
					die(buf.WriteByte(1)) // actualCtx == 1
					ws(buf, "aid")
					ws(buf, "ns")
					for i := 1; i < 60; i++ {
						die(buf.WriteByte(1))
						die(Serialize.KeyTok(buf, KeyTok{Kind: "sup", IntID: int64(i)}))
					}
					die(buf.WriteByte(0))
					_, err := Deserialize.Key(buf)
					assert.Loosely(t, err, should.ErrLike("huge key"))
				})
				t.Run("insufficient tokens", func(t *ftt.Test) {
					die(buf.WriteByte(1)) // actualCtx == 1
					ws(buf, "aid")
					ws(buf, "ns")
					wui(buf, 2)
					_, err := Deserialize.Key(buf)
					assert.Loosely(t, err, should.Equal(io.EOF))
				})
				t.Run("partial token 1", func(t *ftt.Test) {
					die(buf.WriteByte(1)) // actualCtx == 1
					ws(buf, "aid")
					ws(buf, "ns")
					die(buf.WriteByte(1))
					ws(buf, "hi")
					_, err := Deserialize.Key(buf)
					assert.Loosely(t, err, should.Equal(io.EOF))
				})
				t.Run("partial token 2", func(t *ftt.Test) {
					die(buf.WriteByte(1)) // actualCtx == 1
					ws(buf, "aid")
					ws(buf, "ns")
					die(buf.WriteByte(1))
					ws(buf, "hi")
					die(buf.WriteByte(byte(PTString)))
					_, err := Deserialize.Key(buf)
					assert.Loosely(t, err, should.Equal(io.EOF))
				})
				t.Run("bad token (invalid type)", func(t *ftt.Test) {
					die(buf.WriteByte(1)) // actualCtx == 1
					ws(buf, "aid")
					ws(buf, "ns")
					die(buf.WriteByte(1))
					ws(buf, "hi")
					die(buf.WriteByte(byte(PTBlobKey)))
					_, err := Deserialize.Key(buf)
					assert.Loosely(t, err, should.ErrLike("invalid type PTBlobKey"))
				})
				t.Run("bad token (invalid IntID)", func(t *ftt.Test) {
					die(buf.WriteByte(1)) // actualCtx == 1
					ws(buf, "aid")
					ws(buf, "ns")
					die(buf.WriteByte(1))
					ws(buf, "hi")
					die(buf.WriteByte(byte(PTInt)))
					wi(buf, -2)
					_, err := Deserialize.Key(buf)
					assert.Loosely(t, err, should.ErrLike("zero/negative"))
				})
			})
		})

		t.Run("ReadGeoPoint", func(t *ftt.Test) {
			buf := mkBuf(nil)
			t.Run("trunc 1", func(t *ftt.Test) {
				_, err := Deserialize.GeoPoint(buf)
				assert.Loosely(t, err, should.Equal(io.EOF))
			})
			t.Run("trunc 2", func(t *ftt.Test) {
				wf(buf, 100)
				_, err := Deserialize.GeoPoint(buf)
				assert.Loosely(t, err, should.Equal(io.EOF))
			})
			t.Run("invalid", func(t *ftt.Test) {
				wf(buf, 100)
				wf(buf, 1000)
				_, err := Deserialize.GeoPoint(buf)
				assert.Loosely(t, err, should.ErrLike("invalid GeoPoint"))
			})
		})

		t.Run("WriteTime", func(t *ftt.Test) {
			t.Run("in non-UTC!", func(t *ftt.Test) {
				pst, err := time.LoadLocation("America/Los_Angeles")
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, func() {
					die(Serialize.Time(mkBuf(nil), time.Now().In(pst)))
				}, should.Panic)
			})
		})

		t.Run("ReadTime", func(t *ftt.Test) {
			t.Run("trunc 1", func(t *ftt.Test) {
				_, err := Deserialize.Time(mkBuf(nil))
				assert.Loosely(t, err, should.Equal(io.EOF))
			})
		})

		t.Run("ReadProperty", func(t *ftt.Test) {
			buf := mkBuf(nil)
			t.Run("trunc 1", func(t *ftt.Test) {
				p, err := Deserialize.Property(buf)
				assert.Loosely(t, err, should.Equal(io.EOF))
				assert.Loosely(t, p.Type(), should.Equal(PTNull))
				assert.Loosely(t, p.Value(), should.BeNil)
			})
			t.Run("trunc (PTBytes)", func(t *ftt.Test) {
				die(buf.WriteByte(byte(PTBytes)))
				_, err := Deserialize.Property(buf)
				assert.Loosely(t, err, should.Equal(io.EOF))
			})
			t.Run("trunc (PTBlobKey)", func(t *ftt.Test) {
				die(buf.WriteByte(byte(PTBlobKey)))
				_, err := Deserialize.Property(buf)
				assert.Loosely(t, err, should.Equal(io.EOF))
			})
			t.Run("invalid type", func(t *ftt.Test) {
				die(buf.WriteByte(byte(PTUnknown + 1)))
				_, err := Deserialize.Property(buf)
				assert.Loosely(t, err, should.ErrLike("unknown type!"))
			})
		})

		t.Run("ReadPropertyMap", func(t *ftt.Test) {
			buf := mkBuf(nil)
			t.Run("trunc 1", func(t *ftt.Test) {
				_, err := Deserialize.PropertyMap(buf)
				assert.Loosely(t, err, should.Equal(io.EOF))
			})
			t.Run("too many rows", func(t *ftt.Test) {
				wui(buf, 1000000)
				_, err := Deserialize.PropertyMap(buf)
				assert.Loosely(t, err, should.ErrLike("huge number of rows"))
			})
			t.Run("trunc 2", func(t *ftt.Test) {
				wui(buf, 10)
				_, err := Deserialize.PropertyMap(buf)
				assert.Loosely(t, err, should.Equal(io.EOF))
			})
			t.Run("trunc 3", func(t *ftt.Test) {
				wui(buf, 10)
				ws(buf, "ohai")
				_, err := Deserialize.PropertyMap(buf)
				assert.Loosely(t, err, should.Equal(io.EOF))
			})
			t.Run("too many values", func(t *ftt.Test) {
				wui(buf, 10)
				ws(buf, "ohai")
				wui(buf, 100000)
				_, err := Deserialize.PropertyMap(buf)
				assert.Loosely(t, err, should.ErrLike("huge number of properties"))
			})
			t.Run("trunc 4", func(t *ftt.Test) {
				wui(buf, 10)
				ws(buf, "ohai")
				wui(buf, 10)
				_, err := Deserialize.PropertyMap(buf)
				assert.Loosely(t, err, should.Equal(io.EOF))
			})
		})

		t.Run("IndexDefinition", func(t *ftt.Test) {
			id := IndexDefinition{Kind: "kind"}
			data := Serialize.ToBytes(*id.PrepForIdxTable())
			newID, err := Deserialize.IndexDefinition(mkBuf(data))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, newID.Flip(), should.Resemble(id.Normalize()))

			id.SortBy = append(id.SortBy, IndexColumn{Property: "prop"})
			data = Serialize.ToBytes(*id.PrepForIdxTable())
			newID, err = Deserialize.IndexDefinition(mkBuf(data))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, newID.Flip(), should.Resemble(id.Normalize()))

			id.SortBy = append(id.SortBy, IndexColumn{Property: "other", Descending: true})
			id.Ancestor = true
			data = Serialize.ToBytes(*id.PrepForIdxTable())
			newID, err = Deserialize.IndexDefinition(mkBuf(data))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, newID.Flip(), should.Resemble(id.Normalize()))

			// invalid
			id.SortBy = append(id.SortBy, IndexColumn{Property: "", Descending: true})
			data = Serialize.ToBytes(*id.PrepForIdxTable())
			newID, err = Deserialize.IndexDefinition(mkBuf(data))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, newID.Flip(), should.Resemble(id.Normalize()))

			t.Run("too many", func(t *ftt.Test) {
				id := IndexDefinition{Kind: "wat"}
				for i := 0; i < maxIndexColumns+1; i++ {
					id.SortBy = append(id.SortBy, IndexColumn{Property: "Hi", Descending: true})
				}
				data := Serialize.ToBytes(*id.PrepForIdxTable())
				newID, err = Deserialize.IndexDefinition(mkBuf(data))
				assert.Loosely(t, err, should.ErrLike("over 64 sort orders"))
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

	ftt.Run("TestIndexedProperties", t, func(t *ftt.Test) {
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

		t.Run("IndexedProperties", func(t *ftt.Test) {
			sip := Serialize.IndexedProperties(fakeKey, pm)
			assert.Loosely(t, len(sip), should.Equal(8))
			sip.Sort()

			assert.Loosely(t, sip, should.Resemble(IndexedProperties{
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
			}))
		})

		t.Run("IndexedPropertiesForIndicies", func(t *ftt.Test) {
			sip := Serialize.IndexedPropertiesForIndicies(fakeKey, pm, []IndexColumn{
				{Property: "wat"},
				{Property: "wat", Descending: true},
				{Property: "unknown"},
				{Property: "nested.__key__"},
			})
			assert.Loosely(t, len(sip), should.Equal(4))
			sip.Sort()

			assert.Loosely(t, sip, should.Resemble(IndexedProperties{
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
			}))
		})
	})
}
