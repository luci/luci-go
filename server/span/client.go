// Copyright 2020 The LUCI Authors.
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

package span

import (
	"context"
	"sync"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
)

// Use installs a Spanner client into the context.
//
// Primarily used by the module initialization code. May be useful in tests as
// well.
func Use(ctx context.Context, client *spanner.Client) context.Context {
	return context.WithValue(ctx, &clientContextKey, client)
}

// Apply applies a list of mutations atomically to the database.
func Apply(ctx context.Context, ms []*spanner.Mutation, opts ...spanner.ApplyOption) (commitTimestamp time.Time, err error) {
	return client(ctx).Apply(ctx, ms, opts...)
}

// Single provides a read-only snapshot transaction optimized for the case where
// only a single read or query is needed. This is more efficient than using
// ReadOnlyTransaction() for a single read or query.
func Single(ctx context.Context) *spanner.ReadOnlyTransaction {
	return client(ctx).Single()
}

// ReadOnlyTransaction returns a ReadOnlyTransaction that can be used for
// multiple reads from the database. You must call Close() when the
// ReadOnlyTransaction is no longer needed to release resources on the server.
func ReadOnlyTransaction(ctx context.Context) *spanner.ReadOnlyTransaction {
	return client(ctx).ReadOnlyTransaction()
}

// ReadWriteTransaction executes a read-write transaction, with retries as
// necessary.
//
// The callback may be called multiple times if Spanner client decides to retry
// the transaction. In particular this happens if the callback returns (perhaps
// wrapped) ABORTED error. This error is returned by Spanner client methods if
// they encounter a stale transaction.
//
// See https://godoc.org/cloud.google.com/go/spanner#ReadWriteTransaction for
// more details.
//
// The callback can access the transaction object via Txn(ctx).
func ReadWriteTransaction(ctx context.Context, f func(ctx context.Context) error) (commitTimestamp time.Time, err error) {
	var state *txnState

	cts, err := client(ctx).ReadWriteTransaction(ctx, func(ctx context.Context, rw *spanner.ReadWriteTransaction) error {
		state = &txnState{rw: rw}
		err := f(setTxnState(ctx, state))
		if unwrapped := errors.Unwrap(err); spanner.ErrCode(unwrapped) == codes.Aborted {
			err = unwrapped
		}
		return err
	})

	if err == nil {
		state.execCBs(ctx)
	}

	return cts, err
}

// RW returns the current read-write transaction in the context or nil if it's
// not a read-write transactional context.
func RW(ctx context.Context) *spanner.ReadWriteTransaction {
	if s := getTxnState(ctx); s != nil {
		return s.rw
	}
	return nil
}

// WithoutTxn returns a copy of the context without the transaction in it.
func WithoutTxn(ctx context.Context) context.Context {
	if getTxnState(ctx) == nil {
		return ctx
	}
	return setTxnState(ctx, nil)
}

// Defer schedules `cb` for execution when the current transaction successfully
// lands.
//
// Intended for a best-effort non-transactional follow up to a successful
// transaction. Note that in presence of failures there's no guarantee the
// callback will be called. For example, the callback won't ever be called if
// the process crashes right after landing the transaction. Or if the
// transaction really landed, but ReadWriteTransaction finished with
// "deadline exceeded" (or some similar) error.
//
// Callbacks are executed sequentially in the reverse order they were deferred.
// They receive the non-transactional version of the context initially passed to
// ReadWriteTransaction.
//
// Panics if the given context is not transactional.
func Defer(ctx context.Context, cb func(context.Context)) {
	state := getTxnState(ctx)
	if state == nil {
		panic("not a transactional context")
	}
	state.deferCB(cb)
}

////////////////////////////////////////////////////////////////////////////////

var (
	clientContextKey = "go.chromium.org/luci/server/span:client"
	txnContextKey    = "go.chromium.org/luci/server/span:txn"
)

// client returns a Spanner client installed in the context.
//
// Panics if it is not there.
//
// Intentionally private to force all callers to go through package's functions
// like ReadWriteTransaction, ReadOnlyTransaction, Single, etc. since they
// generally add additional functionality on top of the raw Spanner client that
// other LUCI packages assume to be present. Using the Spanner client directly
// may violate such assumptions leading to undefined behavior when multiple
// packages are used together.
func client(ctx context.Context) *spanner.Client {
	cl, _ := ctx.Value(&clientContextKey).(*spanner.Client)
	if cl == nil {
		panic("no spanner Client in the context")
	}
	return cl
}

type txnState struct {
	rw *spanner.ReadWriteTransaction

	m   sync.Mutex
	cbs []func(context.Context)
}

func (s *txnState) deferCB(cb func(context.Context)) {
	s.m.Lock()
	s.cbs = append(s.cbs, cb)
	s.m.Unlock()
}

func (s *txnState) execCBs(ctx context.Context) {
	// Note: execCBs happens after ReadWriteTransaction has finished. If it
	// spawned any goroutines, they must have been finished already too (calling
	// Defer from a goroutine that outlives a transaction is rightfully a race).
	// Thus all writes to `s.cbs` are finished already and we also passed some
	// synchronization barrier that waited for the goroutines to join. It's fine
	// to avoid locking s.m in this case saving 200ns on hot code path.
	for i := len(s.cbs) - 1; i >= 0; i-- {
		s.cbs[i](ctx)
	}
}

func setTxnState(ctx context.Context, s *txnState) context.Context {
	return context.WithValue(ctx, &txnContextKey, s)
}

func getTxnState(ctx context.Context) *txnState {
	s, _ := ctx.Value(&txnContextKey).(*txnState)
	return s
}
