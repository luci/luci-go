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
	rwlock sync.RWMutex
	// See README.md for store schema.
	store *memStore
	snap  *memStore
}

var (
	_ = memContextObj((*dataStoreData)(nil))
	_ = sync.Locker((*dataStoreData)(nil))
)

func newDataStoreData() *dataStoreData {
	store := newMemStore()
	return &dataStoreData{
		store: store,
		snap:  store.Snapshot(), // empty but better than a nil pointer.
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

func curVersion(ents *memCollection, key []byte) int64 {
	if v := ents.Get(key); v != nil {
		pm, err := rpm(v)
		if err != nil {
			panic(err) // memory corruption
		}
		pl, ok := pm["__version__"]
		if ok && len(pl) > 0 && pl[0].Type() == gae.DSPTInt {
			return pl[0].Value().(int64)
		}
		panic(fmt.Errorf("__version__ property missing or wrong: %v", pm))
	}
	return 0
}

func incrementLocked(ents *memCollection, key []byte) int64 {
	ret := curVersion(ents, key) + 1
	buf := &bytes.Buffer{}
	helper.WriteDSPropertyMap(
		buf, gae.DSPropertyMap{"__version__": {gae.MkDSPropertyNI(ret)}}, helper.WithContext)
	ents.Set(key, buf.Bytes())
	return ret
}

func (d *dataStoreData) entsKeyLocked(key gae.DSKey) (*memCollection, gae.DSKey) {
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
		id := incrementLocked(ents, idKey)
		key = helper.NewDSKey(key.AppID(), key.Namespace(), key.Kind(), "", id, key.Parent())
	}

	return ents, key
}

func (d *dataStoreData) put(ns string, key gae.DSKey, pls gae.DSPropertyLoadSaver) (gae.DSKey, error) {
	keys, errs := d.putMulti(ns, []gae.DSKey{key}, []gae.DSPropertyLoadSaver{pls})
	if errs == nil {
		return keys[0], nil
	}
	return nil, gae.SingleError(errs)
}

func (d *dataStoreData) putMulti(ns string, keys []gae.DSKey, plss []gae.DSPropertyLoadSaver) ([]gae.DSKey, error) {
	pmaps, err := putMultiPrelim(ns, keys, plss)
	if err != nil {
		return nil, err
	}
	return d.putMultiInner(keys, pmaps)
}

func putMultiPrelim(ns string, keys []gae.DSKey, plss []gae.DSPropertyLoadSaver) ([]gae.DSPropertyMap, error) {
	err := multiValid(keys, plss, ns, true, false)
	if err != nil {
		return nil, err
	}
	pmaps := make([]gae.DSPropertyMap, len(keys))
	lme := gae.LazyMultiError{Size: len(keys)}
	for i, pls := range plss {
		pm, err := pls.Save(false)
		lme.Assign(i, err)
		pmaps[i] = pm
	}
	return pmaps, lme.Get()
}

func (d *dataStoreData) putMultiInner(keys []gae.DSKey, data []gae.DSPropertyMap) ([]gae.DSKey, error) {
	retKeys := make([]gae.DSKey, len(keys))
	lme := gae.LazyMultiError{Size: len(keys)}
	for i, k := range keys {
		buf := &bytes.Buffer{}
		helper.WriteDSPropertyMap(buf, data[i], helper.WithoutContext)
		dataBytes := buf.Bytes()

		rKey, err := func() (ret gae.DSKey, err error) {
			d.rwlock.Lock()
			defer d.rwlock.Unlock()

			ents, ret := d.entsKeyLocked(k)
			incrementLocked(ents, groupMetaKey(ret))

			old := ents.Get(keyBytes(helper.WithoutContext, ret))
			oldPM := gae.DSPropertyMap(nil)
			if old != nil {
				if oldPM, err = rpmWoCtx(old, ret.Namespace()); err != nil {
					return
				}
			}
			updateIndicies(d.store, ret, oldPM, data[i])
			ents.Set(keyBytes(helper.WithoutContext, ret), dataBytes)
			return
		}()
		lme.Assign(i, err)
		retKeys[i] = rKey
	}
	return retKeys, lme.Get()
}

func getMultiInner(ns string, keys []gae.DSKey, plss []gae.DSPropertyLoadSaver, getColl func() (*memCollection, error)) error {
	if err := multiValid(keys, plss, ns, false, true); err != nil {
		return err
	}

	lme := gae.LazyMultiError{Size: len(keys)}

	ents, err := getColl()
	if err != nil {
		return err
	}
	if ents == nil {
		for i := range keys {
			lme.Assign(i, gae.ErrDSNoSuchEntity)
		}
		return lme.Get()
	}

	for i, k := range keys {
		pdata := ents.Get(keyBytes(helper.WithoutContext, k))
		if pdata == nil {
			lme.Assign(i, gae.ErrDSNoSuchEntity)
			continue
		}

		got, err := rpmWoCtx(pdata, ns)
		if err != nil {
			lme.Assign(i, err)
			continue
		}

		lme.Assign(i, plss[i].Load(got))
	}
	return lme.Get()
}

func (d *dataStoreData) get(ns string, key gae.DSKey, pls gae.DSPropertyLoadSaver) error {
	return gae.SingleError(d.getMulti(ns, []gae.DSKey{key}, []gae.DSPropertyLoadSaver{pls}))
}

func (d *dataStoreData) getMulti(ns string, keys []gae.DSKey, plss []gae.DSPropertyLoadSaver) error {
	return getMultiInner(ns, keys, plss, func() (*memCollection, error) {
		d.rwlock.RLock()
		s := d.store.Snapshot()
		d.rwlock.RUnlock()

		return s.GetCollection("ents:" + ns), nil
	})
}

func (d *dataStoreData) del(ns string, key gae.DSKey) (err error) {
	return gae.SingleError(d.delMulti(ns, []gae.DSKey{key}))
}

func (d *dataStoreData) delMulti(ns string, keys []gae.DSKey) error {
	lme := gae.LazyMultiError{Size: len(keys)}
	toDel := make([][]byte, 0, len(keys))
	for i, k := range keys {
		if !helper.DSKeyValid(k, ns, false) {
			lme.Assign(i, gae.ErrDSInvalidKey)
			continue
		}
		toDel = append(toDel, keyBytes(helper.WithoutContext, k))
	}
	err := lme.Get()
	if err != nil {
		return err
	}

	d.rwlock.Lock()
	defer d.rwlock.Unlock()

	ents := d.store.GetCollection("ents:" + ns)
	if ents == nil {
		return nil
	}

	for i, k := range keys {
		incrementLocked(ents, groupMetaKey(k))
		kb := toDel[i]
		old := ents.Get(kb)
		oldPM := gae.DSPropertyMap(nil)
		if old != nil {
			if oldPM, err = rpmWoCtx(old, ns); err != nil {
				lme.Assign(i, err)
				continue
			}
		}
		updateIndicies(d.store, k, oldPM, nil)
		ents.Delete(kb)
	}
	return lme.Get()
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
		vHead := curVersion(entsHead, mkey)
		vSnap := curVersion(entsSnap, mkey)
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
				_, err = d.put(m.key.Namespace(), m.key, m.data)
			}
			if err != nil {
				panic(err)
			}
		}
	}
}

