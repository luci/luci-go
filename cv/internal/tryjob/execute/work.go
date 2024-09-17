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
	"sort"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// startTryjobs triggers Tryjobs for the given Definitions by either reusing
// existing Tryjobs or launching new ones.
func (e *Executor) startTryjobs(ctx context.Context, r *run.Run, definitions []*tryjob.Definition, executions []*tryjob.ExecutionState_Execution) ([]*tryjob.Tryjob, error) {
	cls, err := run.LoadRunCLs(ctx, r.ID, r.CLs)
	if err != nil {
		return nil, err
	}
	w := &worker{
		backend:          e.Backend,
		mutator:          tryjob.NewMutator(e.RM),
		run:              r,
		cls:              cls,
		knownTryjobIDs:   make(common.TryjobIDSet),
		knownExternalIDs: make(stringset.Set),
		reuseKey:         computeReuseKey(cls),
		clPatchsets:      make(tryjob.CLPatchsets, len(cls)),
	}
	for _, execution := range executions {
		for _, attempt := range execution.GetAttempts() {
			if tjID := common.TryjobID(attempt.GetTryjobId()); tjID != 0 {
				w.knownTryjobIDs.Add(tjID)
			}
			if eid := attempt.GetExternalId(); eid != "" {
				w.knownExternalIDs.Add(eid)
			}
		}
	}
	for i, cl := range cls {
		w.clPatchsets[i] = tryjob.MakeCLPatchset(cl.ID, cl.Detail.GetPatchset())
	}
	sort.Sort(w.clPatchsets)
	w.findReuseFns = append(w.findReuseFns, w.findReuseInCV, w.findReuseInBackend)

	ret, err := w.start(ctx, definitions)
	for _, le := range w.logEntries {
		e.log(le)
	}
	if err != nil {
		return nil, err
	}
	return ret, nil
}

// worker implements the workflow to trigger Tryjobs for the given Definitions.
//
// It does this by searching for Tryjobs that can be reused first, and then
// launching new Tryjobs if nothing can be reused.
type worker struct {
	run              *run.Run
	cls              []*run.RunCL
	knownTryjobIDs   common.TryjobIDSet
	knownExternalIDs stringset.Set

	reuseKey    string
	clPatchsets tryjob.CLPatchsets
	backend     TryjobBackend
	mutator     *tryjob.Mutator

	findReuseFns []findReuseFn
	logEntries   []*tryjob.ExecutionLogEntry
}

func (w *worker) makeBaseTryjob(ctx context.Context) *tryjob.Tryjob {
	now := datastore.RoundTime(clock.Now(ctx).UTC())
	return &tryjob.Tryjob{
		EVersion:         1,
		EntityCreateTime: now,
		EntityUpdateTime: now,
		ReuseKey:         w.reuseKey,
		CLPatchsets:      w.clPatchsets,
	}
}

// makePendingTryjob makes a pending Tryjob that is triggered by this Run.
func (w *worker) makePendingTryjob(ctx context.Context, def *tryjob.Definition) *tryjob.Tryjob {
	tj := w.makeBaseTryjob(ctx)
	tj.Definition = def
	tj.Status = tryjob.Status_PENDING
	tj.LaunchedBy = w.run.ID
	return tj
}

