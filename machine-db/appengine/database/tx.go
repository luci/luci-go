// Copyright 2018 The LUCI Authors.
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

package database

import (
	"database/sql"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/errors"
)

// RollbackTx is a sql.Tx which supports automatic rollback.
// defer a call to MaybeRollback(), then call (*RollbackTx).Commit() instead of
// (*sql.Tx).Commit() to avoid having the transaction rolled back by the defer.
type RollbackTx struct {
	*sql.Tx
	committed bool
}

// Commit commits the transaction.
func (r *RollbackTx) Commit() error {
	if err := r.Tx.Commit(); err != nil {
		return err
	}
	r.committed = true
	return nil
}

// MaybeRollback rolls back the transaction if (*RollbackTx).Commit() hasn't been called.
// Logs the error if the rollback itself fails.
func (r *RollbackTx) MaybeRollback(c context.Context) error {
	if !r.committed {
		if err := r.Tx.Rollback(); err != nil {
			errors.Log(c, errors.Annotate(err, "failed to roll back transaction").Err())
			return err
		}
	}
	return nil
}
