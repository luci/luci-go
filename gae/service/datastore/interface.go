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

package datastore

import (
	"container/heap"
	"context"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"sync"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
)

// AllocateIDs allows you to allocate IDs from the datastore without putting
// any data.
//
// A partial valid key will be constructed from each entity's kind and parent,
// if present. An allocation will then be performed against the datastore for
// each key, and the partial key will be populated with a unique integer ID.
// The resulting keys will be applied to their objects using PopulateKey. If
// successful, any existing ID will be destroyed.
//
// If the object is supplied that cannot accept an integer key, this method
// will panic.
//
// ent must be one of:
//   - *S where S is a struct
//   - *P where *P is a concrete type implementing PropertyLoadSaver
//   - []S or []*S where S is a struct
//   - []P or []*P where *P is a concrete type implementing PropertyLoadSaver
//   - []I, where I is some interface type. Each element of the slice must have
//     either *S or *P as its underlying type.
//   - []*Key, to populate a slice of partial-valid keys.
//
// nil values (or interface-typed nils) are not allowed, neither as standalone
// arguments nor inside slices. Passing them will cause a panic.
//
// If an error is encountered, the returned error value will depend on the
// input arguments. If one argument is supplied, the result will be the
// encountered error type. If multiple arguments are supplied, the result will
// be a MultiError whose error index corresponds to the argument in which the
// error was encountered.
//
// If an ent argument is a slice, its error type will be a MultiError. Note
// that in the scenario where multiple slices are provided, this will return a
// MultiError containing a nested MultiError for each slice argument.
func AllocateIDs(ctx context.Context, ent ...any) error {
	if len(ent) == 0 {
		return nil
	}

	mma, err := makeMetaMultiArg(ent, mmaWriteKeys)
	if err != nil {
		panic(err)
	}

	keys, _, et := mma.getKeysPMs(GetKeyContext(ctx), false)
	if len(keys) == 0 {
		return nil
	}

	var dat DroppedArgTracker
	dat.MarkNilKeys(keys)
	keys, dal := dat.DropKeys(keys)

	// Convert each key to be partial valid, assigning an integer ID of 0.
	// Confirm that each object can be populated with such a key.
	for compressedIdx, key := range keys {
		keys[compressedIdx] = key.Incomplete()
	}

	err = Raw(ctx).AllocateIDs(keys, func(compressedIdx int, key *Key, err error) {
		idx := dal.OriginalIndex(compressedIdx)

		index := mma.index(idx)

		if err != nil {
			et.trackError(index, err)
			return
		}

		mat, v := mma.get(index)
		if !mat.setKey(v, key) {
			et.trackError(index, errors.Fmt("failed to export key [%s]: %w", key, ErrInvalidKey))
			return
		}
	})
	if err == nil {
		err = et.error()
	}
	return maybeSingleError(err, ent)
}

// KeyForObj extracts a key from src.
//
// It is the same as KeyForObjErr, except that if KeyForObjErr would have
// returned an error, this method panics. It's safe to use if you know that
// src statically meets the metadata constraints described by KeyForObjErr.
func KeyForObj(ctx context.Context, src any) *Key {
	ret, err := KeyForObjErr(ctx, src)
	if err != nil {
		panic(err)
	}
	return ret
}

// KeyForObjErr extracts a key from src.
//
// src must be one of:
//   - *S, where S is a struct
//   - a PropertyLoadSaver
//
// It is expected that the struct exposes the following metadata (as retrieved
// by MetaGetter.GetMeta):
//   - "key" (type: Key) - The full datastore key to use. Must not be nil.
//     OR
//   - "id" (type: int64 or string) - The id of the Key to create.
//   - "kind" (optional, type: string) - The kind of the Key to create. If
//     blank or not present, KeyForObjErr will extract the name of the src
//     object's type.
//   - "parent" (optional, type: Key) - The parent key to use.
//
// By default, the metadata will be extracted from the struct and its tagged
// properties. However, if the struct implements MetaGetterSetter it is
// wholly responsible for exporting the required fields. A struct that
// implements GetMeta to make some minor tweaks can evoke the defualt behavior
// by using GetPLS(s).GetMeta.
//
// If a required metadata item is missing or of the wrong type, then this will
// return an error.
func KeyForObjErr(ctx context.Context, src any) (*Key, error) {
	return GetKeyContext(ctx).NewKeyFromMeta(getMGS(src))
}

// MakeKey is a convenience method for manufacturing a *Key. It should only be
// used when elems... is known statically (e.g. in the code) to be correct.
//
// elems is pairs of (string, string|int|int32|int64) pairs, which correspond
// to Kind/id pairs. Example:
//
//	dstore.MakeKey("Parent", 1, "Child", "id")
//
// Would create the key:
//
//	<current appID>:<current Namespace>:/Parent,1/Child,id
//
// If elems is not parsable (e.g. wrong length, wrong types, etc.) this method
// will panic.
func MakeKey(ctx context.Context, elems ...any) *Key {
	kc := GetKeyContext(ctx)
	return kc.MakeKey(elems...)
}

