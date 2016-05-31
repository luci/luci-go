// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package memory

import (
	"sort"
	"testing"
	"time"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/serialize"
	"github.com/luci/gkvlite"
	. "github.com/smartystreets/goconvey/convey"
)

var fakeKey = key("parentKind", "sid", "knd", 10)

var rgenComplexTime = time.Date(
	1986, time.October, 26, 1, 20, 00, 00, time.UTC)
var rgenComplexKey = key("kind", "id")

var _, rgenComplexTimeInt = prop(rgenComplexTime).IndexTypeAndValue()
var rgenComplexTimeIdx = prop(rgenComplexTimeInt)

var rowGenTestCases = []struct {
	name        string
	pmap        ds.PropertyMap
	withBuiltin bool
	idxs        []*ds.IndexDefinition

	// These are checked in TestIndexRowGen. nil to skip test case.
	expected []serialize.SerializedPslice

	// just the collections you want to assert. These are checked in
	// TestIndexEntries. nil to skip test case.
	collections map[string][][]byte
}{

	{
		name: "simple including builtins",
		pmap: ds.PropertyMap{
			"wat":  {propNI("thing"), prop("hat"), prop(100)},
			"nerd": {prop(103.7)},
			"spaz": {propNI(false)},
		},
		withBuiltin: true,
		idxs: []*ds.IndexDefinition{
			indx("knd", "-wat", "nerd"),
		},
		expected: []serialize.SerializedPslice{
			{cat(prop(fakeKey))},              // B:knd
			{cat(prop(103.7), prop(fakeKey))}, // B:knd/nerd
			{ // B:knd/wat
				cat(prop(100), prop(fakeKey)),
				cat(prop("hat"), prop(fakeKey)),
			},
			{ // B:knd/-nerd
				cat(icat(prop(103.7)), prop(fakeKey)),
			},
			{ // B:knd/-wat
				cat(icat(prop("hat")), prop(fakeKey)),
				cat(icat(prop(100)), prop(fakeKey)),
			},
			{ // C:knd/-wat/nerd
				cat(icat(prop("hat")), prop(103.7), prop(fakeKey)),
				cat(icat(prop(100)), prop(103.7), prop(fakeKey)),
			},
		},

		collections: map[string][][]byte{
			"idx": {
				cat("knd", byte(0), byte(1), byte(0), "__key__", byte(0)),
				cat("knd", byte(0), byte(1), byte(0), "__key__", byte(1), byte(0), "nerd", byte(0)),
				cat("knd", byte(0), byte(1), byte(0), "__key__", byte(1), byte(0), "nerd", byte(1), byte(1), "wat", byte(0)),
				cat("knd", byte(0), byte(1), byte(0), "__key__", byte(1), byte(0), "wat", byte(0)),
				cat("knd", byte(0), byte(1), byte(0), "__key__", byte(1), byte(1), "nerd", byte(0)),
				cat("knd", byte(0), byte(1), byte(0), "__key__", byte(1), byte(1), "wat", byte(0)),
			},
			"idx:ns:" + sat(indx("knd").PrepForIdxTable()): {
				cat(prop(fakeKey)),
			},
			"idx:ns:" + sat(indx("knd", "wat").PrepForIdxTable()): {
				cat(prop(100), prop(fakeKey)),
				cat(prop("hat"), prop(fakeKey)),
			},
			"idx:ns:" + sat(indx("knd", "-wat").PrepForIdxTable()): {
				cat(icat(prop("hat")), prop(fakeKey)),
				cat(icat(prop(100)), prop(fakeKey)),
			},
		},
	},

	{
		name: "complex",
		pmap: ds.PropertyMap{
			"yerp": {prop("hat"), prop(73.9)},
			"wat": {
				prop(rgenComplexTime),
				prop([]byte("value")),
				prop(rgenComplexKey)},
			"spaz": {prop(nil), prop(false), prop(true)},
		},
		idxs: []*ds.IndexDefinition{
			indx("knd", "-wat", "nerd", "spaz"), // doesn't match, so empty
			indx("knd", "yerp", "-wat", "spaz"),
		},
		expected: []serialize.SerializedPslice{
			{}, // C:knd/-wat/nerd/spaz, no match
			{ // C:knd/yerp/-wat/spaz
				// thank goodness the binary serialization only happens 1/val in the
				// real code :).
				cat(prop("hat"), icat(prop(rgenComplexKey)), prop(nil), prop(fakeKey)),
				cat(prop("hat"), icat(prop(rgenComplexKey)), prop(false), prop(fakeKey)),
				cat(prop("hat"), icat(prop(rgenComplexKey)), prop(true), prop(fakeKey)),
				cat(prop("hat"), icat(prop("value")), prop(nil), prop(fakeKey)),
				cat(prop("hat"), icat(prop("value")), prop(false), prop(fakeKey)),
				cat(prop("hat"), icat(prop("value")), prop(true), prop(fakeKey)),
				cat(prop("hat"), icat(rgenComplexTimeIdx), prop(nil), prop(fakeKey)),
				cat(prop("hat"), icat(rgenComplexTimeIdx), prop(false), prop(fakeKey)),
				cat(prop("hat"), icat(rgenComplexTimeIdx), prop(true), prop(fakeKey)),

				cat(prop(73.9), icat(prop(rgenComplexKey)), prop(nil), prop(fakeKey)),
				cat(prop(73.9), icat(prop(rgenComplexKey)), prop(false), prop(fakeKey)),
				cat(prop(73.9), icat(prop(rgenComplexKey)), prop(true), prop(fakeKey)),
				cat(prop(73.9), icat(prop("value")), prop(nil), prop(fakeKey)),
				cat(prop(73.9), icat(prop("value")), prop(false), prop(fakeKey)),
				cat(prop(73.9), icat(prop("value")), prop(true), prop(fakeKey)),
				cat(prop(73.9), icat(rgenComplexTimeIdx), prop(nil), prop(fakeKey)),
				cat(prop(73.9), icat(rgenComplexTimeIdx), prop(false), prop(fakeKey)),
				cat(prop(73.9), icat(rgenComplexTimeIdx), prop(true), prop(fakeKey)),
			},
		},
	},

	{
		name: "ancestor",
		pmap: ds.PropertyMap{
			"wat": {prop("sup")},
		},
		idxs: []*ds.IndexDefinition{
			indx("knd!", "wat"),
		},
		collections: map[string][][]byte{
			"idx:ns:" + sat(indx("knd!", "wat").PrepForIdxTable()): {
				cat(prop(fakeKey.Parent()), prop("sup"), prop(fakeKey)),
				cat(prop(fakeKey), prop("sup"), prop(fakeKey)),
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
				mvals := serialize.PropertyMapPartially(fakeKey, tc.pmap)
				idxs := []*ds.IndexDefinition(nil)
				if tc.withBuiltin {
					idxs = append(defaultIndexes("coolKind", tc.pmap), tc.idxs...)
				} else {
					idxs = tc.idxs
				}

				m := matcher{}
				for i, idx := range idxs {
					Convey(idx.String(), func() {
						iGen, ok := m.match(idx.GetFullSortOrder(), mvals)
						if len(tc.expected[i]) > 0 {
							So(ok, ShouldBeTrue)
							actual := make(serialize.SerializedPslice, 0, len(tc.expected[i]))
							iGen.permute(func(row, _ []byte) {
								actual = append(actual, row)
							})
							So(len(actual), ShouldEqual, len(tc.expected[i]))
							sort.Sort(actual)
							for j, act := range actual {
								So(act, ShouldResemble, tc.expected[i][j])
							}
						} else {
							So(ok, ShouldBeFalse)
						}
					})
				}
			})
		}
	})

	Convey("default indexes", t, func() {
		Convey("nil collated", func() {
			Convey("defaultIndexes (nil)", func() {
				idxs := defaultIndexes("knd", ds.PropertyMap(nil))
				So(len(idxs), ShouldEqual, 1)
				So(idxs[0].String(), ShouldEqual, "B:knd")
			})

			Convey("indexEntries", func() {
				sip := serialize.PropertyMapPartially(fakeKey, nil)
				s := indexEntries(sip, "ns", defaultIndexes("knd", ds.PropertyMap(nil)))
				numItems, _ := s.GetCollection("idx").GetTotals()
				So(numItems, ShouldEqual, 1)
				itm := s.GetCollection("idx").MinItem(false)
				So(itm.Key, ShouldResemble, cat(indx("knd").PrepForIdxTable()))
				numItems, _ = s.GetCollection("idx:ns:" + string(itm.Key)).GetTotals()
				So(numItems, ShouldEqual, 1)
			})

			Convey("defaultIndexes", func() {
				pm := ds.PropertyMap{
					"wat":  {propNI("thing"), prop("hat"), prop(100)},
					"nerd": {prop(103.7)},
					"spaz": {propNI(false)},
				}
				idxs := defaultIndexes("knd", pm)
				So(len(idxs), ShouldEqual, 5)
				So(idxs[0].String(), ShouldEqual, "B:knd")
				So(idxs[1].String(), ShouldEqual, "B:knd/nerd")
				So(idxs[2].String(), ShouldEqual, "B:knd/wat")
				So(idxs[3].String(), ShouldEqual, "B:knd/-nerd")
				So(idxs[4].String(), ShouldEqual, "B:knd/-wat")
			})

		})
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
				store := (memStore)(nil)
				if tc.withBuiltin {
					store = indexEntriesWithBuiltins(fakeKey, tc.pmap, tc.idxs)
				} else {
					sip := serialize.PropertyMapPartially(fakeKey, tc.pmap)
					store = indexEntries(sip, fakeKey.Namespace(), tc.idxs)
				}
				for colName, vals := range tc.collections {
					i := 0
					coll := store.Snapshot().GetCollection(colName)
					numItems, _ := coll.GetTotals()
					So(numItems, ShouldEqual, len(tc.collections[colName]))
					coll.VisitItemsAscend(nil, true, func(itm *gkvlite.Item) bool {
						So(itm.Key, ShouldResemble, vals[i])
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
	key   *ds.Key
	props ds.PropertyMap
}

var updateIndexesTests = []struct {
	name     string
	idxs     []*ds.IndexDefinition
	data     []dumbItem
	expected map[string][][]byte
}{

	{
		name: "basic",
		data: []dumbItem{
			{key("knd", 1), ds.PropertyMap{
				"wat":  {prop(10)},
				"yerp": {prop(10)}},
			},
			{key("knd", 10), ds.PropertyMap{
				"wat":  {prop(1)},
				"yerp": {prop(200)}},
			},
			{key("knd", 1), ds.PropertyMap{
				"wat":  {prop(10)},
				"yerp": {prop(202)}},
			},
		},
		expected: map[string][][]byte{
			"idx:ns:" + sat(indx("knd", "wat").PrepForIdxTable()): {
				cat(prop(1), prop(key("knd", 10))),
				cat(prop(10), prop(key("knd", 1))),
			},
			"idx:ns:" + sat(indx("knd", "-wat").PrepForIdxTable()): {
				cat(icat(prop(10)), prop(key("knd", 1))),
				cat(icat(prop(1)), prop(key("knd", 10))),
			},
			"idx:ns:" + sat(indx("knd", "yerp").PrepForIdxTable()): {
				cat(prop(200), prop(key("knd", 10))),
				cat(prop(202), prop(key("knd", 1))),
			},
		},
	},

	{
		name: "compound",
		idxs: []*ds.IndexDefinition{indx("knd", "yerp", "-wat")},
		data: []dumbItem{
			{key("knd", 1), ds.PropertyMap{
				"wat":  {prop(10)},
				"yerp": {prop(100)}},
			},
			{key("knd", 10), ds.PropertyMap{
				"wat":  {prop(1)},
				"yerp": {prop(200)}},
			},
			{key("knd", 11), ds.PropertyMap{
				"wat":  {prop(20)},
				"yerp": {prop(200)}},
			},
			{key("knd", 14), ds.PropertyMap{
				"wat":  {prop(20)},
				"yerp": {prop(200)}},
			},
			{key("knd", 1), ds.PropertyMap{
				"wat":  {prop(10)},
				"yerp": {prop(202)}},
			},
		},
		expected: map[string][][]byte{
			"idx:ns:" + sat(indx("knd", "yerp", "-wat").PrepForIdxTable()): {
				cat(prop(200), icat(prop(20)), prop(key("knd", 11))),
				cat(prop(200), icat(prop(20)), prop(key("knd", 14))),
				cat(prop(200), icat(prop(1)), prop(key("knd", 10))),
				cat(prop(202), icat(prop(10)), prop(key("knd", 1))),
			},
		},
	},
}

func TestUpdateIndexes(t *testing.T) {
	t.Parallel()

	Convey("Test updateIndexes", t, func() {
		for _, tc := range updateIndexesTests {
			Convey(tc.name, func() {
				store := newMemStore()
				idxColl := store.GetOrCreateCollection("idx")
				for _, i := range tc.idxs {
					idxColl.Set(cat(i.PrepForIdxTable()), []byte{})
				}

				tmpLoader := map[string]ds.PropertyMap{}
				for _, itm := range tc.data {
					ks := itm.key.String()
					prev := tmpLoader[ks]
					updateIndexes(store, itm.key, prev, itm.props)
					tmpLoader[ks] = itm.props
				}
				tmpLoader = nil

				for colName, data := range tc.expected {
					coll := store.Snapshot().GetCollection(colName)
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
