// Copyright 2019 The LUCI Authors.
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

package spanutil

import (
	"context"
	"reflect"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/proto"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
)

// ErrNoResults is an error returned when a query unexpectedly has no results.
var ErrNoResults = iterator.Done

// This file implements utility functions that make spanner API slightly easier
// to use.

// Txn is implemented by all spanner transactions.
type Txn interface {
	// ReadRow reads a single row from the database.
	ReadRow(ctx context.Context, table string, key spanner.Key, columns []string) (*spanner.Row, error)

	// Read reads multiple rows from the database.
	Read(ctx context.Context, table string, key spanner.KeySet, columns []string) *spanner.RowIterator

	// ReadOptions reads multiple rows from the database, and allows customizing
	// options.
	ReadWithOptions(ctx context.Context, table string, keys spanner.KeySet, columns []string, opts *spanner.ReadOptions) (ri *spanner.RowIterator)

	// Query reads multiple rows returned by SQL statement.
	Query(ctx context.Context, statement spanner.Statement) *spanner.RowIterator
}

func slices(m map[string]interface{}) (keys []string, values []interface{}) {
	keys = make([]string, 0, len(m))
	values = make([]interface{}, 0, len(m))
	for k, v := range m {
		keys = append(keys, k)
		values = append(values, v)
	}
	return
}

// ReadRow reads a single row from the database and reads its values.
// ptrMap must map from column names to pointers where the values will be
// written.
func ReadRow(ctx context.Context, txn Txn, table string, key spanner.Key, ptrMap map[string]interface{}) error {
	columns, ptrs := slices(ptrMap)
	row, err := txn.ReadRow(ctx, table, key, columns)
	if err != nil {
		return err
	}

	return FromSpanner(row, ptrs...)
}

// ReadWriteTransaction calls Client(ctx).ReadWriteTransaction and unwraps
// a "transaction is aborted" errors such that the spanner client properly
// retries the function.
func ReadWriteTransaction(ctx context.Context, f func(context.Context, *spanner.ReadWriteTransaction) error) (commitTimestamp time.Time, err error) {
	return Client(ctx).ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		err := f(ctx, txn)
		if unwrapped := errors.Unwrap(err); spanner.ErrCode(unwrapped) == codes.Aborted {
			err = unwrapped
		}
		return err
	})
}

// QueryFirstRow executes a query, reads the first row into ptrs and stops the
// iterator. Returns ErrNoResults if the query does not return at least one row.
func QueryFirstRow(ctx context.Context, txn Txn, st spanner.Statement, ptrs ...interface{}) error {
	st.Params = ToSpannerMap(st.Params)
	it := txn.Query(ctx, st)
	defer it.Stop()
	row, err := it.Next()
	if err != nil {
		return err
	}
	return FromSpanner(row, ptrs...)
}

// Query executes a query.
// Ensures st.Params are Spanner-compatible by modifying st.Params in place.
func Query(ctx context.Context, txn Txn, st spanner.Statement, fn func(row *spanner.Row) error) error {
	st.Params = ToSpannerMap(st.Params)
	return txn.Query(ctx, st).Do(fn)
}

func isMessageNil(m proto.Message) bool {
	return reflect.ValueOf(m).IsNil()
}