// NewKey constructs a new key in the current appID/Namespace, using the
// specified parameters.
func NewKey(ctx context.Context, kind, stringID string, intID int64, parent *Key) *Key {
	kc := GetKeyContext(ctx)
	return kc.NewKey(kind, stringID, intID, parent)
}

// NewIncompleteKeys allocates count incomplete keys sharing the same kind and
// parent. It is useful as input to AllocateIDs.
func NewIncompleteKeys(ctx context.Context, count int, kind string, parent *Key) (keys []*Key) {
	kc := GetKeyContext(ctx)
	if count > 0 {
		keys = make([]*Key, count)
		for i := range keys {
			keys[i] = kc.NewKey(kind, "", 0, parent)
		}
	}
	return
}

// NewKeyToks constructs a new key in the current appID/Namespace, using the
// specified key tokens.
func NewKeyToks(ctx context.Context, toks []KeyTok) *Key {
	kc := GetKeyContext(ctx)
	return kc.NewKeyToks(toks)
}

// PopulateKey loads key into obj.
//
// obj is any object that Interface.Get is able to accept.
//
// Upon successful application, this method will return true. If the key could
// not be applied to the object, this method will return false. It will panic if
// obj is an invalid datastore model.
func PopulateKey(obj any, key *Key) bool {
	return populateKeyMGS(getMGS(obj), key)
}

func populateKeyMGS(mgs MetaGetterSetter, key *Key) bool {
	setViaKey := mgs.SetMeta("key", key)

	lst := key.LastTok()
	mgs.SetMeta("kind", lst.Kind)
	mgs.SetMeta("parent", key.Parent())

	setViaID := false
	if lst.StringID != "" {
		setViaID = mgs.SetMeta("id", lst.StringID)
	} else {
		setViaID = mgs.SetMeta("id", lst.IntID)
	}

	return setViaKey || setViaID
}

// RunInTransaction runs f inside of a transaction. See the appengine SDK's
// documentation for full details on the behavior of transactions in the
// datastore.
//
// Note that the behavior of transactions may change depending on what filters
// have been installed. It's possible that we'll end up implementing things
// like nested/buffered transactions as filters.
func RunInTransaction(ctx context.Context, f func(ctx context.Context) error, opts *TransactionOptions) error {
	return Raw(ctx).RunInTransaction(f, opts)
}

// QueryIter holds the cursor callback and result iterator for `RunQuery`ing a
// Query.
//
// `V` must be one of:
//   - S or *S, where S is a struct
//   - P or *P, where *P is a concrete type implementing PropertyLoadSaver
//   - PropertyMap (effectively passes-through from the underlying
//     raw query, but will apply other QueryIter level features).
//   - *Key (implies a keys-only query)
type QueryIter[V any] struct {
	ctx context.Context
	q   *Query

	mat           *multiArgType
	isKey         bool
	isPropertyMap bool

	mu   sync.Mutex
	err  error
	used bool
	// nil means 'no limit was set'; this is used by AsSlice.
	// 0 means 'infinite'.
	sizeLimit *int
	filters   []func(V) (bool, error)
	raw       *RawQueryIter
}

func (q *QueryIter[V]) populateMat() {
	if q.mat == nil {
		var zero V

		t := reflect.TypeFor[V]()
		q.isKey = t == typeOfKey
		q.isPropertyMap = t == typeOfPropertyMap

		if !q.isKey && !q.isPropertyMap {
			q.mat = mustParseArg(t, false)
			if q.mat.newElem == nil {
				panic(fmt.Errorf("QueryIter[%T]: `V` is not a concrete type: %s", zero, t.Kind()))
			}
		}
	}
}

func (q *QueryIter[V]) ensureFinalizedLocked() RawQueryIter {
	if q.raw != nil {
		return *q.raw
	}

	if q.err != nil || q.q == nil {
		raw := RawQueryIterStub(q.err)
		q.raw = &raw
		return raw
	}

	qCopy := q.q
	if q.isKey {
		qCopy = qCopy.KeysOnly(true)
	}
	fq, err := qCopy.Finalize()

	var raw RawQueryIter
	if err != nil {
		raw = RawQueryIterStub(err)
	} else {
		raw = Raw(q.ctx).RunQuery(fq)
	}
	q.raw = &raw
	return raw
}

func (q *QueryIter[V]) mod(name string, cb func() error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.used {
		panic(fmt.Sprintf("QueryIter[V].%s: cannot call after using Results", name))
	}
	if q.err == nil {
		q.err = cb()
	}
}

