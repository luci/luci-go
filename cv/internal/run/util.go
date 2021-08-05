// Copyright 2020 The LUCI Authors.
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

package run

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	commonpb "go.chromium.org/luci/cv/api/common/v1"
	"go.chromium.org/luci/cv/internal/common"
)

// IsEnded returns true if the given status is final.
func IsEnded(status commonpb.Run_Status) bool {
	return status&commonpb.Run_ENDED_MASK == commonpb.Run_ENDED_MASK
}

// LoadRunCLs loads `RunCL` entities of the provided cls in the Run.
func LoadRunCLs(ctx context.Context, runID common.RunID, clids common.CLIDs) ([]*RunCL, error) {
	runCLs := make([]*RunCL, len(clids))
	runKey := datastore.MakeKey(ctx, RunKind, string(runID))
	for i, clID := range clids {
		runCLs[i] = &RunCL{
			ID:  clID,
			Run: runKey,
		}
	}
	err := datastore.Get(ctx, runCLs)
	switch merr, ok := err.(errors.MultiError); {
	case ok:
		for i, err := range merr {
			if err == datastore.ErrNoSuchEntity {
				return nil, errors.Reason("RunCL %d not found in Datastore", runCLs[i].ID).Err()
			}
		}
		count, err := merr.Summary()
		return nil, errors.Annotate(err, "failed to load %d out of %d RunCLs", count, len(runCLs)).Tag(transient.Tag).Err()
	case err != nil:
		return nil, errors.Annotate(err, "failed to load %d RunCLs", len(runCLs)).Tag(transient.Tag).Err()
	}
	return runCLs, nil
}

// LoadRunLogEntries loads all log entries of a given Run.
//
// Ordered from logically oldest to newest.
func LoadRunLogEntries(ctx context.Context, runID common.RunID) ([]*LogEntry, error) {
	// Since RunLog entities are immutable, it's cheapest to load them from
	// DS cache. So, perform KeysOnly query first, which is cheap & fast, and then
	// additional multi-Get which will go via DS cache.

	var keys []*datastore.Key
	runKey := datastore.MakeKey(ctx, RunKind, string(runID))
	q := datastore.NewQuery(RunLogKind).KeysOnly(true).Ancestor(runKey)
	if err := datastore.GetAll(ctx, q, &keys); err != nil {
		return nil, errors.Annotate(err, "failed to fetch keys of RunLog entities").Tag(transient.Tag).Err()
	}

	entities := make([]*RunLog, len(keys))
	for i, key := range keys {
		entities[i] = &RunLog{
			Run: runKey,
			ID:  key.IntID(),
		}
	}
	if err := datastore.Get(ctx, entities); err != nil {
		// It's possible to get EntityNotExists, it may only happen if data
		// retention enforcement is deleting old entities at the same time.
		// Thus, treat all errors as transient.
		return nil, errors.Annotate(common.MostSevereError(err), "failed to fetch RunLog entities").Tag(transient.Tag).Err()
	}

	// Each RunLog entity contains at least 1 LogEntry.
	out := make([]*LogEntry, 0, len(entities))
	for _, e := range entities {
		out = append(out, e.Entries.GetEntries()...)
	}
	return out, nil
}
