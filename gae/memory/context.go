// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"errors"
	"sync"

	"golang.org/x/net/context"

	"infra/gae/libs/gae"
)

type memContextObj interface {
	sync.Locker
	canApplyTxn(m memContextObj) bool
	applyTxn(c context.Context, m memContextObj)

	endTxn()
	mkTxn(*gae.DSTransactionOptions) memContextObj
}

type memContext []memContextObj

var _ = memContextObj((memContext)(nil))

func newMemContext() memContext {
	return memContext{
		newTaskQueueData(),
		newDataStoreData(),
	}
}

type memContextIdx int

const (
	memContextTQIdx memContextIdx = iota
	memContextDSIdx
)

func (m memContext) Get(itm memContextIdx) memContextObj {
	return m[itm]
}

func (m memContext) Lock() {
	for _, itm := range m {
		itm.Lock()
	}
}

func (m memContext) Unlock() {
	for i := len(m) - 1; i >= 0; i-- {
		m[i].Unlock()
	}
}

func (m memContext) endTxn() {
	for _, itm := range m {
		itm.endTxn()
	}
}

func (m memContext) mkTxn(o *gae.DSTransactionOptions) memContextObj {
	ret := make(memContext, len(m))
	for i, itm := range m {
		ret[i] = itm.mkTxn(o)
	}
	return ret
}

func (m memContext) canApplyTxn(txnCtxObj memContextObj) bool {
	txnCtx := txnCtxObj.(memContext)
	for i := range m {
		if !m[i].canApplyTxn(txnCtx[i]) {
			return false
		}
	}
	return true
}

func (m memContext) applyTxn(c context.Context, txnCtxObj memContextObj) {
	txnCtx := txnCtxObj.(memContext)
	for i := range m {
		m[i].applyTxn(c, txnCtx[i])
	}
}

// Use adds implementations for the following gae interfaces to the
// context:
//   * gae.Datastore
//   * gae.TaskQueue
//   * gae.Memcache
//   * gae.GlobalInfo
//
// These can be retrieved with the gae.Get functions.
//
// The implementations are all backed by an in-memory implementation, and start
// with an empty state.
//
// Using this more than once per context.Context will cause a panic.
func Use(c context.Context) context.Context {
	if c.Value(memContextKey) != nil {
		panic(errors.New("memory.Use: called twice on the same Context"))
	}
	c = context.WithValue(
		context.WithValue(c, memContextKey, newMemContext()),
		giContextKey, &globalInfoData{})
	return useTQ(useRDS(useMC(useGI(c))))
}

func cur(c context.Context) (p memContext) {
	p, _ = c.Value(memContextKey).(memContext)
	return
}

type memContextKeyType int

var memContextKey memContextKeyType

// weird stuff

// RunInTransaction is here because it's really a service-wide transaction, not
// just in the datastore. TaskQueue behaves differently in a transaction in
// a couple ways, for example.
//
// It really should have been appengine.Context.RunInTransaction(func(tc...)),
// but because it's not, this method is on dsImpl instead to mirror the official
// API.
//
// The fake implementation also differs from the real implementation because the
// fake TaskQueue is NOT backed by the fake Datastore. This is done to make the
// test-access API for TaskQueue better (instead of trying to reconstitute the
// state of the task queue from a bunch of datastore accesses).
func (d *dsImpl) RunInTransaction(f func(context.Context) error, o *gae.DSTransactionOptions) error {
	curMC := cur(d.c)

	txnMC := curMC.mkTxn(o)

	defer func() {
		txnMC.Lock()
		defer txnMC.Unlock()

		txnMC.endTxn()
	}()

	if err := f(context.WithValue(d.c, memContextKey, txnMC)); err != nil {
		return err
	}

	txnMC.Lock()
	defer txnMC.Unlock()

	if curMC.canApplyTxn(txnMC) {
		curMC.applyTxn(d.c, txnMC)
	} else {
		return gae.ErrDSConcurrentTransaction
	}
	return nil
}
