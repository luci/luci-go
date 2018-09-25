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

package mapper

import (
	"golang.org/x/net/context"

	"go.chromium.org/gae/service/datastore"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
)

// runTxn runs a datastore transaction retrying the body on transient errors or
// when encountering a commit conflict.
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
		if transient.Tag.In(innerErr) {
			return datastore.ErrConcurrentTransaction // causes a retry
		}
		return innerErr
	}, nil)

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
