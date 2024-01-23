// Copyright 2016 The LUCI Authors.
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

package cloud

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/datastore"
	"google.golang.org/api/iterator"
	pb "google.golang.org/genproto/googleapis/datastore/v1"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/gae/impl/prod/constraints"
	ds "go.chromium.org/luci/gae/service/datastore"
)

type cloudDatastore struct {
	client *datastore.Client
}

func (cds *cloudDatastore) use(c context.Context) context.Context {
	return ds.SetRawFactory(c, func(ic context.Context) ds.RawInterface {
		return &boundDatastore{
			Context:        ic,
			cloudDatastore: cds,
			transaction:    datastoreTransaction(ic),
			kc:             ds.GetKeyContext(ic),
		}
	})
}

// boundDatastore is a bound instance of the cloudDatastore installed in the
// Context.
type boundDatastore struct {
	context.Context

	// Context is the bound user Context. It includes the datastore namespace, if
	// one is set.
	*cloudDatastore

	transaction *transactionWrapper
	kc          ds.KeyContext
}

func (bds *boundDatastore) AllocateIDs(keys []*ds.Key, cb ds.NewKeyCB) error {
	nativeKeys, err := bds.client.AllocateIDs(bds, gaeKeysToNative(keys))
	if err != nil {
		return normalizeError(err)
	}
	for i, key := range nativeKeys {
		cb(i, nativeKeyToGAE(bds.kc, key), nil)
	}
	return nil
}

func (bds *boundDatastore) RunInTransaction(fn func(context.Context) error, opts *ds.TransactionOptions) error {
	if bds.transaction != nil {
		return errors.New("nested transactions are not supported")
	}

	var txOpts []datastore.TransactionOption
	if opts != nil {
		if opts.ReadOnly {
			txOpts = append(txOpts, datastore.ReadOnly)
		}
		if opts.Attempts > 0 {
			txOpts = append(txOpts, datastore.MaxAttempts(opts.Attempts))
		}
	}

	_, err := bds.client.RunInTransaction(bds, func(tx *datastore.Transaction) error {
		return fn(withDatastoreTransaction(bds, tx))
	}, txOpts...)
	return normalizeError(err)
}

func (bds *boundDatastore) DecodeCursor(s string) (ds.Cursor, error) {
	cursor, err := datastore.DecodeCursor(s)
	return cursor, normalizeError(err)
}

func (bds *boundDatastore) Run(q *ds.FinalizedQuery, cb ds.RawRunCB) error {
	it := bds.client.Run(bds, bds.prepareNativeQuery(q))
	cursorFn := func() (ds.Cursor, error) {
		return it.Cursor()
	}

	for {
		var npl *nativePropertyLoader
		if !q.KeysOnly() {
			npl = &nativePropertyLoader{kc: bds.kc}
		}
		nativeKey, err := it.Next(npl)
		if err != nil {
			if err == iterator.Done {
				return nil
			}
			return normalizeError(err)
		}

		var pmap ds.PropertyMap
		if npl != nil {
			pmap = npl.pmap
		}
		if err := cb(nativeKeyToGAE(bds.kc, nativeKey), pmap, cursorFn); err != nil {
			if err == ds.Stop {
				return nil
			}
			return normalizeError(err)
		}
	}
}

func (bds *boundDatastore) Count(q *ds.FinalizedQuery) (int64, error) {
	// If the query is eventually consistent, use faster server-side aggregation.
	// For strongly-consistent queries we'll have to do local counting.
	if q.EventuallyConsistent() {
		res, err := bds.client.RunAggregationQuery(bds,
			bds.prepareNativeQuery(q).
				NewAggregationQuery().
				WithCount("total"),
		)
		if err != nil {
			return -1, normalizeError(err)
		}
		total, _ := res["total"].(*pb.Value)
		if total == nil {
			return -1, fmt.Errorf("aggregation result is unexpectedly missing")
		}
		return int64(total.GetIntegerValue()), nil
	}
	// Local counting. It is the only strongly-consistent method.
	v, err := bds.client.Count(bds, bds.prepareNativeQuery(q))
	if err != nil {
		return -1, normalizeError(err)
	}
	return int64(v), nil
}

func fixMultiError(err error) error {
	if err == nil {
		return nil
	}
	if baseME, ok := err.(datastore.MultiError); ok {
		return errors.NewMultiError(baseME...)
	}
	return err
}

func idxCallbacker(err error, amt int, cb func(idx int, err error)) error {
	if err == nil {
		for i := 0; i < amt; i++ {
			cb(i, nil)
		}
		return nil
	}

	err = fixMultiError(err)
	if me, ok := err.(errors.MultiError); ok {
		for i, err := range me {
			cb(i, normalizeError(err))
		}
		return nil
	}
	return normalizeError(err)
}

