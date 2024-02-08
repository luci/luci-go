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

package tryjob

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
)

// LoadTryjobsByIDs loads the tryjobs with the given ids from datastore.
func LoadTryjobsByIDs(ctx context.Context, ids []common.TryjobID) ([]*Tryjob, error) {
	tryjobs := make([]*Tryjob, len(ids))
	for i, tjid := range ids {
		tryjobs[i] = &Tryjob{ID: tjid}
	}
	err := datastore.Get(ctx, tryjobs)
	switch merr, ok := err.(errors.MultiError); {
	case err == nil:
		return tryjobs, nil
	case ok:
		for i, err := range merr {
			if err == datastore.ErrNoSuchEntity {
				return nil, errors.Reason("tryjob %d not found in datastore", ids[i]).Err()
			}
		}
		count, err := merr.Summary()
		return nil, errors.Annotate(err, "failed to load %d out of %d tryjobs", count, len(ids)).Tag(transient.Tag).Err()
	default:
		return nil, errors.Annotate(err, "failed to load tryjobs").Tag(transient.Tag).Err()
	}
}

// LoadTryjobsMapByIDs get a map of tryjobs with the given ids from datastore.
func LoadTryjobsMapByIDs(ctx context.Context, ids []common.TryjobID) (map[common.TryjobID]*Tryjob, error) {
	switch tryjobs, err := LoadTryjobsByIDs(ctx, ids); {
	case err != nil:
		return nil, err
	default:
		tryjobMap := make(map[common.TryjobID]*Tryjob, len(ids))
		for _, tj := range tryjobs {
			tryjobMap[tj.ID] = tj
		}
		return tryjobMap, nil
	}
}

// NotifyTryjobsUpdatedFn is used to notify Run about the updated Tryjobs.
type NotifyTryjobsUpdatedFn func(context.Context, common.RunID, *TryjobUpdatedEvents) error

// SaveTryjobs saves Tryjobs to datastore and notifies the interested Runs.
//
// Assigns a new internal ID if not given. If an external ID is given, the
// function saves a mapping from it to this new internal ID.
//
// Note that if an external ID is given, it must not already map to another
// Tryjob, or this function will fail.
//
// If the provided `notifyFn` is nil, notify nothing.
//
// MUST be called in datastore transaction context.
//
// TODO(yiwzhang): inspect the code and avoid unnecessary notification. Right
// now, for simplicity, notify all Runs about all Tryjobs. The Run manager
// will be waken up and realize there's nothing to do.
func SaveTryjobs(ctx context.Context, tryjobs []*Tryjob, notifyFn NotifyTryjobsUpdatedFn) error {
	if datastore.CurrentTransaction(ctx) == nil {
		panic(fmt.Errorf("SaveTryjobs must be called in a transaction"))
	}

	tryjobsWithExternalID := filterTryjobsWithExternalID(tryjobs)
	if len(tryjobsWithExternalID) == 0 {
		// early return for easy cases
		if err := datastore.Put(ctx, tryjobs); err != nil {
			return errors.Annotate(err, "failed to save Tryjobs").Tag(transient.Tag).Err()
		}
		return notifyRuns(ctx, tryjobs, notifyFn)
	}

	eids := make([]ExternalID, len(tryjobsWithExternalID))
	for i, tj := range tryjobsWithExternalID {
		eids[i] = tj.ExternalID
	}
	ids, err := Resolve(ctx, eids...)
	if err != nil {
		return err
	}

	// Allocate IDs for Tryjobs missing internal ID and figure out all
	// new mapping that need to be stored.
	var tryjobsToAllocateID []*Tryjob
	var tjms []*tryjobMap
	for i, existingID := range ids {
		switch tj := tryjobsWithExternalID[i]; {
		case tj.ID == 0 && existingID == 0:
			tryjobsToAllocateID = append(tryjobsToAllocateID, tj)
		case tj.ID == 0:
			return errors.Reason("external Tryjob id %q has already mapped to internal id %d", tj.ExternalID, existingID).Err()
		case existingID == 0:
			tjms = append(tjms, &tryjobMap{
				ExternalID: tj.ExternalID,
				InternalID: tj.ID,
			})
		case existingID != tj.ID:
			return errors.Reason("external Tryjob id %q has already mapped to internal id %d; got internal id %d", tj.ExternalID, existingID, tj.ID).Err()
		}
	}
	if len(tryjobsToAllocateID) > 0 {
		if err := datastore.AllocateIDs(ctx, tryjobsToAllocateID); err != nil {
			return errors.Annotate(err, "allocating Tryjob ids").Tag(transient.Tag).Err()
		}
		for _, tj := range tryjobsToAllocateID {
			tjms = append(tjms, &tryjobMap{
				ExternalID: tj.ExternalID,
				InternalID: tj.ID,
			})
		}
	}

	// Store Tryjobs and new mappings if any and notify interested Runs.
	if len(tjms) > 0 {
		err = datastore.Put(ctx, tryjobs, tjms)
	} else {
		err = datastore.Put(ctx, tryjobs)
	}
	if err != nil {
		return errors.Annotate(err, "saving Tryjobs and TryjobMaps").Tag(transient.Tag).Err()
	}
	return notifyRuns(ctx, tryjobs, notifyFn)
}

