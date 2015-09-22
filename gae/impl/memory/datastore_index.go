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
	"github.com/luci/luci-go/common/stringset"
)

type qIndexSlice []*ds.IndexDefinition

func (s qIndexSlice) Len() int           { return len(s) }
func (s qIndexSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s qIndexSlice) Less(i, j int) bool { return s[i].Less(s[j]) }

func defaultIndexes(kind string, pmap ds.PropertyMap) []*ds.IndexDefinition {
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
		ret = append(ret, &ds.IndexDefinition{Kind: kind, SortBy: []ds.IndexColumn{{Property: name, Descending: true}}})
	}
	if serializationDeterministic {
		sort.Sort(ret)
	}
	return ret
}

func indexEntriesWithBuiltins(k *ds.Key, pm ds.PropertyMap, complexIdxs []*ds.IndexDefinition) *memStore {
	sip := partiallySerialize(k, pm)
	return sip.indexEntries(k.Namespace(), append(defaultIndexes(k.Kind(), pm), complexIdxs...))
}

// serializedPvals is all of the serialized DSProperty values in qASC order.
type serializedPvals [][]byte

func (s serializedPvals) Len() int           { return len(s) }
func (s serializedPvals) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s serializedPvals) Less(i, j int) bool { return bytes.Compare(s[i], s[j]) < 0 }

// prop name -> [<serialized DSProperty>, ...]
// includes special values '__key__' and '__ancestor__' which contains all of
// the ancestor entries for this key.
type serializedIndexablePmap map[string]serializedPvals

func serializeRow(vals []ds.Property) serializedPvals {
	dups := stringset.New(0)
	ret := make(serializedPvals, 0, len(vals))
	for _, v := range vals {
		if v.IndexSetting() == ds.NoIndex {
			continue
		}
		data := serialize.ToBytes(v.ForIndex())
		dataS := string(data)
		if !dups.Add(dataS) {
			continue
		}
		ret = append(ret, data)
	}
	return ret
}

func partiallySerialize(k *ds.Key, pm ds.PropertyMap) (ret serializedIndexablePmap) {
	ret = make(serializedIndexablePmap, len(pm)+2)
	if k == nil {
		impossible(fmt.Errorf("key to partiallySerialize is nil"))
	}
	ret["__key__"] = [][]byte{serialize.ToBytes(ds.MkProperty(k))}
	for k != nil {
		ret["__ancestor__"] = append(ret["__ancestor__"], serialize.ToBytes(ds.MkProperty(k)))
		k = k.Parent()
	}
	for k, vals := range pm {
		newVals := serializeRow(vals)
		if len(newVals) > 0 {
			ret[k] = newVals
		}
	}
	return
}

// indexRowGen contains enough information to generate all of the index rows which
// correspond with a propertyList and a ds.IndexDefinition.
type indexRowGen struct {
	propVec   []serializedPvals
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
func (m *matcher) match(sortBy []ds.IndexColumn, sip serializedIndexablePmap) (indexRowGen, bool) {
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

func (sip serializedIndexablePmap) indexEntries(ns string, idxs []*ds.IndexDefinition) *memStore {
	ret := newMemStore()
	idxColl := ret.SetCollection("idx", nil)

	mtch := matcher{}
	for _, idx := range idxs {
		idx = idx.Normalize()
		if irg, ok := mtch.match(idx.GetFullSortOrder(), sip); ok {
			idxBin := serialize.ToBytes(*idx.PrepForIdxTable())
			idxColl.Set(idxBin, []byte{})
			coll := ret.SetCollection(fmt.Sprintf("idx:%s:%s", ns, idxBin), nil)
			irg.permute(coll.Set)
		}
	}

	return ret
}

// walkCompIdxs walks the table of compound indexes in the store. If `endsWith`
// is provided, this will only walk over compound indexes which match
// Kind, Ancestor, and whose SortBy has `endsWith.SortBy` as a suffix.
func walkCompIdxs(store *memStore, endsWith *ds.IndexDefinition, cb func(*ds.IndexDefinition) bool) {
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
	defer it.stop()
	for !it.stopped {
		it.next(nil, func(i *gkvlite.Item) {
			if i == nil {
				return
			}
			qi, err := serialize.ReadIndexDefinition(bytes.NewBuffer(i.Key))
			memoryCorruption(err)
			if !cb(qi.Flip()) {
				it.stop()
			}
		})
	}
}

func mergeIndexes(ns string, store, oldIdx, newIdx *memStore) {
	prefixBuf := []byte("idx:" + ns + ":")
	origPrefixBufLen := len(prefixBuf)
	gkvCollide(oldIdx.GetCollection("idx"), newIdx.GetCollection("idx"), func(k, ov, nv []byte) {
		prefixBuf = append(prefixBuf[:origPrefixBufLen], k...)
		ks := string(prefixBuf)

		coll := store.GetCollection(ks)
		if coll == nil {
			coll = store.SetCollection(ks, nil)
		}

		oldColl := oldIdx.GetCollection(ks)
		newColl := newIdx.GetCollection(ks)

		switch {
		case ov == nil && nv != nil: // all additions
			newColl.VisitItemsAscend(nil, false, func(i *gkvlite.Item) bool {
				coll.Set(i.Key, []byte{})
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
					coll.Set(k, []byte{})
				}
			})
		default:
			impossible(fmt.Errorf("both values from gkvCollide were nil?"))
		}
		// TODO(riannucci): remove entries from idxColl and remove index collections
		// when there are no index entries for that index any more.
	})
}

func addIndex(store *memStore, ns string, compIdx []*ds.IndexDefinition) {
	normalized := make([]*ds.IndexDefinition, len(compIdx))
	idxColl := store.SetCollection("idx", nil)
	for i, idx := range compIdx {
		normalized[i] = idx.Normalize()
		idxColl.Set(serialize.ToBytes(*normalized[i].PrepForIdxTable()), []byte{})
	}

	if allEnts := store.GetCollection("ents:" + ns); allEnts != nil {
		allEnts.VisitItemsAscend(nil, true, func(i *gkvlite.Item) bool {
			pm, err := rpmWoCtx(i.Val, ns)
			memoryCorruption(err)

			prop, err := serialize.ReadProperty(bytes.NewBuffer(i.Key), serialize.WithoutContext, globalAppID, ns)
			memoryCorruption(err)

			k := prop.Value().(*ds.Key)

			sip := partiallySerialize(k, pm)

			mergeIndexes(ns, store,
				newMemStore(),
				sip.indexEntries(ns, normalized))
			return true
		})
	}
}

func updateIndexes(store *memStore, key *ds.Key, oldEnt, newEnt ds.PropertyMap) {
	// load all current complex query index definitions.
	compIdx := []*ds.IndexDefinition{}
	walkCompIdxs(store, nil, func(i *ds.IndexDefinition) bool {
		compIdx = append(compIdx, i)
		return true
	})

	mergeIndexes(key.Namespace(), store,
		indexEntriesWithBuiltins(key, oldEnt, compIdx),
		indexEntriesWithBuiltins(key, newEnt, compIdx))
}
