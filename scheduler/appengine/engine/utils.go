// Copyright 2017 The LUCI Authors.
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

package engine

import (
	"fmt"

	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
)

// debugLog mutates a string by appending a line to it.
func debugLog(c context.Context, str *string, format string, args ...interface{}) {
	prefix := clock.Now(c).UTC().Format("[15:04:05.000] ")
	*str += prefix + fmt.Sprintf(format+"\n", args...)
}

// defaultTransactionOptions is used for all transactions.
//
// Almost all transactions done by the scheduler service happen in background
// task queues, it is fine to retry more there.
var defaultTransactionOptions = datastore.TransactionOptions{
	Attempts: 10,
}

// abortTransaction makes the error abort the transaction (even if it is marked
// as transient).
//
// See runTxn for more info. This is used primarily by errUpdateConflict.
var abortTransaction = errors.BoolTag{Key: errors.NewTagKey("this error aborts the transaction")}

// runTxn runs a datastore transaction retrying the body on transient errors or
// when encountering a commit conflict.
//
// It will NOT retry errors (even if transient) marked with abortTransaction
// tag. This is primarily used to tag errors that are transient at a level
// higher than the transaction: errors marked with both transient.Tag and
// abortTransaction are not retried by runTxn, but may be retried by something
// on top (like Task Queue).
func runTxn(c context.Context, cb func(context.Context) error) error {
	var attempt int
	var innerErr error

	err := datastore.RunInTransaction(c, func(c context.Context) error {
		attempt++
		if attempt != 1 {
			if innerErr != nil {
				logging.Warningf(c, "Retrying the transaction after the error: %s", innerErr)
			} else {
				logging.Warningf(c, "Retrying the transaction: failed to commit")
			}
		}
		innerErr = cb(c)
		if transient.Tag.In(innerErr) && !abortTransaction.In(innerErr) {
			return datastore.ErrConcurrentTransaction // causes a retry
		}
		return innerErr
	}, &defaultTransactionOptions)

	if err != nil {
		logging.WithError(err).Errorf(c, "Transaction failed")
		if innerErr != nil {
			return innerErr
		}
		// Here it can only be a commit error (i.e. produced by RunInTransaction
		// itself, not by its callback). We treat them as transient.
		return transient.Tag.Apply(err)
	}

	return nil
}

// equalSortedLists returns true if lists contain the same sequence of strings.
func equalSortedLists(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i, s := range a {
		if s != b[i] {
			return false
		}
	}
	return true
}