// AddFilter will add `filt` to the stack of filters in this QueryIter.
//
// For each non-error item yielded by the query, filt will be called before
// yielding it.
//
// This should return (true, nil) if the item should be yielded, (false, nil)
// if the item should be skipped, or (*, <error>) if the iterator should yield
// an error and halt. If there are multiple filters, they will all be called
// in the order they were added, as long as they return (true, nil).
//
// Panics if the query has already yielded any results.
func (q *QueryIter[V]) AddFilter(filt func(V) (bool, error)) {
	if filt == nil {
		return
	}
	q.mod("AddFilter", func() error {
		q.filters = append(q.filters, filt)
		return nil
	})
}

// SetSizeLimit makes `Results` yield `ErrLimitExceeded` if the iterator yields
// more than this number of bytes (as computed by [PropertyMap.EstimateSize]).
//
// By default, use of Results does not have a limit, because it's understood
// that consumers are processing the results iteratively.
//
// However, for consumers which are simply stuffing the results into an
// ever-growing slice, it's a good idea to limit the amount of memory allocated
// in the service.
//
// Calling this with <= 0 removes the limit.
//
// Panics if the query has already yielded any results.
func (q *QueryIter[V]) SetSizeLimit(bytes int) {
	q.mod("SetSizeLimit", func() error {
		lim := max(0, bytes)
		q.sizeLimit = &lim
		return nil
	})
}

// Cursor returns a cursor to the *next* item which will be yielded by Results.
//
// Safe to call before pulling anything from Results.
func (q *QueryIter[V]) Cursor() (Cursor, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ensureFinalizedLocked()
	single, err := q.raw.Cursor()
	if err != nil {
		return nil, err
	}
	if mc, ok := single.(Cursor); ok {
		return mc, nil
	}
	return Cursor{single}, nil
}

// Results is an `iter.Seq2[V, error]` which yields query results in order.
//
// If an error is encountered, it is yielded and iteration stops.
//
// Using Results more than once (e.g. for more than one loop, or more than
// one pull iterator) will panic.
func (q *QueryIter[V]) Results(yield func(V, error) bool) {
	q.mu.Lock()
	if q.used {
		q.mu.Unlock()
		panic("cannot use QueryIter more than once.")
	}
	q.used = true
	rslts := q.ensureFinalizedLocked().Results
	var lim int
	if q.sizeLimit != nil {
		lim = *q.sizeLimit
	}
	q.mu.Unlock()

	var zero V
	var readSoFar int

	for pm, err := range rslts {
		if err != nil {
			yield(zero, err)
			return
		}

		if lim > 0 {
			readSoFar += int(pm.EstimateSize())
			if readSoFar > lim {
				yield(zero, ErrLimitExceeded)
				return
			}
		}

		key := mustGetKeyFromPM(pm)
		var val V
		if q.isKey {
			val = any(key).(V)
		} else if q.isPropertyMap {
			val = any(pm).(V)
			PopulateKey(pm, key)
		} else {
			itm := q.mat.newElem()
			cleanPM := pm
			if key != nil && len(pm) > 0 {
				cleanPM = pm.Clone()
				delete(cleanPM, "$key")
			}
			if err := q.mat.setPM(itm, cleanPM); err != nil {
				yield(zero, err)
				return
			}
			if key != nil {
				q.mat.setKey(itm, key)
			}
			val = itm.Interface().(V)
		}

		shouldYield := true
		var err error
		for _, filt := range q.filters {
			shouldYield, err = filt(val)
			if !shouldYield || err != nil {
				break
			}
		}

		if err != nil {
			yield(zero, err)
			return
		}
		if shouldYield {
			if !yield(val, nil) {
				return
			}
		}
	}
}

// queryIterStub wraps RawQueryIterStub into a QueryIter.
func queryIterStub[V any](err error) *QueryIter[V] {
	return QueryIterFromRaw[V](RawQueryIterStub(err))
}

// QueryIterFromRaw converts a RawQueryIter to a typed QueryIter.
func QueryIterFromRaw[V any](raw RawQueryIter) *QueryIter[V] {
	ret := &QueryIter[V]{raw: &raw}
	ret.populateMat()
	return ret
}

// RunQuery starts the given query, and returns the [*QueryIter[V]] which will
// yield the results.
//
// By default, datastore applies a short (~5s) timeout to queries. This can be
// increased, usually to around several minutes, by explicitly setting a
// deadline on the supplied Context.
//
// `V` must be one of:
//   - S or *S, where S is a struct
//   - P or *P, where *P is a concrete type implementing PropertyLoadSaver
//   - PropertyMap (effectively passes-through from the underlying
//     raw query, but will apply other QueryIter level features).
//   - *Key (implies a keys-only query)
//
// RunQuery will stop on the first datastore error encountered, which can occur
// due to flakiness, timeout, etc. If it encounters such an error, it will be
// yielded and the iterator stopped.
func RunQuery[V any](ctx context.Context, q *Query) *QueryIter[V] {
	ret := &QueryIter[V]{ctx: ctx, q: q}
	ret.populateMat()
	return ret
}

