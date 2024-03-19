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
	sppb "cloud.google.com/go/spanner/apiv1/spannerpb"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
)

// Transaction is a common interface of spanner.ReadOnlyTransaction and
// spanner.ReadWriteTransaction.
type Transaction interface {
	// ReadRow reads a single row from the database.
	ReadRow(ctx context.Context, table string, key spanner.Key, columns []string) (*spanner.Row, error)

	// ReadRowWithOptions reads a single row from the database, and allows customizing options.
	ReadRowWithOptions(ctx context.Context, table string, key spanner.Key, columns []string, opts *spanner.ReadOptions) (*spanner.Row, error)

	// Read reads multiple rows from the database.
	Read(ctx context.Context, table string, keys spanner.KeySet, columns []string) *spanner.RowIterator

	// ReadWithOptions reads multiple rows from the database, and allows
	// customizing options.
	ReadWithOptions(ctx context.Context, table string, keys spanner.KeySet, columns []string, opts *spanner.ReadOptions) *spanner.RowIterator

	// Query reads multiple rows returned by SQL statement.
	Query(ctx context.Context, statement spanner.Statement) *spanner.RowIterator

	// QueryWithOptions reads multiple rows returned by SQL statement, and allows
	// customizing options.
	QueryWithOptions(ctx context.Context, statement spanner.Statement, opts spanner.QueryOptions) *spanner.RowIterator
}

// UseClient installs a Spanner client into the context.
//
// Primarily used by the module initialization code. May be useful in tests as
// well.
func UseClient(ctx context.Context, client *spanner.Client) context.Context {
	return context.WithValue(ctx, &clientContextKey, client)
}

// Apply applies a list of mutations atomically to the database.
//
// Panics if called from inside a read-write transaction. Use BufferWrite to
// apply mutations there.
func Apply(ctx context.Context, ms []*spanner.Mutation, opts ...spanner.ApplyOption) (commitTimestamp time.Time, err error) {
	panicOnNestedRW(ctx)
	if ctxOpts, ok := ctx.Value(&requestOptionsContextKey).(*RequestOptions); ok {
		opts = append([]spanner.ApplyOption{spanner.Priority(ctxOpts.Priority)}, opts...)
	}
	return client(ctx).Apply(ctx, ms, opts...)
}

// PartitionedUpdate executes a DML statement in parallel across the database,
// using separate, internal transactions that commit independently.
//
// Panics if called from inside a read-write transaction.
func PartitionedUpdate(ctx context.Context, st spanner.Statement) (count int64, err error) {
	panicOnNestedRW(ctx)
	return client(ctx).PartitionedUpdateWithOptions(ctx, st, queryOptionsFromContext(ctx))
}

// Single returns a derived context with a single-use read-only transaction.
//
// It provides a read-only snapshot transaction optimized for the case where
// only a single read or query is needed. This is more efficient than using
// ReadOnlyTransaction() for a single read or query.
//
// The transaction object can be obtained through RO(ctx) or Txn(ctx). It is
// also transparently used by ReadRow, Read, Query, etc. wrappers.
//
// Panics if `ctx` already holds a transaction (either read-write or read-only).
func Single(ctx context.Context) context.Context {
	panicOnNestedRO(ctx)
	return setTxnState(ctx, &txnState{ro: client(ctx).Single()})
}

