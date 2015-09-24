// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"fmt"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/serialize"
	"github.com/luci/luci-go/common/cmpbin"
	"github.com/luci/luci-go/common/stringset"
)

type queryStrategy interface {
	// handle applies the strategy to the embedded user callback.
	//   - rawData is the slice of encoded Properties from the index row
	//     (correctly de-inverted).
	//   - decodedProps is the slice of decoded Properties from the index row
	//   - key is the decoded Key from the index row (the last item in rawData and
	//     decodedProps)
	//   - gc is the getCursor function to be passed to the user's callback
	handle(rawData [][]byte, decodedProps []ds.Property, key *ds.Key, gc func() (ds.Cursor, error)) bool
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

func (s *projectionStrategy) handle(rawData [][]byte, decodedProps []ds.Property, key *ds.Key, gc func() (ds.Cursor, error)) bool {
	projectedRaw := [][]byte(nil)
	if s.distinct != nil {
		projectedRaw = make([][]byte, len(decodedProps))
	}
	pmap := make(ds.PropertyMap, len(s.project))
	for i, p := range s.project {
		if s.distinct != nil {
			projectedRaw[i] = rawData[p.suffixIndex]
		}
		pmap[p.propertyName] = []ds.Property{decodedProps[p.suffixIndex]}
	}
	if s.distinct != nil {
		if !s.distinct.Add(string(serialize.Join(projectedRaw...))) {
			return true
		}
	}
	return s.cb(key, pmap, gc)
}

type keysOnlyStrategy struct {
	cb ds.RawRunCB

	dedup stringset.Set
}

func (s *keysOnlyStrategy) handle(rawData [][]byte, _ []ds.Property, key *ds.Key, gc func() (ds.Cursor, error)) bool {
	if !s.dedup.Add(string(rawData[len(rawData)-1])) {
		return true
	}
	return s.cb(key, nil, gc)
}

type normalStrategy struct {
	cb ds.RawRunCB

	ns    string
	head  *memCollection
	dedup stringset.Set
}

func newNormalStrategy(ns string, cb ds.RawRunCB, head *memStore) queryStrategy {
	coll := head.GetCollection("ents:" + ns)
	if coll == nil {
		return nil
	}
	return &normalStrategy{cb, ns, coll, stringset.New(0)}
}

func (s *normalStrategy) handle(rawData [][]byte, _ []ds.Property, key *ds.Key, gc func() (ds.Cursor, error)) bool {
	rawKey := rawData[len(rawData)-1]
	if !s.dedup.Add(string(rawKey)) {
		return true
	}

	rawEnt := s.head.Get(rawKey)
	if rawEnt == nil {
		// entity doesn't exist at head
		return true
	}
	pm, err := serialize.ReadPropertyMap(bytes.NewBuffer(rawEnt), serialize.WithoutContext, globalAppID, s.ns)
	memoryCorruption(err)

	return s.cb(key, pm, gc)
}

func pickQueryStrategy(fq *ds.FinalizedQuery, rq *reducedQuery, cb ds.RawRunCB, head *memStore) queryStrategy {
	if fq.KeysOnly() {
		return &keysOnlyStrategy{cb, stringset.New(0)}
	}
	if len(fq.Project()) > 0 {
		return newProjectionStrategy(fq, rq, cb)
	}
	return newNormalStrategy(rq.ns, cb, head)
}

func parseSuffix(ns string, suffixFormat []ds.IndexColumn, suffix []byte, count int) (raw [][]byte, decoded []ds.Property) {
	buf := serialize.Invertible(bytes.NewBuffer(suffix))
	decoded = make([]ds.Property, len(suffixFormat))
	raw = make([][]byte, len(suffixFormat))

	err := error(nil)
	for i := range decoded {
		if count > 0 && i > count {
			break
		}
		needInvert := suffixFormat[i].Descending

		buf.SetInvert(needInvert)
		decoded[i], err = serialize.ReadProperty(buf, serialize.WithoutContext, globalAppID, ns)
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

func executeQuery(fq *ds.FinalizedQuery, ns string, isTxn bool, idx, head *memStore, cb ds.RawRunCB) error {
	rq, err := reduce(fq, ns, isTxn)
	if err == ds.ErrNullQuery {
		return nil
	}
	if err != nil {
		return err
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

	multiIterate(idxs, func(suffix []byte) bool {
		if offset > 0 {
			offset--
			return true
		}
		if hasLimit {
			if limit <= 0 {
				return false
			}
			limit--
		}

		rawData, decodedProps := parseSuffix(ns, rq.suffixFormat, suffix, -1)

		keyProp := decodedProps[len(decodedProps)-1]
		if keyProp.Type() != ds.PTKey {
			impossible(fmt.Errorf("decoded index row doesn't end with a Key: %#v", keyProp))
		}

		return strategy.handle(
			rawData, decodedProps, keyProp.Value().(*ds.Key),
			getCursorFn(suffix))
	})

	return nil
}
