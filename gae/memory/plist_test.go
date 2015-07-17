// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"infra/gae/libs/gae"
	"testing"
	"time"

	"github.com/luci/gkvlite"
	. "github.com/smartystreets/goconvey/convey"
)

func init() {
	indexCreationDeterministic = true
}

var fakeKey = key("knd", 10, key("parentKind", "sid"))

func TestCollated(t *testing.T) {
	t.Parallel()

	Convey("TestCollated", t, func() {
		Convey("nil list", func() {
			pm := (gae.DSPropertyMap)(nil)
			sip := partiallySerialize(pm)
			So(sip, ShouldBeNil)

			Convey("nil collated", func() {
				Convey("defaultIndicies", func() {
					idxs := defaultIndicies("knd", pm)
					So(len(idxs), ShouldEqual, 1)
					So(idxs[0].String(), ShouldEqual, "B:knd")
				})
				Convey("indexEntries", func() {
					s := sip.indexEntries(fakeKey, defaultIndicies("knd", pm))
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
			pm := gae.DSPropertyMap{
				"wat":  {propNI("thing"), prop("hat"), prop(100)},
				"nerd": {prop(103.7)},
				"spaz": {propNI(false)},
			}
			sip := partiallySerialize(pm)
			So(len(sip), ShouldEqual, 2)

			Convey("single collated", func() {
				Convey("indexableMap", func() {
					So(sip, ShouldResemble, serializedIndexablePmap{
						"wat": {
							cat(gae.DSPTInt, 100),
							cat(gae.DSPTString, "hat"),
							// 'thing' is skipped, because it's not NoIndex
						},
						"nerd": {
							cat(gae.DSPTFloat, 103.7),
						},
					})
				})
				Convey("defaultIndicies", func() {
					idxs := defaultIndicies("knd", pm)
					So(len(idxs), ShouldEqual, 5)
					So(idxs[0].String(), ShouldEqual, "B:knd")
					So(idxs[1].String(), ShouldEqual, "B:knd/-nerd")
					So(idxs[2].String(), ShouldEqual, "B:knd/-wat")
					So(idxs[3].String(), ShouldEqual, "B:knd/nerd")
					So(idxs[4].String(), ShouldEqual, "B:knd/wat")
				})
			})
		})
	})
}

var rgenComplexTime = time.Date(
	1986, time.October, 26, 1, 20, 00, 00, time.UTC)
var rgenComplexKey = key("kind", "id")

var rowGenTestCases = []struct {
	name        string
	pmap        gae.DSPropertyMap
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
		pmap: gae.DSPropertyMap{
			"wat":  {propNI("thing"), prop("hat"), prop(100)},
			"nerd": {prop(103.7)},
			"spaz": {propNI(false)},
		},
		withBuiltin: true,
		idxs: []*qIndex{
			indx("knd", "-wat", "nerd"),
		},
		expected: []serializedPvals{
			{{}}, // B:knd
			{icat(gae.DSPTFloat, 103.7)},                          // B:knd/-nerd
			{icat(gae.DSPTString, "hat"), icat(gae.DSPTInt, 100)}, // B:knd/-wat
			{cat(gae.DSPTFloat, 103.7)},                           // B:knd/nerd
			{cat(gae.DSPTInt, 100), cat(gae.DSPTString, "hat")},   // B:knd/wat
			{ // B:knd/-wat/nerd
				cat(icat(gae.DSPTString, "hat"), cat(gae.DSPTFloat, 103.7)),
				cat(icat(gae.DSPTInt, 100), cat(gae.DSPTFloat, 103.7)),
			},
		},
		collections: map[string][]kv{
			"idx": {
				// 0 == builtin, 1 == complex
				{cat(byte(0), "knd", byte(1), 0), []byte{}},
				{cat(byte(0), "knd", byte(1), 1, byte(0), "nerd"), []byte{}},
				{cat(byte(0), "knd", byte(1), 1, byte(0), "wat"), []byte{}},
				{cat(byte(0), "knd", byte(1), 1, byte(1), "nerd"), []byte{}},
				{cat(byte(0), "knd", byte(1), 1, byte(1), "wat"), []byte{}},
				{cat(byte(1), "knd", byte(1), 2, byte(1), "wat", byte(0), "nerd"), []byte{}},
			},
			"idx:ns:" + sat(indx("knd")): {
				{cat(fakeKey), []byte{}},
			},
			"idx:ns:" + sat(indx("knd", "wat")): {
				{cat(gae.DSPTInt, 100, fakeKey), []byte{}},
				{cat(gae.DSPTString, "hat", fakeKey), cat(gae.DSPTInt, 100)},
			},
			"idx:ns:" + sat(indx("knd", "-wat")): {
				{cat(icat(gae.DSPTString, "hat"), fakeKey), []byte{}},
				{cat(icat(gae.DSPTInt, 100), fakeKey), icat(gae.DSPTString, "hat")},
			},
		},
	},
	{
		name: "complex",
		pmap: gae.DSPropertyMap{
			"yerp": {prop("hat"), prop(73.9)},
			"wat": {
				prop(rgenComplexTime),
				prop(gae.DSByteString("value")),
				prop(rgenComplexKey)},
			"spaz": {prop(nil), prop(false), prop(true)},
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
				cat(cat(gae.DSPTString, "hat"), icat(gae.DSPTKey, rgenComplexKey), cat(gae.DSPTNull)),
				cat(cat(gae.DSPTString, "hat"), icat(gae.DSPTKey, rgenComplexKey), cat(gae.DSPTBoolFalse)),
				cat(cat(gae.DSPTString, "hat"), icat(gae.DSPTKey, rgenComplexKey), cat(gae.DSPTBoolTrue)),
				cat(cat(gae.DSPTString, "hat"), icat(gae.DSPTBytes, "value"), cat(gae.DSPTNull)),
				cat(cat(gae.DSPTString, "hat"), icat(gae.DSPTBytes, "value"), cat(gae.DSPTBoolFalse)),
				cat(cat(gae.DSPTString, "hat"), icat(gae.DSPTBytes, "value"), cat(gae.DSPTBoolTrue)),
				cat(cat(gae.DSPTString, "hat"), icat(gae.DSPTTime, rgenComplexTime), cat(gae.DSPTNull)),
				cat(cat(gae.DSPTString, "hat"), icat(gae.DSPTTime, rgenComplexTime), cat(gae.DSPTBoolFalse)),
				cat(cat(gae.DSPTString, "hat"), icat(gae.DSPTTime, rgenComplexTime), cat(gae.DSPTBoolTrue)),

				cat(cat(gae.DSPTFloat, 73.9), icat(gae.DSPTKey, rgenComplexKey), cat(gae.DSPTNull)),
				cat(cat(gae.DSPTFloat, 73.9), icat(gae.DSPTKey, rgenComplexKey), cat(gae.DSPTBoolFalse)),
				cat(cat(gae.DSPTFloat, 73.9), icat(gae.DSPTKey, rgenComplexKey), cat(gae.DSPTBoolTrue)),
				cat(cat(gae.DSPTFloat, 73.9), icat(gae.DSPTBytes, "value"), cat(gae.DSPTNull)),
				cat(cat(gae.DSPTFloat, 73.9), icat(gae.DSPTBytes, "value"), cat(gae.DSPTBoolFalse)),
				cat(cat(gae.DSPTFloat, 73.9), icat(gae.DSPTBytes, "value"), cat(gae.DSPTBoolTrue)),
				cat(cat(gae.DSPTFloat, 73.9), icat(gae.DSPTTime, rgenComplexTime), cat(gae.DSPTNull)),
				cat(cat(gae.DSPTFloat, 73.9), icat(gae.DSPTTime, rgenComplexTime), cat(gae.DSPTBoolFalse)),
				cat(cat(gae.DSPTFloat, 73.9), icat(gae.DSPTTime, rgenComplexTime), cat(gae.DSPTBoolTrue)),
			},
		},
	},
	{
		name: "ancestor",
		pmap: gae.DSPropertyMap{
			"wat": {prop("sup")},
		},
		idxs: []*qIndex{
			indx("knd!", "wat"),
		},
		collections: map[string][]kv{
			"idx:ns:" + sat(indx("knd!", "wat")): {
				{cat(fakeKey.Parent(), gae.DSPTString, "sup", fakeKey), []byte{}},
				{cat(fakeKey, gae.DSPTString, "sup", fakeKey), []byte{}},
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
				mvals := partiallySerialize(tc.pmap)
				idxs := []*qIndex(nil)
				if tc.withBuiltin {
					idxs = append(defaultIndicies("coolKind", tc.pmap), tc.idxs...)
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
								So([]byte(row), ShouldResemble, tc.expected[i][j])
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
				if tc.withBuiltin {
					store = indexEntriesWithBuiltins(fakeKey, tc.pmap, tc.idxs)
				} else {
					store = partiallySerialize(tc.pmap).indexEntries(fakeKey, tc.idxs)
				}
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
	key   gae.DSKey
	props gae.DSPropertyMap
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
			{key("knd", 1), gae.DSPropertyMap{
				"wat":  {prop(10)},
				"yerp": {prop(10)}},
			},
			{key("knd", 10), gae.DSPropertyMap{
				"wat":  {prop(1)},
				"yerp": {prop(200)}},
			},
			{key("knd", 1), gae.DSPropertyMap{
				"wat":  {prop(10)},
				"yerp": {prop(202)}},
			},
		},
		expected: map[string][][]byte{
			"idx:ns:" + sat(indx("knd", "wat")): {
				cat(gae.DSPTInt, 1, key("knd", 10)),
				cat(gae.DSPTInt, 10, key("knd", 1)),
			},
			"idx:ns:" + sat(indx("knd", "-wat")): {
				cat(icat(gae.DSPTInt, 10), key("knd", 1)),
				cat(icat(gae.DSPTInt, 1), key("knd", 10)),
			},
			"idx:ns:" + sat(indx("knd", "yerp")): {
				cat(gae.DSPTInt, 200, key("knd", 10)),
				cat(gae.DSPTInt, 202, key("knd", 1)),
			},
		},
	},
	{
		name: "compound",
		idxs: []*qIndex{indx("knd", "yerp", "-wat")},
		data: []dumbItem{
			{key("knd", 1), gae.DSPropertyMap{
				"wat":  {prop(10)},
				"yerp": {prop(100)}},
			},
			{key("knd", 10), gae.DSPropertyMap{
				"wat":  {prop(1)},
				"yerp": {prop(200)}},
			},
			{key("knd", 11), gae.DSPropertyMap{
				"wat":  {prop(20)},
				"yerp": {prop(200)}},
			},
			{key("knd", 14), gae.DSPropertyMap{
				"wat":  {prop(20)},
				"yerp": {prop(200)}},
			},
			{key("knd", 1), gae.DSPropertyMap{
				"wat":  {prop(10)},
				"yerp": {prop(202)}},
			},
		},
		expected: map[string][][]byte{
			"idx:ns:" + sat(indx("knd", "yerp", "-wat")): {
				cat(gae.DSPTInt, 200, icat(gae.DSPTInt, 20), key("knd", 11)),
				cat(gae.DSPTInt, 200, icat(gae.DSPTInt, 20), key("knd", 14)),
				cat(gae.DSPTInt, 200, icat(gae.DSPTInt, 1), key("knd", 10)),
				cat(gae.DSPTInt, 202, icat(gae.DSPTInt, 10), key("knd", 1)),
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

				tmpLoader := map[string]gae.DSPropertyMap{}
				for _, itm := range tc.data {
					ks := itm.key.String()
					prev := tmpLoader[ks]
					updateIndicies(store, itm.key, prev, itm.props)
					tmpLoader[ks] = itm.props
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
