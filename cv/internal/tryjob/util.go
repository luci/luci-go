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

// SaveTryjobs saves given tryjobs to datastore.
//
// Assigns a new internal ID if not given. If an external ID is given, the
// function saves a mapping from it to this new internal ID.
//
// Note that if an external ID is given, it must not already map to another
// tryjob, or this function will fail.
func SaveTryjobs(ctx context.Context, tryjobs []*Tryjob) error {
	tryjobsWithExternalID := filterTryjobWithExternalID(tryjobs)

	if len(tryjobsWithExternalID) == 0 { // early return for easy cases
		if err := datastore.Put(ctx, tryjobs); err != nil {
			return errors.Annotate(err, "failed to save Tryjobs").Tag(transient.Tag).Err()
		}
	}
	saveFn := func(ctx context.Context) error {
		eids := make([]ExternalID, len(tryjobsWithExternalID))
		for i, tj := range tryjobsWithExternalID {
			eids[i] = tj.ExternalID
		}
		ids, err := Resolve(ctx, eids...)
		if err != nil {
			return err
		}

		var tryjobsToAllocateID []*Tryjob
		var tjms []*tryjobMap
		for i, existingID := range ids {
			switch tj := tryjobsWithExternalID[i]; {
			case tj.ID == 0 && existingID == 0:
				tryjobsToAllocateID = append(tryjobsToAllocateID, tj)
			case tj.ID == 0:
				return errors.Reason("external tryjob id %q has already mapped to internal id %d", tj.ExternalID, existingID).Err()
			case existingID == 0:
				tjms = append(tjms, &tryjobMap{
					ExternalID: tj.ExternalID,
					InternalID: tj.ID,
				})
			case existingID != tj.ID:
				return errors.Reason("external tryjob id %q has already mapped to internal id %d; got internal id %d", tj.ExternalID, existingID, tj.ID).Err()
			}
		}
		if len(tryjobsToAllocateID) > 0 {
			if err := datastore.AllocateIDs(ctx, tryjobsToAllocateID); err != nil {
				return errors.Annotate(err, "allocating tryjob ids").Tag(transient.Tag).Err()
			}
			for _, tj := range tryjobsToAllocateID {
				tjms = append(tjms, &tryjobMap{
					ExternalID: tj.ExternalID,
					InternalID: tj.ID,
				})
			}
		}

		if len(tjms) > 0 {
			err = datastore.Put(ctx, tryjobs, tjms)
		} else {
			err = datastore.Put(ctx, tryjobs)
		}
		if err != nil {
			return errors.Annotate(err, "saving tryjobs and tryjobMaps").Tag(transient.Tag).Err()
		}
		return nil
	}

	if datastore.CurrentTransaction(ctx) == nil {
		return datastore.RunInTransaction(ctx, saveFn, nil)
	}
	return saveFn(ctx)
}

func filterTryjobWithExternalID(tryjobs []*Tryjob) []*Tryjob {
	var ret []*Tryjob
	for _, tj := range tryjobs {
		if tj.ExternalID != "" {
			ret = append(ret, tj)
		}
	}
	return ret
}
