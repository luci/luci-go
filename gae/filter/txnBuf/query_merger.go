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

package txnBuf

import (
	"bytes"
	"sort"

	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/datastore/serialize"
	"go.chromium.org/luci/common/data/stringset"
)

// queryToIter takes a FinalizedQuery and returns an iterator function which
// will produce either *items or errors.
//
//  - d is the raw datastore to run this query on
//  - filter is a function which will return true if the given key should be
//    excluded from the result set.
func queryToIter(stopChan chan struct{}, fq *ds.FinalizedQuery, d ds.RawInterface) func() (*item, error) {
	c := make(chan *item)

	go func() {
		defer close(c)

		err := d.Run(fq, func(k *ds.Key, pm ds.PropertyMap, _ ds.CursorCB) error {
			i := &item{key: k, data: pm}
			select {
			case c <- i:
				return nil
			case <-stopChan:
				return ds.Stop
			}
		})
		if err != nil {
			c <- &item{err: err}
		}
	}()

	return func() (*item, error) {
		itm := <-c
		if itm == nil {
			return nil, nil
		}
		if itm.err != nil {
			return nil, itm.err
		}
		return itm, nil
	}
}

// adjustQuery applies various mutations to the query to make it suitable for
// merging. In general, this removes limits and offsets the 'distinct' modifier,
// and it ensures that if there are sort orders which won't appear in the
// result data that the query is transformed into a projection query which
// contains all of the data. A non-projection query will never be transformed
// in this way.
func adjustQuery(fq *ds.FinalizedQuery) (*ds.FinalizedQuery, error) {
	q := fq.Original()

	// The limit and offset must be done in-memory because otherwise we may
	// request too few entities from the underlying store if many matching
	// entities have been deleted in the buffered transaction.
	q = q.Limit(-1)
	q = q.Offset(-1)

	// distinction must be done in-memory, because otherwise there's no way
	// to merge in the effect of the in-flight changes (because there's no way
	// to push back to the datastore "yeah, I know you told me that the (1, 2)
	// result came from `/Bob,1`, but would you mind pretending that it didn't
	// and tell me next the one instead?
	q = q.Distinct(false)

	// since we need to merge results, we must have all order-related fields
	// in each result. The only time we wouldn't have all the data available would
	// be for a keys-only or projection query. To fix this, we convert all
	// Projection and KeysOnly queries to project on /all/ Orders.
	//
	// FinalizedQuery already guarantees that all projected fields show up in
	// the Orders, but the projected fields could be a subset of the orders.
	//
	// Additionally on a keys-only query, any orders other than __key__ require
	// conversion of this query to a projection query including those orders in
	// order to merge the results correctly.
	//
	// In both cases, the resulting objects returned to the higher layers of the
	// stack will only include the information requested by the user; keys-only
	// queries will discard all PropertyMap data, and projection queries will
	// discard any field data that the user didn't ask for.
	orders := fq.Orders()
	if len(fq.Project()) > 0 || (fq.KeysOnly() && len(orders) > 1) {
		q = q.KeysOnly(false)

		for _, o := range orders {
			if o.Property == "__key__" {
				continue
			}
			q = q.Project(o.Property)
		}
	}

	return q.Finalize()
}

