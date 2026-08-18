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
	"errors"
	"fmt"

	"go.chromium.org/luci/common/data/cmpbin"
	"go.chromium.org/luci/common/data/stringset"

	ds "go.chromium.org/luci/gae/service/datastore"
)

type queryStrategy interface {
	// handle applies the strategy.
	//   - rawData is the slice of encoded Properties from the index row
	//     (correctly de-inverted).
	//   - decodedProps is the slice of decoded Properties from the index row
	//   - key is the decoded Key from the index row (the last item in rawData and
	//     decodedProps)
	handle(rawData [][]byte, decodedProps []ds.Property, key *ds.Key) ds.PropertyMap
}

type projectionLookup struct {
	suffixIndex  int
	propertyName string
}

type projectionStrategy struct {
	project  []projectionLookup
	distinct stringset.Set
}

func newProjectionStrategy(fq *ds.FinalizedQuery, rq *reducedQuery) queryStrategy {
	proj := fq.Project()

	projectionLookups := make([]projectionLookup, len(proj))
	for i, prop := range proj {
		projectionLookups[i].propertyName = prop
		lookupErr := fmt.Errorf("planning a strategy for an unfulfillable query?")
		for j, col := range rq.suffixFormat {
			if col.Property == prop {
				projectionLookups[i].suffixIndex = j
				lookupErr = nil
				break
			}
		}
		impossible(lookupErr)
	}
	ret := &projectionStrategy{project: projectionLookups}
	if fq.Distinct() {
		ret.distinct = stringset.New(0)
	}
	return ret
}

func (s *projectionStrategy) handle(rawData [][]byte, decodedProps []ds.Property, key *ds.Key) ds.PropertyMap {
	projectedRaw := [][]byte(nil)
	if s.distinct != nil {
		projectedRaw = make([][]byte, len(decodedProps))
	}
	pmap := make(ds.PropertyMap, len(s.project)+1)
	for i, p := range s.project {
		if s.distinct != nil {
			projectedRaw[i] = rawData[p.suffixIndex]
		}
		pmap[p.propertyName] = decodedProps[p.suffixIndex]
	}
	if s.distinct != nil {
		if !s.distinct.Add(string(cmpbin.ConcatBytes(projectedRaw...))) {
			return nil
		}
	}
	pmap["$key"] = ds.MkPropertyNI(key)
	return pmap
}

type keysOnlyStrategy struct {
	dedup stringset.Set
}

func (s *keysOnlyStrategy) handle(rawData [][]byte, _ []ds.Property, key *ds.Key) ds.PropertyMap {
	if !s.dedup.Add(string(rawData[len(rawData)-1])) {
		return nil
	}
	return ds.PropertyMap{"$key": ds.MkPropertyNI(key)}
}

type normalStrategy struct {
	kc    ds.KeyContext
	head  memCollection
	dedup stringset.Set
}

func newNormalStrategy(kc ds.KeyContext, head memStore) queryStrategy {
	coll := head.GetCollection("ents:" + kc.Namespace)
	if coll == nil {
		return nil
	}
	return &normalStrategy{kc, coll, stringset.New(0)}
}

func (s *normalStrategy) handle(rawData [][]byte, _ []ds.Property, key *ds.Key) ds.PropertyMap {
	rawKey := rawData[len(rawData)-1]
	if !s.dedup.Add(string(rawKey)) {
		return nil
	}

	rawEnt := s.head.Get(rawKey)
	if rawEnt == nil {
		// entity doesn't exist at head
		return nil
	}
	pm, err := ds.Deserializer{KeyContext: s.kc}.PropertyMap(bytes.NewBuffer(rawEnt))
	memoryCorruption(err)
	pm["$key"] = ds.MkPropertyNI(key)
	return pm
}

func pickQueryStrategy(fq *ds.FinalizedQuery, rq *reducedQuery, head memStore) queryStrategy {
	if fq.KeysOnly() {
		return &keysOnlyStrategy{stringset.New(0)}
	}
	if len(fq.Project()) > 0 {
		return newProjectionStrategy(fq, rq)
	}
	return newNormalStrategy(rq.kc, head)
}

func parseSuffix(aid, ns string, suffixFormat []ds.IndexColumn, suffix []byte, count int) (raw [][]byte, decoded []ds.Property) {
	buf := cmpbin.Invertible(bytes.NewBuffer(suffix))
	decoded = make([]ds.Property, len(suffixFormat))
	raw = make([][]byte, len(suffixFormat))

	err := error(nil)
	kc := ds.MkKeyContext(aid, ns)
	for i := range decoded {
		if count >= 0 && i >= count {
			break
		}
		needInvert := suffixFormat[i].Descending

		buf.SetInvert(needInvert)
		decoded[i], err = ds.Deserializer{KeyContext: kc}.Property(buf)
		memoryCorruption(err)

		offset := len(suffix) - buf.Len()
		raw[i] = suffix[:offset]
		suffix = suffix[offset:]
		if needInvert {
			raw[i] = cmpbin.InvertBytes(raw[i])
		}
	}

	return
}

