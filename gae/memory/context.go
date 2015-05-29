// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"infra/gae/libs/wrapper"
	"math/rand"
	"sync"

	"golang.org/x/net/context"

	"appengine/datastore"
)

type memContextObj interface {
	sync.Locker
	canApplyTxn(m memContextObj) bool
	applyTxn(rnd *rand.Rand, m memContextObj)

	endTxn()
	mkTxn(*datastore.TransactionOptions) (memContextObj, error)
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

func (m memContext) mkTxn(o *datastore.TransactionOptions) (memContextObj, error) {
	ret := make(memContext, len(m))
	for i, itm := range m {
		newItm, err := itm.mkTxn(o)
		if err != nil {
			return nil, err
		}
		ret[i] = newItm
	}
	return ret, nil
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

func (m memContext) applyTxn(rnd *rand.Rand, txnCtxObj memContextObj) {
	txnCtx := txnCtxObj.(memContext)
	for i := range m {
		m[i].applyTxn(rnd, txnCtx[i])
	}
}

// Enable adds a new memory context to c. This new memory context will have
// a zeroed state.
func Enable(c context.Context) context.Context {
	return context.WithValue(
		context.WithValue(c, memContextKey, newMemContext()),
		giContextKey, &globalInfoData{})
}

// Use calls ALL of this packages Use* methods on c. This enables all
// gae/wrapper Get* api's.
func Use(c context.Context) context.Context {
	return UseTQ(UseDS(UseMC(UseGI(c))))
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
func (d *dsImpl) RunInTransaction(f func(context.Context) error, o *datastore.TransactionOptions) error {
	curMC := cur(d.c)

	txnMC, err := curMC.mkTxn(o)
	if err != nil {
		return err
	}

	defer func() {
		txnMC.Lock()
		defer txnMC.Unlock()

		txnMC.endTxn()
	}()

	if err = f(context.WithValue(d.c, memContextKey, txnMC)); err != nil {
		return err
	}

	txnMC.Lock()
	defer txnMC.Unlock()

	if curMC.canApplyTxn(txnMC) {
		curMC.applyTxn(wrapper.GetMathRand(d.c), txnMC)
	} else {
		return datastore.ErrConcurrentTransaction
	}
	return nil
}