// ReadOnlyTransaction returns a derived context with a read-only transaction.
//
// It can be used for multiple reads from the database. To avoid leaking
// resources on the server this context *must* be canceled as soon as all reads
// are done.
//
// The transaction object can be obtained through RO(ctx) or Txn(ctx). It is
// also transparently used by ReadRow, Read, Query, etc. wrappers.
//
// Panics if `ctx` already holds a transaction (either read-write or read-only).
func ReadOnlyTransaction(ctx context.Context) (context.Context, context.CancelFunc) {
	panicOnNestedRO(ctx)
	txn := client(ctx).ReadOnlyTransaction()
	ctx, cancel := context.WithCancel(setTxnState(ctx, &txnState{ro: txn}))
	return ctx, func() { cancel(); txn.Close() }
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
// The callback can access the transaction object via RW(ctx) or Txn(ctx). It is
// also transparently used by ReadRow, Read, Query, BufferWrite, etc. wrappers.
//
// Panics if `ctx` already holds a read-write transaction. Starting a read-write
// transaction from a read-only transaction is OK though, but beware that they
// are completely separate unrelated transactions.
func ReadWriteTransaction(ctx context.Context, f func(ctx context.Context) error) (commitTimestamp time.Time, err error) {
	panicOnNestedRW(ctx)

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

// RO returns the current read-only transaction in the context or nil if it's
// not a read-only transactional context.
func RO(ctx context.Context) *spanner.ReadOnlyTransaction {
	if s := getTxnState(ctx); s != nil {
		return s.ro
	}
	return nil
}

// MustRO is like RO except it panics if `ctx` is not read-only transactional.
func MustRO(ctx context.Context) *spanner.ReadOnlyTransaction {
	if ro := RO(ctx); ro != nil {
		return ro
	}
	panic("not a read-only Spanner transactional context")
}

// RW returns the current read-write transaction in the context or nil if it's
// not a read-write transactional context.
func RW(ctx context.Context) *spanner.ReadWriteTransaction {
	if s := getTxnState(ctx); s != nil {
		return s.rw
	}
	return nil
}

// MustRW is like RW except it panics if `ctx` is not read-write transactional.
func MustRW(ctx context.Context) *spanner.ReadWriteTransaction {
	if rw := RW(ctx); rw != nil {
		return rw
	}
	panic("not a read-write Spanner transactional context")
}

// Txn returns an interface that can be used to read data in the current
// read-only or read-write transaction.
//
// Returns nil if `ctx` is not a transactional context.
func Txn(ctx context.Context) Transaction {
	switch s := getTxnState(ctx); {
	case s == nil:
		return nil
	case s.ro != nil:
		return s.ro
	default:
		return s.rw
	}
}

// MustTxn is like Txn except it panics if `ctx` is not transactional.
func MustTxn(ctx context.Context) Transaction {
	if txn := Txn(ctx); txn != nil {
		return txn
	}
	panic("not a transactional Spanner context")
}

// WithoutTxn returns a copy of the context without the transaction in it.
//
// This can be used to spawn separate independent transactions from within
// a transaction.
func WithoutTxn(ctx context.Context) context.Context {
	if getTxnState(ctx) == nil {
		return ctx
	}
	return setTxnState(ctx, nil)
}

// Defer schedules `cb` for execution when the current read-write transaction
// successfully lands.
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
	if state == nil || state.rw == nil {
		panic("not a read-write Spanner transactional context")
	}
	state.deferCB(cb)
}

// ReadRow reads a single row from the database.
//
// It is a shortcut for MustTxn(ctx).ReadRow(ctx, ...). Panics if the context
// is not transactional.
func ReadRow(ctx context.Context, table string, key spanner.Key, columns []string) (*spanner.Row, error) {
	return MustTxn(ctx).ReadRowWithOptions(ctx, table, key, columns, readOptionsFromContext(ctx))
}

// ReadRowWithOptions reads a single row from the database, and allows customizing
// options.
//
// It is a shortcut for MustTxn(ctx).ReadRowWithOptions(ctx, ...). Panics if
// the context is not transactional.
//
// It does not use the default RequestOptions from the ctx.  Use opts to pass
// these explicitly.
func ReadRowWithOptions(ctx context.Context, table string, key spanner.Key, columns []string, opts *spanner.ReadOptions) (*spanner.Row, error) {
	return MustTxn(ctx).ReadRowWithOptions(ctx, table, key, columns, opts)
}

// Read reads multiple rows from the database.
//
// It is a shortcut for MustTxn(ctx).Read(ctx, ...). Panics if the context
// is not transactional.
func Read(ctx context.Context, table string, keys spanner.KeySet, columns []string) *spanner.RowIterator {
	return MustTxn(ctx).ReadWithOptions(ctx, table, keys, columns, readOptionsFromContext(ctx))
}

// ReadWithOptions reads multiple rows from the database, and allows customizing
// options.
//
// It is a shortcut for MustTxn(ctx).ReadWithOptions(ctx, ...). Panics if the
// context is not transactional.
//
// It does not use the default RequestOptions from the ctx.  Use opts to pass
// these explicitly.
func ReadWithOptions(ctx context.Context, table string, keys spanner.KeySet, columns []string, opts *spanner.ReadOptions) *spanner.RowIterator {
	return MustTxn(ctx).ReadWithOptions(ctx, table, keys, columns, opts)
}

// Query reads multiple rows returned by SQL statement.
//
// It is a shortcut for MustTxn(ctx).Query(ctx, ...). Panics if the context is
// not transactional.
func Query(ctx context.Context, statement spanner.Statement) *spanner.RowIterator {
	return MustTxn(ctx).QueryWithOptions(ctx, statement, queryOptionsFromContext(ctx))
}

// BufferWrite adds a list of mutations to the set of updates that will be
// applied when the transaction is committed.
//
// It does not actually apply the write until the transaction is committed, so
// the operation does not block. The effects of the write won't be visible to
// any reads (including reads done in the same transaction) until the
// transaction commits.
//
// It is a wrapper over MustRW(ctx).BufferWrite(...). Panics if the context is
// not read-write transactional.
func BufferWrite(ctx context.Context, ms ...*spanner.Mutation) {
	// BufferWrite just appends mutation to an internal buffer. It fails only if
	// the transaction has already landed. We are OK to panic in this case:
	// calling BufferWrite outside of a transaction is a programming error.
	if err := MustRW(ctx).BufferWrite(ms); err != nil {
		panic(err)
	}
}

// Update executes a DML statement against the database. It returns the number
// of affected rows. Update returns an error if the statement is a query.
// However, the query is executed, and any data read will be validated upon
// commit.
//
// It is a shortcut for MustRW(ctx).Update(...). Panics if the context is not
// read-write transactional.
func Update(ctx context.Context, stmt spanner.Statement) (rowCount int64, err error) {
	return MustRW(ctx).UpdateWithOptions(ctx, stmt, queryOptionsFromContext(ctx))
}

// UpdateWithOptions executes a DML statement against the database. It returns
// the number of affected rows. The sql query execution will be optimized based
// on the given query options.
//
// It is a shortcut for MustRW(ctx).Update(...). Panics if the context is not
// read-write transactional.
//
// It does not use the default RequestOptions from the ctx.  Use opts to pass
// these explicitly.
func UpdateWithOptions(ctx context.Context, stmt spanner.Statement, opts spanner.QueryOptions) (rowCount int64, err error) {
	return MustRW(ctx).UpdateWithOptions(ctx, stmt, opts)
}

// BatchWrite applies a list of mutation groups in a collection of efficient
// transactions.
//
// See https://pkg.go.dev/cloud.google.com/go/spanner#Client.BatchWrite for
// details.
//
// Must be used outside of any transactions since it launches transactions
// itself and nested transactions are not supported. Panics if the context is
// transactional.
func BatchWrite(ctx context.Context, mgs []*spanner.MutationGroup) *spanner.BatchWriteResponseIterator {
	return BatchWriteWithOptions(ctx, mgs, batchWriteOptionsFromContext(ctx))
}

// BatchWriteWithOptions applies a list of mutation groups in a collection of
// efficient transactions.
//
// See https://pkg.go.dev/cloud.google.com/go/spanner#Client.BatchWrite for
// details.
//
// Must be used outside of any transactions since it launches transactions
// itself and nested transactions are not supported. Panics if the context is
// transactional.
func BatchWriteWithOptions(ctx context.Context, mgs []*spanner.MutationGroup, opts spanner.BatchWriteOptions) *spanner.BatchWriteResponseIterator {
	if getTxnState(ctx) != nil {
		panic("BatchWrite cannot be used in a transaction: it launches transactions itself")
	}
	return client(ctx).BatchWriteWithOptions(ctx, mgs, opts)
}

////////////////////////////////////////////////////////////////////////////////

var (
	clientContextKey         = "go.chromium.org/luci/server/span:client"
	txnContextKey            = "go.chromium.org/luci/server/span:txn"
	requestOptionsContextKey = "go.chromium.org/luci/server/span:requestOptions"
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
		panic("no Spanner client in the context")
	}
	return cl
}