// start triggers Tryjobs for the given Definitions.
//
// First it searches for any Tryjobs that can be reused, then launches
// new Tryjobs for Definitions where nothing can be reused.
func (w *worker) start(ctx context.Context, definitions []*tryjob.Definition) ([]*tryjob.Tryjob, error) {
	reuse, err := w.findReuse(ctx, definitions)
	if err != nil {
		return nil, err
	}
	ret := make([]*tryjob.Tryjob, len(definitions))
	tryjobsToLaunch := make([]*tryjob.Tryjob, 0, len(definitions))
	reusedTryjobsCount := 0
	for i, def := range definitions {
		switch reuseTryjob, hasReuse := reuse[def]; {
		case !hasReuse:
			tryjobsToLaunch = append(tryjobsToLaunch, w.makePendingTryjob(ctx, def))
		case reuseTryjob.LaunchedBy == w.run.ID && reuseTryjob.Status == tryjob.Status_PENDING:
			// This typically happens when a previous task created the Tryjob entity
			// but failed to launch the Tryjob at the backend. Such Tryjob entity will
			// be surfaced again when searching for reusable Tryjob within CV.
			// Therefore, try to launch the Tryjob again.
			tryjobsToLaunch = append(tryjobsToLaunch, reuseTryjob)
		default:
			ret[i] = reuseTryjob
			reusedTryjobsCount += 1
		}
	}

	if len(tryjobsToLaunch) > 0 {
		// Save the newly created Tryjobs and ensure Tryjob IDs are populated.
		var newlyCreatedTryjobs []*tryjob.Tryjob
		for _, tj := range tryjobsToLaunch {
			if tj.ID == 0 {
				newlyCreatedTryjobs = append(newlyCreatedTryjobs, tj)
			}
		}
		if len(newlyCreatedTryjobs) > 0 {
			if err := datastore.Put(ctx, newlyCreatedTryjobs); err != nil {
				return nil, err
			}
		}
		tryjobsToLaunch, err = w.launchTryjobs(ctx, tryjobsToLaunch)
		if err != nil {
			return nil, err
		}
		// Copy the launched Tryjobs to the returned Tryjobs at the
		// corresponding location.
		if reusedTryjobsCount+len(tryjobsToLaunch) != len(definitions) {
			panic(fmt.Errorf("impossible; requested %d Tryjob Definition, reused %d Tryjobs but launched %d new Tryjobs",
				len(definitions), reusedTryjobsCount, len(tryjobsToLaunch)))
		}
		idx := 0
		for i, tj := range ret {
			if tj == nil {
				ret[i] = tryjobsToLaunch[idx]
				idx += 1
			}
		}
	}

	return ret, nil
}

type findReuseFn func(context.Context, []*tryjob.Definition) (map[*tryjob.Definition]*tryjob.Tryjob, error)

// findReuse finds Tryjobs that shall be reused.
func (w *worker) findReuse(ctx context.Context, definitions []*tryjob.Definition) (map[*tryjob.Definition]*tryjob.Tryjob, error) {
	if len(w.findReuseFns) == 0 {
		return nil, nil
	}
	ret := make(map[*tryjob.Definition]*tryjob.Tryjob, len(definitions))
	remainingDefinitions := make([]*tryjob.Definition, 0, len(definitions))
	// Start with Tryjobs' Definitions that enable reuse.
	for _, def := range definitions {
		if !def.GetDisableReuse() {
			remainingDefinitions = append(remainingDefinitions, def)
		}
	}

	for _, fn := range w.findReuseFns {
		reuse, err := fn(ctx, remainingDefinitions)
		if err != nil {
			return nil, err
		}
		for def, tj := range reuse {
			ret[def] = tj
		}
		// Reuse the `remainingDefinitions` slice and filter out the
		// Definitions that have found reuse Tryjobs.
		tmp := remainingDefinitions[:0]
		for _, def := range remainingDefinitions {
			if _, ok := reuse[def]; !ok {
				tmp = append(tmp, def)
			}
		}
		remainingDefinitions = tmp
		if len(remainingDefinitions) == 0 {
			break
		}
	}

	if len(ret) > 0 {
		reusedTryjobLogs := make([]*tryjob.ExecutionLogEntry_TryjobSnapshot, 0, len(ret))
		for def, tj := range ret {
			reusedTryjobLogs = append(reusedTryjobLogs, makeLogTryjobSnapshot(def, tj, true))
		}
		w.logEntries = append(w.logEntries, &tryjob.ExecutionLogEntry{
			Time: timestamppb.New(clock.Now(ctx).UTC()),
			Kind: &tryjob.ExecutionLogEntry_TryjobsReused_{
				TryjobsReused: &tryjob.ExecutionLogEntry_TryjobsReused{
					Tryjobs: reusedTryjobLogs,
				},
			},
		})
	}
	return ret, nil
}
