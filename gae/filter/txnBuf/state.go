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
	"sync"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/datastore/serialize"
	"go.chromium.org/gae/service/info"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
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

// DefaultWriteCountBudget is the maximum number of entities that can be written
// in a single call.
//
// This is not known to be documented, and has instead been extracted from a
// datastore error message.
const DefaultWriteCountBudget = 500

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

// numWrites returns the number of tracked write operations.
func (s *sizeTracker) numWrites() int {
	return len(s.keyToSize)
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
	bufDS    datastore.RawInterface

	roots     stringset.Set
	rootLimit int

	kc       datastore.KeyContext
	parentDS datastore.RawInterface

	// sizeBudget is the number of bytes that this transaction has to operate
	// within. It's only used when attempting to apply() the transaction, and
	// it is the threshold for the delta of applying this transaction to the
	// parent transaction. Note that a buffered transaction could actually have
	// a negative delta if the parent transaction had many large entities which
	// the inner transaction deleted.
	sizeBudget int64
	// countBudget is the number of entity writes that this transaction has to
	// operate in.
	writeCountBudget int
}

func withTxnBuf(ctx context.Context, cb func(context.Context) error, opts *datastore.TransactionOptions) error {
	parentState, _ := ctx.Value(&dsTxnBufParent).(*txnBufState)
	roots := stringset.New(0)
	rootLimit := 1
	if opts != nil && opts.XG {
		rootLimit = XGTransactionGroupLimit
	}
	sizeBudget, writeCountBudget := DefaultSizeBudget, DefaultWriteCountBudget
	if parentState != nil {
		// TODO(riannucci): this is a bit wonky since it means that a child
		// transaction declaring XG=true will only get to modify 25 groups IF
		// they're same groups affected by the parent transactions. So instead of
		// respecting opts.XG for inner transactions, we just dup everything from
		// the parent transaction.
		roots = parentState.roots.Dup()
		rootLimit = parentState.rootLimit

		sizeBudget = parentState.sizeBudget - parentState.entState.total
		writeCountBudget = parentState.writeCountBudget - parentState.entState.numWrites()
	}

	state := &txnBufState{
		entState:         &sizeTracker{},
		bufDS:            memory.NewDatastore(ctx, info.Raw(ctx)),
		roots:            roots,
		rootLimit:        rootLimit,
		kc:               datastore.GetKeyContext(ctx),
		parentDS:         datastore.Raw(context.WithValue(ctx, &dsTxnBufHaveLock, true)),
		sizeBudget:       sizeBudget,
		writeCountBudget: writeCountBudget,
	}
	if err := cb(context.WithValue(ctx, &dsTxnBufParent, state)); err != nil {
		return err
	}

	// no reason to unlock this ever. At this point it's toast.
	state.Lock()

	if parentState == nil {
		return commitToReal(state)
	}

	if err := parentState.canApplyLocked(state); err != nil {
		return err
	}

	parentState.commitLocked(state)
	return nil
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

func (t *txnBufState) getMulti(keys []*datastore.Key, metas datastore.MultiMetaGetter, cb datastore.GetMultiCB, haveLock bool) error {
	encKeys, roots := toEncoded(keys)
	data := make([]item, len(keys))

	idxMap := []int(nil)
	toGetKeys := []*datastore.Key(nil)

	lme := errors.NewLazyMultiError(len(keys))
	err := func() error {
		if !haveLock {
			t.Lock()
			defer t.Unlock()
		}

		if err := t.updateRootsLocked(roots); err != nil {
			return err
		}

		for i, key := range keys {
			data[i].key = key
			data[i].encKey = encKeys[i]
			if size, ok := t.entState.get(data[i].getEncKey()); ok {
				data[i].buffered = true
				if size > 0 {
					idxMap = append(idxMap, i)
					toGetKeys = append(toGetKeys, key)
				}
			}
		}

		if len(toGetKeys) > 0 {
			t.bufDS.GetMulti(toGetKeys, nil, func(j int, pm datastore.PropertyMap, err error) error {
				impossible(err)
				data[idxMap[j]].data = pm
				return nil
			})
		}

		idxMap = nil
		getKeys := []*datastore.Key(nil)
		getMetas := datastore.MultiMetaGetter(nil)

		for i, itm := range data {
			if !itm.buffered {
				idxMap = append(idxMap, i)
				getKeys = append(getKeys, itm.key)
				getMetas = append(getMetas, metas.GetSingle(i))
			}
		}

		if len(idxMap) > 0 {
			err := t.parentDS.GetMulti(getKeys, getMetas, func(j int, pm datastore.PropertyMap, err error) error {
				if err != datastore.ErrNoSuchEntity {
					i := idxMap[j]
					if !lme.Assign(i, err) {
						data[i].data = pm
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	}()
	if err != nil {
		return err
	}

	for i, itm := range data {
		err := lme.GetOne(i)
		var cbErr error
		if err != nil {
			cbErr = cb(i, nil, err)
		} else if itm.data == nil {
			cbErr = cb(i, nil, datastore.ErrNoSuchEntity)
		} else {
			cbErr = cb(i, itm.data, nil)
		}
		if cbErr != nil {
			return cbErr
		}
	}
	return nil
}

func (t *txnBufState) deleteMulti(keys []*datastore.Key, cb datastore.DeleteMultiCB, haveLock bool) error {
	encKeys, roots := toEncoded(keys)

	err := func() error {
		if !haveLock {
			t.Lock()
			defer t.Unlock()
		}

		if err := t.updateRootsLocked(roots); err != nil {
			return err
		}

		err := t.bufDS.DeleteMulti(keys, func(i int, err error) error {
			impossible(err)
			t.entState.set(encKeys[i], 0)
			return nil
		})
		impossible(err)
		return nil
	}()
	if err != nil {
		return err
	}

	for i := range keys {
		if err := cb(i, nil); err != nil {
			return err
		}
	}

	return nil
}

func (t *txnBufState) fixKeys(keys []*datastore.Key) ([]*datastore.Key, error) {
	// Identify any incomplete keys and allocate IDs for them.
	//
	// In order to facilitate this, we will maintain a mapping of the
	// incompleteKeys index to the key's corresponding index in the keys array.
	// Any errors or allocations on incompleteKeys operations will be propagated
	// to the correct keys index using this map.
	var (
		incompleteKeys []*datastore.Key
		incompleteMap  map[int]int
	)

	for i, key := range keys {
		if key.IsIncomplete() {
			if incompleteMap == nil {
				incompleteMap = make(map[int]int)
			}
			incompleteMap[len(incompleteKeys)] = i
			incompleteKeys = append(incompleteKeys, key)
		}
	}
	if len(incompleteKeys) == 0 {
		return keys, nil
	}

	// We're going to update keys, so clone it.
	keys, origKeys := make([]*datastore.Key, len(keys)), keys
	copy(keys, origKeys)

	// Intentionally call AllocateIDs without lock.
	outerErr := errors.NewLazyMultiError(len(keys))
	err := t.parentDS.AllocateIDs(incompleteKeys, func(i int, key *datastore.Key, err error) error {
		outerIdx := incompleteMap[i]

		if err != nil {
			outerErr.Assign(outerIdx, err)
		} else {
			keys[outerIdx] = key
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return keys, outerErr.Get()
}

func (t *txnBufState) putMulti(keys []*datastore.Key, vals []datastore.PropertyMap, cb datastore.NewKeyCB, haveLock bool) error {
	keys, err := t.fixKeys(keys)
	if err != nil {
		for i, e := range err.(errors.MultiError) {
			if err := cb(i, nil, e); err != nil {
				return err
			}
		}
		return nil
	}

	encKeys, roots := toEncoded(keys)

	err = func() error {
		if !haveLock {
			t.Lock()
			defer t.Unlock()
		}

		if err := t.updateRootsLocked(roots); err != nil {
			return err
		}

		err := t.bufDS.PutMulti(keys, vals, func(i int, k *datastore.Key, err error) error {
			impossible(err)
			t.entState.set(encKeys[i], vals[i].EstimateSize())
			return nil
		})
		impossible(err)
		return nil
	}()
	if err != nil {
		return err
	}

	for i, k := range keys {
		if err := cb(i, k, nil); err != nil {
			return err
		}
	}
	return nil
}

func commitToReal(s *txnBufState) error {
	toPut, toPutKeys, toDel := s.effect()

	return parallel.FanOutIn(func(ch chan<- func() error) {
		if len(toPut) > 0 {
			ch <- func() error {
				mErr := errors.NewLazyMultiError(len(toPut))
				err := s.parentDS.PutMulti(toPutKeys, toPut, func(i int, _ *datastore.Key, err error) error {
					mErr.Assign(i, err)
					return nil
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
				err := s.parentDS.DeleteMulti(toDel, func(i int, err error) error {
					mErr.Assign(i, err)
					return nil
				})
				if err == nil {
					err = mErr.Get()
				}
				return err
			}
		}
	})
}

func (t *txnBufState) effect() (toPut []datastore.PropertyMap, toPutKeys, toDel []*datastore.Key) {
	// TODO(riannucci): preallocate return slices

	// need to pull all items out of the in-memory datastore. Fortunately we have
	// kindless queries, and we disabled all the special entities, so just
	// run a kindless query without any filters and it will return all data
	// currently in bufDS :).
	fq, err := datastore.NewQuery("").Finalize()
	impossible(err)

	err = t.bufDS.Run(fq, func(key *datastore.Key, data datastore.PropertyMap, _ datastore.CursorCB) error {
		toPutKeys = append(toPutKeys, key)
		toPut = append(toPut, data)
		return nil
	})
	memoryCorruption(err)

	for keyStr, size := range t.entState.keyToSize {
		if size == 0 {
			k, err := serialize.ReadKey(bytes.NewBufferString(keyStr), serialize.WithoutContext, t.kc)
			memoryCorruption(err)
			toDel = append(toDel, k)
		}
	}

	return
}

func (t *txnBufState) canApplyLocked(s *txnBufState) error {
	proposedState := t.entState.dup()

	for k, v := range s.entState.keyToSize {
		proposedState.set(k, v)
	}
	switch {
	case proposedState.numWrites() > t.writeCountBudget:
		// The new net number of writes must be below the parent's write count
		// cutoff.
		fallthrough

	case proposedState.total > t.sizeBudget:
		// Make sure our new calculated size is within the parent's size budget.
		//
		// We have:
		// - proposedState.total: The "new world" total bytes were this child
		//   transaction committed to the parent.
		// - t.sizeBudget: The maximum number of bytes that this parent can
		//   accommodate.
		return ErrTransactionTooLarge
	}

	return nil
}

func (t *txnBufState) commitLocked(s *txnBufState) {
	toPut, toPutKeys, toDel := s.effect()

	if len(toPut) > 0 {
		impossible(t.putMulti(toPutKeys, toPut,
			func(_ int, _ *datastore.Key, err error) error { return err }, true))
	}

	if len(toDel) > 0 {
		impossible(t.deleteMulti(toDel, func(_ int, err error) error { return err }, true))
	}
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
