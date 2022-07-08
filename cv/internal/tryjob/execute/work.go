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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func (e *Executor) startTryjobs(ctx context.Context, r *run.Run, definitions []*tryjob.Definition, executions []*tryjob.ExecutionState_Execution) ([]*tryjob.Tryjob, error) {
	cls, err := run.LoadRunCLs(ctx, r.ID, r.CLs)
	if err != nil {
		return nil, err
	}
	w := &worker{
		backend:          e.Backend,
		rm:               e.RM,
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

	return w.start(ctx, definitions)
}

// worker implements the workflow to trigger tryjobs for the given definitions
// by searching for Tryjobs that can be reused first and then launch new
// Tryjobs if nothing can be reused.
type worker struct {
	run              *run.Run
	cls              []*run.RunCL
	knownTryjobIDs   common.TryjobIDSet
	knownExternalIDs stringset.Set

	reuseKey    string
	clPatchsets tryjob.CLPatchsets
	backend     TryjobBackend
	rm          rm

	findReuseFns []findReuseFn
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

// makePendingTryjob makes a pending tryjob that is triggered by this Run.
func (w *worker) makePendingTryjob(ctx context.Context, def *tryjob.Definition) *tryjob.Tryjob {
	tj := w.makeBaseTryjob(ctx)
	tj.Definition = def
	tj.Status = tryjob.Status_PENDING
	tj.TriggeredBy = w.run.ID
	return tj
}

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
		case reuseTryjob.TriggeredBy == w.run.ID && reuseTryjob.Status == tryjob.Status_PENDING:
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
		// Save the newly created tryjobs and ensure Tryjob IDs are populated.
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
		// copy the launched tryjobs to the returned tryjobs at the corresponding
		// location.
		if reusedTryjobsCount+len(tryjobsToLaunch) != len(definitions) {
			panic(fmt.Errorf("impossible; requested %d tryjob definition, reused %d tryjobs but launched %d new tryjobs", len(definitions), reusedTryjobsCount, len(tryjobsToLaunch)))
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

func (w *worker) findReuse(ctx context.Context, definitions []*tryjob.Definition) (map[*tryjob.Definition]*tryjob.Tryjob, error) {
	if len(w.findReuseFns) == 0 {
		return nil, nil
	}
	ret := make(map[*tryjob.Definition]*tryjob.Tryjob, len(definitions))
	remainingDefinitions := make([]*tryjob.Definition, 0, len(definitions))
	// start with tryjobs definitions that enable reuse.
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
		// reuse remainingDefinitions slice  and filter out the definitions that
		// have found reuse Tryjobs.
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
	return ret, nil
}
