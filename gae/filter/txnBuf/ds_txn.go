// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package txnBuf

import (
	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/errors"
	"golang.org/x/net/context"
)

// ErrTransactionTooLarge is returned when applying an inner transaction would
// cause an outer transaction to become too large.
var ErrTransactionTooLarge = errors.New(
	"applying the transaction would make the parent transaction too large")

// ErrTooManyRoots is returned when executing an operation which would cause
// the transaction to exceed it's allotted number of entity groups.
var ErrTooManyRoots = errors.New(
	"operating on too many entity groups in nested transaction")

type dsTxnBuf struct {
	ic    context.Context
	state *txnBufState
}

var _ ds.RawInterface = (*dsTxnBuf)(nil)

func (d *dsTxnBuf) DecodeCursor(s string) (ds.Cursor, error) {
	return d.state.parentDS.DecodeCursor(s)
}

func (d *dsTxnBuf) AllocateIDs(incomplete *ds.Key, n int) (start int64, err error) {
	return d.state.parentDS.AllocateIDs(incomplete, n)
}

func (d *dsTxnBuf) GetMulti(keys []*ds.Key, metas ds.MultiMetaGetter, cb ds.GetMultiCB) error {
	data, err := d.state.getMulti(keys)
	if err != nil {
		return err
	}

	idxMap := []int(nil)
	getKeys := []*ds.Key(nil)
	getMetas := ds.MultiMetaGetter(nil)
	lme := errors.NewLazyMultiError(len(keys))

	for i, itm := range data {
		if !itm.buffered {
			idxMap = append(idxMap, i)
			getKeys = append(getKeys, itm.key)
			getMetas = append(getMetas, metas.GetSingle(i))
		}
	}

	if len(idxMap) > 0 {
		j := 0
		err := d.state.parentDS.GetMulti(getKeys, getMetas, func(pm ds.PropertyMap, err error) {
			if err != ds.ErrNoSuchEntity {
				i := idxMap[j]
				if !lme.Assign(i, err) {
					data[i].data = pm
				}
			}
			j++
		})
		if err != nil {
			return err
		}
	}

	for i, itm := range data {
		err := lme.GetOne(i)
		if err != nil {
			cb(nil, err)
		} else if itm.data == nil {
			cb(nil, ds.ErrNoSuchEntity)
		} else {
			cb(itm.data, nil)
		}
	}
	return nil
}

func (d *dsTxnBuf) PutMulti(keys []*ds.Key, vals []ds.PropertyMap, cb ds.PutMultiCB) error {
	lme := errors.NewLazyMultiError(len(keys))
	realKeys := []*ds.Key(nil)
	for i, key := range keys {
		if key.Incomplete() {
			start, err := d.AllocateIDs(key, 1)
			if !lme.Assign(i, err) {
				if realKeys == nil {
					realKeys = make([]*ds.Key, len(keys))
					copy(realKeys, keys)
				}

				aid, ns, toks := key.Split()
				toks[len(toks)-1].IntID = start
				realKeys[i] = ds.NewKeyToks(aid, ns, toks)
			}
		}
	}
	if err := lme.Get(); err != nil {
		for _, e := range err.(errors.MultiError) {
			if e == nil {
				e = errors.New("putMulti failed because some keys were unable to AllocateIDs")
			}
			cb(nil, e)
		}
		return nil
	}

	if realKeys == nil {
		realKeys = keys
	}

	err := d.state.putMulti(realKeys, vals)
	if err != nil {
		return err
	}

	for _, k := range realKeys {
		cb(k, nil)
	}
	return nil
}

func (d *dsTxnBuf) DeleteMulti(keys []*ds.Key, cb ds.DeleteMultiCB) error {
	if err := d.state.deleteMulti(keys); err != nil {
		return err
	}

	for range keys {
		cb(nil)
	}
	return nil
}

func (d *dsTxnBuf) Count(fq *ds.FinalizedQuery) (count int64, err error) {
	// Unfortunately there's no fast-path here. We literally have to run the
	// query and count. Fortunately we can optimize to count keys if it's not
	// a projection query. This will save on bandwidth a bit.
	if len(fq.Project()) == 0 && !fq.KeysOnly() {
		fq, err = fq.Original().KeysOnly(true).Finalize()
		if err != nil {
			return
		}
	}
	err = d.Run(fq, func(_ *ds.Key, _ ds.PropertyMap, _ ds.CursorCB) bool {
		count++
		return true
	})
	return
}

func (d *dsTxnBuf) Run(fq *ds.FinalizedQuery, cb ds.RawRunCB) error {
	if start, end := fq.Bounds(); start != nil || end != nil {
		return errors.New("txnBuf filter does not support query cursors")
	}

	limit, limitSet := fq.Limit()
	offset, _ := fq.Offset()
	keysOnly := fq.KeysOnly()

	project := fq.Project()

	d.state.Lock()
	memDS := d.state.memDS
	parentDS := d.state.parentDS
	sizes := d.state.entState.dup()
	d.state.Unlock()

	return runMergedQueries(fq, sizes, memDS, parentDS, func(key *ds.Key, data ds.PropertyMap) bool {
		if offset > 0 {
			offset--
			return true
		}
		if limitSet {
			if limit == 0 {
				return false
			}
			limit--
		}
		if keysOnly {
			data = nil
		} else if len(project) > 0 {
			newData := make(ds.PropertyMap, len(project))
			for _, p := range project {
				newData[p] = data[p]
			}
			data = newData
		}
		return cb(key, data, nil)
	})
}

func (d *dsTxnBuf) RunInTransaction(cb func(context.Context) error, opts *ds.TransactionOptions) error {
	return withTxnBuf(d.ic, cb, opts)
}

func (d *dsTxnBuf) Testable() ds.Testable {
	return d.state.parentDS.Testable()
}
