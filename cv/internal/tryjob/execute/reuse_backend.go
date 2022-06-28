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

package execute

import (
	"context"
	"fmt"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/tryjob"
)

func (w *worker) findReuseInBackend(ctx context.Context, definitions []*tryjob.Definition) (map[*tryjob.Definition]*tryjob.Tryjob, error) {
	candidates := make(map[*tryjob.Definition]*tryjob.Tryjob, len(definitions))
	err := w.backend.Search(ctx, w.cls, definitions, w.run.ID.LUCIProject(), func(tj *tryjob.Tryjob) bool {
		switch candidate, ok := candidates[tj.Definition]; {
		case ok:
			// matching Tryjob already found.
			if tj.Result.GetCreateTime().AsTime().After(candidate.Result.GetCreateTime().AsTime()) {
				panic(fmt.Errorf("backend.Search should return tryjob from newest to oldest; However, got %s before %s", candidate.Result, tj.Result))
			}
		case w.knownExternalIDs.Has(string(tj.ExternalID)):
		case canReuseTryjob(ctx, tj, w.run.Mode) == reuseDenied:
		default:
			candidates[tj.Definition] = tj
		}
		return len(candidates) < len(definitions)
	})
	switch {
	case err != nil:
		return nil, err
	case len(candidates) == 0:
		return nil, nil
	}

	var innerErr error
	var tryjobs []*tryjob.Tryjob
	err = datastore.RunInTransaction(ctx, func(ctx context.Context) (err error) {
		defer func() { innerErr = err }()
		eids := make([]tryjob.ExternalID, 0, len(candidates))
		definitions = make([]*tryjob.Definition, 0, len(candidates))
		for def, tj := range candidates {
			eids = append(eids, tj.ExternalID)
			definitions = append(definitions, def)
		}
		tryjobs, err = tryjob.ResolveToTryjobs(ctx, eids...)
		if err != nil {
			return err
		}
		for i, tj := range tryjobs {
			if tj == nil {
				tj = w.makeBaseTryjob(ctx)
				tryjobs[i] = tj
			} else {
				// Tryjob already in datastore. This shouldn't normally happen but be
				// defensive here when it actually happens.
				tj.EVersion++
				tj.EntityUpdateTime = clock.Now(ctx).UTC()
				logging.Warningf(ctx, "tryjob %q was found reusable in backend, but "+
					"it already has a corresponding Tryjob entity (ID: %d). Ideally, "+
					"this Tryjob should be surfaced at the first attempt to search for "+
					"reusable Tryjob in datastore", tj.ExternalID, tj.ID)
			}
			tj.Definition = definitions[i]
			candidate := candidates[tj.Definition]
			tj.ExternalID = candidate.ExternalID
			tj.Status = candidate.Status
			tj.Result = candidate.Result
			if runID := w.run.ID; tj.AllWatchingRuns().Index(runID) < 0 {
				tj.ReusedBy = append(tj.ReusedBy, runID)
			}
			candidates[tj.Definition] = tj
		}
		return tryjob.SaveTryjobs(ctx, tryjobs)
	}, nil)
	switch {
	case innerErr != nil:
		return nil, innerErr
	case err != nil:
		return nil, errors.Annotate(err, "failed to commit transaction").Tag(transient.Tag).Err()
	}

	return candidates, nil
}
