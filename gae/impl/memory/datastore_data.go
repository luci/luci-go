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
	"fmt"
	"strings"
	"sync"

	prodConstraints "go.chromium.org/gae/impl/prod/constraints"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/datastore/serialize"
	"go.chromium.org/luci/common/errors"

	"golang.org/x/net/context"
)

//////////////////////////////// dataStoreData /////////////////////////////////

type dataStoreData struct {
	// Protects internal guts of this object, including overall consistency of the
	// memStore.
	//
	// While memStore is consistent by itself, each individual datastore mutation
	// (puts and deletes) actually translate into multiple memStore modifications
	// (for example, putting an entity updates this entity's data as well as
	// entity group version metadata entity). Thus we need additional lock around
	// such "batch" head modifications to ensure their consistency. In particular,
	// it is very important that any snapshots are taken under the reader lock, to
	// make sure we are not snapshotting some intermediary inconsistent state.
	rwlock sync.RWMutex

	// the 'appid' of this datastore
	aid string

	// See README.md for head schema.
	head memStore
	// if snap is nil, that means that this is always-consistent, and
	// getQuerySnaps will return (head, head)
	snap memStore
	// For testing, see SetTransactionRetryCount.
	txnFakeRetry int
	// true means that queries with insufficent indexes will pause to add them
	// and then continue instead of failing.
	autoIndex bool
	// true means that all of the __...__ keys which are normally automatically
	// maintained will be omitted. This also means that Put with an incomplete
	// key will become an error.
	disableSpecialEntities bool

	// constraints is the fake datastore constraints. By default, this will match
	// the Constraints of the "impl/prod" datastore.
	constraints ds.Constraints
}

var (
	_ = memContextObj((*dataStoreData)(nil))
)

func newDataStoreData(aid string) *dataStoreData {
	head := newMemStore()
	return &dataStoreData{
		aid:         aid,
		head:        head,
		snap:        head.Snapshot(), // empty but better than a nil pointer.
		constraints: prodConstraints.DS(),
	}
}

func (d *dataStoreData) setTxnRetry(count int) {
	d.rwlock.Lock()
	defer d.rwlock.Unlock()
	d.txnFakeRetry = count
}

func (d *dataStoreData) setConsistent(always bool) {
	d.rwlock.Lock()
	defer d.rwlock.Unlock()

	if always {
		d.snap = nil
	} else {
		d.snap = d.head.Snapshot()
	}
}

func (d *dataStoreData) addIndexes(idxs []*ds.IndexDefinition) {
	d.rwlock.Lock()
	defer d.rwlock.Unlock()
	addIndexes(d.head, d.aid, idxs)
}

func (d *dataStoreData) setAutoIndex(enable bool) {
	d.rwlock.Lock()
	defer d.rwlock.Unlock()
	d.autoIndex = enable
}

func (d *dataStoreData) maybeAutoIndex(err error) bool {
	mi, ok := err.(*ErrMissingIndex)
	if !ok {
		return false
	}

	d.rwlock.RLock()
	ai := d.autoIndex
	d.rwlock.RUnlock()

	if !ai {
		return false
	}

	d.addIndexes([]*ds.IndexDefinition{mi.Missing})
	return true
}

func (d *dataStoreData) setDisableSpecialEntities(enabled bool) {
	d.rwlock.Lock()
	defer d.rwlock.Unlock()
	d.disableSpecialEntities = true
}

func (d *dataStoreData) getDisableSpecialEntities() bool {
	d.rwlock.RLock()
	defer d.rwlock.RUnlock()
	return d.disableSpecialEntities
}

func (d *dataStoreData) getQuerySnaps(consistent bool) (idx, head memStore) {
	d.rwlock.RLock()
	defer d.rwlock.RUnlock()
	if d.snap == nil {
		// we're 'always consistent'
		snap := d.head.Snapshot()
		return snap, snap
	}

	head = d.head.Snapshot()
	if consistent {
		idx = head
	} else {
		idx = d.snap
	}
	return
}

func (d *dataStoreData) takeSnapshot() memStore {
	d.rwlock.RLock()
	defer d.rwlock.RUnlock()
	return d.head.Snapshot()
}

func (d *dataStoreData) setSnapshot(snap memStore) {
	d.rwlock.Lock()
	defer d.rwlock.Unlock()
	if d.snap == nil {
		// we're 'always consistent'
		return
	}
	d.snap = snap
}

func (d *dataStoreData) catchupIndexes() {
	d.rwlock.Lock()
	defer d.rwlock.Unlock()
	if d.snap == nil {
		// we're 'always consistent'
		return
	}
	d.snap = d.head.Snapshot()
}

