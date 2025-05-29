// Copyright 2022 The LUCI Authors.
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
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/deploy/api/modelpb"
)

// ExpireActuations moves expired actuations into EXPIRED state.
func ExpireActuations(ctx context.Context) error {
	ctx, done := clock.WithTimeout(ctx, time.Minute)
	defer done()

	q := datastore.NewQuery("Actuation").
		Eq("State", modelpb.Actuation_EXECUTING).
		Lt("Expiry", clock.Now(ctx).UTC()).
		KeysOnly(true)

	err := parallel.WorkPool(16, func(work chan<- func() error) {
		err := datastore.Run(ctx, q, func(key *datastore.Key) {
			work <- func() error {
				ctx := logging.SetField(ctx, "actuation", key.StringID())
				logging.Infof(ctx, "Expiring")
				if err := expireActuation(ctx, key.StringID()); err != nil {
					logging.Errorf(ctx, "Failed: %s", err)
				}
				return nil
			}
		})
		if err != nil {
			logging.Errorf(ctx, "Datastore query failed: %s", err)
			work <- func() error { return err }
		}
	})
	return transient.Tag.Apply(err)
}

func expireActuation(ctx context.Context, actuationID string) error {
	return Txn(ctx, func(ctx context.Context) error {
		a := Actuation{ID: actuationID}
		if err := datastore.Get(ctx, &a); err != nil {
			return errors.Fmt("fetching Actuation entity: %w", err)
		}
		if a.State == modelpb.Actuation_EXECUTING {
			op, err := NewActuationEndOp(ctx, &a)
			if err != nil {
				return errors.Fmt("starting Actuation end op: %w", err)
			}
			op.Expire(ctx)
			if err := op.Apply(ctx); err != nil {
				return errors.Fmt("applying changes: %w", err)
			}
		}
		return nil
	})
}
