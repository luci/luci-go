// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"errors"
	"infra/gae/libs/wrapper"
	goon_internal "infra/gae/libs/wrapper/memory/internal/goon"
	"math/rand"
	"sync"
	"sync/atomic"

	"github.com/mjibson/goon"

	"appengine/datastore"
	pb "appengine_internal/datastore"
)

////////////////////////////////// knrKeeper ///////////////////////////////////

type knrKeeper struct {
	knrLock sync.Mutex
	knrFunc goon.KindNameResolver
}

var _ = wrapper.DSKindSetter((*knrKeeper)(nil))

func (k *knrKeeper) KindNameResolver() goon.KindNameResolver {
	k.knrLock.Lock()
	defer k.knrLock.Unlock()
	if k.knrFunc == nil {
		k.knrFunc = goon.DefaultKindName
	}
	return k.knrFunc
}

func (k *knrKeeper) SetKindNameResolver(knr goon.KindNameResolver) {
	k.knrLock.Lock()
	defer k.knrLock.Unlock()
	if knr == nil {
		knr = goon.DefaultKindName
	}
	k.knrFunc = knr
}

//////////////////////////////// dataStoreData /////////////////////////////////

type dataStoreData struct {
	wrapper.BrokenFeatures
	knrKeeper

	rwlock sync.RWMutex
	// See README.md for store schema.
	store *memStore
	snap  *memStore
}

var (
	_ = memContextObj((*dataStoreData)(nil))
	_ = sync.Locker((*dataStoreData)(nil))
	_ = wrapper.Testable((*dataStoreData)(nil))
	_ = wrapper.DSKindSetter((*dataStoreData)(nil))
)

func newDataStoreData() *dataStoreData {
	store := newMemStore()
	return &dataStoreData{
		BrokenFeatures: wrapper.BrokenFeatures{DefaultError: newDSError(pb.Error_INTERNAL_ERROR)},
		store:          store,
		snap:           store.Snapshot(), // empty but better than a nil pointer.
	}
}

func (d *dataStoreData) Lock() {
	d.rwlock.Lock()
}

func (d *dataStoreData) Unlock() {
	d.rwlock.Unlock()
}

func groupMetaKey(key *datastore.Key) []byte {
	return keyBytes(noNS, newKey("", "__entity_group__", "", 1, rootKey(key)))
}

func groupIDsKey(key *datastore.Key) []byte {
	return keyBytes(noNS, newKey("", "__entity_group_ids__", "", 1, rootKey(key)))
}

func rootIDsKey(kind string) []byte {
	return keyBytes(noNS, newKey("", "__entity_root_ids__", kind, 0, nil))
}

func curVersion(ents *memCollection, key []byte) (int64, error) {
	if v := ents.Get(key); v != nil {
		numData := &propertyList{}
		if err := numData.UnmarshalBinary(v); err != nil {
			return 0, err
		}
		return (*numData)[0].Value.(int64), nil
	}
	return 0, nil
}

func incrementLocked(ents *memCollection, key []byte) (int64, error) {
	num := int64(0)
	numData := &propertyList{}
	if v := ents.Get(key); v != nil {
		if err := numData.UnmarshalBinary(v); err != nil {
			return 0, err
		}
		num = (*numData)[0].Value.(int64)
	} else {
		*numData = append(*numData, datastore.Property{Name: "__version__"})
	}
	num++
	(*numData)[0].Value = num
	incData, err := numData.MarshalBinary()
	if err != nil {
		return 0, err
	}
	ents.Set(key, incData)

	return num, nil
}

func (d *dataStoreData) entsKeyLocked(key *datastore.Key) (*memCollection, *datastore.Key, error) {
	coll := "ents:" + key.Namespace()
	ents := d.store.GetCollection(coll)
	if ents == nil {
		ents = d.store.SetCollection(coll, nil)
	}

	if key.Incomplete() {
		idKey := []byte(nil)
		if key.Parent() == nil {
			idKey = rootIDsKey(key.Kind())
		} else {
			idKey = groupIDsKey(key)
		}
		id, err := incrementLocked(ents, idKey)
		if err != nil {
			return nil, nil, err
		}
		key = newKey(key.Namespace(), key.Kind(), "", id, key.Parent())
	}

	return ents, key, nil
}

func putPrelim(ns string, knr goon.KindNameResolver, src interface{}) (*datastore.Key, *propertyList, error) {
	key := newKeyObj(ns, knr, src)
	if !KeyCouldBeValid(ns, key, UserKeyOnly) {
		// TODO(riannucci): different error for Put-ing to reserved Keys?
		return nil, nil, datastore.ErrInvalidKey
	}

	data, err := toPL(src)
	return key, data, err
}

