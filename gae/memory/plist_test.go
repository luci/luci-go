// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"testing"
	"time"

	"github.com/luci/gkvlite"
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
				pl := &propertyList{prop("Bob", 301.23)}
				data, err := pl.MarshalBinary()
				So(err, ShouldBeNil)
				pl2 := &propertyList{}

				err = pl2.UnmarshalBinary(data)
				So(err, ShouldBeNil)
				So(pl, ShouldResemble, pl2)
			})

			Convey("one of each", func() {
				pl := &propertyList{
					prop("null", nil),
					prop("int", int64(100)),
					prop("time", time.Now().Round(time.Microsecond)),
					prop("float", float64(301.23)),
					prop("bool", true),
					prop("bool", false),
					prop("bool", "mixed types are allowed!"),
					prop("bool", true),
					prop("[]byte", []byte("sup"), true),
					prop("ByteString", datastore.ByteString("sup")),
					prop("BlobKey", appengine.BlobKey("bkey")),
					prop("string", "stringy"),
					prop("GeoPoint", appengine.GeoPoint{Lat: 123.3, Lng: 456.6}),
					prop("*Key", fakeKey),
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

var fakeKey = key("knd", 10, key("parentKind", "sid"))

func TestCollated(t *testing.T) {
	t.Parallel()

	Convey("TestCollated", t, func() {
		Convey("nil list", func() {
			pl := (*propertyList)(nil)
			c, err := pl.collate()
			So(err, ShouldBeNil)
			So(c, ShouldBeNil)

			Convey("nil collated", func() {
				Convey("indexableMap", func() {
					m, err := c.indexableMap()
					So(err, ShouldBeNil)
					So(m, ShouldBeEmpty)
				})
				Convey("defaultIndicies", func() {
					idxs := c.defaultIndicies("knd")
					So(len(idxs), ShouldEqual, 1)
					So(idxs[0].String(), ShouldEqual, "B:knd")
				})
				Convey("indexEntries", func() {
					s, err := c.indexEntries(fakeKey, c.defaultIndicies("knd"))
					So(err, ShouldBeNil)
					numItems, _ := s.GetCollection("idx").GetTotals()
					So(numItems, ShouldEqual, 1)
					itm := s.GetCollection("idx").MinItem(false)
					So(itm.Key, ShouldResemble, cat(indx("knd")))
					numItems, _ = s.GetCollection("idx:ns:" + string(itm.Key)).GetTotals()
					So(numItems, ShouldEqual, 1)
				})
			})
		})

		Convey("list", func() {
			pl := &propertyList{
				// intentionally out of order
				prop("wat", "thing", true),
				prop("nerd", 103.7),
				prop("wat", "hat"),
				prop("wat", int64(100)),
				prop("spaz", false, true),
			}
			c, err := pl.collate()
			So(err, ShouldBeNil)
			So(len(c), ShouldEqual, 3)
			So(len(c[0].vals), ShouldEqual, 3)
			So(len(c[1].vals), ShouldEqual, 1)
			So(len(c[2].vals), ShouldEqual, 1)
			// collate keeps first-seen order, discards interleaving.
			So(c[0].vals[0].typ, ShouldEqual, pvStr)
			So(c[0].vals[1].typ, ShouldEqual, pvStr)
			So(c[0].vals[2].typ, ShouldEqual, pvInt)
			So(c[1].vals[0].typ, ShouldEqual, pvFloat)
			So(c[2].vals[0].typ, ShouldEqual, pvBoolFalse)

			Convey("single collated", func() {
				Convey("indexableMap", func() {
					m, err := c.indexableMap()
					So(err, ShouldBeNil)
					So(m, ShouldResemble, mappedPlist{
						"wat": {
							cat(pvInt, 100),
							cat(pvStr, "hat"),
							// 'thing' is skipped, because it's not NoIndex
						},
						"nerd": {
							cat(pvFloat, 103.7),
						},
					})
				})
				Convey("defaultIndicies", func() {
					idxs := c.defaultIndicies("knd")
					So(len(idxs), ShouldEqual, 5)
					So(idxs[0].String(), ShouldEqual, "B:knd")
					So(idxs[1].String(), ShouldEqual, "B:knd/wat")
					So(idxs[2].String(), ShouldEqual, "B:knd/-wat")
					So(idxs[3].String(), ShouldEqual, "B:knd/nerd")
					So(idxs[4].String(), ShouldEqual, "B:knd/-nerd")
				})
			})
		})
	})
}

var rgenComplexTime = time.Date(
	1986, time.October, 26, 1, 20, 00, 00,
	mustLoadLocation("America/Los_Angeles"))
var rgenComplexKey = key("kind", "id")

var rowGenTestCases = []struct {
	name        string
	plist       *propertyList
	withBuiltin bool
	idxs        []*qIndex

	// These are checked in TestIndexRowGen. nil to skip test case.
	expected []serializedPvals

	// just the collections you want to assert. These are checked in
	// TestIndexEntries. nil to skip test case.
	collections map[string][]kv
}{
	{
		name: "simple including builtins",
		plist: &propertyList{
			// intentionally out of order
			prop("wat", "thing", true),
			prop("nerd", 103.7),
			prop("wat", "hat"),
			prop("wat", int64(100)),
			prop("spaz", false, true),
		},
		withBuiltin: true,
		idxs: []*qIndex{
			indx("knd", "-wat", "nerd"),
		},
		expected: []serializedPvals{
			{{}}, // B:knd
			{cat(pvInt, 100), cat(pvStr, "hat")},   // B:knd/wat
			{icat(pvStr, "hat"), icat(pvInt, 100)}, // B:knd/-wat
			{cat(pvFloat, 103.7)},                  // B:knd/nerd
			{icat(pvFloat, 103.7)},                 // B:knd/-nerd
			{ // B:knd/-wat/nerd
				cat(icat(pvStr, "hat"), cat(pvFloat, 103.7)),
				cat(icat(pvInt, 100), cat(pvFloat, 103.7)),
			},
		},
		collections: map[string][]kv{
			"idx": {
				// 0 == builtin, 1 == complex
				{cat(byte(0), "knd", byte(1), 0), []byte{}},
				{cat(byte(0), "knd", byte(1), 1, byte(0), "wat"), []byte{}},
				{cat(byte(0), "knd", byte(1), 1, byte(0), "nerd"), []byte{}},
				{cat(byte(0), "knd", byte(1), 1, byte(1), "wat"), []byte{}},
				{cat(byte(0), "knd", byte(1), 1, byte(1), "nerd"), []byte{}},
				{cat(byte(1), "knd", byte(1), 2, byte(1), "wat", byte(0), "nerd"), []byte{}},
			},
			"idx:ns:" + sat(indx("knd")): {
				{cat(fakeKey), []byte{}},
			},
			"idx:ns:" + sat(indx("knd", "wat")): {
				{cat(pvInt, 100, fakeKey), []byte{}},
				{cat(pvStr, "hat", fakeKey), cat(pvInt, 100)},
			},
			"idx:ns:" + sat(indx("knd", "-wat")): {
				{cat(icat(pvStr, "hat"), fakeKey), []byte{}},
				{cat(icat(pvInt, 100), fakeKey), icat(pvStr, "hat")},
			},
		},
	},
	{
		name: "complex",
		plist: &propertyList{
			// in order for sanity, grouped by property.
			prop("yerp", "hat"),
			prop("yerp", 73.9),

			prop("wat", rgenComplexTime),
			prop("wat", datastore.ByteString("value")),
			prop("wat", rgenComplexKey),

			prop("spaz", nil),
			prop("spaz", false),
			prop("spaz", true),
		},
		idxs: []*qIndex{
			indx("knd", "-wat", "nerd", "spaz"), // doesn't match, so empty
			indx("knd", "yerp", "-wat", "spaz"),
		},
		expected: []serializedPvals{
			{}, // C:knd/-wat/nerd/spaz, no match
			{ // C:knd/yerp/-wat/spaz
				// thank goodness the binary serialization only happens 1/val in the
				// real code :).
				cat(cat(pvStr, "hat"), icat(pvKey, rgenComplexKey), cat(pvNull)),
				cat(cat(pvStr, "hat"), icat(pvKey, rgenComplexKey), cat(pvBoolFalse)),
				cat(cat(pvStr, "hat"), icat(pvKey, rgenComplexKey), cat(pvBoolTrue)),
				cat(cat(pvStr, "hat"), icat(pvBytes, "value"), cat(pvNull)),
				cat(cat(pvStr, "hat"), icat(pvBytes, "value"), cat(pvBoolFalse)),
				cat(cat(pvStr, "hat"), icat(pvBytes, "value"), cat(pvBoolTrue)),
				cat(cat(pvStr, "hat"), icat(pvTime, rgenComplexTime), cat(pvNull)),
				cat(cat(pvStr, "hat"), icat(pvTime, rgenComplexTime), cat(pvBoolFalse)),
				cat(cat(pvStr, "hat"), icat(pvTime, rgenComplexTime), cat(pvBoolTrue)),

				cat(cat(pvFloat, 73.9), icat(pvKey, rgenComplexKey), cat(pvNull)),
				cat(cat(pvFloat, 73.9), icat(pvKey, rgenComplexKey), cat(pvBoolFalse)),
				cat(cat(pvFloat, 73.9), icat(pvKey, rgenComplexKey), cat(pvBoolTrue)),
				cat(cat(pvFloat, 73.9), icat(pvBytes, "value"), cat(pvNull)),
				cat(cat(pvFloat, 73.9), icat(pvBytes, "value"), cat(pvBoolFalse)),
				cat(cat(pvFloat, 73.9), icat(pvBytes, "value"), cat(pvBoolTrue)),
				cat(cat(pvFloat, 73.9), icat(pvTime, rgenComplexTime), cat(pvNull)),
				cat(cat(pvFloat, 73.9), icat(pvTime, rgenComplexTime), cat(pvBoolFalse)),
				cat(cat(pvFloat, 73.9), icat(pvTime, rgenComplexTime), cat(pvBoolTrue)),
			},
		},
	},
	{
		name: "ancestor",
		plist: &propertyList{
			prop("wat", "sup"),
		},
		idxs: []*qIndex{
			indx("knd!", "wat"),
		},
		collections: map[string][]kv{
			"idx:ns:" + sat(indx("knd!", "wat")): {
				{cat(fakeKey.Parent(), pvStr, "sup", fakeKey), []byte{}},
				{cat(fakeKey, pvStr, "sup", fakeKey), []byte{}},
			},
		},
	},
}

func TestIndexRowGen(t *testing.T) {
	t.Parallel()

	Convey("Test Index Row Generation", t, func() {
		for _, tc := range rowGenTestCases {
			if tc.expected == nil {
				Convey(tc.name, nil) // shows up as 'skipped'
				continue
			}

			Convey(tc.name, func() {
				c, err := tc.plist.collate()
				if err != nil {
					panic(err)
				}
				mvals, err := c.indexableMap()
				if err != nil {
					panic(err)
				}
				idxs := []*qIndex(nil)
				if tc.withBuiltin {
					idxs = append(c.defaultIndicies("coolKind"), tc.idxs...)
				} else {
					idxs = tc.idxs
				}

				m := matcher{}
				for i, idx := range idxs {
					Convey(idx.String(), func() {
						iGen, ok := m.match(idx, mvals)
						if len(tc.expected[i]) > 0 {
							So(ok, ShouldBeTrue)
							j := 0
							iGen.permute(func(row []byte) {
								So(serializedPval(row), ShouldResemble, tc.expected[i][j])
								j++
							})
							So(j, ShouldEqual, len(tc.expected[i]))
						} else {
							So(ok, ShouldBeFalse)
						}
					})
				}
			})
		}
	})
}

func TestIndexEntries(t *testing.T) {
	t.Parallel()

	Convey("Test indexEntriesWithBuiltins", t, func() {
		for _, tc := range rowGenTestCases {
			if tc.collections == nil {
				Convey(tc.name, nil) // shows up as 'skipped'
				continue
			}

			Convey(tc.name, func() {
				store := (*memStore)(nil)
				err := error(nil)
				if tc.withBuiltin {
					store, err = tc.plist.indexEntriesWithBuiltins(fakeKey, tc.idxs)
				} else {
					c, err := tc.plist.collate()
					if err == nil {
						store, err = c.indexEntries(fakeKey, tc.idxs)
					}
				}
				So(err, ShouldBeNil)
				for colName, vals := range tc.collections {
					i := 0
					store.GetCollection(colName).VisitItemsAscend(nil, true, func(itm *gkvlite.Item) bool {
						So(itm.Key, ShouldResemble, vals[i].k)
						So(itm.Val, ShouldResemble, vals[i].v)
						i++
						return true
					})
					So(i, ShouldEqual, len(vals))
				}
			})
		}
	})
}

type dumbItem struct {
	key  *datastore.Key
	prop *propertyList
}

var updateIndiciesTests = []struct {
	name     string
	idxs     []*qIndex
	data     []dumbItem
	expected map[string][][]byte
}{
	{
		name: "basic",
		data: []dumbItem{
			{key("knd", 1),
				pl(prop("wat", int64(10)), prop("yerp", int64(100)))},
			{key("knd", 10),
				pl(prop("wat", int64(1)), prop("yerp", int64(200)))},
			{key("knd", 1),
				pl(prop("wat", int64(10)), prop("yerp", int64(202)))},
		},
		expected: map[string][][]byte{
			"idx:ns:" + sat(indx("knd", "wat")): {
				cat(pvInt, 1, key("knd", 10)),
				cat(pvInt, 10, key("knd", 1)),
			},
			"idx:ns:" + sat(indx("knd", "-wat")): {
				cat(icat(pvInt, 10), key("knd", 1)),
				cat(icat(pvInt, 1), key("knd", 10)),
			},
			"idx:ns:" + sat(indx("knd", "yerp")): {
				cat(pvInt, 200, key("knd", 10)),
				cat(pvInt, 202, key("knd", 1)),
			},
		},
	},
	{
		name: "compound",
		idxs: []*qIndex{indx("knd", "yerp", "-wat")},
		data: []dumbItem{
			{key("knd", 1),
				pl(prop("wat", int64(10)), prop("yerp", int64(100)))},
			{key("knd", 10),
				pl(prop("wat", int64(1)), prop("yerp", int64(200)))},
			{key("knd", 11),
				pl(prop("wat", int64(20)), prop("yerp", int64(200)))},
			{key("knd", 14),
				pl(prop("wat", int64(20)), prop("yerp", int64(200)))},
			{key("knd", 1),
				pl(prop("wat", int64(10)), prop("yerp", int64(202)))},
		},
		expected: map[string][][]byte{
			"idx:ns:" + sat(indx("knd", "yerp", "-wat")): {
				cat(pvInt, 200, icat(pvInt, 20), key("knd", 11)),
				cat(pvInt, 200, icat(pvInt, 20), key("knd", 14)),
				cat(pvInt, 200, icat(pvInt, 1), key("knd", 10)),
				cat(pvInt, 202, icat(pvInt, 10), key("knd", 1)),
			},
		},
	},
}

func TestUpdateIndicies(t *testing.T) {
	t.Parallel()

	Convey("Test updateIndicies", t, func() {
		for _, tc := range updateIndiciesTests {
			Convey(tc.name, func() {
				store := newMemStore()
				idxColl := store.SetCollection("idx", nil)
				for _, i := range tc.idxs {
					idxColl.Set(cat(i), []byte{})
				}

				tmpLoader := map[string]*propertyList{}
				for _, itm := range tc.data {
					ks := itm.key.String()
					prev := tmpLoader[ks]
					err := updateIndicies(store, itm.key, prev, itm.prop)
					So(err, ShouldBeNil)
					tmpLoader[ks] = itm.prop
				}
				tmpLoader = nil

				for colName, data := range tc.expected {
					coll := store.GetCollection(colName)
					So(coll, ShouldNotBeNil)
					i := 0
					coll.VisitItemsAscend(nil, false, func(itm *gkvlite.Item) bool {
						So(data[i], ShouldResemble, itm.Key)
						i++
						return true
					})
					So(i, ShouldEqual, len(data))
				}
			})
		}
	})
}
