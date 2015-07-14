// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"errors"
	"fmt"
	"golang.org/x/net/context"
	"sync"
	"sync/atomic"

	"infra/gae/libs/gae"
	"infra/gae/libs/gae/helper"
)

//////////////////////////////// dataStoreData /////////////////////////////////

type dataStoreData struct {
	gae.BrokenFeatures

	rwlock sync.RWMutex
	// See README.md for store schema.
	store *memStore
	snap  *memStore
}

var (
	_ = memContextObj((*dataStoreData)(nil))
	_ = sync.Locker((*dataStoreData)(nil))
	_ = gae.Testable((*dataStoreData)(nil))
)

func newDataStoreData() *dataStoreData {
	store := newMemStore()
	return &dataStoreData{
		BrokenFeatures: gae.BrokenFeatures{DefaultError: errors.New("INTERNAL_ERROR")},
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

/////////////////////////// indicies(dataStoreData) ////////////////////////////

func groupMetaKey(key gae.DSKey) []byte {
	return keyBytes(helper.WithoutContext,
		helper.NewDSKey("", "", "__entity_group__", "", 1, helper.DSKeyRoot(key)))
}

func groupIDsKey(key gae.DSKey) []byte {
	return keyBytes(helper.WithoutContext,
		helper.NewDSKey("", "", "__entity_group_ids__", "", 1, helper.DSKeyRoot(key)))
}

func rootIDsKey(kind string) []byte {
	return keyBytes(helper.WithoutContext,
		helper.NewDSKey("", "", "__entity_root_ids__", kind, 0, nil))
}

func curVersion(ents *memCollection, key []byte) (int64, error) {
	if v := ents.Get(key); v != nil {
		pm, err := rpm(v)
		if err != nil {
			return 0, err
		}
		pl, ok := pm["__version__"]
		if ok && len(pl) > 0 && pl[0].Type() == gae.DSPTInt {
			return pl[0].Value().(int64), nil
		}
		return 0, fmt.Errorf("__version__ property missing or wrong: %v", pm)
	}
	return 0, nil
}

func incrementLocked(ents *memCollection, key []byte) (ret int64, err error) {
	if ret, err = curVersion(ents, key); err != nil {
		ret = 0
	}
	ret++
	p := gae.DSProperty{}
	if err = p.SetValue(ret, true); err != nil {
		return
	}
	buf := &bytes.Buffer{}
	helper.WriteDSPropertyMap(
		buf, gae.DSPropertyMap{"__version__": {p}}, helper.WithContext)
	ents.Set(key, buf.Bytes())
	return
}

func (d *dataStoreData) entsKeyLocked(key gae.DSKey) (*memCollection, gae.DSKey, error) {
	coll := "ents:" + key.Namespace()
	ents := d.store.GetCollection(coll)
	if ents == nil {
		ents = d.store.SetCollection(coll, nil)
	}

	if helper.DSKeyIncomplete(key) {
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
		key = helper.NewDSKey(key.AppID(), key.Namespace(), key.Kind(), "", id, key.Parent())
	}

	return ents, key, nil
}

func putPrelim(ns string, key gae.DSKey, src interface{}) (gae.DSPropertyMap, error) {
	if !keyCouldBeValid(key, ns, false) {
		// TODO(riannucci): different error for Put-ing to reserved Keys?
		return nil, gae.ErrDSInvalidKey
	}

	pls, err := helper.GetPLS(src)
	if err != nil {
		return nil, err
	}
	return pls.Save()
}

func (d *dataStoreData) put(ns string, key gae.DSKey, src interface{}) (gae.DSKey, error) {
	pmData, err := putPrelim(ns, key, src)
	if err != nil {
		return nil, err
	}
	if key, err = d.putInner(key, pmData); err != nil {
		return nil, err
	}
	return key, nil
}

func (d *dataStoreData) putInner(key gae.DSKey, data gae.DSPropertyMap) (gae.DSKey, error) {
	buf := &bytes.Buffer{}
	helper.WriteDSPropertyMap(buf, data, helper.WithoutContext)
	dataBytes := buf.Bytes()

	d.rwlock.Lock()
	defer d.rwlock.Unlock()

	ents, key, err := d.entsKeyLocked(key)
	if err != nil {
		return nil, err
	}
	if _, err = incrementLocked(ents, groupMetaKey(key)); err != nil {
		return nil, err
	}

	old := ents.Get(keyBytes(helper.WithoutContext, key))
	oldPM := gae.DSPropertyMap(nil)
	if old != nil {
		if oldPM, err = rpmWoCtx(old, key.Namespace()); err != nil {
			return nil, err
		}
	}
	if err = updateIndicies(d.store, key, oldPM, data); err != nil {
		return nil, err
	}

	ents.Set(keyBytes(helper.WithoutContext, key), dataBytes)

	return key, nil
}

func getInner(ns string, key gae.DSKey, dst interface{}, getColl func() (*memCollection, error)) error {
	if helper.DSKeyIncomplete(key) || !helper.DSKeyValid(key, ns, true) {
		return gae.ErrDSInvalidKey
	}

	ents, err := getColl()
	if err != nil {
		return err
	}
	if ents == nil {
		return gae.ErrDSNoSuchEntity
	}
	pdata := ents.Get(keyBytes(helper.WithoutContext, key))
	if pdata == nil {
		return gae.ErrDSNoSuchEntity
	}

	pm, err := rpmWoCtx(pdata, ns)
	if err != nil {
		return err
	}

	pls, err := helper.GetPLS(dst)
	if err != nil {
		return err
	}

	// TODO(riannucci): should the Get API reveal conversion errors instead of
	// swallowing them?
	_, err = pls.Load(pm)
	return err
}

func (d *dataStoreData) get(ns string, key gae.DSKey, dst interface{}) error {
	return getInner(ns, key, dst, func() (*memCollection, error) {
		d.rwlock.RLock()
		s := d.store.Snapshot()
		d.rwlock.RUnlock()

		return s.GetCollection("ents:" + ns), nil
	})
}

func (d *dataStoreData) del(ns string, key gae.DSKey) (err error) {
	if !helper.DSKeyValid(key, ns, false) {
		return gae.ErrDSInvalidKey
	}

	keyBuf := keyBytes(helper.WithoutContext, key)

	d.rwlock.Lock()
	defer d.rwlock.Unlock()

	ents := d.store.GetCollection("ents:" + ns)
	if ents == nil {
		return nil
	}
	if _, err = incrementLocked(ents, groupMetaKey(key)); err != nil {
		return
	}

	old := ents.Get(keyBuf)
	oldPM := gae.DSPropertyMap(nil)
	if old != nil {
		if oldPM, err = rpmWoCtx(old, ns); err != nil {
			return
		}
	}
	if err := updateIndicies(d.store, key, oldPM, nil); err != nil {
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
		k, err := helper.ReadDSKey(bytes.NewBufferString(rk), helper.WithContext, "", "")
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

func (d *dataStoreData) applyTxn(c context.Context, obj memContextObj) {
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

func (d *dataStoreData) mkTxn(o *gae.DSTransactionOptions) (memContextObj, error) {
	return &txnDataStoreData{
		// alias to the main datastore's so that testing code can have primitive
		// access to break features inside of transactions.
		BrokenFeatures: &d.BrokenFeatures,
		parent:         d,
		isXG:           o != nil && o.XG,
		snap:           d.store.Snapshot(),
		muts:           map[string][]txnMutation{},
	}, nil
}

func (d *dataStoreData) endTxn() {}

/////////////////////////////// txnDataStoreData ///////////////////////////////

type txnMutation struct {
	key  gae.DSKey
	data gae.DSPropertyMap
}

type txnDataStoreData struct {
	*gae.BrokenFeatures
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
	_ = gae.Testable((*txnDataStoreData)(nil))
)

const xgEGLimit = 25

func (*txnDataStoreData) canApplyTxn(memContextObj) bool { return false }
func (td *txnDataStoreData) endTxn() {
	if atomic.LoadInt32(&td.closed) == 1 {
		panic("cannot end transaction twice")
	}
	atomic.StoreInt32(&td.closed, 1)
}
func (*txnDataStoreData) applyTxn(context.Context, memContextObj) {
	panic("txnDataStoreData cannot apply transactions")
}
func (*txnDataStoreData) mkTxn(*gae.DSTransactionOptions) (memContextObj, error) {
	return nil, errors.New("datastore: nested transactions are not supported")
}

func (td *txnDataStoreData) RunIfNotBroken(f func() error) error {
	// Slightly different from the SDK... datastore and taskqueue each implement
	// this here, where in the SDK only datastore.transaction.Call does.
	if atomic.LoadInt32(&td.closed) == 1 {
		return errors.New("datastore: transaction context has expired")
	}
	return td.BrokenFeatures.RunIfNotBroken(f)
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
func (td *txnDataStoreData) writeMutation(getOnly bool, key gae.DSKey, data gae.DSPropertyMap) error {
	rk := string(keyBytes(helper.WithContext, helper.DSKeyRoot(key)))

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
			return errors.New(msg)
		}
		td.muts[rk] = []txnMutation{}
	}
	if !getOnly {
		td.muts[rk] = append(td.muts[rk], txnMutation{key, data})
	}

	return nil
}

func (td *txnDataStoreData) put(ns string, key gae.DSKey, src interface{}) (gae.DSKey, error) {
	pMap, err := putPrelim(ns, key, src)
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

	if err = td.writeMutation(false, key, pMap); err != nil {
		return nil, err
	}

	return key, nil
}

func (td *txnDataStoreData) get(ns string, key gae.DSKey, dst interface{}) error {
	return getInner(ns, key, dst, func() (*memCollection, error) {
		if err := td.writeMutation(true, key, nil); err != nil {
			return nil, err
		}
		return td.snap.GetCollection("ents:" + ns), nil
	})
}

func (td *txnDataStoreData) del(ns string, key gae.DSKey) error {
	if !helper.DSKeyValid(key, ns, false) {
		return gae.ErrDSInvalidKey
	}
	return td.writeMutation(false, key, nil)
}

func keyCouldBeValid(k gae.DSKey, ns string, allowSpecial bool) bool {
	// adds an id to k if it's incomplete.
	if helper.DSKeyIncomplete(k) {
		k = helper.NewDSKey(k.AppID(), k.Namespace(), k.Kind(), "", 1, k.Parent())
	}
	return helper.DSKeyValid(k, ns, allowSpecial)
}

func keyBytes(ctx helper.DSKeyContext, key gae.DSKey) []byte {
	buf := &bytes.Buffer{}
	helper.WriteDSKey(buf, ctx, key)
	return buf.Bytes()
}

func rpmWoCtx(data []byte, ns string) (gae.DSPropertyMap, error) {
	return helper.ReadDSPropertyMap(bytes.NewBuffer(data), helper.WithoutContext, globalAppID, ns)
}

func rpm(data []byte) (gae.DSPropertyMap, error) {
	return helper.ReadDSPropertyMap(bytes.NewBuffer(data), helper.WithContext, "", "")
}
