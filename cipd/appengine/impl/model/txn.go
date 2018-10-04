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

package model

import (
	"context"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
)

// Txn runs the callback in a datastore transaction, marking commit errors with
// transient tag.
//
// The given name will be used for logging and error messages.
func Txn(c context.Context, name string, cb func(context.Context) error) error {
	c = logging.SetField(c, "txn", name)

	var attempt int
	var innerErr error

	err := datastore.RunInTransaction(c, func(c context.Context) error {
		attempt++
		if attempt != 1 {
			logging.Warningf(c, "Retrying the transaction: failed to commit")
		}
		innerErr = cb(c)
		return innerErr
	}, nil)

	if err != innerErr {
		return errors.Annotate(err, "failed to land %s transaction", name).Tag(transient.Tag).Err()
	}
	return innerErr
}