func (bds *boundDatastore) GetMulti(keys []*ds.Key, _meta ds.MultiMetaGetter, cb ds.GetMultiCB) error {
	nativeKeys := gaeKeysToNative(keys)
	nativePLS := make([]*nativePropertyLoader, len(nativeKeys))
	for i := range nativePLS {
		nativePLS[i] = &nativePropertyLoader{kc: bds.kc}
	}

	var err error
	if bds.transaction != nil {
		// Transactional GetMulti.
		err = bds.transaction.GetMulti(nativeKeys, nativePLS)
	} else {
		// Non-transactional GetMulti.
		err = bds.client.GetMulti(bds, nativeKeys, nativePLS)
	}

	return idxCallbacker(err, len(nativePLS), func(idx int, err error) {
		cb(idx, nativePLS[idx].pmap, err)
	})
}

func (bds *boundDatastore) PutMulti(keys []*ds.Key, vals []ds.PropertyMap, cb ds.NewKeyCB) error {
	nativeKeys := gaeKeysToNative(keys)
	nativePLS := make([]*nativePropertySaver, len(vals))
	for i := range nativePLS {
		nativePLS[i] = &nativePropertySaver{kc: bds.kc, pmap: vals[i]}
	}

	var err error
	if bds.transaction != nil {
		// Transactional PutMulti.
		//
		// In order to simulate the presence of mid-transaction key allocation, we
		// will identify any incomplete keys and allocate IDs for them. This is
		// potentially wasteful in the event of failed or retried transactions, but
		// it is required to maintain API compatibility with the datastore
		// interface.
		var incompleteKeys []*datastore.Key
		var incompleteKeyMap map[int]int
		for i, k := range nativeKeys {
			if k.Incomplete() {
				if incompleteKeyMap == nil {
					// Optimization: if there are any incomplete keys, allocate room for
					// the full range.
					incompleteKeyMap = make(map[int]int, len(nativeKeys)-i)
					incompleteKeys = make([]*datastore.Key, 0, len(nativeKeys)-i)
				}
				incompleteKeyMap[len(incompleteKeys)] = i
				incompleteKeys = append(incompleteKeys, k)
			}
		}
		if len(incompleteKeys) > 0 {
			idKeys, err := bds.client.AllocateIDs(bds, incompleteKeys)
			if err != nil {
				return err
			}
			for i, idKey := range idKeys {
				nativeKeys[incompleteKeyMap[i]] = idKey
			}
		}

		_, err = bds.transaction.PutMulti(nativeKeys, nativePLS)
	} else {
		// Non-transactional PutMulti.
		nativeKeys, err = bds.client.PutMulti(bds, nativeKeys, nativePLS)
	}

	return idxCallbacker(err, len(nativeKeys), func(idx int, err error) {
		if err == nil {
			cb(idx, nativeKeyToGAE(bds.kc, nativeKeys[idx]), nil)
			return
		}
		cb(idx, nil, err)
	})
}

func (bds *boundDatastore) DeleteMulti(keys []*ds.Key, cb ds.DeleteMultiCB) error {
	nativeKeys := gaeKeysToNative(keys)

	var err error
	if bds.transaction != nil {
		// Transactional DeleteMulti.
		err = bds.transaction.DeleteMulti(nativeKeys)
	} else {
		// Non-transactional DeleteMulti.
		err = bds.client.DeleteMulti(bds, nativeKeys)
	}

	return idxCallbacker(err, len(nativeKeys), cb)
}

func (bds *boundDatastore) WithoutTransaction() context.Context {
	return withoutDatastoreTransaction(bds)
}

func (bds *boundDatastore) CurrentTransaction() ds.Transaction {
	if bds.transaction == nil {
		return nil
	}
	return bds.transaction
}

func (bds *boundDatastore) Constraints() ds.Constraints { return constraints.DS() }

func (bds *boundDatastore) GetTestable() ds.Testable { return nil }

