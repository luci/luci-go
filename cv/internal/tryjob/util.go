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
	"slices"
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
				return nil, errors.Fmt("tryjob %d not found in datastore", ids[i])
			}
		}
		count, err := merr.Summary()
		return nil, transient.Tag.Apply(errors.Fmt("failed to load %d out of %d tryjobs: %w", count, len(ids), err))
	default:
		return nil, transient.Tag.Apply(errors.Fmt("failed to load tryjobs: %w", err))
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
	for shard := range retentionKeyShards {
		eg.Go(func() error {
			q := datastore.NewQuery(TryjobKind).
				Lt("RetentionKey", fmt.Sprintf("%02d/%010d", shard, before.Unix())).
				Gt("RetentionKey", fmt.Sprintf("%02d/", shard)).
				KeysOnly(true)
			var keys []*datastore.Key
			switch err := datastore.GetAll(ectx, q, &keys); {
			case err != nil:
				return transient.Tag.Apply(errors.Fmt("failed to query Tryjob keys: %w", err))
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
	slices.Sort(ret)
	return ret, nil
}

// IsEnded returns true if the given Tryjob status is final.
func IsEnded(status Status) bool {
	switch status {
	case Status_STATUS_UNSPECIFIED, Status_PENDING, Status_TRIGGERED:
		return false
	default:
		return true
	}
}
