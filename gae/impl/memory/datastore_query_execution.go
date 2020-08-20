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

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/datastore/serialize"
	"go.chromium.org/luci/common/data/cmpbin"
	"go.chromium.org/luci/common/data/stringset"
)

type queryStrategy interface {
	// handle applies the strategy to the embedded user callback.
	//   - rawData is the slice of encoded Properties from the index row
	//     (correctly de-inverted).
	//   - decodedProps is the slice of decoded Properties from the index row
	//   - key is the decoded Key from the index row (the last item in rawData and
	//     decodedProps)
	//   - gc is the getCursor function to be passed to the user's callback
	handle(rawData [][]byte, decodedProps []ds.Property, key *ds.Key, gc func() (ds.Cursor, error)) error
}

type projectionLookup struct {
	suffixIndex  int
	propertyName string
}

type projectionStrategy struct {
	cb ds.RawRunCB

	project  []projectionLookup
	distinct stringset.Set
}

func newProjectionStrategy(fq *ds.FinalizedQuery, rq *reducedQuery, cb ds.RawRunCB) queryStrategy {
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
	ret := &projectionStrategy{cb: cb, project: projectionLookups}
	if fq.Distinct() {
		ret.distinct = stringset.New(0)
	}
	return ret
}

func (s *projectionStrategy) handle(rawData [][]byte, decodedProps []ds.Property, key *ds.Key, gc func() (ds.Cursor, error)) error {
	projectedRaw := [][]byte(nil)
	if s.distinct != nil {
		projectedRaw = make([][]byte, len(decodedProps))
	}
	pmap := make(ds.PropertyMap, len(s.project))
	for i, p := range s.project {
		if s.distinct != nil {
			projectedRaw[i] = rawData[p.suffixIndex]
		}
		pmap[p.propertyName] = decodedProps[p.suffixIndex]
	}
	if s.distinct != nil {
		if !s.distinct.Add(string(serialize.Join(projectedRaw...))) {
			return nil
		}
	}
	return s.cb(key, pmap, gc)
}

type keysOnlyStrategy struct {
	cb ds.RawRunCB

	dedup stringset.Set
}

func (s *keysOnlyStrategy) handle(rawData [][]byte, _ []ds.Property, key *ds.Key, gc func() (ds.Cursor, error)) error {
	if !s.dedup.Add(string(rawData[len(rawData)-1])) {
		return nil
	}
	return s.cb(key, nil, gc)
}

type normalStrategy struct {
	cb ds.RawRunCB

	kc    ds.KeyContext
	head  memCollection
	dedup stringset.Set
}

func newNormalStrategy(kc ds.KeyContext, cb ds.RawRunCB, head memStore) queryStrategy {
	coll := head.GetCollection("ents:" + kc.Namespace)
	if coll == nil {
		return nil
	}
	return &normalStrategy{cb, kc, coll, stringset.New(0)}
}

func (s *normalStrategy) handle(rawData [][]byte, _ []ds.Property, key *ds.Key, gc func() (ds.Cursor, error)) error {
	rawKey := rawData[len(rawData)-1]
	if !s.dedup.Add(string(rawKey)) {
		return nil
	}

	rawEnt := s.head.Get(rawKey)
	if rawEnt == nil {
		// entity doesn't exist at head
		return nil
	}
	pm, err := serialize.ReadPropertyMap(bytes.NewBuffer(rawEnt), serialize.WithoutContext, s.kc)
	memoryCorruption(err)

	return s.cb(key, pm, gc)
}

func pickQueryStrategy(fq *ds.FinalizedQuery, rq *reducedQuery, cb ds.RawRunCB, head memStore) queryStrategy {
	if fq.KeysOnly() {
		return &keysOnlyStrategy{cb, stringset.New(0)}
	}
	if len(fq.Project()) > 0 {
		return newProjectionStrategy(fq, rq, cb)
	}
	return newNormalStrategy(rq.kc, cb, head)
}

func parseSuffix(aid, ns string, suffixFormat []ds.IndexColumn, suffix []byte, count int) (raw [][]byte, decoded []ds.Property) {
	buf := serialize.Invertible(bytes.NewBuffer(suffix))
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
		decoded[i], err = serialize.ReadProperty(buf, serialize.WithoutContext, kc)
		memoryCorruption(err)

		offset := len(suffix) - buf.Len()
		raw[i] = suffix[:offset]
		suffix = suffix[offset:]
		if needInvert {
			raw[i] = serialize.Invert(raw[i])
		}
	}

	return
}