func (bds *boundDatastore) prepareNativeQuery(fq *ds.FinalizedQuery) *datastore.Query {
	nq := datastore.NewQuery(fq.Kind())
	if bds.transaction != nil {
		// NOTE: As of 2021 Q1 this is safe because it's documented that:
		//
		//   "Queries are re-usable and it is safe to call Query.Run from concurrent
		//   goroutines"
		//
		// Inspecting the datastore client code reveals that it only uses the `id`
		// field of the *Transaction object, not any of the state within the
		// *Transaction object which needs protection via the *transactionWrapper.
		nq = nq.Transaction(bds.transaction.tx)
	}
	if ns := bds.kc.Namespace; ns != "" {
		nq = nq.Namespace(ns)
	}

	// nativeFilter translates a filter field. If the translation fails, we'll
	// pass the result through to the underlying datastore and allow it to
	// reject it.
	nativeFilter := func(prop ds.Property) any {
		if np, err := gaePropertyToNative(bds.kc, "", prop); err == nil {
			return np.Value
		}
		return prop.Value()
	}

	// Equality filters.
	for field, props := range fq.EqFilters() {
		if field != "__ancestor__" {
			for _, prop := range props {
				nq = nq.FilterField(field, "=", nativeFilter(prop))
			}
		}
	}

	// Inequality filters.
	if ineq := fq.IneqFilterProp(); ineq != "" {
		if field, op, prop := fq.IneqFilterLow(); field != "" {
			nq = nq.FilterField(field, op, nativeFilter(prop))
		}
		if field, op, prop := fq.IneqFilterHigh(); field != "" {
			nq = nq.FilterField(field, op, nativeFilter(prop))
		}
	}

	start, end := fq.Bounds()
	if start != nil {
		nq = nq.Start(start.(datastore.Cursor))
	}
	if end != nil {
		nq = nq.End(end.(datastore.Cursor))
	}

	if fq.Distinct() {
		nq = nq.Distinct()
	}
	if fq.KeysOnly() {
		nq = nq.KeysOnly()
	}
	if limit, ok := fq.Limit(); ok {
		nq = nq.Limit(int(limit))
	}
	if offset, ok := fq.Offset(); ok {
		nq = nq.Offset(int(offset))
	}
	if proj := fq.Project(); proj != nil {
		nq = nq.Project(proj...)
	}
	if ancestor := fq.Ancestor(); ancestor != nil {
		nq = nq.Ancestor(gaeKeyToNative(ancestor))
	}
	if fq.EventuallyConsistent() {
		nq = nq.EventualConsistency()
	}

	for _, ic := range fq.Orders() {
		prop := ic.Property
		if ic.Descending {
			prop = "-" + prop
		}
		nq = nq.Order(prop)
	}

	return nq
}

func gaePropertyToNative(kc ds.KeyContext, name string, pdata ds.PropertyData) (nativeProp datastore.Property, err error) {
	nativeProp.Name = name

	convert := func(prop *ds.Property) (any, error) {
		switch pt := prop.Type(); pt {
		case ds.PTNull, ds.PTInt, ds.PTTime, ds.PTBool, ds.PTBytes, ds.PTString, ds.PTFloat:
			return prop.Value(), nil

		case ds.PTGeoPoint:
			gp := prop.Value().(ds.GeoPoint)
			return datastore.GeoPoint{Lat: gp.Lat, Lng: gp.Lng}, nil

		case ds.PTKey:
			return gaeKeyToNative(prop.Value().(*ds.Key)), nil

		case ds.PTPropertyMap:
			return gaeEntityToNative(kc, prop.Value().(ds.PropertyMap)), nil

		default:
			return nil, fmt.Errorf("unsupported property type: %v", pt)
		}
	}

	switch t := pdata.(type) {
	case ds.Property:
		if nativeProp.Value, err = convert(&t); err != nil {
			return
		}
		nativeProp.NoIndex = (t.IndexSetting() != ds.ShouldIndex)

	case ds.PropertySlice:
		// Don't index by default. If *any* sub-property requests being indexed,
		// then we will index.
		nativeProp.NoIndex = true

		// Pack this into an any so it is marked as a multi-value.
		multiProp := make([]any, len(t))
		for i := range t {
			prop := &t[i]
			if multiProp[i], err = convert(prop); err != nil {
				return
			}

			if prop.IndexSetting() == ds.ShouldIndex {
				nativeProp.NoIndex = false
			}
		}
		nativeProp.Value = multiProp

	default:
		err = fmt.Errorf("unsupported PropertyData type for %q: %T", name, pdata)
	}

	return
}

