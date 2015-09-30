// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package txnBuf

import (
	"bytes"
	"sync"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/serialize"
	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/common/parallel"
	"github.com/luci/luci-go/common/stringset"
	"golang.org/x/net/context"
)

// DefaultSizeBudget is the size budget for the root transaction.
//
// Because our estimation algorithm isn't entirely correct, we take 5% off
// the limit for encoding and estimate inaccuracies.
//
// 10MB taken on 2015/09/24:
// https://cloud.google.com/appengine/docs/go/datastore/#Go_Quotas_and_limits
const DefaultSizeBudget = int64((10 * 1000 * 1000) * 0.95)

// DefaultSizeThreshold prevents the root transaction from getting too close
// to the budget. If the code attempts to begin a transaction which would have
// less than this threshold for its budget, the transaction will immediately
// return ErrTransactionTooLarge.
const DefaultSizeThreshold = int64(10 * 1000)

// XGTransactionGroupLimit is the number of transaction groups to allow in an
// XG transaction.
//
// 25 taken on 2015/09/24:
// https://cloud.google.com/appengine/docs/go/datastore/transactions#Go_What_can_be_done_in_a_transaction
const XGTransactionGroupLimit = 25

// sizeTracker tracks the size of a buffered transaction. The rules are simple:
//   * deletes count for the size of their key, but 0 data
//   * puts count for the size of their key plus the 'EstimateSize' for their
//     data.
type sizeTracker struct {
	keyToSize map[string]int64
	total     int64
}

// set states that the given key is being set to an entity with the size `val`.
// A val of 0 means "I'm deleting this key"
func (s *sizeTracker) set(key string, val int64) {
	if s.keyToSize == nil {
		s.keyToSize = make(map[string]int64)
	}
	prev, existed := s.keyToSize[key]
	s.keyToSize[key] = val
	s.total += val - prev
	if !existed {
		s.total += int64(len(key))
	}
}

// get returns the currently tracked size for key, and wheter or not the key
// has any tracked value.
func (s *sizeTracker) get(key string) (int64, bool) {
	size, has := s.keyToSize[key]
	return size, has
}

// has returns true iff key has a tracked value.
func (s *sizeTracker) has(key string) bool {
	_, has := s.keyToSize[key]
	return has
}

// dup returns a duplicate sizeTracker.
func (s *sizeTracker) dup() *sizeTracker {
	if len(s.keyToSize) == 0 {
		return &sizeTracker{}
	}
	k2s := make(map[string]int64, len(s.keyToSize))
	for k, v := range s.keyToSize {
		k2s[k] = v
	}
	return &sizeTracker{k2s, s.total}
}

type txnBufState struct {
	sync.Mutex

	// encoded key -> size of entity. A size of 0 means that the entity is
	// deleted.
	entState *sizeTracker
	memDS    datastore.RawInterface

	roots     stringset.Set
	rootLimit int

	aid         string
	ns          string
	parentDS    datastore.RawInterface
	parentState *txnBufState

	// sizeBudget is the number of bytes that this transaction has to operate
	// within. It's only used when attempting to apply() the transaction, and
	// it is the threshold for the delta of applying this transaction to the
	// parent transaction. Note that a buffered transaction could actually have
	// a negative delta if the parent transaction had many large entities which
	// the inner transaction deleted.
	sizeBudget int64

	// siblingLock is to prevent two nested transactions from running at the same
	// time.
	//
	// Example:
	//   RunInTransaction() { // root
	//     RunInTransaction() // A
	//     RunInTransaction() // B
	//   }
	//
	// This will prevent A and B from running simulatneously.
	siblingLock sync.Mutex
}

