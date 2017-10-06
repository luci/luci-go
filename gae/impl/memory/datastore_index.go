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
	"bytes"
	"fmt"
	"sort"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/datastore/serialize"
)

type qIndexSlice []*ds.IndexDefinition

func (s qIndexSlice) Len() int           { return len(s) }
func (s qIndexSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s qIndexSlice) Less(i, j int) bool { return s[i].Less(s[j]) }

func defaultIndexes(kind string, pmap ds.PropertyMap) []*ds.IndexDefinition {
	ret := make(qIndexSlice, 0, 2*len(pmap)+1)
	ret = append(ret, &ds.IndexDefinition{Kind: kind})
	for name := range pmap {
		pvals := pmap.Slice(name)
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
		ret = append(ret, &ds.IndexDefinition{Kind: kind, SortBy: []ds.IndexColumn{{Property: name, Descending: true}}})
	}
	if serializationDeterministic {
		sort.Sort(ret)
	}
	return ret
}

// indexEntriesWithBuiltins generates a new memStore containing the default
// indexes for (k, pm) combined with complexIdxs.
//
// If "pm" is nil, this indicates an absence of a value. This is used
// specifically for deletion.
func indexEntriesWithBuiltins(k *ds.Key, pm ds.PropertyMap, complexIdxs []*ds.IndexDefinition) memStore {
	var sip serialize.SerializedPmap
	if pm == nil {
		return newMemStore()
	}
	sip = serialize.PropertyMapPartially(k, pm)
	return indexEntries(k, sip, append(defaultIndexes(k.Kind(), pm), complexIdxs...))
}

// indexRowGen contains enough information to generate all of the index rows which
// correspond with a propertyList and a ds.IndexDefinition.
type indexRowGen struct {
	propVec   []serialize.SerializedPslice
	decending []bool
}