func nativePropertyToGAE(kc ds.KeyContext, nativeProp datastore.Property) (name string, pdata ds.PropertyData, err error) {
	name = nativeProp.Name

	convert := func(nv any, prop *ds.Property) error {
		switch nvt := nv.(type) {
		case nil:
			nv = nil

		case int64, bool, string, float64:
			break

		case []byte:
			if len(nvt) == 0 {
				// Cloud datastore library returns []byte{} if it is empty.
				// Make it nil as more convenient to deal with in tests.
				nv = []byte(nil)
			}

		case time.Time:
			// Cloud datastore library returns local time.
			nv = nvt.UTC()

		case datastore.GeoPoint:
			nv = ds.GeoPoint{Lat: nvt.Lat, Lng: nvt.Lng}

		case *datastore.Key:
			nv = nativeKeyToGAE(kc, nvt)

		case *datastore.Entity:
			nv = nativeEntityToGAE(kc, nvt)

		default:
			return fmt.Errorf("unsupported datastore.Value type for %q: %T", name, nvt)
		}

		indexSetting := ds.ShouldIndex
		if nativeProp.NoIndex {
			indexSetting = ds.NoIndex
		}
		prop.SetValue(nv, indexSetting)
		return nil
	}

	// Slice of supported native type. Convert this into PropertySlice.
	//
	// It must be an []any.
	if nativeValues, ok := nativeProp.Value.([]any); ok {
		pslice := make(ds.PropertySlice, len(nativeValues))
		for i, nv := range nativeValues {
			if err = convert(nv, &pslice[i]); err != nil {
				return
			}
		}
		pdata = pslice
		return
	}

	var prop ds.Property
	if err = convert(nativeProp.Value, &prop); err != nil {
		return
	}
	pdata = prop
	return
}

func gaeKeyToNative(key *ds.Key) *datastore.Key {
	var nativeKey *datastore.Key

	_, _, toks := key.Split()
	for _, tok := range toks {
		nativeKey = &datastore.Key{
			Kind:      tok.Kind,
			ID:        tok.IntID,
			Name:      tok.StringID,
			Parent:    nativeKey,
			Namespace: key.Namespace(),
		}
	}

	return nativeKey
}

func gaeKeysToNative(keys []*ds.Key) []*datastore.Key {
	nativeKeys := make([]*datastore.Key, len(keys))
	for i, key := range keys {
		nativeKeys[i] = gaeKeyToNative(key)
	}
	return nativeKeys
}

func nativeKeyToGAE(kc ds.KeyContext, nativeKey *datastore.Key) *ds.Key {
	toks := make([]ds.KeyTok, 0, 2)

	cur := nativeKey
	for {
		toks = append(toks, ds.KeyTok{Kind: cur.Kind, IntID: cur.ID, StringID: cur.Name})
		cur = cur.Parent
		if cur == nil {
			break
		}
	}

	// Reverse "toks" so we have ancestor-to-child lineage.
	for i := 0; i < len(toks)/2; i++ {
		ri := len(toks) - i - 1
		toks[i], toks[ri] = toks[ri], toks[i]
	}

	kc.Namespace = nativeKey.Namespace
	return kc.NewKeyToks(toks)
}

// nativeEntityToGAE returns a ds.PropertyMap representation of the given
// *datastore.Entity. Since properties can themselves be *datastore.Entities,
// the caller is responsible for ensuring there are no reference cycles.
func nativeEntityToGAE(kc ds.KeyContext, ent *datastore.Entity) ds.PropertyMap {
	if ent == nil {
		return nil
	}
	pm := make(ds.PropertyMap, len(ent.Properties)+4)
	if ent.Key != nil {
		// Populate all potentially supported meta properties. Whatever consumes
		// the property map (usually the default struct PLS) will choose properties
		// it cares about and ignore the rest.
		ds.PopulateKey(pm, nativeKeyToGAE(kc, ent.Key))
	}
	// Property ordering is lost since it's encoded to a map, but *datastore.Entity is
	// sourced from https://godoc.org/google.golang.org/genproto/googleapis/datastore/v1#Entity
	// which originally held properties in a map to begin with, meaning order is irrelevant.
	for _, p := range ent.Properties {
		_, prop, err := nativePropertyToGAE(kc, p)
		if err != nil {
			// Shouldn't happen. It means the *datastore.Entity contained an unsupported type.
			panic(err)
		}
		pm[p.Name] = prop
	}
	return pm
}

// gaeEntityToNative returns a *datastore.Entity representation of the given
// PropertyMap (assumed to have been produced by nativeEntityToGAE).
func gaeEntityToNative(kc ds.KeyContext, pm ds.PropertyMap) *datastore.Entity {
	// Ensure stable order. Skip meta fields, they'll be used in NewKeyFromMeta.
	keys := make([]string, 0, len(pm))
	for name := range pm {
		if !strings.HasPrefix(name, "$") {
			keys = append(keys, name)
		}
	}
	sort.Strings(keys)

	ent := &datastore.Entity{
		Properties: make([]datastore.Property, 0, len(keys)),
	}

	// Try to extract the entity key from available meta fields. Ignore incomplete
	// keys. This actually happens for structs that don't have any explicitly
	// defined meta properties (because `$kind` is implicitly defined, so they end
	// up with an incomplete key, since they have no `$id`).
	if key, _ := kc.NewKeyFromMeta(pm); key != nil && !key.IsIncomplete() {
		ent.Key = gaeKeyToNative(key)
	}

	// Convert non-meta fields.
	for _, name := range keys {
		p, err := gaePropertyToNative(kc, name, pm[name])
		if err != nil {
			// Shouldn't happen. It means nativeEntityToGAE encoded an unsupported type.
			panic(err)
		}
		ent.Properties = append(ent.Properties, p)
	}
	return ent
}

