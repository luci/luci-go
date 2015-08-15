// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"fmt"
	"sort"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/serialize"
	"github.com/luci/gkvlite"
)

var indexCreationDeterministic = false

type qIndexSlice []*ds.IndexDefinition

func (s qIndexSlice) Len() int           { return len(s) }
func (s qIndexSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s qIndexSlice) Less(i, j int) bool { return s[i].Less(s[j]) }

func defaultIndicies(kind string, pmap ds.PropertyMap) []*ds.IndexDefinition {
	ret := make(qIndexSlice, 0, 2*len(pmap)+1)
	ret = append(ret, &ds.IndexDefinition{Kind: kind})
	for name, pvals := range pmap {
		needsIndex := false
		for _, v := range pvals {
			if v.IndexSetting() == ds.ShouldIndex {
				needsIndex = true
				break
			}
		}
		if !needsIndex {
			continue
		}
		ret = append(ret, &ds.IndexDefinition{Kind: kind, SortBy: []ds.IndexColumn{{Property: name}}})
		ret = append(ret, &ds.IndexDefinition{Kind: kind, SortBy: []ds.IndexColumn{{Property: name, Direction: ds.DESCENDING}}})
	}
	if indexCreationDeterministic {
		sort.Sort(ret)
	}
	return ret
}

func indexEntriesWithBuiltins(k ds.Key, pm ds.PropertyMap, complexIdxs []*ds.IndexDefinition) *memStore {
	sip := partiallySerialize(pm)
	return sip.indexEntries(k, append(defaultIndicies(k.Kind(), pm), complexIdxs...))
}

// serializedPvals is all of the serialized DSProperty values in qASC order.
type serializedPvals [][]byte

func (s serializedPvals) Len() int           { return len(s) }
func (s serializedPvals) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s serializedPvals) Less(i, j int) bool { return bytes.Compare(s[i], s[j]) < 0 }

// prop name -> [<serialized DSProperty>, ...]
type serializedIndexablePmap map[string]serializedPvals

func partiallySerialize(pm ds.PropertyMap) (ret serializedIndexablePmap) {
	if len(pm) == 0 {
		return
	}

	buf := &bytes.Buffer{}
	ret = make(serializedIndexablePmap, len(pm))
	for k, vals := range pm {
		newVals := make(serializedPvals, 0, len(vals))
		for _, v := range vals {
			if v.IndexSetting() == ds.NoIndex {
				continue
			}
			buf.Reset()
			serialize.WriteProperty(buf, serialize.WithoutContext, v)
			newVal := make([]byte, buf.Len())
			copy(newVal, buf.Bytes())
			newVals = append(newVals, newVal)
		}
		if len(newVals) > 0 {
			sort.Sort(newVals)
			ret[k] = newVals
		}
	}
	return
}

// indexRowGen contains enough information to generate all of the index rows which
// correspond with a propertyList and a ds.IndexDefinition.
type indexRowGen struct {
	propVec []serializedPvals
	orders  []ds.IndexDirection
}

// permute calls cb for each index row, in the sorted order of the rows.
func (s indexRowGen) permute(cb func([]byte)) {
	iVec := make([]int, len(s.propVec))
	iVecLim := make([]int, len(s.propVec))

	incPos := func() bool {
		for i := len(iVec) - 1; i >= 0; i-- {
			var done bool
			var newVal int
			if s.orders[i] == ds.ASCENDING {
				newVal = (iVec[i] + 1) % iVecLim[i]
				done = newVal != 0
			} else {
				newVal = (iVec[i] - 1)
				if newVal < 0 {
					newVal = iVecLim[i] - 1
				} else {
					done = true
				}
			}
			iVec[i] = newVal
			if done {
				return true
			}
		}
		return false
	}

	for i, sps := range s.propVec {
		iVecLim[i] = len(sps)
	}

	for i := range iVec {
		if s.orders[i] == ds.DESCENDING {
			iVec[i] = iVecLim[i] - 1
		}
	}

	for {
		bufsiz := 0
		for pvalSliceIdx, pvalIdx := range iVec {
			bufsiz += len(s.propVec[pvalSliceIdx][pvalIdx])
		}
		buf := bytes.NewBuffer(make([]byte, 0, bufsiz))
		for pvalSliceIdx, pvalIdx := range iVec {
			data := s.propVec[pvalSliceIdx][pvalIdx]
			if s.orders[pvalSliceIdx] == ds.ASCENDING {
				buf.Write(data)
			} else {
				for _, b := range data {
					buf.WriteByte(b ^ 0xFF)
				}
			}
		}
		cb(buf.Bytes())
		if !incPos() {
			break
		}
	}
}

type matcher struct {
	buf indexRowGen
}

// matcher.match checks to see if the mapped, serialized property values
// match the index. If they do, it returns a indexRowGen. Do not write or modify
// the data in the indexRowGen.
func (m *matcher) match(idx *ds.IndexDefinition, sip serializedIndexablePmap) (indexRowGen, bool) {
	m.buf.propVec = m.buf.propVec[:0]
	m.buf.orders = m.buf.orders[:0]
	for _, sb := range idx.SortBy {
		if sb.Property == "__key__" {
			panic("don't know how to build compound index on __key__")
		}
		if pv, ok := sip[sb.Property]; ok {
			m.buf.propVec = append(m.buf.propVec, pv)
			m.buf.orders = append(m.buf.orders, sb.Direction)
		} else {
			return indexRowGen{}, false
		}
	}
	return m.buf, true
}

