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

package memory

import (
	"sort"
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	ds "go.chromium.org/luci/gae/service/datastore"
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
	expected []ds.IndexedPropertySlice

	// just the collections you want to assert. These are checked in
	// TestIndexEntries. nil to skip test case.
	collections map[string][][]byte
}{

	{
		name: "simple including builtins",
		pmap: ds.PropertyMap{
			"wat":  ds.PropertySlice{propNI("thing"), prop("hat"), prop(100)},
			"nerd": prop(103.7),
			"spaz": propNI(false),
		},
		withBuiltin: true,
		idxs: []*ds.IndexDefinition{
			indx("knd", "-wat", "nerd"),
		},
		expected: []ds.IndexedPropertySlice{
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
			"yerp": ds.PropertySlice{prop("hat"), prop(73.9)},
			"wat": ds.PropertySlice{
				prop(rgenComplexTime),
				prop([]byte("value")),
				prop(rgenComplexKey)},
			"spaz": ds.PropertySlice{prop(nil), prop(false), prop(true)},
		},
		idxs: []*ds.IndexDefinition{
			indx("knd", "-wat", "nerd", "spaz"), // doesn't match, so empty
			indx("knd", "yerp", "-wat", "spaz"),
		},
		expected: []ds.IndexedPropertySlice{
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
			"wat": prop("sup"),
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

	ftt.Run("Test Index Row Generation", t, func(t *ftt.Test) {
		for _, tc := range rowGenTestCases {
			if tc.expected == nil {
				t.Run(tc.name, nil) // shows up as 'skipped'
				continue
			}

			t.Run(tc.name, func(t *ftt.Test) {
				mvals := ds.Serialize.IndexedProperties(fakeKey, tc.pmap)
				idxs := []*ds.IndexDefinition(nil)
				if tc.withBuiltin {
					idxs = append(defaultIndexes("coolKind", mvals), tc.idxs...)
				} else {
					idxs = tc.idxs
				}

				m := matcher{}
				for i, idx := range idxs {
					t.Run(idx.String(), func(t *ftt.Test) {
						iGen, ok := m.match(idx.GetFullSortOrder(), mvals)
						if len(tc.expected[i]) > 0 {
							assert.Loosely(t, ok, should.BeTrue)
							actual := make(ds.IndexedPropertySlice, 0, len(tc.expected[i]))
							iGen.permute(func(row, _ []byte) {
								actual = append(actual, row)
							})
							assert.Loosely(t, len(actual), should.Equal(len(tc.expected[i])))
							sort.Sort(actual)
							for j, act := range actual {
								assert.Loosely(t, act, should.Match(tc.expected[i][j]))
							}
						} else {
							assert.Loosely(t, ok, should.BeFalse)
						}
					})
				}
			})
		}
	})

	ftt.Run("default indexes", t, func(t *ftt.Test) {
		t.Run("nil collated", func(t *ftt.Test) {
			t.Run("defaultIndexes (nil)", func(t *ftt.Test) {
				idxs := defaultIndexes("knd", nil)
				assert.Loosely(t, len(idxs), should.Equal(1))
				assert.Loosely(t, idxs[0].String(), should.Equal("B:knd"))
			})

			t.Run("indexEntries", func(t *ftt.Test) {
				sip := ds.Serialize.IndexedProperties(fakeKey, nil)
				s := indexEntries(fakeKey, sip, defaultIndexes("knd", nil))
				assert.Loosely(t, countItems(s.Snapshot().GetCollection("idx")), should.Equal(1))
				itm := s.GetCollection("idx").MinItem()
				assert.Loosely(t, itm.key, should.Match(cat(indx("knd").PrepForIdxTable())))
				assert.Loosely(t, countItems(s.Snapshot().GetCollection("idx:ns:"+string(itm.key))), should.Equal(1))
			})

			t.Run("defaultIndexes", func(t *ftt.Test) {
				pm := ds.PropertyMap{
					"wat":  ds.PropertySlice{propNI("thing"), prop("hat"), prop(100)},
					"nerd": prop(103.7),
					"spaz": propNI(false),
				}
				idxs := defaultIndexes("knd", ds.Serialize.IndexedProperties(fakeKey, pm))
				assert.Loosely(t, len(idxs), should.Equal(5))
				assert.Loosely(t, idxs[0].String(), should.Equal("B:knd"))
				assert.Loosely(t, idxs[1].String(), should.Equal("B:knd/nerd"))
				assert.Loosely(t, idxs[2].String(), should.Equal("B:knd/wat"))
				assert.Loosely(t, idxs[3].String(), should.Equal("B:knd/-nerd"))
				assert.Loosely(t, idxs[4].String(), should.Equal("B:knd/-wat"))
			})

		})
	})
}

func TestIndexEntries(t *testing.T) {
	t.Parallel()

	ftt.Run("Test indexEntriesWithBuiltins", t, func(t *ftt.Test) {
		for _, tc := range rowGenTestCases {
			if tc.collections == nil {
				t.Run(tc.name, nil) // shows up as 'skipped'
				continue
			}

			t.Run(tc.name, func(t *ftt.Test) {
				store := (memStore)(nil)
				if tc.withBuiltin {
					store = indexEntriesWithBuiltins(fakeKey, tc.pmap, tc.idxs)
				} else {
					sip := ds.Serialize.IndexedProperties(fakeKey, tc.pmap)
					store = indexEntries(fakeKey, sip, tc.idxs)
				}
				for colName, vals := range tc.collections {
					i := 0
					coll := store.Snapshot().GetCollection(colName)
					assert.Loosely(t, countItems(coll), should.Equal(len(tc.collections[colName])))

					coll.ForEachItem(func(k, _ []byte) bool {
						assert.Loosely(t, k, should.Match(vals[i]))
						i++
						return true
					})
					assert.Loosely(t, i, should.Equal(len(vals)))
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
				"wat":  prop(10),
				"yerp": prop(10)},
			},
			{key("knd", 10), ds.PropertyMap{
				"wat":  prop(1),
				"yerp": prop(200)},
			},
			{key("knd", 1), ds.PropertyMap{
				"wat":  prop(10),
				"yerp": prop(202)},
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
				"wat":  prop(10),
				"yerp": prop(100)},
			},
			{key("knd", 10), ds.PropertyMap{
				"wat":  prop(1),
				"yerp": prop(200)},
			},
			{key("knd", 11), ds.PropertyMap{
				"wat":  prop(20),
				"yerp": prop(200)},
			},
			{key("knd", 14), ds.PropertyMap{
				"wat":  prop(20),
				"yerp": prop(200)},
			},
			{key("knd", 1), ds.PropertyMap{
				"wat":  prop(10),
				"yerp": prop(202)},
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

	ftt.Run("Test updateIndexes", t, func(t *ftt.Test) {
		for _, tc := range updateIndexesTests {
			t.Run(tc.name, func(t *ftt.Test) {
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
					assert.Loosely(t, coll, should.NotBeNil)
					i := 0
					coll.ForEachItem(func(k, _ []byte) bool {
						assert.Loosely(t, data[i], should.Match(k))
						i++
						return true
					})
					assert.Loosely(t, i, should.Equal(len(data)))
				}
			})
		}
	})
}