// nativePropertyLoader is a datastore.PropertyLoadSaver that implement Load
// by writing properties into a ds.PropertyMap.
type nativePropertyLoader struct {
	kc   ds.KeyContext
	pmap ds.PropertyMap // starts as nil, gets created and populated in Load
}

var _ datastore.PropertyLoadSaver = (*nativePropertyLoader)(nil)

func (npl *nativePropertyLoader) Load(props []datastore.Property) error {
	if npl.pmap == nil {
		npl.pmap = make(ds.PropertyMap, len(props))
	}

	for _, nativeProp := range props {
		name, pdata, err := nativePropertyToGAE(npl.kc, nativeProp)
		if err != nil {
			return err
		}
		if _, ok := npl.pmap[name]; ok {
			return fmt.Errorf("duplicate properties for %q", name)
		}
		npl.pmap[name] = pdata
	}
	return nil
}

func (npl *nativePropertyLoader) Save() ([]datastore.Property, error) {
	panic("must not be called")
}

// nativePropertySaver is a datastore.PropertyLoadSaver that implement Save
// by reading properties from a ds.PropertyMap.
type nativePropertySaver struct {
	kc   ds.KeyContext
	pmap ds.PropertyMap // must be set by the caller
}

var _ datastore.PropertyLoadSaver = (*nativePropertySaver)(nil)

func (nps *nativePropertySaver) Load(props []datastore.Property) error {
	panic("must not be called")
}

func (nps *nativePropertySaver) Save() ([]datastore.Property, error) {
	if len(nps.pmap) == 0 {
		return nil, nil
	}

	props := make([]datastore.Property, 0, len(nps.pmap))
	for name, pdata := range nps.pmap {
		// Strip meta.
		if strings.HasPrefix(name, "$") {
			continue
		}

		nativeProp, err := gaePropertyToNative(nps.kc, name, pdata)
		if err != nil {
			return nil, err
		}
		props = append(props, nativeProp)
	}
	return props, nil
}

// transactionWrapper provides a Mutex around mutation calls on the Transaction.
//
// This is required until https://github.com/googleapis/google-cloud-go/issues/3750
// is fixed.
type transactionWrapper struct {
	mu sync.Mutex
	tx *datastore.Transaction
}

func (tw *transactionWrapper) GetMulti(keys []*datastore.Key, dst any) (err error) {
	// We don't acquire a lock here because as of 2021 Q1 Transaction.GetMulti
	// only reads the Transaction.id field, and doesn't make any mutations to the
	// *Transaction state at all.
	return tw.tx.GetMulti(keys, dst)
}

func (tw *transactionWrapper) PutMulti(keys []*datastore.Key, src any) (ret []*datastore.PendingKey, err error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	return tw.tx.PutMulti(keys, src)
}

func (tw *transactionWrapper) DeleteMulti(keys []*datastore.Key) (err error) {
	tw.mu.Lock()
	defer tw.mu.Unlock()
	return tw.tx.DeleteMulti(keys)
}

var datastoreTransactionKey = "*transactionWrapper"

func withDatastoreTransaction(c context.Context, tx *datastore.Transaction) context.Context {
	return context.WithValue(c, &datastoreTransactionKey, &transactionWrapper{tx: tx})
}

func withoutDatastoreTransaction(c context.Context) context.Context {
	return context.WithValue(c, &datastoreTransactionKey, nil)
}

func datastoreTransaction(c context.Context) *transactionWrapper {
	if tw, ok := c.Value(&datastoreTransactionKey).(*transactionWrapper); ok {
		return tw
	}
	return nil
}

func normalizeError(err error) error {
	switch err {
	case datastore.ErrNoSuchEntity:
		return ds.ErrNoSuchEntity
	case datastore.ErrConcurrentTransaction:
		return ds.ErrConcurrentTransaction
	case datastore.ErrInvalidKey:
		return ds.MakeErrInvalidKey("").Err()
	default:
		return err
	}
}