func (d *dataStoreData) put(ns string, src interface{}) (*datastore.Key, error) {
	key, plData, err := putPrelim(ns, d.KindNameResolver(), src)
	if err != nil {
		return nil, err
	}
	if key, err = d.putInner(key, plData); err != nil {
		return nil, err
	}
	return key, goon_internal.SetStructKey(src, key, d.KindNameResolver())
}

func (d *dataStoreData) putInner(key *datastore.Key, data *propertyList) (*datastore.Key, error) {
	dataBytes, err := data.MarshalBinary()
	if err != nil {
		return nil, err
	}

	d.rwlock.Lock()
	defer d.rwlock.Unlock()

	ents, key, err := d.entsKeyLocked(key)
	if err != nil {
		return nil, err
	}
	if _, err = incrementLocked(ents, groupMetaKey(key)); err != nil {
		return nil, err
	}

	ents.Set(keyBytes(noNS, key), dataBytes)

	return key, nil
}

func getInner(ns string, knr goon.KindNameResolver, dst interface{}, getColl func(*datastore.Key) (*memCollection, error)) error {
	key := newKeyObj(ns, knr, dst)
	if !KeyValid(ns, key, AllowSpecialKeys) {
		return datastore.ErrInvalidKey
	}

	ents, err := getColl(key)
	if err != nil {
		return err
	}
	if ents == nil {
		return datastore.ErrNoSuchEntity
	}
	pdata := ents.Get(keyBytes(noNS, key))
	if pdata == nil {
		return datastore.ErrNoSuchEntity
	}
	pl := &propertyList{}
	if err = pl.UnmarshalBinary(pdata); err != nil {
		return err
	}
	return fromPL(pl, dst)
}

func (d *dataStoreData) get(ns string, dst interface{}) error {
	return getInner(ns, d.KindNameResolver(), dst, func(*datastore.Key) (*memCollection, error) {
		d.rwlock.RLock()
		s := d.store.Snapshot()
		d.rwlock.RUnlock()

		return s.GetCollection("ents:" + ns), nil
	})
}

func (d *dataStoreData) del(ns string, key *datastore.Key) error {
	if !KeyValid(ns, key, UserKeyOnly) {
		return datastore.ErrInvalidKey
	}

	keyBuf := keyBytes(noNS, key)

	d.rwlock.Lock()
	defer d.rwlock.Unlock()

	ents := d.store.GetCollection("ents:" + ns)
	if ents == nil {
		return nil
	}
	if _, err := incrementLocked(ents, groupMetaKey(key)); err != nil {
		return err
	}

	ents.Delete(keyBuf)
	return nil
}

func (d *dataStoreData) canApplyTxn(obj memContextObj) bool {
	// TODO(riannucci): implement with Flush/FlushRevert for persistance.

	txn := obj.(*txnDataStoreData)
	for rk, muts := range txn.muts {
		if len(muts) == 0 { // read-only
			continue
		}
		k, err := keyFromByteString(withNS, rk)
		if err != nil {
			panic(err)
		}
		entKey := "ents:" + k.Namespace()
		mkey := groupMetaKey(k)
		entsHead := d.store.GetCollection(entKey)
		entsSnap := txn.snap.GetCollection(entKey)
		vHead, err := curVersion(entsHead, mkey)
		if err != nil {
			panic(err)
		}
		vSnap, err := curVersion(entsSnap, mkey)
		if err != nil {
			panic(err)
		}
		if vHead != vSnap {
			return false
		}
	}
	return true
}

func (d *dataStoreData) applyTxn(r *rand.Rand, obj memContextObj) {
	txn := obj.(*txnDataStoreData)
	for _, muts := range txn.muts {
		if len(muts) == 0 { // read-only
			continue
		}
		for _, m := range muts {
			err := error(nil)
			if m.data == nil {
				err = d.del(m.key.Namespace(), m.key)
			} else {
				_, err = d.putInner(m.key, m.data)
			}
			if err != nil {
				panic(err)
			}
		}
	}
}

func (d *dataStoreData) mkTxn(o *datastore.TransactionOptions) (memContextObj, error) {
	return &txnDataStoreData{
		// alias to the main datastore's so that testing code can have primitive
		// access to break features inside of transactions.
		BrokenFeatures: &d.BrokenFeatures,
		parent:         d,
		knrKeeper:      knrKeeper{knrFunc: d.knrFunc},
		isXG:           o != nil && o.XG,
		snap:           d.store.Snapshot(),
		muts:           map[string][]txnMutation{},
	}, nil
}

