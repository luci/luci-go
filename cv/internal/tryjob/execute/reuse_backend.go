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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cv/internal/tryjob"
)

// findReuseInBackend finds reusable Tryjobs by querying the backend (e.g.
// Buildbucket).
func (w *worker) findReuseInBackend(ctx context.Context, definitions []*tryjob.Definition) (map[*tryjob.Definition]*tryjob.Tryjob, error) {
	candidates := make(map[*tryjob.Definition]*tryjob.Tryjob, len(definitions))
	cutOffTime := clock.Now(ctx).Add(-staleTryjobAge)
	err := w.backend.Search(ctx, w.cls, definitions, w.run.ID.LUCIProject(), func(tj *tryjob.Tryjob) bool {
		// backend.Search returns matching Tryjob from newest to oldest, if backend
		// starts to return stale Tryjob (Tryjob created before cutoff time), then
		// there's no point resuming the search as none of the returning Tryjobs
		// will be reusable anyway.
		if createTime := tj.Result.GetCreateTime(); createTime != nil && createTime.AsTime().Before(cutOffTime) {
			return false
		}

		switch candidate, ok := candidates[tj.Definition]; {
		case ok:
			// Matching Tryjob already found.
			if tj.Result.GetCreateTime().AsTime().After(candidate.Result.GetCreateTime().AsTime()) {
				logging.Errorf(ctx, "FIXME(crbug/1369200): backend.Search is expected to return tryjob from newest to oldest; However, got %s before %s.", candidate.Result, tj.Result)
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

	ret := make(map[*tryjob.Definition]*tryjob.Tryjob)
	for def, candidate := range candidates {
		tj, err := w.mutator.Upsert(ctx, candidate.ExternalID, func(tj *tryjob.Tryjob) error {
			tj.ReuseKey = w.reuseKey
			tj.CLPatchsets = w.clPatchsets
			tj.Definition = def
			tj.ExternalID = candidate.ExternalID
			tj.Status = candidate.Status
			tj.Result = candidate.Result
			if runID := w.run.ID; tj.AllWatchingRuns().Index(runID) < 0 {
				tj.ReusedBy = append(tj.ReusedBy, runID)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		ret[def] = tj
	}

	return ret, nil
}