// permute calls cb for each index row, in the sorted order of the rows.
func (s indexRowGen) permute(collSetFn func(k, v []byte)) {
	iVec := make([]int, len(s.propVec))
	iVecLim := make([]int, len(s.propVec))

	incPos := func() bool {
		for i := len(iVec) - 1; i >= 0; i-- {
			var done bool
			var newVal int
			if !s.decending[i] {
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
		if s.decending[i] {
			iVec[i] = iVecLim[i] - 1
		}
	}

	for {
		bufsiz := 0
		for pvalSliceIdx, pvalIdx := range iVec {
			bufsiz += len(s.propVec[pvalSliceIdx][pvalIdx])
		}
		buf := serialize.Invertible(bytes.NewBuffer(make([]byte, 0, bufsiz)))
		for pvalSliceIdx, pvalIdx := range iVec {
			data := s.propVec[pvalSliceIdx][pvalIdx]
			buf.SetInvert(s.decending[pvalSliceIdx])
			_, _ = buf.Write(data)
		}
		collSetFn(buf.Bytes(), []byte{})
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
func (m *matcher) match(sortBy []ds.IndexColumn, sip serialize.SerializedPmap) (indexRowGen, bool) {
	m.buf.propVec = m.buf.propVec[:0]
	m.buf.decending = m.buf.decending[:0]
	for _, sb := range sortBy {
		if pv, ok := sip[sb.Property]; ok {
			m.buf.propVec = append(m.buf.propVec, pv)
			m.buf.decending = append(m.buf.decending, sb.Descending)
		} else {
			return indexRowGen{}, false
		}
	}
	return m.buf, true
}

// indexEntries generates a new memStore containing index entries for sip for
// the supplied index definitions.
func indexEntries(key *ds.Key, sip serialize.SerializedPmap, idxs []*ds.IndexDefinition) memStore {
	ret := newMemStore()
	idxColl := ret.GetOrCreateCollection("idx")

	mtch := matcher{}
	for _, idx := range idxs {
		idx = idx.Normalize()
		if idx.Kind != "" && idx.Kind != key.Kind() {
			continue
		}
		if irg, ok := mtch.match(idx.GetFullSortOrder(), sip); ok {
			idxBin := serialize.ToBytes(*idx.PrepForIdxTable())
			idxColl.Set(idxBin, []byte{})
			coll := ret.GetOrCreateCollection(
				fmt.Sprintf("idx:%s:%s", key.Namespace(), idxBin))
			irg.permute(coll.Set)
		}
	}

	return ret
}

// walkCompIdxs walks the table of compound indexes in the store. If `endsWith`
// is provided, this will only walk over compound indexes which match
// Kind, Ancestor, and whose SortBy has `endsWith.SortBy` as a suffix.
func walkCompIdxs(store memStore, endsWith *ds.IndexDefinition, cb func(*ds.IndexDefinition) bool) {
	idxColl := store.GetCollection("idx")
	if idxColl == nil {
		return
	}
	itrDef := iterDefinition{c: idxColl}

	if endsWith != nil {
		full := serialize.ToBytes(*endsWith.Flip())
		// chop off the null terminating byte
		itrDef.prefix = full[:len(full)-1]
	}

	it := itrDef.mkIter()
	for ent := it.next(); ent != nil; ent = it.next() {
		qi, err := serialize.ReadIndexDefinition(bytes.NewReader(ent.key))
		memoryCorruption(err)
		if !cb(qi.Flip()) {
			break
		}
	}
}

func mergeIndexes(ns string, store, oldIdx, newIdx memStore) {
	prefixBuf := []byte("idx:" + ns + ":")
	origPrefixBufLen := len(prefixBuf)

	oldIdx = oldIdx.Snapshot()
	newIdx = newIdx.Snapshot()

	memStoreCollide(oldIdx.GetCollection("idx"), newIdx.GetCollection("idx"), func(k, ov, nv []byte) {
		prefixBuf = append(prefixBuf[:origPrefixBufLen], k...)
		ks := string(prefixBuf)

		coll := store.GetOrCreateCollection(ks)

		oldColl := oldIdx.GetCollection(ks)
		newColl := newIdx.GetCollection(ks)

		switch {
		case ov == nil && nv != nil: // all additions
			newColl.ForEachItem(func(k, _ []byte) bool {
				coll.Set(k, []byte{})
				return true
			})
		case ov != nil && nv == nil: // all deletions
			oldColl.ForEachItem(func(k, _ []byte) bool {
				coll.Delete(k)
				return true
			})
		case ov != nil && nv != nil: // merge
			memStoreCollide(oldColl, newColl, func(k, ov, nv []byte) {
				if nv == nil {
					coll.Delete(k)
				} else {
					coll.Set(k, []byte{})
				}
			})
		default:
			impossible(fmt.Errorf("both values from memStoreCollide were nil?"))
		}
		// TODO(riannucci): remove entries from idxColl and remove index collections
		// when there are no index entries for that index any more.
	})
}

func addIndexes(store memStore, aid string, compIdx []*ds.IndexDefinition) {
	normalized := make([]*ds.IndexDefinition, len(compIdx))
	idxColl := store.GetOrCreateCollection("idx")
	for i, idx := range compIdx {
		normalized[i] = idx.Normalize()
		idxColl.Set(serialize.ToBytes(*normalized[i].PrepForIdxTable()), []byte{})
	}

	for _, ns := range namespaces(store) {
		kctx := ds.MkKeyContext(aid, ns)
		if allEnts := store.Snapshot().GetCollection("ents:" + ns); allEnts != nil {
			allEnts.ForEachItem(func(ik, iv []byte) bool {
				pm, err := rpm(iv)
				memoryCorruption(err)

				prop, err := serialize.ReadProperty(bytes.NewBuffer(ik), serialize.WithoutContext, kctx)
				memoryCorruption(err)

				k := prop.Value().(*ds.Key)

				sip := serialize.PropertyMapPartially(k, pm)

				mergeIndexes(ns, store,
					newMemStore(),
					indexEntries(k, sip, normalized))
				return true
			})
		}
	}
}

// updateIndexes updates the indexes in store to accommodate a change in entity
// value.
//
// oldEnt is the previous entity value, and newEnt is the new entity value. If
// newEnt is nil, that signifies deletion.
func updateIndexes(store memStore, key *ds.Key, oldEnt, newEnt ds.PropertyMap) {
	// load all current complex query index definitions.
	var compIdx []*ds.IndexDefinition
	walkCompIdxs(store.Snapshot(), nil, func(i *ds.IndexDefinition) bool {
		compIdx = append(compIdx, i)
		return true
	})

	mergeIndexes(key.Namespace(), store,
		indexEntriesWithBuiltins(key, oldEnt, compIdx),
		indexEntriesWithBuiltins(key, newEnt, compIdx))
}