func withTxnBuf(ctx context.Context, cb func(context.Context) error, opts *datastore.TransactionOptions) error {
	inf := info.Get(ctx)
	ns := inf.GetNamespace()

	parentState, _ := ctx.Value(dsTxnBufParent).(*txnBufState)
	roots := stringset.New(0)
	rootLimit := 1
	if opts != nil && opts.XG {
		rootLimit = XGTransactionGroupLimit
	}
	sizeBudget := DefaultSizeBudget
	if parentState != nil {
		parentState.siblingLock.Lock()
		defer parentState.siblingLock.Unlock()

		// TODO(riannucci): this is a bit wonky since it means that a child
		// transaction declaring XG=true will only get to modify 25 groups IF
		// they're same groups affected by the parent transactions. So instead of
		// respecting opts.XG for inner transactions, we just dup everything from
		// the parent transaction.
		roots = parentState.roots.Dup()
		rootLimit = parentState.rootLimit

		sizeBudget = parentState.sizeBudget - parentState.entState.total
		if sizeBudget < DefaultSizeThreshold {
			return ErrTransactionTooLarge
		}
	}

	memDS, err := memory.NewDatastore(inf.FullyQualifiedAppID(), ns)
	if err != nil {
		return err
	}

	state := &txnBufState{
		entState:    &sizeTracker{},
		memDS:       memDS.Raw(),
		roots:       roots,
		rootLimit:   rootLimit,
		ns:          ns,
		aid:         inf.AppID(),
		parentDS:    datastore.Get(ctx).Raw(),
		parentState: parentState,
		sizeBudget:  sizeBudget,
	}
	if err = cb(context.WithValue(ctx, dsTxnBufParent, state)); err != nil {
		return err
	}
	return state.apply()
}

// item is a temporary object for representing key/entity pairs and their cache
// state (e.g. if they exist in the in-memory datastore buffer or not).
// Additionally item memoizes some common comparison strings. item objects
// must never be persisted outside of a single function/query context.
type item struct {
	key      *datastore.Key
	data     datastore.PropertyMap
	buffered bool

	encKey string

	// cmpRow is used to hold the toComparableString value for this item during
	// a query.
	cmpRow string

	// err is a bit of a hack for passing back synchronized errors from
	// queryToIter.
	err error
}

func (i *item) getEncKey() string {
	if i.encKey == "" {
		i.encKey = string(serialize.ToBytes(i.key))
	}
	return i.encKey
}

func (i *item) getCmpRow(lower, upper []byte, order []datastore.IndexColumn) string {
	if i.cmpRow == "" {
		row, key := toComparableString(lower, upper, order, i.key, i.data)
		i.cmpRow = string(row)
		if i.encKey == "" {
			i.encKey = string(key)
		}
	}
	return i.cmpRow
}

func (t *txnBufState) updateRootsLocked(roots stringset.Set) error {
	curRootLen := t.roots.Len()
	proposedRoots := stringset.New(1)
	roots.Iter(func(root string) bool {
		if !t.roots.Has(root) {
			proposedRoots.Add(root)
		}
		return proposedRoots.Len()+curRootLen <= t.rootLimit
	})
	if proposedRoots.Len()+curRootLen > t.rootLimit {
		return ErrTooManyRoots
	}
	// only need to update the roots if they did something that required updating
	if proposedRoots.Len() > 0 {
		proposedRoots.Iter(func(root string) bool {
			t.roots.Add(root)
			return true
		})
	}
	return nil
}

func (t *txnBufState) getMulti(keys []*datastore.Key) ([]item, error) {
	encKeys, roots := toEncoded(keys)
	ret := make([]item, len(keys))

	idxMap := []int(nil)
	toGetKeys := []*datastore.Key(nil)

	t.Lock()
	defer t.Unlock()

	if err := t.updateRootsLocked(roots); err != nil {
		return nil, err
	}

	for i, key := range keys {
		ret[i].key = key
		ret[i].encKey = encKeys[i]
		if size, ok := t.entState.get(ret[i].getEncKey()); ok {
			ret[i].buffered = true
			if size > 0 {
				idxMap = append(idxMap, i)
				toGetKeys = append(toGetKeys, key)
			}
		}
	}

	if len(toGetKeys) > 0 {
		j := 0
		t.memDS.GetMulti(toGetKeys, nil, func(pm datastore.PropertyMap, err error) {
			impossible(err)
			ret[idxMap[j]].data = pm
			j++
		})
	}

	return ret, nil
}