// RunMultiQuery executes the logical OR of multiple queries, returning a QueryIter[V]
// containing a Cursor function and an iter.Seq2[V, error] of results typed as V.
// Results will be returned in the order of the provided queries; All queries must
// have matching Orders.
//
// The cursor returned by Cursor() cannot be used on a single query by doing
// `query.Start(cursor)` (in some cases it may not even complain when you try to
// do this, but the results are undefined). Apply the cursor to the same list of
// queries using ApplyCursors.
//
// Note: projection queries are not supported, as they are non-trivial in
// complexity and haven't been needed yet.
//
// DANGER: Cursors are buggy when using Cloud Datastore production backend.
// Paginated queries skip entities sitting on page boundaries. This doesn't
// happen when using `impl/memory` and thus hard to spot in unit tests. See
// queryIterator doc for more details.
func RunMultiQuery[V any](ctx context.Context, queries []*Query) *QueryIter[V] {
	var zero V
	_, isKey := any(zero).(*Key)

	// Finalize queries and do some basic validation. At very least queries must
	// use the same kind and ordering, otherwise putting their results in a single
	// sorted heap makes no sense.
	finalized := make([]*FinalizedQuery, len(queries))
	overallKind := ""
	overallOrder := ""
	for i, q := range queries {
		if isKey {
			q = q.KeysOnly(true)
		}
		fq, err := q.Finalize()
		if err != nil {
			return queryIterStub[V](err)
		}
		finalized[i] = fq
		// Build a string identifying ordering of this query, e.g.
		// "-field1,field2,__key__".
		order := ""
		for j, col := range fq.orders {
			if j != 0 {
				order += ","
			}
			order += col.String()
		}
		switch {
		case i == 0:
			overallKind = fq.kind
			overallOrder = order
		case fq.kind != overallKind:
			return queryIterStub[V](fmt.Errorf("all RunMultiQuery queries should query the same kind, but got %q and %q", fq.kind, overallKind))
		case order != overallOrder:
			return queryIterStub[V](fmt.Errorf("all RunMultiQuery queries should use the same order, but got %q and %q", order, overallOrder))
		}
	}

	// No queries to run => no results to return. This is an edge case.
	if len(finalized) == 0 {
		return queryIterStub[V](nil)
	}

	if len(queries) == 1 {
		return RunQuery[V](ctx, queries[0])
	}

	var iterators []*queryIterator
	var iteratorsMu sync.Mutex

	cursorCB := func() (RawCursor, error) {
		iteratorsMu.Lock()
		iters := slices.Clone(iterators)
		iteratorsMu.Unlock()
		if len(iters) == 0 {
			return nil, errors.New("no cursor available")
		}

		// Sort the list of queries. It is OK to update `iterators` in-place here.
		// It is only used in the defer, the order doesn't matter there.
		sort.Slice(iters, func(i, j int) bool {
			queryI := iters[i].Query()
			queryJ := iters[j].Query()
			return queryI.Less(queryJ)
		})

		// Create the cursor. It points to all items currently sitting in heap.
		// We'll need to refetch them all again to repopulate the heap when
		// resuming the query.
		ret := make(Cursor, len(iters))
		for i, iter := range iters {
			cur, err := iter.CurrentCursor()
			if err != nil {
				return nil, err
			}
			ret[i] = cur
		}
		return ret, nil
	}

	results := func(yield func(PropertyMap, error) bool) {
		// All iterators (active and exhausted) in some arbitrary order.
		iters := make([]*queryIterator, 0, len(finalized))
		cCtx, cancel := context.WithCancel(ctx)
		eg, ectx := errgroup.WithContext(cCtx)

		// Make sure all spawned goroutines have fully stopped before returning.
		defer func() {
			// Signal all iterators to stop ASAP.
			cancel()
			// Wait for all of them to stop. Calling Next makes sure internal goroutines
			// are not getting stuck trying to write to a channel that nothing is
			// reading from (this blocks forever).
			for _, iter := range iters {
				for done := false; !done; done, _ = iter.Next() {
				}
			}
			// All goroutines should be stopping now. Wait until they are fully stopped.
			_ = eg.Wait()
		}()

		// Launch all queries in parallel. Do it before ordering them as a heap, since
		// to build a heap we need to have the first result from each query. We want
		// all such first results to be fetched *in parallel*.
		for _, fq := range finalized {
			iters = append(iters, startQueryIterator(ectx, eg, fq))
		}

		iteratorsMu.Lock()
		iterators = iters
		iteratorsMu.Unlock()

		// Wait for first items from all iterators. Gather all non-exhausted iterators
		// to make a sorted heap out of them.
		iHeap := make(iteratorHeap, 0, len(iters))
		for _, iter := range iters {
			switch done, err := iter.Next(); {
			case err != nil:
				yield(nil, err)
				return
			case !done:
				iHeap = append(iHeap, iter)
			}
		}
		heap.Init(&iHeap)

		// If queries are ordered only by key, all duplicates will be returned from
		// the heap one after another and we can use a simple check to skip them. This
		// is important for CountMulti(...) that can be visiting tens of thousands
		// of entities: storing them all in a hash map for deduplication is a waste of
		// memory.
		//
		// Use a hash map for any other ordering. There may be weird results if this
		// is running non-transactionally and two different subqueries see two
		// different versions of the same entity (with different values of fields
		// affecting the order). Such entity will appear twice in the output, with
		// some other entities in between these appearances. A simple check will not
		// detect such deduplication.
		var seenKey func(keyStr string) bool
		if overallOrder == "__key__" || overallOrder == "-__key__" {
			lastSeen := ""
			seenKey = func(keyStr string) bool {
				if lastSeen == keyStr {
					return true
				}
				lastSeen = keyStr
				return false
			}
		} else {
			seenKeys := stringset.New(128)
			seenKey = func(keyStr string) bool {
				return !seenKeys.Add(keyStr)
			}
		}

		// Merge query results.
		for iHeap.Len() > 0 {
			pm, key, keyStr, err := iHeap.nextData()
			if err != nil {
				if !yield(nil, err) {
					return
				}
				continue
			}
			if !seenKey(keyStr) {
				if pm == nil {
					pm = make(PropertyMap, 1)
				}
				pm.SetMeta("key", key)
				if !yield(pm, nil) {
					return
				}
			}
		}
	}

	ret := &QueryIter[V]{
		raw: &RawQueryIter{
			Results: results,
			Cursor:  cursorCB,
		},
	}
	ret.populateMat()
	return ret
}

