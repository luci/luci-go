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

package dsmapper

import (
	"context"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/server/dsmapper/dsmapperpb"
)

// runTxn runs a datastore transaction retrying the body on transient errors or
// when encountering a commit conflict.
func runTxn(ctx context.Context, cb func(context.Context) error) error {
	var attempt int
	var innerErr error

	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		attempt++
		if attempt != 1 {
			if innerErr != nil {
				logging.Warningf(ctx, "Retrying the transaction after the error: %s", innerErr)
			} else {
				logging.Warningf(ctx, "Retrying the transaction: failed to commit")
			}
		}
		innerErr = cb(ctx)
		if transient.Tag.In(innerErr) {
			return datastore.ErrConcurrentTransaction // causes a retry
		}
		return innerErr
	}, nil)

	if err != nil {
		logging.WithError(err).Errorf(ctx, "Transaction failed")
		if innerErr != nil {
			return innerErr
		}
		// Here it can only be a commit error (i.e. produced by RunInTransaction
		// itself, not by its callback). We treat them as transient.
		return transient.Tag.Apply(err)
	}

	return nil
}

func isFinalState(s dsmapperpb.State) bool {
	return s == dsmapperpb.State_SUCCESS || s == dsmapperpb.State_FAIL || s == dsmapperpb.State_ABORTED
}
