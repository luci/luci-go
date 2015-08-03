// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package datastore

import (
	"fmt"

	"github.com/luci/gae/service/info"
	"github.com/luci/luci-go/common/errors"
	"golang.org/x/net/context"
)

type checkFilter struct {
	Interface

	aid string
	ns  string
}

func (tcf *checkFilter) RunInTransaction(f func(c context.Context) error, opts *TransactionOptions) error {
	if f == nil {
		return nil
	}
	return tcf.Interface.RunInTransaction(f, opts)
}

func (tcf *checkFilter) Run(q Query, cb RunCB) error {
	if q == nil || cb == nil {
		return nil
	}
	return tcf.Interface.Run(q, cb)
}

func (tcf *checkFilter) GetMulti(keys []Key, cb GetMultiCB) error {
	if len(keys) == 0 || cb == nil {
		return nil
	}
	lme := errors.LazyMultiError{Size: len(keys)}
	for i, k := range keys {
		if KeyIncomplete(k) || !KeyValid(k, true, tcf.aid, tcf.ns) {
			lme.Assign(i, ErrInvalidKey)
		}
	}
	if me := lme.Get(); me != nil {
		for _, err := range me.(errors.MultiError) {
			cb(nil, err)
		}
		return nil
	}
	return tcf.Interface.GetMulti(keys, cb)
}

func (tcf *checkFilter) PutMulti(keys []Key, vals []PropertyLoadSaver, cb PutMultiCB) error {
	if len(keys) != len(vals) {
		return fmt.Errorf("datastore: GetMulti with mismatched keys/vals lengths (%d/%d)", len(keys), len(vals))
	}
	if len(keys) == 0 {
		return nil
	}
	lme := errors.LazyMultiError{Size: len(keys)}
	for i, k := range keys {
		if KeyIncomplete(k) {
			k = NewKey(k.AppID(), k.Namespace(), k.Kind(), "", 1, k.Parent())
		}
		if !KeyValid(k, false, tcf.aid, tcf.ns) {
			lme.Assign(i, ErrInvalidKey)
			continue
		}
		v := vals[i]
		if v == nil {
			lme.Assign(i, errors.New("datastore: PutMulti got nil vals entry"))
		} else {
			lme.Assign(i, v.Problem())
		}
	}
	if me := lme.Get(); me != nil {
		for _, err := range me.(errors.MultiError) {
			cb(nil, err)
		}
		return nil
	}

	err := error(nil)
	pmVals := make([]PropertyLoadSaver, len(vals))
	for i, val := range vals {
		pmVals[i], err = val.Save(true)
		lme.Assign(i, err)
	}
	if me := lme.Get(); me != nil {
		for _, err := range me.(errors.MultiError) {
			cb(nil, err)
		}
		return nil
	}

	return tcf.Interface.PutMulti(keys, pmVals, cb)
}

func (tcf *checkFilter) DeleteMulti(keys []Key, cb DeleteMultiCB) error {
	if len(keys) == 0 {
		return nil
	}
	lme := errors.LazyMultiError{Size: len(keys)}
	for i, k := range keys {
		if KeyIncomplete(k) || !KeyValid(k, false, tcf.aid, tcf.ns) {
			lme.Assign(i, ErrInvalidKey)
		}
	}
	if me := lme.Get(); me != nil {
		for _, err := range me.(errors.MultiError) {
			cb(err)
		}
		return nil
	}
	return tcf.Interface.DeleteMulti(keys, cb)
}

func applyCheckFilter(c context.Context, i Interface) Interface {
	inf := info.Get(c)
	return &checkFilter{i, inf.AppID(), inf.GetNamespace()}
}