func countQuery(fq *ds.FinalizedQuery, kc ds.KeyContext, data *dataStoreData, isTxn bool, idx, head memStore) (ret int64, err error) {
	if len(fq.Project()) == 0 && !fq.KeysOnly() {
		fq, err = fq.Original().KeysOnly(true).Finalize()
		if err != nil {
			return
		}
	}
	for _, err := range executeQuery(fq, kc, nil, isTxn, idx, head).Results {
		if err != nil {
			return 0, err
		}
		ret++
	}
	return
}

func executeNamespaceQuery(fq *ds.FinalizedQuery, kc ds.KeyContext, head memStore) ds.RawQueryIter {
	// these objects have no properties, so any filters on properties cause an
	// empty result.
	if len(fq.EqFilters()) > 0 ||
		len(fq.InFilters()) > 0 ||
		len(fq.Project()) > 0 ||
		len(fq.Orders()) > 1 {
		return ds.RawQueryIterStub(nil)
	}
	if !(fq.IneqFilterProp() == "" || fq.IneqFilterProp() == "__key__") {
		return ds.RawQueryIterStub(nil)
	}
	limit, hasLimit := fq.Limit()
	offset, hasOffset := fq.Offset()
	start, end := fq.Bounds()

	cursErr := errors.New("cursors not supported for __namespace__ query")
	if !(start == nil && end == nil) {
		return ds.RawQueryIterStub(cursErr)
	}

	kc.Namespace = ""
	return ds.RawQueryIter{
		Cursor: func() (ds.RawCursor, error) { return nil, cursErr },
		Results: func(yield func(ds.PropertyMap, error) bool) {
			for _, ns := range namespaces(head) {
				if hasOffset && offset > 0 {
					offset--
					continue
				}
				if hasLimit {
					if limit <= 0 {
						return
					}
					limit--
				}
				k := (*ds.Key)(nil)
				if ns == "" {
					// Datastore uses an id of 1 to indicate the default namespace in its
					// metadata API.
					k = kc.MakeKey("__namespace__", 1)
				} else {
					k = kc.MakeKey("__namespace__", ns)
				}
				pm := ds.PropertyMap{"$key": ds.MkPropertyNI(k)}
				if !yield(pm, nil) {
					return
				}
			}
		},
	}
}

func executeQuery(fq *ds.FinalizedQuery, kc ds.KeyContext, data *dataStoreData, isTxn bool, idx, head memStore) ds.RawQueryIter {
	rq, err := reduce(fq, kc, isTxn)
	if err == ds.ErrNullQuery {
		return ds.RawQueryIterStub(nil)
	}
	if err != nil {
		return ds.RawQueryIterStub(err)
	}

	if rq.kind == "__namespace__" {
		return executeNamespaceQuery(fq, kc, head)
	}

	idxs, err := getIndexes(rq, idx)
	if err != nil && err != ds.ErrNullQuery && data != nil && data.maybeAutoIndex(err) {
		idx, head = data.getQuerySnaps(!fq.EventuallyConsistent())
		idxs, err = getIndexes(rq, idx)
	}
	if err == ds.ErrNullQuery {
		return ds.RawQueryIterStub(nil)
	}
	if err != nil {
		return ds.RawQueryIterStub(err)
	}

	strategy := pickQueryStrategy(fq, rq, head)
	if strategy == nil {
		// e.g. the normalStrategy found that there were NO entities in the current
		// namespace.
		return ds.RawQueryIterStub(nil)
	}

	offset, _ := fq.Offset()
	limit, hasLimit := fq.Limit()

	buf := &bytes.Buffer{}
	_, err = cmpbin.WriteUint(buf, uint64(len(rq.suffixFormat)))
	memoryCorruption(err)

	for _, col := range rq.suffixFormat {
		err := ds.Serialize.IndexColumn(buf, col)
		memoryCorruption(err)
	}
	cursorPrefix := buf.Bytes()

	var lastSuffix []byte
	return ds.RawQueryIter{
		Cursor: func() (ds.RawCursor, error) {
			return queryCursor(cmpbin.ConcatBytes(cursorPrefix, increment(bytes.Clone(lastSuffix)))), nil
		},
		Results: func(yield func(ds.PropertyMap, error) bool) {
			for curSuffix := range multiIterate(idxs) {
				rawData, decodedProps := parseSuffix(kc.AppID, kc.Namespace, rq.suffixFormat, curSuffix, -1)

				keyProp := decodedProps[len(decodedProps)-1]
				if keyProp.Type() != ds.PTKey {
					impossible(fmt.Errorf("decoded index row doesn't end with a Key: %#v", keyProp))
				}

				key := keyProp.Value().(*ds.Key)
				if key.LastTok().Kind == "__entity_group__" {
					// These are internal entities and so shouldn't count to user-observable
					// offset/limit. Real datastore doesn't include these in query output
					// (they are 'synthetic' entities), but we store them in the main table.
					continue
				}

				pm := strategy.handle(rawData, decodedProps, key)
				if pm == nil {
					continue
				}

				if offset > 0 {
					offset--
					continue
				}
				if hasLimit {
					if limit <= 0 {
						return
					}
					limit--
				}

				lastSuffix = curSuffix

				if !yield(pm, nil) {
					return
				}
			}
		},
	}
}