// Count executes the given query and returns the number of entries which
// match it.
//
// If the query is marked as eventually consistent via EventualConsistency(true)
// will use a fast server-side aggregation, with the downside that such queries
// may return slightly stale results and can't be used inside transactions.
//
// If the query is strongly consistent, will essentially do a full keys-only
// query and count the number of matches locally.
func Count(ctx context.Context, q *Query) (int64, error) {
	fq, err := q.Finalize()
	if err != nil {
		return 0, err
	}
	v, err := Raw(ctx).Count(fq)
	return v, filterStop(err)
}

// CountMulti runs multiple queries in parallel and counts the total number of
// unique entities produced by them.
//
// Unlike Count, this method doesn't support server-side aggregation. It always
// does full keys-only queries. If you have only one query and don't care about
// strong consistency, use `Count(c, q.EventualConsistency(true))`: it will use
// the server-side aggregation which is orders of magnitude faster than the
// local counting.
func CountMulti(ctx context.Context, queries []*Query) (int64, error) {
	var count int64
	for _, err := range RunMultiQuery[*Key](ctx, queries).Results {
		if err != nil {
			return 0, err
		}
		count++
	}
	return count, nil
}

type getAllOptions struct {
	limit int
}

type getAllOption = func(*getAllOptions) error

// Function limit controls the behavior of GetAll. A positive limit indicates how many results
// to return.
func limit(n int) getAllOption {
	return func(o *getAllOptions) error {
		if n < 0 {
			return fmt.Errorf("n (%d) cannot be negative", n)
		}
		o.limit = n
		return nil
	}
}

// GetAll retrieves all of the Query results into dst.
//
// By default, datastore applies a short (~5s) timeout to queries. This can be
// increased, usually to around several minutes, by explicitly setting a
// deadline on the supplied Context.
//
// dst must be one of:
//   - *[]S or *[]*S, where S is a struct
//   - *[]P or *[]*P, where *P is a concrete type implementing
//     PropertyLoadSaver
//   - *[]*Key implies a keys-only query.
//
// Deprecated - Use GetAllWithLimit instead. If database happens to have many
// entities which matchq, GetAll can easily exhaust the available memory before
// returning, leading to an OOM error. If you use GetAllWithLimit you can pick
// an 'impossible' limit, which will still be safer by default than GetAll, and
// easier to debug, too.
func GetAll(ctx context.Context, q *Query, dst any) error {
	return getAllRaw(Raw(ctx), q, dst)
}