func (d *dataStoreData) namespaces() []string {
	d.rwlock.Lock()
	defer d.rwlock.Unlock()

	return namespaces(d.head)
}

func (d *dataStoreData) getConstraints() ds.Constraints {
	d.rwlock.RLock()
	defer d.rwlock.RUnlock()
	return d.constraints
}

func (d *dataStoreData) setConstraints(c ds.Constraints) {
	d.rwlock.Lock()
	defer d.rwlock.Unlock()
	d.constraints = c
}

/////////////////////////// indexes(dataStoreData) ////////////////////////////

func groupMetaKey(key *ds.Key) []byte {
	return keyBytes(ds.MkKeyContext("", "").NewKey("__entity_group__", "", 1, key.Root()))
}

func groupIDsKey(key *ds.Key) []byte {
	return keyBytes(ds.MkKeyContext("", "").NewKey("__entity_group_ids__", "", 1, key.Root()))
}

func rootIDsKey(kind string) []byte {
	return keyBytes(ds.MkKeyContext("", "").NewKey("__entity_root_ids__", kind, 0, nil))
}

func curVersion(ents memCollection, key []byte) int64 {
	if ents != nil {
		if v := ents.Get(key); v != nil {
			pm, err := rpm(v)
			memoryCorruption(err)

			pl := pm.Slice("__version__")
			if len(pl) > 0 && pl[0].Type() == ds.PTInt {
				return pl[0].Value().(int64)
			}

			memoryCorruption(fmt.Errorf("__version__ property missing or wrong: %v", pm))
		}
	}
	return 0
}

func incrementLocked(ents memCollection, key []byte, amt int) int64 {
	if amt <= 0 {
		panic(fmt.Errorf("incrementLocked called with bad `amt`: %d", amt))
	}
	ret := curVersion(ents, key) + 1
	ents.Set(key, serialize.ToBytes(ds.PropertyMap{
		"__version__": ds.MkPropertyNI(ret + int64(amt-1)),
	}))
	return ret
}

func (d *dataStoreData) allocateIDs(keys []*ds.Key, cb ds.NewKeyCB) error {
	// Map keys by entity type.
	entityMap := make(map[string][]int)
	for i, key := range keys {
		ks := key.String()
		entityMap[ks] = append(entityMap[ks], i)
	}

	// Allocate IDs for our keys. We use an inline function so we can ensure that
	// the lock is released.
	err := func() error {
		d.rwlock.Lock()
		defer d.rwlock.Unlock()

		for _, idxs := range entityMap {
			baseKey := keys[idxs[0]]

			ents := d.head.GetOrCreateCollection("ents:" + baseKey.Namespace())

			// Allocate IDs. The only possible error is when disableSpecialEntities is
			// true, in which case we will return a full method error instead of
			// individual callback errors.
			start, err := d.allocateIDsLocked(ents, baseKey, len(idxs))
			if err != nil {
				return err
			}

			for i, idx := range idxs {
				keys[idx] = baseKey.WithID("", start+int64(i))
			}
		}
		return nil
	}()
	if err != nil {
		return err
	}

	// Execute Callbacks.
	for i, key := range keys {
		cb(i, key, nil)
	}
	return nil
}

func (d *dataStoreData) allocateIDsLocked(ents memCollection, incomplete *ds.Key, n int) (int64, error) {
	if d.disableSpecialEntities {
		return 0, errors.New("disableSpecialEntities is true so allocateIDs is disabled")
	}

	idKey := []byte(nil)
	if incomplete.Parent() == nil {
		idKey = rootIDsKey(incomplete.Kind())
	} else {
		idKey = groupIDsKey(incomplete)
	}
	return incrementLocked(ents, idKey, n), nil
}

func (d *dataStoreData) fixKeyLocked(ents memCollection, key *ds.Key) (*ds.Key, error) {
	if key.IsIncomplete() {
		id, err := d.allocateIDsLocked(ents, key, 1)
		if err != nil {
			return key, err
		}
		key = key.KeyContext().NewKey(key.Kind(), "", id, key.Parent())
	}
	return key, nil
}

func (d *dataStoreData) fixKey(key *ds.Key) (*ds.Key, error) {
	if key.IsIncomplete() {
		d.rwlock.Lock()
		defer d.rwlock.Unlock()
		ents := d.head.GetOrCreateCollection("ents:" + key.Namespace())
		return d.fixKeyLocked(ents, key)
	}
	return key, nil
}

