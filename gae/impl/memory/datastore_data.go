// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"bytes"
	"fmt"
	"sync"
	"sync/atomic"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/datastore/dskey"
	"github.com/luci/gae/service/datastore/serialize"
	"github.com/luci/luci-go/common/errors"
	"golang.org/x/net/context"
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

func (d *dataStoreData) getQuerySnaps(consistent bool) (idx, head *memStore) {
	d.rwlock.RLock()
	defer d.rwlock.RUnlock()
	head = d.store.Snapshot()
	if consistent {
		idx = head
	} else {
		idx = d.snap
	}
	return
}

func (d *dataStoreData) takeSnapshot() *memStore {
	d.rwlock.RLock()
	defer d.rwlock.RUnlock()
	return d.store.Snapshot()
}

func (d *dataStoreData) setSnapshot(snap *memStore) {
	d.rwlock.Lock()
	defer d.rwlock.Unlock()
	d.snap = snap
}

func (d *dataStoreData) catchupIndexes() {
	d.rwlock.Lock()
	defer d.rwlock.Unlock()
	d.snap = d.store.Snapshot()
}

/////////////////////////// indicies(dataStoreData) ////////////////////////////

func groupMetaKey(key ds.Key) []byte {
	return keyBytes(serialize.WithoutContext,
		dskey.New("", "", "__entity_group__", "", 1, dskey.Root(key)))
}

func groupIDsKey(key ds.Key) []byte {
	return keyBytes(serialize.WithoutContext,
		dskey.New("", "", "__entity_group_ids__", "", 1, dskey.Root(key)))
}

func rootIDsKey(kind string) []byte {
	return keyBytes(serialize.WithoutContext,
		dskey.New("", "", "__entity_root_ids__", kind, 0, nil))
}

func curVersion(ents *memCollection, key []byte) int64 {
	if ents != nil {
		if v := ents.Get(key); v != nil {
			pm, err := rpm(v)
			if err != nil {
				panic(err) // memory corruption
			}
			pl, ok := pm["__version__"]
			if ok && len(pl) > 0 && pl[0].Type() == ds.PTInt {
				return pl[0].Value().(int64)
			}
			panic(fmt.Errorf("__version__ property missing or wrong: %v", pm))
		}
	}
	return 0
}

func incrementLocked(ents *memCollection, key []byte) int64 {
	ret := curVersion(ents, key) + 1
	buf := &bytes.Buffer{}
	serialize.WritePropertyMap(buf, serialize.WithContext, ds.PropertyMap{
		"__version__": {ds.MkPropertyNI(ret)}})
	ents.Set(key, buf.Bytes())
	return ret
}

func (d *dataStoreData) entsKeyLocked(key ds.Key) (*memCollection, ds.Key) {
	coll := "ents:" + key.Namespace()
	ents := d.store.GetCollection(coll)
	if ents == nil {
		ents = d.store.SetCollection(coll, nil)
	}

	if dskey.Incomplete(key) {
		idKey := []byte(nil)
		if key.Parent() == nil {
			idKey = rootIDsKey(key.Kind())
		} else {
			idKey = groupIDsKey(key)
		}
		id := incrementLocked(ents, idKey)
		key = dskey.New(key.AppID(), key.Namespace(), key.Kind(), "", id, key.Parent())
	}

	return ents, key
}

func (d *dataStoreData) putMulti(keys []ds.Key, vals []ds.PropertyMap, cb ds.PutMultiCB) {
	for i, k := range keys {
		buf := &bytes.Buffer{}
		pmap, _ := vals[i].Save(false)
		serialize.WritePropertyMap(buf, serialize.WithoutContext, pmap)
		dataBytes := buf.Bytes()

		k, err := func() (ret ds.Key, err error) {
			d.rwlock.Lock()
			defer d.rwlock.Unlock()

			ents, ret := d.entsKeyLocked(k)
			incrementLocked(ents, groupMetaKey(ret))

			old := ents.Get(keyBytes(serialize.WithoutContext, ret))
			oldPM := ds.PropertyMap(nil)
			if old != nil {
				if oldPM, err = rpmWoCtx(old, ret.Namespace()); err != nil {
					return
				}
			}
			updateIndicies(d.store, ret, oldPM, pmap)
			ents.Set(keyBytes(serialize.WithoutContext, ret), dataBytes)
			return
		}()
		if cb != nil {
			cb(k, err)
		}
	}
}

