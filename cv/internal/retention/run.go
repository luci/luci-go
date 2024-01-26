// Copyright 2024 The LUCI Authors.
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

package retention

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
)

var retentionPeriod = 540 * 24 * time.Hour // ~= 1.5 years

// wipeoutRun wipes out the given run if it is no longer in retention period.
//
// No-op if it doesn't exists or is still in the retention period.
func wipeoutRun(ctx context.Context, runID common.RunID) error {
	ctx = logging.SetField(ctx, "run", string(runID))
	r := &run.Run{ID: runID}
	switch err := datastore.Get(ctx, r); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		logging.Warningf(ctx, "run does not exist")
		return nil
	case err != nil:
		return errors.Annotate(err, "failed to load run").Tag(transient.Tag).Err()
	case !r.CreateTime.Before(clock.Now(ctx).Add(-retentionPeriod)):
		// skip if it is still in the retention period.
		logging.Warningf(ctx, "WipeoutRun: too young to wipe out: %s < %s",
			clock.Now(ctx).Sub(r.CreateTime), retentionPeriod)
		return nil
	}

	// Find out all the child entities of Run entities. As of Jan. 2024, this
	// includes:
	//  - RunLog
	//  - RunCL
	//  - TryjobExecutionState
	//  - TryjobExecutionLog
	runKey := datastore.KeyForObj(ctx, r)
	var toDelete []*datastore.Key
	q := datastore.NewQuery("").Ancestor(runKey).KeysOnly(true)
	if err := datastore.GetAll(ctx, q, &toDelete); err != nil {
		return errors.Annotate(err, "failed to query all child entities of run").Tag(transient.Tag).Err()
	}
	toDelete = append(toDelete, runKey)

	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		return datastore.Delete(ctx, toDelete)
	}, nil)

	if err != nil {
		return errors.Annotate(err, "failed to delete run entities and it's child entities in a transaction").Tag(transient.Tag).Err()
	}
	return nil
}