func (t *txnBufState) deleteMulti(keys []*datastore.Key) error {
	encKeys, roots := toEncoded(keys)

	t.Lock()
	defer t.Unlock()

	if err := t.updateRootsLocked(roots); err != nil {
		return err
	}

	i := 0
	err := t.memDS.DeleteMulti(keys, func(err error) {
		impossible(err)
		t.entState.set(encKeys[i], 0)
		i++
	})
	impossible(err)
	return nil
}

func (t *txnBufState) putMulti(keys []*datastore.Key, vals []datastore.PropertyMap) error {
	encKeys, roots := toEncoded(keys)

	t.Lock()
	defer t.Unlock()

	if err := t.updateRootsLocked(roots); err != nil {
		return err
	}

	i := 0
	err := t.memDS.PutMulti(keys, vals, func(k *datastore.Key, err error) {
		impossible(err)
		t.entState.set(encKeys[i], vals[i].EstimateSize())
		i++
	})
	impossible(err)
	return nil
}

// apply actually takes the buffered transaction and applies it to the parent
// transaction. It will only return an error if the underlying 'real' datastore
// returns an error on PutMulti or DeleteMulti.
func (t *txnBufState) apply() error {
	t.Lock()
	defer t.Unlock()

	// if parentState is nil... just try to commit this anyway. The estimates
	// we're using here are just educated guesses. If it fits for real, then
	// hooray. If not, then the underlying datastore will error.
	if t.parentState != nil {
		t.parentState.Lock()
		proposedState := t.parentState.entState.dup()
		t.parentState.Unlock()
		for k, v := range t.entState.keyToSize {
			proposedState.set(k, v)
		}
		if proposedState.total > t.sizeBudget {
			return ErrTransactionTooLarge
		}
	}

	toPutKeys := []*datastore.Key(nil)
	toPut := []datastore.PropertyMap(nil)
	toDel := []*datastore.Key(nil)

	// need to pull all items out of the in-memory datastore. Fortunately we have
	// kindless queries, and we disabled all the special entities, so just
	// run a kindless query without any filters and it will return all data
	// currently in memDS :).
	fq, err := datastore.NewQuery("").Finalize()
	impossible(err)

	err = t.memDS.Run(fq, func(key *datastore.Key, data datastore.PropertyMap, _ datastore.CursorCB) bool {
		toPutKeys = append(toPutKeys, key)
		toPut = append(toPut, data)
		return true
	})
	memoryCorruption(err)

	for keyStr, size := range t.entState.keyToSize {
		if size == 0 {
			k, err := serialize.ReadKey(bytes.NewBufferString(keyStr), serialize.WithoutContext, t.aid, t.ns)
			memoryCorruption(err)
			toDel = append(toDel, k)
		}
	}

	ds := t.parentDS

	return parallel.FanOutIn(func(ch chan<- func() error) {
		if len(toPut) > 0 {
			ch <- func() error {
				mErr := errors.NewLazyMultiError(len(toPut))
				i := 0
				err := ds.PutMulti(toPutKeys, toPut, func(_ *datastore.Key, err error) {
					mErr.Assign(i, err)
					i++
				})
				if err == nil {
					err = mErr.Get()
				}
				return err
			}
		}
		if len(toDel) > 0 {
			ch <- func() error {
				mErr := errors.NewLazyMultiError(len(toDel))
				i := 0
				err := ds.DeleteMulti(toDel, func(err error) {
					mErr.Assign(i, err)
					i++
				})
				if err == nil {
					err = mErr.Get()
				}
				return err
			}
		}
	})
}

// toEncoded returns a list of all of the serialized versions of these keys,
// plus a stringset of all the encoded root keys that `keys` represents.
func toEncoded(keys []*datastore.Key) (full []string, roots stringset.Set) {
	roots = stringset.New(len(keys))
	full = make([]string, len(keys))
	for i, k := range keys {
		roots.Add(string(serialize.ToBytes(k.Root())))
		full[i] = string(serialize.ToBytes(k))
	}
	return
}
