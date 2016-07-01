// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package datastore

import (
	"fmt"

	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/errors"
	"golang.org/x/net/context"
)

type checkFilter struct {
	RawInterface

	aid string
	ns  string
}

func (tcf *checkFilter) RunInTransaction(f func(c context.Context) error, opts *TransactionOptions) error {
	if f == nil {
		return fmt.Errorf("datastore: RunInTransaction function is nil")
	}
	return tcf.RawInterface.RunInTransaction(f, opts)
}

func (tcf *checkFilter) Run(fq *FinalizedQuery, cb RawRunCB) error {
	if fq == nil {
		return fmt.Errorf("datastore: Run query is nil")
	}
	if cb == nil {
		return fmt.Errorf("datastore: Run callback is nil")
	}
	return tcf.RawInterface.Run(fq, cb)
}

func (tcf *checkFilter) GetMulti(keys []*Key, meta MultiMetaGetter, cb GetMultiCB) error {
	if len(keys) == 0 {
		return nil
	}
	if cb == nil {
		return fmt.Errorf("datastore: GetMulti callback is nil")
	}
	lme := errors.NewLazyMultiError(len(keys))
	for i, k := range keys {
		if k.IsIncomplete() || !k.Valid(true, tcf.aid, tcf.ns) {
			lme.Assign(i, ErrInvalidKey)
		}
	}
	if me := lme.Get(); me != nil {
		for _, err := range me.(errors.MultiError) {
			cb(nil, err)
		}
		return nil
	}
	return tcf.RawInterface.GetMulti(keys, meta, cb)
}

func (tcf *checkFilter) PutMulti(keys []*Key, vals []PropertyMap, cb NewKeyCB) error {
	if len(keys) != len(vals) {
		return fmt.Errorf("datastore: PutMulti with mismatched keys/vals lengths (%d/%d)", len(keys), len(vals))
	}
	if len(keys) == 0 {
		return nil
	}
	if cb == nil {
		return fmt.Errorf("datastore: PutMulti callback is nil")
	}
	lme := errors.NewLazyMultiError(len(keys))
	for i, k := range keys {
		if !k.PartialValid(tcf.aid, tcf.ns) {
			lme.Assign(i, ErrInvalidKey)
			continue
		}
		v := vals[i]
		if v == nil {
			lme.Assign(i, errors.New("datastore: PutMulti got nil vals entry"))
		}
	}
	if me := lme.Get(); me != nil {
		for _, err := range me.(errors.MultiError) {
			cb(nil, err)
		}
		return nil
	}

	return tcf.RawInterface.PutMulti(keys, vals, cb)
}

func (tcf *checkFilter) DeleteMulti(keys []*Key, cb DeleteMultiCB) error {
	if len(keys) == 0 {
		return nil
	}
	if cb == nil {
		return fmt.Errorf("datastore: DeleteMulti callback is nil")
	}
	lme := errors.NewLazyMultiError(len(keys))
	for i, k := range keys {
		if k.IsIncomplete() || !k.Valid(false, tcf.aid, tcf.ns) {
			lme.Assign(i, ErrInvalidKey)
		}
	}
	if me := lme.Get(); me != nil {
		for _, err := range me.(errors.MultiError) {
			cb(err)
		}
		return nil
	}
	return tcf.RawInterface.DeleteMulti(keys, cb)
}

func applyCheckFilter(c context.Context, i RawInterface) RawInterface {
	inf := info.Get(c)
	ns, _ := inf.GetNamespace()
	return &checkFilter{i, inf.FullyQualifiedAppID(), ns}
}