func (d *dataStoreData) putMulti(keys []*ds.Key, vals []ds.PropertyMap, cb ds.NewKeyCB, lockedAlready bool) error {
	ns := keys[0].Namespace()

	for i, k := range keys {
		pmap, _ := vals[i].Save(false)
		dataBytes := serialize.ToBytesWithContext(pmap)

		k, err := func() (ret *ds.Key, err error) {
			if !lockedAlready {
				d.rwlock.Lock()
				defer d.rwlock.Unlock()
			}

			ents := d.head.GetOrCreateCollection("ents:" + ns)

			ret, err = d.fixKeyLocked(ents, k)
			if err != nil {
				return
			}
			if !d.disableSpecialEntities {
				incrementLocked(ents, groupMetaKey(ret), 1)
			}

			old := ents.Get(keyBytes(ret))
			oldPM := ds.PropertyMap(nil)
			if old != nil {
				if oldPM, err = rpm(old); err != nil {
					return
				}
			}
			ents.Set(keyBytes(ret), dataBytes)
			updateIndexes(d.head, ret, oldPM, pmap)
			return
		}()
		if cb != nil {
			if err := cb(i, k, err); err != nil {
				return err
			}
		}
	}
	return nil
}

func getMultiInner(keys []*ds.Key, cb ds.GetMultiCB, ents memCollection) {
	if ents == nil {
		for i := range keys {
			cb(i, nil, ds.ErrNoSuchEntity)
		}
		return
	}

	for i, k := range keys {
		pdata := ents.Get(keyBytes(k))
		if pdata == nil {
			cb(i, nil, ds.ErrNoSuchEntity)
		} else {
			pm, err := rpm(pdata)
			cb(i, pm, err)
		}
	}
}

func (d *dataStoreData) getMulti(keys []*ds.Key, cb ds.GetMultiCB) error {
	ents := d.takeSnapshot().GetCollection("ents:" + keys[0].Namespace())
	getMultiInner(keys, cb, ents)
	return nil
}

