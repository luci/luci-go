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
	"slices"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/gerrit/metadata"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// findReuseInBackend finds reusable Tryjobs by querying the backend (e.g.
// Buildbucket) in order to reuse Tryjobs that were triggered outside of CV,
// e.g. manually through Gerrit.
func (w *worker) findReuseInBackend(ctx context.Context, reuseKey string, definitions []*tryjob.Definition) (map[*tryjob.Definition]*tryjob.Tryjob, error) {
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
		case w.disableReuseFootersChanged(ctx, tj):
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
			tj.ReuseKey = reuseKey
			tj.CLPatchsets = candidate.CLPatchsets
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

// disableReuseFootersChanged determines whether any of the tryjob's
// DisableReuseFooters have changed between between the patchset that the
// previous attempt ran against and the current patchset.
func (w *worker) disableReuseFootersChanged(ctx context.Context, tj *tryjob.Tryjob) bool {
	disableReuseFooters := stringset.NewFromSlice(tj.Definition.GetDisableReuseFooters()...)
	if len(disableReuseFooters) == 0 {
		return false
	}

	clsByID := make(map[common.CLID]*run.RunCL)
	for _, cl := range w.cls {
		clsByID[cl.ID] = cl
	}

	for _, clp := range tj.CLPatchsets {
		clID, prevPatchset, err := clp.Parse()
		if err != nil {
			logging.Errorf(ctx, "FIXME: parsing CLPatchset failed: %s", err)
			return true
		}
		cl, ok := clsByID[clID]
		if !ok {
			logging.Errorf(ctx, "FIXME: CL %s from old tryjob missing in current attempt's CLs", clp)
			return true
		}

		// The same patchset is still the latest so the footers cannot have
		// changed.
		if prevPatchset == cl.Detail.GetPatchset() {
			continue
		}

		currentFooters := filterDisableReuseFooters(cl.Detail.GetMetadata(), disableReuseFooters)

		revs := cl.Detail.GetGerrit().GetInfo().GetRevisions()
		var previousRev *gerritpb.RevisionInfo
		for _, rev := range revs {
			if rev.Number == prevPatchset {
				previousRev = rev
				break
			}
		}
		if previousRev == nil {
			logging.Errorf(ctx, "FIXME: Information for CL patchset %s missing from datastore", clp)
			return true
		}

		previousMsg := previousRev.GetCommit().GetMessage()
		previousFooters := filterDisableReuseFooters(metadata.Extract(previousMsg), disableReuseFooters)

		if !slices.EqualFunc(previousFooters, currentFooters, func(f1, f2 *changelist.StringPair) bool {
			return proto.Equal(f1, f2)
		}) {
			return true
		}
	}
	return false
}

func filterDisableReuseFooters(footers []*changelist.StringPair, disableReuseFooters stringset.Set) []*changelist.StringPair {
	ret := make([]*changelist.StringPair, 0, len(footers))
	for _, footer := range footers {
		if disableReuseFooters.Has(footer.Key) {
			ret = append(ret, footer)
		}
	}
	// Ordering of footers doesn't matter, so sort before comparing.
	sortFooters(ret)
	return ret
}
