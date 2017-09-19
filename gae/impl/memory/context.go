// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package memory

import (
	"errors"
	"strings"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/logging/memlogger"

	"golang.org/x/net/context"
)

var serializationDeterministic = false

type memContextObj interface {
	// beginCommit locks both m and self in preparation for the committing pending
	// data represented by 'm' into self.
	//
	// Must be called with both m and self unlocked. There can be only one commit
	// operation at a time.
	//
	// Returns nil if the commit can't be applied (e.g due to a collision).
	//
	// Call either txnCommitOp.submit or txnCommitOp.discard to finish the commit.
	beginCommit(c context.Context, m memContextObj) txnCommitOp

	// mkTxn creates an object that holds temporary transaction data.
	mkTxn(*ds.TransactionOptions) memContextObj

	// endTxn is used to mark the transaction represented by self as closed.
	endTxn()
}

type txnCommitOp interface {
	submit()
	discard()
}

type txnCommitCallback struct {
	apply  func() // called only on 'submit'
	unlock func() // called on both 'submit' and 'discard'
}

func (b *txnCommitCallback) submit() {
	b.apply()
	b.unlock()
}

func (b *txnCommitCallback) discard() {
	b.unlock()
}

type txnBatchCommitOp []txnCommitOp

func (b txnBatchCommitOp) submit() {
	for i := len(b) - 1; i >= 0; i-- {
		b[i].submit()
	}
}

func (b txnBatchCommitOp) discard() {
	for i := len(b) - 1; i >= 0; i-- {
		b[i].discard()
	}
}

type memContext []memContextObj

var _ memContextObj = (memContext)(nil)

func newMemContext(aid string) memContext {
	return memContext{
		newTaskQueueData(),
		newDataStoreData(aid),
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

func (m memContext) endTxn() {
	for _, itm := range m {
		itm.endTxn()
	}
}

func (m memContext) mkTxn(o *ds.TransactionOptions) memContextObj {
	ret := make(memContext, len(m))
	for i, itm := range m {
		ret[i] = itm.mkTxn(o)
	}
	return ret
}

func (m memContext) beginCommit(c context.Context, txnCtxObj memContextObj) txnCommitOp {
	batch := make(txnBatchCommitOp, 0, len(m))
	txnCtx := txnCtxObj.(memContext)
	for i := range m {
		op := m[i].beginCommit(c, txnCtx[i])
		if op == nil {
			batch.discard()
			return nil
		}
		batch = append(batch, op)
	}
	return batch
}

// Use calls UseWithAppID with the appid of "app"
func Use(c context.Context) context.Context {
	return UseWithAppID(c, "dev~app")
}

// UseInfo adds an implementation for:
//   * go.chromium.org/gae/service/info
// The application id wil be set to 'aid', and will not be modifiable in this
// context. If 'aid' contains a "~" character, it will be treated as the
// fully-qualified App ID and the AppID will be the string following the "~".
func UseInfo(c context.Context, aid string) context.Context {
	if c.Value(&memContextKey) != nil {
		panic(errors.New("memory.Use: called twice on the same Context"))
	}

	fqAppID := aid
	if parts := strings.SplitN(fqAppID, "~", 2); len(parts) == 2 {
		aid = parts[1]
	}

	memctx := newMemContext(fqAppID)
	c = context.WithValue(c, &memContextKey, memctx)

	return useGI(useGID(c, func(mod *globalInfoData) {
		mod.appID = aid
		mod.fqAppID = fqAppID
	}))
}

// UseWithAppID adds implementations for the following gae services to the
// context:
//   * go.chromium.org/gae/service/datastore
//   * go.chromium.org/gae/service/info
//   * go.chromium.org/gae/service/mail
//   * go.chromium.org/gae/service/memcache
//   * go.chromium.org/gae/service/taskqueue
//   * go.chromium.org/gae/service/user
//   * go.chromium.org/luci/common/logger (using memlogger)
//
// The application id wil be set to 'aid', and will not be modifiable in this
// context. If 'aid' contains a "~" character, it will be treated as the
// fully-qualified App ID and the AppID will be the string following the "~".
//
// These can be retrieved with the gae.Get functions.
//
// The implementations are all backed by an in-memory implementation, and start
// with an empty state.
//
// Using this more than once per context.Context will cause a panic.
func UseWithAppID(c context.Context, aid string) context.Context {
	c = memlogger.Use(c)
	c = UseInfo(c, aid) // Panics if UseWithAppID is called twice.
	return useMod(useMail(useUser(useTQ(useRDS(useMC(c))))))
}

func cur(c context.Context) (memContext, bool) {
	if txn := c.Value(&currentTxnKey); txn != nil {
		// We are in a Transaction.
		return txn.(memContext), true
	}
	return c.Value(&memContextKey).(memContext), false
}

var (
	memContextKey = "gae:memory:context"
	currentTxnKey = "gae:memory:currentTxn"
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
		curMC, inTxn := cur(d)
		if inTxn {
			return errors.New("datastore: nested transactions are not supported")
		}

		txnMC := curMC.mkTxn(o)
		defer txnMC.endTxn()

		if err := f(context.WithValue(d, &currentTxnKey, txnMC)); err != nil {
			return err
		}

		if !applyForReal {
			return ds.ErrConcurrentTransaction
		}

		commitOp := curMC.beginCommit(d, txnMC)
		if commitOp == nil {
			return ds.ErrConcurrentTransaction
		}
		commitOp.submit()
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