// GetAllWithLimit retrieves all of the Query results into dst up to a limit.
//
// GetAllWithLimit is like GetAll, but it applies a limit.
// If the limit is negative, we return an error.
// Additionally, if we exceed the limit, then we return ErrLimitExceeded indicating that
// a truncation has occurred.
//
// Note that GetAllWithLimit does NOT return the cursor. It is primarily intended as
// a way to migrate calls to GetAll to a version with more predictable behavior so
// that you get a nice failed RPC when the result set is too big rather than an a
// hard-to-debug OOM.
//
// By default, datastore applies a short (~5s) timeout to queries. This can be
// increased, usually to around several minutes, by explicitly setting a
// deadline on the supplied Context.
//
// dst must be one of:
//   - *[]S or *[]*S, where S is a struct
//   - *[]P or *[]*P, where *P is a concrete type implementing
//     PropertyLoadSaver
//   - *[]*Key implies a keys-only query.
func GetAllWithLimit(ctx context.Context, q *Query, dst any, lim int) error {
	if lim <= 0 {
		return fmt.Errorf("GetAllWithLimit: invalid limit %d <= 0", lim)
	}
	return getAllRaw(Raw(ctx), q, dst, limit(lim))
}

func getAllRaw(raw RawInterface, q *Query, dst any, o ...getAllOption) error {
	var cfg getAllOptions
	for _, f := range o {
		if err := f(&cfg); err != nil {
			return err
		}
	}
	v := reflect.ValueOf(dst)
	if v.Kind() != reflect.Pointer {
		panic(fmt.Errorf("invalid GetAll dst: must have a ptr-to-slice: %T", dst))
	}
	if !v.IsValid() || v.IsNil() {
		panic(errors.New("invalid GetAll dst: <nil>"))
	}

	if keys, ok := dst.(*[]*Key); ok {
		fq, err := q.KeysOnly(true).Finalize()
		if err != nil {
			return err
		}

		it := raw.RunQuery(fq)
		for pm, err := range it.Results {
			if err != nil {
				return err
			}
			*keys = append(*keys, mustGetKeyFromPM(pm))
		}
		return nil
	}
	fq, err := q.Finalize()
	if err != nil {
		return err
	}

	slice := v.Elem()
	mat := mustParseMultiArg(slice.Type())
	if mat.newElem == nil {
		panic(fmt.Errorf("invalid GetAll dst (non-concrete element type): %T", dst))
	}

	errs := map[int]error{}
	i := 0
	it := raw.RunQuery(fq)
	var runErr error
	for pm, err := range it.Results {
		if err != nil {
			runErr = filterStop(err)
			break
		}
		if cfg.limit > 0 && i >= cfg.limit {
			runErr = ErrLimitExceeded
			break
		}
		k := mustGetKeyFromPM(pm)
		slice.Set(reflect.Append(slice, mat.newElem()))
		itm := slice.Index(i)
		mat.setKey(itm, k)
		cleanPM := pm
		if k != nil && len(pm) > 0 {
			cleanPM = pm.Clone()
			delete(cleanPM, "$key")
		}
		if setErr := mat.setPM(itm, cleanPM); setErr != nil {
			errs[i] = setErr
		}
		i++
	}
	switch {
	case errors.Is(runErr, ErrLimitExceeded):
		return runErr
	case runErr == nil:
		if len(errs) > 0 {
			me := make(errors.MultiError, slice.Len())
			for i, e := range errs {
				me[i] = e
			}
			return me
		}
		return nil
	default:
		return runErr
	}
}

// Exists tests if the supplied objects are present in the datastore.
//
// ent must be one of:
//   - *S, where S is a struct
//   - *P, where *P is a concrete type implementing PropertyLoadSaver
//   - []S or []*S, where S is a struct
//   - []P or []*P, where *P is a concrete type implementing PropertyLoadSaver
//   - []I, where I is some interface type. Each element of the slice must have
//     either *S or *P as its underlying type.
//   - *Key, to check a specific key from the datastore.
//   - []*Key, to check a slice of keys from the datastore.
//
// nil values (or interface-typed nils) are not allowed, neither as standalone
// arguments nor inside slices. Passing them will cause a panic.
//
// If an error is encountered, the returned error value will depend on the
// input arguments. If one argument is supplied, the result will be the
// encountered error type. If multiple arguments are supplied, the result will
// be a MultiError whose error index corresponds to the argument in which the
// error was encountered.
//
// If an ent argument is a slice, its error type will be a MultiError. Note
// that in the scenario, where multiple slices are provided, this will return a
// MultiError containing a nested MultiError for each slice argument.
func Exists(ctx context.Context, ent ...any) (*ExistsResult, error) {
	if len(ent) == 0 {
		return nil, nil
	}

	mma, err := makeMetaMultiArg(ent, mmaKeysOnly)
	if err != nil {
		panic(err)
	}

	keys, _, et := mma.getKeysPMs(GetKeyContext(ctx), false)
	if len(keys) == 0 {
		return nil, nil
	}

	var dat DroppedArgTracker
	dat.MarkNilKeys(keys)
	keys, dal := dat.DropKeys(keys)

	bt := newBoolTracker(mma, et)
	err = Raw(ctx).GetMulti(keys, nil, func(compressedIdx int, _ PropertyMap, err error) {
		idx := dal.OriginalIndex(compressedIdx)
		bt.trackExistsResult(mma.index(idx), err)
	})

	if err == nil {
		err = bt.error()
	}
	return bt.result(), maybeSingleError(err, ent)
}