func getMultiInner(keys []ds.Key, cb ds.GetMultiCB, getColl func() (*memCollection, error)) error {
	ents, err := getColl()
	if err != nil {
		return err
	}
	if ents == nil {
		for range keys {
			cb(nil, ds.ErrNoSuchEntity)
		}
		return nil
	}

	for _, k := range keys {
		pdata := ents.Get(keyBytes(serialize.WithoutContext, k))
		if pdata == nil {
			cb(nil, ds.ErrNoSuchEntity)
			continue
		}
		cb(rpmWoCtx(pdata, k.Namespace()))
	}
	return nil
}

func (d *dataStoreData) getMulti(keys []ds.Key, cb ds.GetMultiCB) error {
	getMultiInner(keys, cb, func() (*memCollection, error) {
		s := d.takeSnapshot()

		return s.GetCollection("ents:" + keys[0].Namespace()), nil
	})
	return nil
}

func (d *dataStoreData) delMulti(keys []ds.Key, cb ds.DeleteMultiCB) {
	toDel := make([][]byte, 0, len(keys))
	for _, k := range keys {
		toDel = append(toDel, keyBytes(serialize.WithoutContext, k))
	}
	ns := keys[0].Namespace()

	d.rwlock.Lock()
	defer d.rwlock.Unlock()

	ents := d.store.GetCollection("ents:" + ns)

	for i, k := range keys {
		if ents != nil {
			incrementLocked(ents, groupMetaKey(k))
			kb := toDel[i]
			if old := ents.Get(kb); old != nil {
				oldPM, err := rpmWoCtx(old, ns)
				if err != nil {
					if cb != nil {
						cb(err)
					}
					continue
				}
				updateIndicies(d.store, k, oldPM, nil)
				ents.Delete(kb)
			}
		}
		if cb != nil {
			cb(nil)
		}
	}
}

func (d *dataStoreData) canApplyTxn(obj memContextObj) bool {
	// TODO(riannucci): implement with Flush/FlushRevert for persistance.

	txn := obj.(*txnDataStoreData)
	for rk, muts := range txn.muts {
		if len(muts) == 0 { // read-only
			continue
		}
		k, err := serialize.ReadKey(bytes.NewBufferString(rk), serialize.WithContext, "", "")
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
		// TODO(riannucci): refactor to do just 1 putMulti, and 1 delMulti
		for _, m := range muts {
			err := error(nil)
			k := m.key
			if m.data == nil {
				d.delMulti([]ds.Key{k},
					func(e error) { err = e })
			} else {
				d.putMulti([]ds.Key{m.key}, []ds.PropertyMap{m.data},
					func(_ ds.Key, e error) { err = e })
			}
			err = errors.SingleError(err)
			if err != nil {
				panic(err)
			}
		}
	}
}

func (d *dataStoreData) mkTxn(o *ds.TransactionOptions) memContextObj {
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
	key  ds.Key
	data ds.PropertyMap
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
func (*txnDataStoreData) mkTxn(*ds.TransactionOptions) memContextObj {
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
func (td *txnDataStoreData) writeMutation(getOnly bool, key ds.Key, data ds.PropertyMap) error {
	rk := string(keyBytes(serialize.WithContext, dskey.Root(key)))

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

func (td *txnDataStoreData) putMulti(keys []ds.Key, vals []ds.PropertyMap, cb ds.PutMultiCB) {
	for i, k := range keys {
		func() {
			td.parent.Lock()
			defer td.parent.Unlock()
			_, k = td.parent.entsKeyLocked(k)
		}()
		err := td.writeMutation(false, k, vals[i])
		if cb != nil {
			cb(k, err)
		}
	}
}

func (td *txnDataStoreData) getMulti(keys []ds.Key, cb ds.GetMultiCB) error {
	return getMultiInner(keys, cb, func() (*memCollection, error) {
		err := error(nil)
		for _, key := range keys {
			err = td.writeMutation(true, key, nil)
			if err != nil {
				return nil, err
			}
		}
		return td.snap.GetCollection("ents:" + keys[0].Namespace()), nil
	})
}

func (td *txnDataStoreData) delMulti(keys []ds.Key, cb ds.DeleteMultiCB) error {
	for _, k := range keys {
		err := td.writeMutation(false, k, nil)
		if cb != nil {
			cb(err)
		}
	}
	return nil
}

func keyBytes(ctx serialize.KeyContext, key ds.Key) []byte {
	buf := &bytes.Buffer{}
	serialize.WriteKey(buf, ctx, key)
	return buf.Bytes()
}

func rpmWoCtx(data []byte, ns string) (ds.PropertyMap, error) {
	return serialize.ReadPropertyMap(bytes.NewBuffer(data),
		serialize.WithoutContext, globalAppID, ns)
}

func rpm(data []byte) (ds.PropertyMap, error) {
	return serialize.ReadPropertyMap(bytes.NewBuffer(data),
		serialize.WithContext, "", "")
}

type keyitem interface {
	Key() ds.Key
}
