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
	"sort"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"

	multicursor "go.chromium.org/luci/gae/service/datastore/internal/protos/multicursor"
)

type resolvedRunCallback func(reflect.Value, CursorCB) error

func parseRunCallback(cbIface any) (rcb resolvedRunCallback, isKey bool, mat *multiArgType, hasCursorCB bool) {
	badSig := func() {
		panic(fmt.Errorf(
			"cb does not match the required callback signature: `%T` != `func(TYPE, [CursorCB]) [error]`",
			cbIface))
	}

	if cbIface == nil {
		badSig()
	}

	// TODO(riannucci): Profile and determine if any of this is causing a real
	// slowdown. Could potentially cache reflection stuff by cbTyp?
	cbVal := reflect.ValueOf(cbIface)
	cbTyp := cbVal.Type()

	if cbTyp.Kind() != reflect.Func {
		badSig()
	}

	numIn := cbTyp.NumIn()
	if numIn != 1 && numIn != 2 {
		badSig()
	}

	firstArg := cbTyp.In(0)
	if firstArg == typeOfKey {
		isKey = true
	} else {
		mat = mustParseArg(firstArg, false)
		if mat.newElem == nil {
			badSig()
		}
	}

	hasCursorCB = numIn == 2
	if hasCursorCB && cbTyp.In(1) != typeOfCursorCB {
		badSig()
	}

	if cbTyp.NumOut() > 1 {
		badSig()
	} else if cbTyp.NumOut() == 1 && cbTyp.Out(0) != typeOfError {
		badSig()
	}
	hasErr := cbTyp.NumOut() == 1

	// Resolve to generic function.
	switch {
	case hasErr && hasCursorCB:
		// func(reflect.Value, CursorCB) error
		rcb = func(v reflect.Value, cb CursorCB) error {
			err := cbVal.Call([]reflect.Value{v, reflect.ValueOf(cb)})[0].Interface()
			if err != nil {
				return err.(error)
			}
			return nil
		}

	case hasErr && !hasCursorCB:
		// func(reflect.Value) error
		rcb = func(v reflect.Value, _ CursorCB) error {
			err := cbVal.Call([]reflect.Value{v})[0].Interface()
			if err != nil {
				return err.(error)
			}
			return nil
		}

	case !hasErr && hasCursorCB:
		// func(reflect.Value, CursorCB)
		rcb = func(v reflect.Value, cb CursorCB) error {
			cbVal.Call([]reflect.Value{v, reflect.ValueOf(cb)})
			return nil
		}

	case !hasErr && !hasCursorCB:
		// func(reflect.Value)
		rcb = func(v reflect.Value, _ CursorCB) error {
			cbVal.Call([]reflect.Value{v})
			return nil
		}

	default:
		badSig()
	}

	return
}

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
func AllocateIDs(c context.Context, ent ...any) error {
	if len(ent) == 0 {
		return nil
	}

	mma, err := makeMetaMultiArg(ent, mmaWriteKeys)
	if err != nil {
		panic(err)
	}

	keys, _, et := mma.getKeysPMs(GetKeyContext(c), false)
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

	err = Raw(c).AllocateIDs(keys, func(compressedIdx int, key *Key, err error) {
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
func KeyForObj(c context.Context, src any) *Key {
	ret, err := KeyForObjErr(c, src)
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
func KeyForObjErr(c context.Context, src any) (*Key, error) {
	return GetKeyContext(c).NewKeyFromMeta(getMGS(src))
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
func MakeKey(c context.Context, elems ...any) *Key {
	kc := GetKeyContext(c)
	return kc.MakeKey(elems...)
}

// NewKey constructs a new key in the current appID/Namespace, using the
// specified parameters.
func NewKey(c context.Context, kind, stringID string, intID int64, parent *Key) *Key {
	kc := GetKeyContext(c)
	return kc.NewKey(kind, stringID, intID, parent)
}

// NewIncompleteKeys allocates count incomplete keys sharing the same kind and
// parent. It is useful as input to AllocateIDs.
func NewIncompleteKeys(c context.Context, count int, kind string, parent *Key) (keys []*Key) {
	kc := GetKeyContext(c)
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
func NewKeyToks(c context.Context, toks []KeyTok) *Key {
	kc := GetKeyContext(c)
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
func RunInTransaction(c context.Context, f func(c context.Context) error, opts *TransactionOptions) error {
	return Raw(c).RunInTransaction(f, opts)
}

// Run executes the given query, and calls `cb` for each successfully
// retrieved item.
//
// By default, datastore applies a short (~5s) timeout to queries. This can be
// increased, usually to around several minutes, by explicitly setting a
// deadline on the supplied Context.
//
// cb is a callback function whose signature is
//
//	func(obj TYPE[, getCursor CursorCB]) [error]
//
// Where TYPE is one of:
//   - S or *S, where S is a struct
//   - P or *P, where *P is a concrete type implementing PropertyLoadSaver
//   - *Key (implies a keys-only query)
//
// If the error is omitted from the signature, this will run until the query
// returns all its results, or has an error/times out.
//
// If error is in the signature, the query will continue as long as the
// callback returns nil. If it returns `Stop`, the query will stop and Run
// will return nil. Otherwise, the query will stop and Run will return the
// user's error.
//
// Run may also stop on the first datastore error encountered, which can occur
// due to flakiness, timeout, etc. If it encounters such an error, it will
// be returned.
func Run(c context.Context, q *Query, cb any) error {
	rcb, isKey, mat, _ := parseRunCallback(cb)

	if isKey {
		q = q.KeysOnly(true)
	}
	fq, err := q.Finalize()
	if err != nil {
		return err
	}

	raw := Raw(c)

	if isKey {
		err = raw.Run(fq, func(k *Key, _ PropertyMap, gc CursorCB) error {
			return rcb(reflect.ValueOf(k), gc)
		})
	} else {
		err = raw.Run(fq, func(k *Key, pm PropertyMap, gc CursorCB) error {
			itm := mat.newElem()
			if err := mat.setPM(itm, pm); err != nil {
				return err
			}
			mat.setKey(itm, k)
			return rcb(itm, gc)
		})
	}
	return filterStop(err)
}

// RunMulti executes the logical OR of multiple queries, calling `cb` for each
// unique entity (by *Key) that it finds. Results will be returned in the order
// of the provided queries; All queries must have matching Orders.
//
// cb is a callback function (please refer to the `Run` function comments for
// formats and restrictions for `cb` in this file).
//
// The cursor that is returned by the callback cannot be used on a single query
// by doing `query.Start(cursor)` (In some cases it may not even complain when
// you try to do this. But the results are undefined). Apply the cursor to the
// same list of queries using ApplyCursors.
//
// Note: projection queries are not supported, as they are non-trivial in
// complexity and haven't been needed yet.
//
// Note: The cb is called for every unique entity (by *Key) that is retrieved
// on the current run. It is possible to get the same entity twice over two
// calls to RunMulti with different cursors.
//
// DANGER: Cursors are buggy when using Cloud Datastore production backend.
// Paginated queries skip entities sitting on page boundaries. This doesn't
// happen when using `impl/memory` and thus hard to spot in unit tests. See
// queryIterator doc for more details.
func RunMulti(c context.Context, queries []*Query, cb any) error {
	rcb, isKey, mat, hasCursorCB := parseRunCallback(cb)

	// A helper that passes an entity to the user callback.
	var dispatchEntity func(key *Key, pm PropertyMap, ccb CursorCB) error
	if isKey {
		dispatchEntity = func(key *Key, _ PropertyMap, ccb CursorCB) error {
			return rcb(reflect.ValueOf(key), ccb)
		}
	} else {
		dispatchEntity = func(key *Key, pm PropertyMap, ccb CursorCB) error {
			itm := mat.newElem()
			if err := mat.setPM(itm, pm); err != nil {
				return err
			}
			mat.setKey(itm, key)
			return rcb(itm, ccb)
		}
	}

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
			return err
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
			return fmt.Errorf("all RunMulti queries should query the same kind, but got %q and %q", fq.kind, overallKind)
		case order != overallOrder:
			return fmt.Errorf("all RunMulti queries should use the same order, but got %q and %q", order, overallOrder)
		}
	}

	// No queries to run => no results to return. This is an edge case.
	if len(finalized) == 0 {
		return nil
	}

	// If we have only one query, just run it directly without any extra
	// synchronization overhead. Just make sure to use the correct cursor format.
	// This is worth optimizing since running only one query is a very very
	// common case.
	if len(finalized) == 1 {
		var err error
		if hasCursorCB {
			err = Raw(c).Run(finalized[0], func(key *Key, pm PropertyMap, cursorCB CursorCB) error {
				return dispatchEntity(key, pm, func() (Cursor, error) {
					cur, err := cursorCB()
					if err != nil {
						return nil, err
					}
					cursorStr := ""
					if cur != nil {
						cursorStr = cur.String()
					}
					return multiCursor{
						curs: &multicursor.Cursors{
							Version:     multiCursorVersion,
							MagicNumber: multiCursorMagic,
							Cursors:     []string{cursorStr},
						},
					}, nil
				})
			})
		} else {
			err = Raw(c).Run(finalized[0], func(key *Key, pm PropertyMap, _ CursorCB) error {
				return dispatchEntity(key, pm, nil)
			})
		}
		return filterStop(err)
	}

	// All iterators (active and exhausted) in some arbitrary order.
	iterators := make([]*queryIterator, 0, len(finalized))

	c, cancel := context.WithCancel(c)
	eg, ectx := errgroup.WithContext(c)

	// Make sure all spawned goroutines have fully stopped before returning.
	defer func() {
		// Signal all iterators to stop ASAP.
		cancel()
		// Wait for all of them to stop. Calling Next makes sure internal goroutines
		// are not getting stuck trying to write to a channel that nothing is
		// reading from (this blocks forever).
		for _, iter := range iterators {
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
		iterators = append(iterators, startQueryIterator(ectx, eg, fq))
	}

	// Wait for first items from all iterators. Gather all non-exhausted iterators
	// to make a sorted heap out of them.
	iHeap := make(iteratorHeap, 0, len(iterators))
	for _, iter := range iterators {
		switch done, err := iter.Next(); {
		case err != nil:
			return err // the defer will clean up everything
		case !done:
			iHeap = append(iHeap, iter)
		}
	}
	heap.Init(&iHeap)

	// ccb is the cursor callback for RunMulti. This grabs all the cursors for the
	// queries involved and returns a single cursor. It is only executed if the
	// user callback invokes it.
	var ccb CursorCB
	if hasCursorCB {
		ccb = func() (Cursor, error) {
			// Sort the list of queries. It is OK to update `iterators` in-place here.
			// It is only used in the defer, the order doesn't matter there.
			sort.Slice(iterators, func(i, j int) bool {
				queryI := iterators[i].Query()
				queryJ := iterators[j].Query()
				return queryI.Less(queryJ)
			})
			// Create the cursor. It points to all items currently sitting in heap.
			// We'll need to refetch them all again to repopulate the heap when
			// resuming the query.
			var curs multicursor.Cursors
			curs.MagicNumber = multiCursorMagic
			curs.Version = multiCursorVersion
			for _, iter := range iterators {
				switch cur, err := iter.CurrentCursor(); {
				case err != nil:
					return nil, err
				case cur != nil:
					curs.Cursors = append(curs.Cursors, cur.String())
				default:
					curs.Cursors = append(curs.Cursors, "")
				}
			}
			return multiCursor{curs: &curs}, nil
		}
	}

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
			return err
		}
		if !seenKey(keyStr) {
			if err := dispatchEntity(key, pm, ccb); err != nil {
				return filterStop(err)
			}
		}
	}
	return nil
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
func Count(c context.Context, q *Query) (int64, error) {
	fq, err := q.Finalize()
	if err != nil {
		return 0, err
	}
	v, err := Raw(c).Count(fq)
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
func CountMulti(c context.Context, queries []*Query) (int64, error) {
	var count int64
	err := RunMulti(c, queries,
		func(_ *Key) error {
			// RunMulti already does deduplication, we just need to count unique hits.
			count++
			return nil
		},
	)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// DecodeCursor converts a string returned by a Cursor into a Cursor instance.
// It will return an error if the supplied string is not valid, or could not
// be decoded by the implementation.
func DecodeCursor(c context.Context, s string) (Cursor, error) {
	return Raw(c).DecodeCursor(s)
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
func GetAll(c context.Context, q *Query, dst any) error {
	return getAllRaw(Raw(c), q, dst)
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
	if v.Kind() != reflect.Ptr {
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

		return raw.Run(fq, func(k *Key, _ PropertyMap, _ CursorCB) error {
			*keys = append(*keys, k)
			return nil
		})
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
	err = filterStop(raw.Run(fq, func(k *Key, pm PropertyMap, _ CursorCB) error {
		if cfg.limit > 0 && i >= cfg.limit {
			return ErrLimitExceeded
		}
		slice.Set(reflect.Append(slice, mat.newElem()))
		itm := slice.Index(i)
		mat.setKey(itm, k)
		err := mat.setPM(itm, pm)
		if err != nil {
			errs[i] = err
		}
		i++
		return nil
	}))
	switch {
	case errors.Is(err, ErrLimitExceeded):
		return err
	case err == nil:
		if len(errs) > 0 {
			me := make(errors.MultiError, slice.Len())
			for i, e := range errs {
				me[i] = e
			}
			err = me
		}
	}
	return err
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
func Exists(c context.Context, ent ...any) (*ExistsResult, error) {
	if len(ent) == 0 {
		return nil, nil
	}

	mma, err := makeMetaMultiArg(ent, mmaKeysOnly)
	if err != nil {
		panic(err)
	}

	keys, _, et := mma.getKeysPMs(GetKeyContext(c), false)
	if len(keys) == 0 {
		return nil, nil
	}

	var dat DroppedArgTracker
	dat.MarkNilKeys(keys)
	keys, dal := dat.DropKeys(keys)

	bt := newBoolTracker(mma, et)
	err = Raw(c).GetMulti(keys, nil, func(compressedIdx int, _ PropertyMap, err error) {
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
func Get(c context.Context, dst ...any) error {
	if len(dst) == 0 {
		return nil
	}

	mma, err := makeMetaMultiArg(dst, mmaReadWrite)
	if err != nil {
		panic(err)
	}

	keys, pms, et := mma.getKeysPMs(GetKeyContext(c), true)
	if len(keys) == 0 {
		return nil
	}

	var dat DroppedArgTracker
	dat.MarkNilKeysVals(keys, pms)
	keys, pms, dal := dat.DropKeysAndVals(keys, pms)

	meta := NewMultiMetaGetter(pms)
	err = Raw(c).GetMulti(keys, meta, func(compressedIdx int, pm PropertyMap, err error) {
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
func Put(c context.Context, src ...any) error {
	return putRaw(Raw(c), GetKeyContext(c), src)
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
func Delete(c context.Context, ent ...any) error {
	if len(ent) == 0 {
		return nil
	}

	mma, err := makeMetaMultiArg(ent, mmaKeysOnly)
	if err != nil {
		panic(err)
	}

	keys, _, et := mma.getKeysPMs(GetKeyContext(c), false)
	if len(keys) == 0 {
		return nil
	}

	var dat DroppedArgTracker
	dat.MarkNilKeys(keys)
	keys, dal := dat.DropKeys(keys)

	err = Raw(c).DeleteMulti(keys, func(compressedIdx int, err error) {
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
func GetTestable(c context.Context) Testable {
	return Raw(c).GetTestable()
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