func (d *dataStoreData) delMulti(keys []*ds.Key, cb ds.DeleteMultiCB, lockedAlready bool) error {
	ns := keys[0].Namespace()

	hasEntsInNS := func() bool {
		if !lockedAlready {
			d.rwlock.Lock()
			defer d.rwlock.Unlock()
		}
		return d.head.GetOrCreateCollection("ents:"+ns) != nil
	}()

	if hasEntsInNS {
		for i, k := range keys {
			err := func() error {
				kb := keyBytes(k)

				if !lockedAlready {
					d.rwlock.Lock()
					defer d.rwlock.Unlock()
				}

				ents := d.head.GetOrCreateCollection("ents:" + ns)

				if !d.disableSpecialEntities {
					incrementLocked(ents, groupMetaKey(k), 1)
				}
				if old := ents.Get(kb); old != nil {
					oldPM, err := rpm(old)
					if err != nil {
						return err
					}
					ents.Delete(kb)
					updateIndexes(d.head, k, oldPM, nil)
				}
				return nil
			}()
			if cb != nil {
				if err := cb(i, err); err != nil {
					return err
				}
			}
		}
	} else if cb != nil {
		for i := range keys {
			if err := cb(i, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func (d *dataStoreData) beginCommit(c context.Context, obj memContextObj) txnCommitOp {
	// TODO(riannucci): implement with Flush/FlushRevert for persistence.

	txn := obj.(*txnDataStoreData)

	txn.lock.Lock()
	d.rwlock.Lock()

	unlock := func() {
		d.rwlock.Unlock()
		txn.lock.Unlock()
	}

	// Check for collisions.
	for _, muts := range txn.muts {
		if len(muts) == 0 { // read-only
			continue
		}

		// All muts keys belong to same entity group. Grab its root. Note that we
		// can try to deserialize txn.muts key instead, by taking it through .Root()
		// is simpler.
		root := muts[0].key.Root()

		entKey := "ents:" + root.Namespace()
		mkey := groupMetaKey(root)
		entsHead := d.head.GetCollection(entKey)
		entsSnap := txn.snap.GetCollection(entKey)
		vHead := curVersion(entsHead, mkey)
		vSnap := curVersion(entsSnap, mkey)

		if vHead != vSnap {
			unlock()
			return nil // a collision, the commit is not possible
		}
	}

	return &txnCommitCallback{
		unlock: unlock,
		apply: func() {
			for _, muts := range txn.muts {
				if len(muts) == 0 { // read-only
					continue
				}
				// TODO(riannucci): refactor to do just 1 putMulti, and 1 delMulti
				for _, m := range muts {
					if m.data == nil {
						impossible(d.delMulti([]*ds.Key{m.key},
							func(_ int, e error) error { return e }, true))
					} else {
						impossible(d.putMulti([]*ds.Key{m.key}, []ds.PropertyMap{m.data},
							func(_ int, _ *ds.Key, e error) error { return e }, true))
					}
				}
			}
		},
	}
}

func (d *dataStoreData) mkTxn(o *ds.TransactionOptions) memContextObj {
	return &txnDataStoreData{
		// alias to the main datastore's so that testing code can have primitive
		// access to break features inside of transactions.
		parent: d,
		txn: &transactionImpl{
			isXG: o != nil && o.XG,
		},
		snap: d.takeSnapshot(),
		muts: map[string][]txnMutation{},
	}
}

func (d *dataStoreData) endTxn() {}

/////////////////////////////// txnDataStoreData ///////////////////////////////

type txnMutation struct {
	key  *ds.Key
	data ds.PropertyMap
}

type txnDataStoreData struct {
	lock   sync.Mutex
	parent *dataStoreData
	txn    *transactionImpl

	snap memStore

	// string is the raw-bytes encoding of the entity root incl. namespace
	muts map[string][]txnMutation
	// TODO(riannucci): account for 'transaction size' limit of 10MB by summing
	// length of encoded keys + values.
}

var _ memContextObj = (*txnDataStoreData)(nil)

const xgEGLimit = 25

func (td *txnDataStoreData) endTxn() {
	if err := td.txn.close(); err != nil {
		panic(err)
	}
}

func (*txnDataStoreData) mkTxn(*ds.TransactionOptions) memContextObj {
	impossible(fmt.Errorf("cannot create a recursive transaction"))
	return nil
}

func (*txnDataStoreData) beginCommit(c context.Context, txnCtxObj memContextObj) txnCommitOp {
	impossible(fmt.Errorf("cannot commit a recursive transaction"))
	return nil
}

func (td *txnDataStoreData) run(f func() error) error {
	// Slightly different from the SDK... datastore and taskqueue each implement
	// this here, where in the SDK only datastore.transaction.Call does.
	if err := td.txn.valid(); err != nil {
		return err
	}
	return f()
}

// writeMutation ensures that this transaction can support the given key/value
// mutation.
//
// If getOnly is true, don't record the actual mutation data, just ensure that
// the key is in an included entity group (or add an empty entry for that
// group).
//
// If !getOnly && data == nil, this counts as a deletion instead of a Put.
//
// Returns an error if this key causes the transaction to cross too many entity
// groups.
func (td *txnDataStoreData) writeMutation(getOnly bool, key *ds.Key, data ds.PropertyMap) error {
	rk := string(keyBytes(key.Root()))

	td.lock.Lock()
	defer td.lock.Unlock()

	if _, ok := td.muts[rk]; !ok {
		limit := 1
		if td.txn.isXG {
			limit = xgEGLimit
		}
		if len(td.muts)+1 > limit {
			msg := "cross-group transaction need to be explicitly specified (xg=True)"
			if td.txn.isXG {
				msg = "operating on too many entity groups in a single transaction"
			}
			return errors.New(msg)
		}
		td.muts[rk] = []txnMutation{}
	}
	if !getOnly {
		td.muts[rk] = append(td.muts[rk], txnMutation{key, data})
	}

	return nil
}

func (td *txnDataStoreData) putMulti(keys []*ds.Key, vals []ds.PropertyMap, cb ds.NewKeyCB) {
	for i, k := range keys {
		k, err := td.parent.fixKey(k)
		if err == nil {
			err = td.writeMutation(false, k, vals[i])
		}
		if cb != nil {
			cb(i, k, err)
		}
	}
}

func (td *txnDataStoreData) getMulti(keys []*ds.Key, cb ds.GetMultiCB) error {
	for _, key := range keys {
		err := td.writeMutation(true, key, nil)
		if err != nil {
			return err
		}
	}
	ents := td.snap.GetCollection("ents:" + keys[0].Namespace())
	getMultiInner(keys, cb, ents)
	return nil
}

func (td *txnDataStoreData) delMulti(keys []*ds.Key, cb ds.DeleteMultiCB) error {
	for i, k := range keys {
		err := td.writeMutation(false, k, nil)
		if cb != nil {
			cb(i, err)
		}
	}
	return nil
}

func keyBytes(key *ds.Key) []byte {
	return serialize.ToBytes(ds.MkProperty(key))
}

func rpm(data []byte) (ds.PropertyMap, error) {
	return serialize.ReadPropertyMap(bytes.NewBuffer(data),
		serialize.WithContext, ds.MkKeyContext("", ""))
}

func namespaces(store memStore) []string {
	var namespaces []string
	for _, c := range store.GetCollectionNames() {
		ns, has := trimPrefix(c, "ents:")
		if !has {
			if len(namespaces) > 0 {
				break
			}
			continue
		}
		namespaces = append(namespaces, ns)
	}
	return namespaces
}

func trimPrefix(v, p string) (string, bool) {
	if strings.HasPrefix(v, p) {
		return v[len(p):], true
	}
	return v, false
}
