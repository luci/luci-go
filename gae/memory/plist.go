// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"fmt"
	"sort"

	"infra/gae/libs/gae"
	"infra/gae/libs/gae/helper"

	"github.com/luci/gkvlite"
)

var indexCreationDeterministic = false

type qIndexSlice []*qIndex

func (s qIndexSlice) Len() int           { return len(s) }
func (s qIndexSlice) Swap(i, j int)      { s[i], s[j] = s[j], s[i] }
func (s qIndexSlice) Less(i, j int) bool { return s[i].Less(s[j]) }

func defaultIndicies(kind string, pmap gae.DSPropertyMap) []*qIndex {
	ret := make(qIndexSlice, 0, 2*len(pmap)+1)
	ret = append(ret, &qIndex{kind, false, nil})
	for name, pvals := range pmap {
		needsIndex := false
		for _, v := range pvals {
			if v.IndexSetting() == gae.ShouldIndex {
				needsIndex = true
				break
			}
		}
		if !needsIndex {
			continue
		}
		ret = append(ret, &qIndex{kind, false, []qSortBy{{name, qASC}}})
		ret = append(ret, &qIndex{kind, false, []qSortBy{{name, qDEC}}})
	}
	if indexCreationDeterministic {
		sort.Sort(ret)
	}
	return ret
}

func indexEntriesWithBuiltins(k gae.DSKey, pm gae.DSPropertyMap, complexIdxs []*qIndex) *memStore {
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

func partiallySerialize(pm gae.DSPropertyMap) (ret serializedIndexablePmap) {
	if len(pm) == 0 {
		return
	}

	buf := &bytes.Buffer{}
	ret = make(serializedIndexablePmap, len(pm))
	for k, vals := range pm {
		newVals := make(serializedPvals, 0, len(vals))
		for _, v := range vals {
			if v.IndexSetting() == gae.NoIndex {
				continue
			}
			buf.Reset()
			helper.WriteDSProperty(buf, v, helper.WithoutContext)
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
// correspond with a propertyList and a qIndex.
type indexRowGen struct {
	propVec []serializedPvals
	orders  []qDirection
}

// permute calls cb for each index row, in the sorted order of the rows.
func (s indexRowGen) permute(cb func([]byte)) {
	iVec := make([]int, len(s.propVec))
	iVecLim := make([]int, len(s.propVec))

	incPos := func() bool {
		for i := len(iVec) - 1; i >= 0; i-- {
			var done bool
			var newVal int
			if s.orders[i] == qASC {
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
		if s.orders[i] == qDEC {
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
			if s.orders[pvalSliceIdx] == qASC {
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
func (m *matcher) match(idx *qIndex, sip serializedIndexablePmap) (indexRowGen, bool) {
	m.buf.propVec = m.buf.propVec[:0]
	m.buf.orders = m.buf.orders[:0]
	for _, sb := range idx.sortby {
		if pv, ok := sip[sb.prop]; ok {
			m.buf.propVec = append(m.buf.propVec, pv)
			m.buf.orders = append(m.buf.orders, sb.dir)
		} else {
			return indexRowGen{}, false
		}
	}
	return m.buf, true
}

func (sip serializedIndexablePmap) indexEntries(k gae.DSKey, idxs []*qIndex) *memStore {
	ret := newMemStore()
	idxColl := ret.SetCollection("idx", nil)
	// getIdxEnts retrieves an index collection or adds it if it's not there.
	getIdxEnts := func(qi *qIndex) *memCollection {
		buf := &bytes.Buffer{}
		qi.WriteBinary(buf)
		b := buf.Bytes()
		idxColl.Set(b, []byte{})
		return ret.SetCollection(fmt.Sprintf("idx:%s:%s", k.Namespace(), b), nil)
	}

	buf := &bytes.Buffer{}
	helper.WriteDSKey(buf, helper.WithoutContext, k)
	keyData := buf.Bytes()

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
			} else if idx.ancestor {
				for ancKey := k; ancKey != nil; ancKey = ancKey.Parent() {
					buf := &bytes.Buffer{}
					helper.WriteDSKey(buf, helper.WithoutContext, ancKey)
					walkPermutations(buf.Bytes(), irg, idxEnts)
				}
			} else {
				walkPermutations(nil, irg, idxEnts)
			}
		}
	}

	return ret
}

func updateIndicies(store *memStore, key gae.DSKey, oldEnt, newEnt gae.DSPropertyMap) error {
	var err error

	idxColl := store.GetCollection("idx")
	if idxColl == nil {
		idxColl = store.SetCollection("idx", nil)
	}

	// load all current complex query index definitions.
	compIdx := []*qIndex{}
	idxColl.VisitItemsAscend(complexQueryPrefix, false, func(i *gkvlite.Item) bool {
		if !bytes.HasPrefix(i.Key, complexQueryPrefix) {
			return false
		}
		qi := &qIndex{}
		if err = qi.ReadBinary(bytes.NewBuffer(i.Key)); err != nil {
			return false
		}
		compIdx = append(compIdx, qi)
		return true
	})
	if err != nil {
		return err
	}

	oldIdx := indexEntriesWithBuiltins(key, oldEnt, compIdx)

	newIdx := indexEntriesWithBuiltins(key, newEnt, compIdx)

	prefix := "idx:" + key.Namespace() + ":"

	gkvCollide(oldIdx.GetCollection("idx"), newIdx.GetCollection("idx"), func(k, ov, nv []byte) {
		ks := prefix + string(k)
		idxColl.Set(k, []byte{})

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

	return nil
}