// Get retrieves objects from the datastore.
//
// Each element in dst must be one of:
//   - *S, where S is a struct
//   - *P, where *P is a concrete type implementing PropertyLoadSaver
//   - []S or []*S, where S is a struct
//   - []P or []*P, where *P is a concrete type implementing PropertyLoadSaver
//   - []I, where I is some interface type. Each element of the slice must have
//     either *S or *P as its underlying type.
//
// nil values (or interface-typed nils) are not allowed, neither as standalone
// arguments nor inside slices. Passing them will cause a panic.
//
// If an error is encountered, the returned error value will depend on the
// input arguments. If one argument is supplied, the result will be the
// encountered error type. If multiple arguments are supplied, the result will
// be a MultiError whose error index corresponds to the argument in which the
// error was encountered.
//
// If a dst argument is a slice, its error type will be a MultiError. Note
// that in the scenario where multiple slices are provided, this will return a
// MultiError containing a nested MultiError for each slice argument.
//
// If there was an issue retrieving the entity, the input `dst` objects will
// not be affected. This means that you can populate an object for dst with some
// values, do a Get, and on an ErrNoSuchEntity, do a Put (inside a transaction,
// of course :)).
func Get(ctx context.Context, dst ...any) error {
	if len(dst) == 0 {
		return nil
	}

	mma, err := makeMetaMultiArg(dst, mmaReadWrite)
	if err != nil {
		panic(err)
	}

	keys, pms, et := mma.getKeysPMs(GetKeyContext(ctx), true)
	if len(keys) == 0 {
		return nil
	}

	var dat DroppedArgTracker
	dat.MarkNilKeysVals(keys, pms)
	keys, pms, dal := dat.DropKeysAndVals(keys, pms)

	meta := NewMultiMetaGetter(pms)
	err = Raw(ctx).GetMulti(keys, meta, func(compressedIdx int, pm PropertyMap, err error) {
		idx := dal.OriginalIndex(compressedIdx)
		index := mma.index(idx)
		if err != nil {
			et.trackError(index, err)
			return
		}

		mat, v := mma.get(index)
		if err := mat.setPM(v, pm); err != nil {
			et.trackError(index, err)
			return
		}
	})

	if err == nil {
		err = et.error()
	}
	return maybeSingleError(err, dst)
}

// Put writes objects into the datastore.
//
// src must be one of:
//   - *S, where S is a struct
//   - *P, where *P is a concrete type implementing PropertyLoadSaver
//   - []S or []*S, where S is a struct
//   - []P or []*P, where *P is a concrete type implementing PropertyLoadSaver
//   - []I, where I is some interface type. Each element of the slice must have
//     either *S or *P as its underlying type.
//
// nil values (or interface-typed nils) are not allowed, neither as standalone
// arguments nor inside slices. Passing them will cause a panic.
//
// A *Key will be extracted from src via KeyForObj. If
// extractedKey.IsIncomplete() is true, and the object is put to the datastore
// successfully, then Put will write the resolved (datastore-generated) *Key
// back to src.
//
// NOTE: The datastore only autogenerates *Keys with integer IDs. Only models
// which use a raw `$key` or integer-typed `$id` field are elegible for this.
// A model with a string-typed `$id` field will not accept an integer id'd *Key
// and will cause the Put to fail.
//
// If an error is encountered, the returned error value will depend on the
// input arguments. If one argument is supplied, the result will be the
// encountered error type. If multiple arguments are supplied, the result will
// be a MultiError whose error index corresponds to the argument in which the
// error was encountered.
//
// If a src argument is a slice, its error type will be a MultiError. Note
// that in the scenario where multiple slices are provided, this will return a
// MultiError containing a nested MultiError for each slice argument.
func Put(ctx context.Context, src ...any) error {
	return putRaw(Raw(ctx), GetKeyContext(ctx), src)
}

