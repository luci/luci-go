// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memory

import (
	"errors"
	"sync"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/luci-go/common/logging/memlogger"
	"golang.org/x/net/context"
)

var serializationDeterministic = false

type memContextObj interface {
	sync.Locker
	canApplyTxn(m memContextObj) bool
	applyTxn(c context.Context, m memContextObj)

	endTxn()
	mkTxn(*ds.TransactionOptions) memContextObj
}

type memContext []memContextObj

var _ memContextObj = (*memContext)(nil)

func newMemContext(aid string) *memContext {
	return &memContext{
		newTaskQueueData(),
		newDataStoreData(aid),
	}
}

type memContextIdx int

const (
	memContextTQIdx memContextIdx = iota
	memContextDSIdx
)

func (m *memContext) Get(itm memContextIdx) memContextObj {
	return (*m)[itm]
}

func (m *memContext) Lock() {
	for _, itm := range *m {
		itm.Lock()
	}
}

func (m *memContext) Unlock() {
	for i := len(*m) - 1; i >= 0; i-- {
		(*m)[i].Unlock()
	}
}

func (m *memContext) endTxn() {
	for _, itm := range *m {
		itm.endTxn()
	}
}

func (m *memContext) mkTxn(o *ds.TransactionOptions) memContextObj {
	ret := make(memContext, len(*m))
	for i, itm := range *m {
		ret[i] = itm.mkTxn(o)
	}
	return &ret
}

func (m *memContext) canApplyTxn(txnCtxObj memContextObj) bool {
	txnCtx := *txnCtxObj.(*memContext)
	for i := range *m {
		if !(*m)[i].canApplyTxn(txnCtx[i]) {
			return false
		}
	}
	return true
}

func (m *memContext) applyTxn(c context.Context, txnCtxObj memContextObj) {
	txnCtx := *txnCtxObj.(*memContext)
	for i := range *m {
		(*m)[i].applyTxn(c, txnCtx[i])
	}
}

// Use calls UseWithAppID with the appid of "dev~app"
func Use(c context.Context) context.Context {
	return UseWithAppID(c, "dev~app")
}

// UseWithAppID adds implementations for the following gae services to the
// context:
//   * github.com/luci/gae/service/datastore
//   * github.com/luci/gae/service/taskqueue
//   * github.com/luci/gae/service/memcache
//   * github.com/luci/gae/service/info
//   * github.com/luci/gae/service/user
//   * github.com/luci/luci-go/common/logger (using memlogger)
//
// The application id wil be set to 'aid', and will not be modifiable in this
// context.
//
// These can be retrieved with the gae.Get functions.
//
// The implementations are all backed by an in-memory implementation, and start
// with an empty state.
//
// Using this more than once per context.Context will cause a panic.
func UseWithAppID(c context.Context, aid string) context.Context {
	if c.Value(memContextKey) != nil {
		panic(errors.New("memory.Use: called twice on the same Context"))
	}
	c = memlogger.Use(c)

	memctx := newMemContext(aid)
	c = context.WithValue(c, memContextKey, memctx)
	c = context.WithValue(c, memContextNoTxnKey, memctx)
	c = context.WithValue(c, giContextKey, &globalInfoData{appid: aid})
	return useTQ(useRDS(useMC(useGI(c, aid))))
}

func cur(c context.Context) (p *memContext) {
	p, _ = c.Value(memContextKey).(*memContext)
	return
}

func curNoTxn(c context.Context) (p *memContext) {
	p, _ = c.Value(memContextNoTxnKey).(*memContext)
	return
}

type memContextKeyType int

var (
	memContextKey      memContextKeyType
	memContextNoTxnKey memContextKeyType = 1
)

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
func (d *dsImpl) RunInTransaction(f func(context.Context) error, o *ds.TransactionOptions) error {
	if d.data.getDisableSpecialEntities() {
		return errors.New("special entities are disabled. no transactions for you")
	}

	// Keep in separate function for defers.
	loopBody := func(applyForReal bool) error {
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

		if applyForReal && curMC.canApplyTxn(txnMC) {
			curMC.applyTxn(d.c, txnMC)
		} else {
			return ds.ErrConcurrentTransaction
		}
		return nil
	}

	// From GAE docs for TransactionOptions: "If omitted, it defaults to 3."
	attempts := 3
	if o != nil && o.Attempts != 0 {
		attempts = o.Attempts
	}
	for attempt := 0; attempt < attempts; attempt++ {
		if err := loopBody(attempt >= d.data.txnFakeRetry); err != ds.ErrConcurrentTransaction {
			return err
		}
	}
	return ds.ErrConcurrentTransaction
}