func countQuery(fq *ds.FinalizedQuery, kc ds.KeyContext, isTxn bool, idx, head memStore) (ret int64, err error) {
	if len(fq.Project()) == 0 && !fq.KeysOnly() {
		fq, err = fq.Original().KeysOnly(true).Finalize()
		if err != nil {
			return
		}
	}
	err = executeQuery(fq, kc, isTxn, idx, head, func(_ *ds.Key, _ ds.PropertyMap, _ ds.CursorCB) error {
		ret++
		return nil
	})
	return
}

func executeNamespaceQuery(fq *ds.FinalizedQuery, kc ds.KeyContext, head memStore, cb ds.RawRunCB) error {
	// these objects have no properties, so any filters on properties cause an
	// empty result.
	if len(fq.EqFilters()) > 0 || len(fq.Project()) > 0 || len(fq.Orders()) > 1 {
		return nil
	}
	if !(fq.IneqFilterProp() == "" || fq.IneqFilterProp() == "__key__") {
		return nil
	}
	limit, hasLimit := fq.Limit()
	offset, hasOffset := fq.Offset()
	start, end := fq.Bounds()

	cursErr := errors.New("cursors not supported for __namespace__ query")
	cursFn := func() (ds.Cursor, error) { return nil, cursErr }
	if !(start == nil && end == nil) {
		return cursErr
	}

	kc.Namespace = ""
	for _, ns := range namespaces(head) {
		if hasOffset && offset > 0 {
			offset--
			continue
		}
		if hasLimit {
			if limit <= 0 {
				return nil
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
		if err := cb(k, nil, cursFn); err != nil {
			return err
		}
	}
	return nil
}

func executeQuery(fq *ds.FinalizedQuery, kc ds.KeyContext, isTxn bool, idx, head memStore, cb ds.RawRunCB) error {
	rq, err := reduce(fq, kc, isTxn)
	if err == ds.ErrNullQuery {
		return nil
	}
	if err != nil {
		return err
	}

	if rq.kind == "__namespace__" {
		return executeNamespaceQuery(fq, kc, head, cb)
	}

	idxs, err := getIndexes(rq, idx)
	if err == ds.ErrNullQuery {
		return nil
	}
	if err != nil {
		return err
	}

	strategy := pickQueryStrategy(fq, rq, cb, head)
	if strategy == nil {
		// e.g. the normalStrategy found that there were NO entities in the current
		// namespace.
		return nil
	}

	offset, _ := fq.Offset()
	limit, hasLimit := fq.Limit()

	cursorPrefix := []byte(nil)
	getCursorFn := func(suffix []byte) func() (ds.Cursor, error) {
		return func() (ds.Cursor, error) {
			if cursorPrefix == nil {
				buf := &bytes.Buffer{}
				_, err := cmpbin.WriteUint(buf, uint64(len(rq.suffixFormat)))
				memoryCorruption(err)

				for _, col := range rq.suffixFormat {
					err := serialize.WriteIndexColumn(buf, col)
					memoryCorruption(err)
				}
				cursorPrefix = buf.Bytes()
			}
			// TODO(riannucci): Do we need to decrement suffix instead of increment
			// if we're sorting by __key__ DESCENDING?
			return queryCursor(serialize.Join(cursorPrefix, increment(suffix))), nil
		}
	}

	return multiIterate(idxs, func(suffix []byte) error {
		if offset > 0 {
			offset--
			return nil
		}
		if hasLimit {
			if limit <= 0 {
				return nil
			}
			limit--
		}

		rawData, decodedProps := parseSuffix(kc.AppID, kc.Namespace, rq.suffixFormat, suffix, -1)

		keyProp := decodedProps[len(decodedProps)-1]
		if keyProp.Type() != ds.PTKey {
			impossible(fmt.Errorf("decoded index row doesn't end with a Key: %#v", keyProp))
		}

		return strategy.handle(
			rawData, decodedProps, keyProp.Value().(*ds.Key),
			getCursorFn(suffix))
	})
}