func (sip serializedIndexablePmap) indexEntries(k ds.Key, idxs []*ds.IndexDefinition) *memStore {
	ret := newMemStore()
	idxColl := ret.SetCollection("idx", nil)
	// getIdxEnts retrieves an index collection or adds it if it's not there.
	getIdxEnts := func(qi *ds.IndexDefinition) *memCollection {
		b := serialize.ToBytes(*qi)
		idxColl.Set(b, []byte{})
		return ret.SetCollection(fmt.Sprintf("idx:%s:%s", k.Namespace(), b), nil)
	}

	keyData := serialize.ToBytes(k)

	walkPermutations := func(prefix []byte, irg indexRowGen, ents *memCollection) {
		prev := []byte{} // intentionally make a non-nil slice, gkvlite hates nil.
		irg.permute(func(data []byte) {
			buf := bytes.NewBuffer(make([]byte, 0, len(prefix)+len(data)+len(keyData)))
			buf.Write(prefix)
			buf.Write(data)
			buf.Write(keyData)
			ents.Set(buf.Bytes(), prev)
			prev = data
		})
	}

	mtch := matcher{}
	for _, idx := range idxs {
		if irg, ok := mtch.match(idx, sip); ok {
			idxEnts := getIdxEnts(idx)
			if len(irg.propVec) == 0 {
				idxEnts.Set(keyData, []byte{}) // propless index, e.g. kind -> key = nil
			} else if idx.Ancestor {
				for ancKey := k; ancKey != nil; ancKey = ancKey.Parent() {
					walkPermutations(serialize.ToBytes(ancKey), irg, idxEnts)
				}
			} else {
				walkPermutations(nil, irg, idxEnts)
			}
		}
	}

	return ret
}

func getCompIdxs(idxColl *memCollection) []*ds.IndexDefinition {
	// load all current complex query index definitions.
	compIdx := []*ds.IndexDefinition{}
	complexQueryPrefix := ds.IndexComplexQueryPrefix()
	idxColl.VisitItemsAscend(complexQueryPrefix, false, func(i *gkvlite.Item) bool {
		if !bytes.HasPrefix(i.Key, complexQueryPrefix) {
			return false
		}
		qi, err := serialize.ReadIndexDefinition(bytes.NewBuffer(i.Key))
		if err != nil {
			panic(err) // memory corruption
		}
		compIdx = append(compIdx, &qi)
		return true
	})
	return compIdx
}

func getIdxColl(store *memStore) *memCollection {
	idxColl := store.GetCollection("idx")
	if idxColl == nil {
		idxColl = store.SetCollection("idx", nil)
	}
	return idxColl
}

func mergeIndexes(ns string, store, oldIdx, newIdx *memStore) {
	idxColl := getIdxColl(store)
	prefix := "idx:" + ns + ":"
	gkvCollide(oldIdx.GetCollection("idx"), newIdx.GetCollection("idx"), func(k, ov, nv []byte) {
		ks := prefix + string(k)
		if idxColl.Get(k) == nil {
			// avoids unnecessary mutation, otherwise the idx collection thrashes on
			// every update.
			idxColl.Set(k, []byte{})
		}

		coll := store.GetCollection(ks)
		if coll == nil {
			coll = store.SetCollection(ks, nil)
		}
		oldColl := oldIdx.GetCollection(ks)
		newColl := newIdx.GetCollection(ks)

		switch {
		case ov == nil && nv != nil: // all additions
			newColl.VisitItemsAscend(nil, false, func(i *gkvlite.Item) bool {
				coll.Set(i.Key, i.Val)
				return true
			})
		case ov != nil && nv == nil: // all deletions
			oldColl.VisitItemsAscend(nil, false, func(i *gkvlite.Item) bool {
				coll.Delete(i.Key)
				return true
			})
		case ov != nil && nv != nil: // merge
			gkvCollide(oldColl, newColl, func(k, ov, nv []byte) {
				if nv == nil {
					coll.Delete(k)
				} else {
					coll.Set(k, nv)
				}
			})
		default:
			panic("impossible")
		}
		// TODO(riannucci): remove entries from idxColl and remove index collections
		// when there are no index entries for that index any more.
	})
}

func addIndex(store *memStore, ns string, compIdx []*ds.IndexDefinition) {
	store.GetCollection("ents:"+ns).VisitItemsAscend(nil, true, func(i *gkvlite.Item) bool {
		pm, err := rpmWoCtx(i.Val, ns)
		if err != nil {
			panic(err) // memory corruption
		}
		k, err := serialize.ReadKey(bytes.NewBuffer(i.Key), serialize.WithoutContext, globalAppID, ns)
		if err != nil {
			panic(err)
		}
		sip := partiallySerialize(pm)
		mergeIndexes(ns, store, newMemStore(), sip.indexEntries(k, compIdx))
		return true
	})
}

func updateIndicies(store *memStore, key ds.Key, oldEnt, newEnt ds.PropertyMap) {
	// load all current complex query index definitions.
	compIdx := getCompIdxs(getIdxColl(store))

	mergeIndexes(key.Namespace(), store,
		indexEntriesWithBuiltins(key, oldEnt, compIdx),
		indexEntriesWithBuiltins(key, newEnt, compIdx))
}