func (d *dataStoreData) mkTxn(o *gae.DSTransactionOptions) memContextObj {
	return &txnDataStoreData{
		// alias to the main datastore's so that testing code can have primitive
		// access to break features inside of transactions.
		parent: d,
		isXG:   o != nil && o.XG,
		snap:   d.store.Snapshot(),
		muts:   map[string][]txnMutation{},
	}
}

func (d *dataStoreData) endTxn() {}

/////////////////////////////// txnDataStoreData ///////////////////////////////

type txnMutation struct {
	key  gae.DSKey
	data gae.DSPropertyMap
}

type txnDataStoreData struct {
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

var _ memContextObj = (*txnDataStoreData)(nil)

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
func (*txnDataStoreData) mkTxn(*gae.DSTransactionOptions) memContextObj {
	panic("impossible")
}

func (td *txnDataStoreData) run(f func() error) error {
	// Slightly different from the SDK... datastore and taskqueue each implement
	// this here, where in the SDK only datastore.transaction.Call does.
	if atomic.LoadInt32(&td.closed) == 1 {
		return errors.New("datastore: transaction context has expired")
	}
	return f()
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

func (td *txnDataStoreData) put(ns string, key gae.DSKey, pls gae.DSPropertyLoadSaver) (gae.DSKey, error) {
	keys, errs := td.putMulti(ns, []gae.DSKey{key}, []gae.DSPropertyLoadSaver{pls})
	if errs == nil {
		return keys[0], nil
	}
	return nil, gae.SingleError(errs)
}

func (td *txnDataStoreData) putMulti(ns string, keys []gae.DSKey, plss []gae.DSPropertyLoadSaver) ([]gae.DSKey, error) {
	pmaps, err := putMultiPrelim(ns, keys, plss)
	if err != nil {
		return nil, err
	}

	retKeys := make([]gae.DSKey, len(keys))
	lme := gae.LazyMultiError{Size: len(keys)}
	for i, k := range keys {
		func() {
			td.parent.Lock()
			defer td.parent.Unlock()
			_, k = td.parent.entsKeyLocked(k)
		}()
		lme.Assign(i, td.writeMutation(false, k, pmaps[i]))
		retKeys[i] = k
	}

	return retKeys, lme.Get()
}

func (td *txnDataStoreData) get(ns string, key gae.DSKey, pls gae.DSPropertyLoadSaver) error {
	return gae.SingleError(td.getMulti(ns, []gae.DSKey{key}, []gae.DSPropertyLoadSaver{pls}))
}

func (td *txnDataStoreData) getMulti(ns string, keys []gae.DSKey, plss []gae.DSPropertyLoadSaver) error {
	return getMultiInner(ns, keys, plss, func() (*memCollection, error) {
		lme := gae.LazyMultiError{Size: len(keys)}
		for i, k := range keys {
			lme.Assign(i, td.writeMutation(true, k, nil))
		}
		return td.snap.GetCollection("ents:" + ns), lme.Get()
	})
}

func (td *txnDataStoreData) del(ns string, key gae.DSKey) error {
	return gae.SingleError(td.delMulti(ns, []gae.DSKey{key}))
}

func (td *txnDataStoreData) delMulti(ns string, keys []gae.DSKey) error {
	lme := gae.LazyMultiError{Size: len(keys)}
	for i, k := range keys {
		if !helper.DSKeyValid(k, ns, false) {
			lme.Assign(i, gae.ErrDSInvalidKey)
		} else {
			lme.Assign(i, td.writeMutation(false, k, nil))
		}
	}
	return lme.Get()
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

func multiValid(keys []gae.DSKey, plss []gae.DSPropertyLoadSaver, ns string, potentialKey, allowSpecial bool) error {
	vfn := func(k gae.DSKey) bool {
		return !helper.DSKeyIncomplete(k) && helper.DSKeyValid(k, ns, allowSpecial)
	}
	if potentialKey {
		vfn = func(k gae.DSKey) bool {
			// adds an id to k if it's incomplete.
			if helper.DSKeyIncomplete(k) {
				k = helper.NewDSKey(k.AppID(), k.Namespace(), k.Kind(), "", 1, k.Parent())
			}
			return helper.DSKeyValid(k, ns, allowSpecial)
		}
	}

	if keys == nil || plss == nil {
		return errors.New("gae: key or plss slices were nil")
	}
	if len(keys) != len(plss) {
		return errors.New("gae: key and dst slices have different length")
	}
	lme := gae.LazyMultiError{Size: len(keys)}
	for i, k := range keys {
		if !vfn(k) {
			lme.Assign(i, gae.ErrDSInvalidKey)
		}
	}
	return lme.Get()
}
