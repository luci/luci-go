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

package spanner

import (
	"time"

	gspan "cloud.google.com/go/spanner"
	"golang.org/x/net/context"
)

// WriteRows writes the given rows in a single read-write transaction, returning commit timestamp.
// If any row fails to translate to the corresponding mutations, the entire slice fails.
func WriteRows(ctx context.Context, client *gspan.Client, rows []Row) (time.Time, error) {
	return client.ReadWriteTransaction(ctx, func (ctx context.Context, txn *gspan.ReadWriteTransaction) error {
		var muts []*gspan.Mutation

		for _, row := range rows {
			rowMuts, err := row.GetWriteMutations(ctx, txn)
			if err != nil {
				return err
			}
			muts = append(muts, rowMuts...)
		}

		txn.BufferWrite(muts)
		return nil
	})
}