func (d *dataStoreData) endTxn() {}

/////////////////////////////// txnDataStoreData ///////////////////////////////

type txnMutation struct {
	key  *datastore.Key
	data *propertyList
}

type txnDataStoreData struct {
	*wrapper.BrokenFeatures
	knrKeeper
	sync.Mutex

	parent *dataStoreData

	// boolean 0 or 1, use atomic.*Int32 to access.
	closed int32
	isXG   bool

	snap *memStore

	// string is the raw-bytes encoding of the entity root incl. namespace
	muts map[string][]txnMutation
	// TODO(riannucci): account for 'transaction size' limit of 10MB by summing
	// length of encoded keys + values.
}

var (
	_ = memContextObj((*txnDataStoreData)(nil))
	_ = sync.Locker((*txnDataStoreData)(nil))
	_ = wrapper.Testable((*txnDataStoreData)(nil))
	_ = wrapper.DSKindSetter((*txnDataStoreData)(nil))
)

const xgEGLimit = 25

func (*txnDataStoreData) canApplyTxn(memContextObj) bool { return false }
func (td *txnDataStoreData) endTxn() {
	if atomic.LoadInt32(&td.closed) == 1 {
		panic("cannot end transaction twice")
	}
	atomic.StoreInt32(&td.closed, 1)
}
func (*txnDataStoreData) applyTxn(*rand.Rand, memContextObj) {
	panic("txnDataStoreData cannot apply transactions")
}
func (*txnDataStoreData) mkTxn(*datastore.TransactionOptions) (memContextObj, error) {
	return nil, errors.New("datastore: nested transactions are not supported")
}

func (td *txnDataStoreData) IsBroken() error {
	// Slightly different from the SDK... datastore and taskqueue each implement
	// this here, where in the SDK only datastore.transaction.Call does.
	if atomic.LoadInt32(&td.closed) == 1 {
		return errors.New("datastore: transaction context has expired")
	}
	return td.BrokenFeatures.IsBroken()
}

// writeMutation ensures that this transaction can support the given key/value
// mutation.
//
//   if getOnly is true, don't record the actual mutation data, just ensure that
//	   the key is in an included entity group (or add an empty entry for that
//	   group).
//
//   if !getOnly && data == nil, this counts as a deletion instead of a Put.
//
// Returns an error if this key causes the transaction to cross too many entity
// groups.
func (td *txnDataStoreData) writeMutation(getOnly bool, key *datastore.Key, data *propertyList) error {
	rk := string(keyBytes(withNS, rootKey(key)))

	td.Lock()
	defer td.Unlock()

	if _, ok := td.muts[rk]; !ok {
		limit := 1
		if td.isXG {
			limit = xgEGLimit
		}
		if len(td.muts)+1 > limit {
			msg := "cross-group transaction need to be explicitly specified (xg=True)"
			if td.isXG {
				msg = "operating on too many entity groups in a single transaction"
			}
			return newDSError(pb.Error_BAD_REQUEST, msg)
		}
		td.muts[rk] = []txnMutation{}
	}
	if !getOnly {
		td.muts[rk] = append(td.muts[rk], txnMutation{key, data})
	}

	return nil
}

func (td *txnDataStoreData) put(ns string, src interface{}) (*datastore.Key, error) {
	key, plData, err := putPrelim(ns, td.KindNameResolver(), src)
	if err != nil {
		return nil, err
	}

	func() {
		td.parent.Lock()
		defer td.parent.Unlock()
		_, key, err = td.parent.entsKeyLocked(key)
	}()
	if err != nil {
		return nil, err
	}

	if err = td.writeMutation(false, key, plData); err != nil {
		return nil, err
	}

	return key, goon_internal.SetStructKey(src, key, td.KindNameResolver())
}

func (td *txnDataStoreData) get(ns string, dst interface{}) error {
	return getInner(ns, td.KindNameResolver(), dst, func(key *datastore.Key) (*memCollection, error) {
		if err := td.writeMutation(true, key, nil); err != nil {
			return nil, err
		}
		return td.snap.GetCollection("ents:" + ns), nil
	})
}

func (td *txnDataStoreData) del(ns string, key *datastore.Key) error {
	if !KeyValid(ns, key, UserKeyOnly) {
		return datastore.ErrInvalidKey
	}
	return td.writeMutation(false, key, nil)
}