func filterTryjobsWithExternalID(tryjobs []*Tryjob) []*Tryjob {
	var ret []*Tryjob
	for _, tj := range tryjobs {
		if tj.ExternalID != "" {
			ret = append(ret, tj)
		}
	}
	return ret
}

// notifyRuns notifies the Runs interested in the given Tryjobs.
func notifyRuns(ctx context.Context, tryjobs []*Tryjob, notifyFn NotifyTryjobsUpdatedFn) error {
	if notifyFn == nil {
		return nil
	}
	notifyTargets := make(map[common.RunID]common.TryjobIDs)
	for _, tj := range tryjobs {
		for _, runID := range tj.AllWatchingRuns() {
			if _, ok := notifyTargets[runID]; !ok {
				notifyTargets[runID] = common.TryjobIDs{tj.ID}
				continue
			}
			notifyTargets[runID] = append(notifyTargets[runID], tj.ID)
		}
	}
	for runID, tjIDs := range notifyTargets {
		events := &TryjobUpdatedEvents{
			Events: make([]*TryjobUpdatedEvent, len(tjIDs)),
		}
		for i, tjID := range tjIDs {
			events.Events[i] = &TryjobUpdatedEvent{
				TryjobId: int64(tjID),
			}
		}
		if err := notifyFn(ctx, runID, events); err != nil {
			return errors.Annotate(err, "failed to notify Tryjobs updated for run %s", runID).Tag(transient.Tag).Err()
		}
	}
	return nil
}

// LatestAttempt returns the latest attempt of the given Tryjob Execution.
//
// Returns nil if the Tryjob execution doesn't have any attempt yet.
func LatestAttempt(execution *ExecutionState_Execution) *ExecutionState_Execution_Attempt {
	switch l := len(execution.GetAttempts()); l {
	case 0:
		return nil
	default:
		return execution.GetAttempts()[l-1]
	}
}

// QueryTryjobIDsUpdatedBefore queries ID of all tryjobs updated before the
// given timestamp.
//
// This is used for data retention purpose. Results are sorted.
func QueryTryjobIDsUpdatedBefore(ctx context.Context, before time.Time) (common.TryjobIDs, error) {
	var ret common.TryjobIDs
	var retMu sync.Mutex
	eg, ectx := errgroup.WithContext(ctx)
	eg.SetLimit(10)
	for shard := 0; shard < retentionKeyShards; shard++ {
		shard := shard
		eg.Go(func() error {
			q := datastore.NewQuery(TryjobKind).
				Lt("RetentionKey", fmt.Sprintf("%02d/%010d", shard, before.Unix())).
				Gt("RetentionKey", fmt.Sprintf("%02d/", shard)).
				KeysOnly(true)
			var keys []*datastore.Key
			switch err := datastore.GetAll(ectx, q, &keys); {
			case err != nil:
				return errors.Annotate(err, "failed to query Tryjob keys").Tag(transient.Tag).Err()
			case len(keys) > 0:
				retMu.Lock()
				for _, key := range keys {
					ret = append(ret, common.TryjobID(key.IntID()))
				}
				retMu.Unlock()
			}
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return nil, err
	}
	sort.Sort(ret)
	return ret, nil
}