type txnState struct {
	ro *spanner.ReadOnlyTransaction
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

func panicOnNestedRW(ctx context.Context) {
	if RW(ctx) != nil {
		panic("nested Spanner write transactions are not allowed")
	}
}

func panicOnNestedRO(ctx context.Context) {
	if getTxnState(ctx) != nil {
		panic("nested Spanner read transactions are not allowed")
	}
}

// RequestOptions holds options common to many Spanner requests.
//
// Used for setting default request options in the context via
// ModifyRequestOptions.  See also spanner.ReadOptions also
// spanner.QueryOptions.
type RequestOptions struct {
	Tag      string
	Priority sppb.RequestOptions_Priority
}

// ModifyRequestOptions returns a new Context that carries default Spanner
// request options.
//
// These request options will be used by all Spanner operations in this module
// if the underlying operation supports it, except the *WithOptions functions
// (where the caller provides options explicitly).
//
// The cb function will be called with the current defaults from the context,
// to allow for incremental updates.
func ModifyRequestOptions(ctx context.Context, cb func(*RequestOptions)) context.Context {
	var next RequestOptions
	if cur, ok := ctx.Value(&requestOptionsContextKey).(*RequestOptions); ok {
		next = *cur
	}
	cb(&next)
	return context.WithValue(ctx, &requestOptionsContextKey, &next)
}

func queryOptionsFromContext(ctx context.Context) spanner.QueryOptions {
	opts, ok := ctx.Value(&requestOptionsContextKey).(*RequestOptions)
	if !ok {
		return spanner.QueryOptions{}
	}
	return spanner.QueryOptions{
		Priority:   opts.Priority,
		RequestTag: opts.Tag,
	}
}

func readOptionsFromContext(ctx context.Context) *spanner.ReadOptions {
	opts, ok := ctx.Value(&requestOptionsContextKey).(*RequestOptions)
	if !ok {
		return &spanner.ReadOptions{}
	}
	return &spanner.ReadOptions{
		Priority:   opts.Priority,
		RequestTag: opts.Tag,
	}
}

func batchWriteOptionsFromContext(ctx context.Context) spanner.BatchWriteOptions {
	opts, ok := ctx.Value(&requestOptionsContextKey).(*RequestOptions)
	if !ok {
		return spanner.BatchWriteOptions{}
	}
	return spanner.BatchWriteOptions{
		Priority:       opts.Priority,
		TransactionTag: opts.Tag,
	}
}