func putRaw(raw RawInterface, kctx KeyContext, src []any) error {
	if len(src) == 0 {
		return nil
	}

	mma, err := makeMetaMultiArg(src, mmaReadWrite)
	if err != nil {
		panic(err)
	}

	keys, vals, et := mma.getKeysPMs(kctx, false)
	if len(keys) == 0 {
		return nil
	}

	var dat DroppedArgTracker
	dat.MarkNilKeysVals(keys, vals)
	keys, vals, dal := dat.DropKeysAndVals(keys, vals)

	err = raw.PutMulti(keys, vals, func(compressedIdx int, key *Key, err error) {
		idx := dal.OriginalIndex(compressedIdx)
		index := mma.index(idx)

		if err != nil {
			et.trackError(index, err)
			return
		}

		if !key.Equal(keys[compressedIdx]) {
			mat, v := mma.get(index)
			mat.setKey(v, key)
		}
	})
	if err == nil {
		err = et.error()
	}
	return maybeSingleError(err, src)
}

// Delete removes the supplied entities from the datastore.
//
// ent must be one of:
//   - *S, where S is a struct
//   - *P, where *P is a concrete type implementing PropertyLoadSaver
//   - []S or []*S, where S is a struct
//   - []P or []*P, where *P is a concrete type implementing PropertyLoadSaver
//   - []I, where I is some interface type. Each element of the slice must have
//     either *S or *P as its underlying type.
//   - *Key, to remove a specific key from the datastore.
//   - []*Key, to remove a slice of keys from the datastore.
//
// nil values (or interface-typed nils) are not allowed, neither as standalone
// arguments nor inside slices. Passing them will cause a panic.
//
// If an error is encountered, the returned error value will depend on the
// input arguments. If one argument is supplied, the result will be the
// encountered error type. If multiple arguments are supplied, the result will
// be a MultiError whose error index corresponds to the argument in which the
// error was encountered.
//
// If an ent argument is a slice, its error type will be a MultiError. Note
// that in the scenario where multiple slices are provided, this will return a
// MultiError containing a nested MultiError for each slice argument.
func Delete(ctx context.Context, ent ...any) error {
	if len(ent) == 0 {
		return nil
	}

	mma, err := makeMetaMultiArg(ent, mmaKeysOnly)
	if err != nil {
		panic(err)
	}

	keys, _, et := mma.getKeysPMs(GetKeyContext(ctx), false)
	if len(keys) == 0 {
		return nil
	}

	var dat DroppedArgTracker
	dat.MarkNilKeys(keys)
	keys, dal := dat.DropKeys(keys)

	err = Raw(ctx).DeleteMulti(keys, func(compressedIdx int, err error) {
		idx := dal.OriginalIndex(compressedIdx)

		if err != nil {
			index := mma.index(idx)
			et.trackError(index, err)
		}
	})

	if err == nil {
		err = et.error()
	}
	return maybeSingleError(err, ent)
}

// GetTestable returns the Testable interface for the implementation, or nil if
// there is none.
func GetTestable(ctx context.Context) Testable {
	return Raw(ctx).GetTestable()
}

// maybeSingleError normalizes the error experience between single- and
// multi-element API calls.
//
// Single-element API calls will return a single error for that element, while
// multi-element API calls will return a MultiError, one for each element. This
// accepts the slice of elements that is being operated on and determines what
// sort of error to return.
func maybeSingleError(err error, elems []any) error {
	if err == nil {
		return nil
	}
	if len(elems) == 1 {
		return errors.SingleError(err)
	}
	return err
}

func filterStop(err error) error {
	if err == Stop {
		err = nil
	}
	return err
}

// a min heap for a slice of queryIterator.
//
// All iterators are in "not done" state.
type iteratorHeap []*queryIterator

var _ heap.Interface = &iteratorHeap{}

func (h iteratorHeap) Len() int { return len(h) }

func (h iteratorHeap) Less(i, j int) bool { return h[i].CurrentItemOrder() < h[j].CurrentItemOrder() }

func (h iteratorHeap) Swap(i, j int) { h[i], h[j] = h[j], h[i] }

func (h *iteratorHeap) Push(x any) {
	*h = append(*h, x.(*queryIterator))
}

func (h *iteratorHeap) Pop() any {
	old := *h
	n := len(old)
	item := old[n-1]
	*h = old[0 : n-1]
	return item
}

// nextData returns data of the peak queryIterator, advances the queryIterator
// and either removes it from the heap (if it has no results left) or adjusts
// its position in the heap.
//
// Must be called only with a non-empty heap.
func (h *iteratorHeap) nextData() (pm PropertyMap, key *Key, keyStr string, err error) {
	if len(*h) == 0 {
		panic("the heap is empty")
	}

	qi := (*h)[0]
	key, pm = qi.CurrentItem()
	keyStr = qi.CurrentItemKey()

	var done bool
	done, err = qi.Next()
	if !done {
		heap.Fix(h, 0)
	} else {
		heap.Remove(h, 0)
	}

	return
}