// runMergedQueries executes a user query `fq` against the parent datastore as
// well as the in-memory datastore, calling `cb` with the merged result set.
//
// It's expected that the caller of this function will apply limit and offset
// if the query contains those restrictions. This may convert the query to
// an expanded projection query with more data than the user asked for. It's the
// caller's responsibility to prune away the extra data.
//
// See also `dsTxnBuf.Run()`.
func runMergedQueries(fq *ds.FinalizedQuery, sizes *sizeTracker,
	memDS, parentDS ds.RawInterface, cb func(k *ds.Key, data ds.PropertyMap) error) error {

	toRun, err := adjustQuery(fq)
	if err != nil {
		return err
	}

	cmpLower, cmpUpper := memory.GetBinaryBounds(fq)
	cmpOrder := fq.Orders()
	cmpFn := func(i *item) string {
		return i.getCmpRow(cmpLower, cmpUpper, cmpOrder)
	}

	dedup := stringset.Set(nil)
	distinct := stringset.Set(nil)
	distinctOrder := []ds.IndexColumn(nil)
	if len(fq.Project()) > 0 { // the original query was a projection query
		if fq.Distinct() {
			// it was a distinct projection query, so we need to dedup by distinct
			// options.
			distinct = stringset.New(0)
			proj := fq.Project()
			distinctOrder = make([]ds.IndexColumn, len(proj))
			for i, p := range proj {
				distinctOrder[i].Property = p
			}
		}
	} else {
		// the original was a normal or keys-only query, so we need to dedup by keys.
		dedup = stringset.New(0)
	}

	stopChan := make(chan struct{})

	parIter := queryToIter(stopChan, toRun, parentDS)
	memIter := queryToIter(stopChan, toRun, memDS)

	parItemGet := func() (*item, error) {
		for {
			itm, err := parIter()
			if itm == nil || err != nil {
				return nil, err
			}
			encKey := itm.getEncKey()
			if sizes.has(encKey) || (dedup != nil && dedup.Has(encKey)) {
				continue
			}
			return itm, nil
		}
	}
	memItemGet := func() (*item, error) {
		for {
			itm, err := memIter()
			if itm == nil || err != nil {
				return nil, err
			}
			if dedup != nil && dedup.Has(itm.getEncKey()) {
				continue
			}
			return itm, nil
		}
	}

	defer func() {
		close(stopChan)
		parItemGet()
		memItemGet()
	}()

	pitm, err := parItemGet()
	if err != nil {
		return err
	}

	mitm, err := memItemGet()
	if err != nil {
		return err
	}

	for {
		// the err can be set during the loop below. If we come around the bend and
		// it's set, then we need to return it. We don't check it immediately
		// because it's set after we already have a good result to return to the
		// user.
		if err != nil {
			return err
		}

		usePitm := pitm != nil
		if pitm != nil && mitm != nil {
			usePitm = cmpFn(pitm) < cmpFn(mitm)
		} else if pitm == nil && mitm == nil {
			break
		}

		toUse := (*item)(nil)
		// we check the error at the beginning of the loop.
		if usePitm {
			toUse = pitm
			pitm, err = parItemGet()
		} else {
			toUse = mitm
			mitm, err = memItemGet()
		}

		if dedup != nil {
			if !dedup.Add(toUse.getEncKey()) {
				continue
			}
		}
		if distinct != nil {
			// NOTE: We know that toUse will not be used after this point for
			// comparison purposes, so re-use its cmpRow property for our distinct
			// filter here.
			toUse.cmpRow = ""
			if !distinct.Add(toUse.getCmpRow(nil, nil, distinctOrder)) {
				continue
			}
		}
		if err := cb(toUse.key, toUse.data); err != nil {
			return err
		}
	}

	return nil
}

// toComparableString computes the byte-sortable 'order' string for the given
// key/PropertyMap.
//
//   * start/end are byte sequences which are the inequality bounds of the
//     query, if any. These are a serialized datastore.Property. If the
//     inequality column is inverted, then start and end are also inverted and
//     swapped with each other.
//   * order is the list of sort orders in the actual executing queries.
//   * k / pm are the data to derive a sortable string for.
//
// The result of this function is the series of serialized properties, one per
// order column, which represent this key/pm's first entry in the composite
// index that would point to it (e.g. the one with `order` sort orders).
func toComparableString(start, end []byte, order []ds.IndexColumn, k *ds.Key, pm ds.PropertyMap) (row, key []byte) {
	doCmp := true
	soFar := []byte{}
	ps := serialize.PropertyMapPartially(k, nil)
	for _, ord := range order {
		row, ok := ps[ord.Property]
		if !ok {
			if pslice := pm.Slice(ord.Property); len(pslice) > 0 {
				row = serialize.PropertySlice(pslice)
			}
		}
		sort.Sort(row)
		foundOne := false
		for _, serialized := range row {
			if ord.Descending {
				serialized = serialize.Invert(serialized)
			}
			if doCmp {
				maybe := serialize.Join(soFar, serialized)
				cmp := bytes.Compare(maybe, start)
				if cmp >= 0 {
					foundOne = true
					soFar = maybe
					doCmp = len(soFar) < len(start)
					break
				}
			} else {
				foundOne = true
				soFar = serialize.Join(soFar, serialized)
				break
			}
		}
		if !foundOne {
			return nil, nil
		}
	}
	if end != nil && bytes.Compare(soFar, end) >= 0 {
		return nil, nil
	}
	return soFar, ps["__key__"][0]
}
