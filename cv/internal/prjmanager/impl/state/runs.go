// Copyright 2021 The LUCI Authors.
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

package state

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
)

// addCreatedRuns adds previously unknown runs.
func (s *State) addCreatedRuns(ctx context.Context, ids map[common.RunID]struct{}) error {
	// Each newly created Run relates to the State in one of 3 ways:
	//   (0) Already tracked in State. This method assumes its caller,
	//   OnRunsCreated, has already filtered these Runs out.
	//
	//   (1) Most likely, there is already a fitting component s.t. all Run's CLs
	//   are in the component. Then, we just append Run to the component's Runs.
	//
	//   (2) But, it's also possible that component doesn't yet exist, e.g. if Run
	//   was just created via API call. Then, we add their CLs to tracked CLs, but
	//   store these Runs separately from components until components are
	//   re-computed.

	// Approach: after loading CL IDs for each Run, loop over all existing
	// components and add all Runs of type (1). All remaining Runs are thus of
	// type (2).
	runs, errs, err := loadRuns(ctx, ids)
	if err != nil {
		return err
	}
	// maps CL ID to index of Run(s) in runs slice.
	// Allocate 2x runs, because most projects have 1..2 CLs per Run on average.
	clToRunIndex := make(map[common.CLID][]int, 2*len(runs))
	// added keeps track of added Runs.
	added := make([]bool, len(runs))
	for i, r := range runs {
		switch err := errs[i]; {
		case err == datastore.ErrNoSuchEntity:
			logging.Errorf(ctx, "Newly created Run %s not found", r.ID)
			added[i] = true
		case err != nil:
			return errors.Annotate(err, "failed to load Run %s", r.ID).Tag(transient.Tag).Err()
		default:
			for _, clid := range r.CLs {
				clToRunIndex[clid] = append(clToRunIndex[clid], i)
			}
		}
	}

	// Add Runs of type (1) to existing components.
	var modified bool
	s.PB.Components, modified = s.PB.COWComponents(func(c *prjpb.Component) *prjpb.Component {
		// Count CLs in this component which match a Run's index in `runs`.
		matchedRunIdx := make(map[int]int, len(c.GetClids()))
		for _, clid := range c.GetClids() {
			for _, idx := range clToRunIndex[common.CLID(clid)] {
				matchedRunIdx[idx]++
			}
		}
		// Add runs to the component iff run's all CLs were matched.
		var toAdd []*prjpb.PRun
		for idx, count := range matchedRunIdx {
			if count == len(runs[idx].CLs) {
				added[idx] = true
				toAdd = append(toAdd, prjpb.MakePRun(runs[idx]))
			}
		}
		if len(toAdd) == 0 {
			return c
		}
		if pruns, modified := c.COWPRuns(nil, toAdd); modified {
			return &prjpb.Component{
				Clids:        c.GetClids(),
				DecisionTime: c.GetDecisionTime(),
				Pruns:        pruns,
				TriageRequired:        true,
			}
		}
		return c
	}, nil)
	if modified {
		s.PB.RepartitionRequired = true
	}

	// Add remaining Runs are of type (2) to CreatedPruns for later processing.
	var toAdd []*prjpb.PRun
	for i, r := range runs {
		if !added[i] {
			toAdd = append(toAdd, prjpb.MakePRun(r))
		}
	}
	s.PB.CreatedPruns, _ = s.PB.COWCreatedRuns(nil, toAdd)
	return nil
}

// removeFinishedRuns removes known runs and returns number of the still tracked
// runs.
func (s *State) removeFinishedRuns(ids map[common.RunID]struct{}) int {
	delIfFinished := func(r *prjpb.PRun) *prjpb.PRun {
		id := common.RunID(r.GetId())
		if _, ok := ids[id]; ok {
			delete(ids, id)
			return nil
		}
		return r
	}

	removeFromComponent := func(c *prjpb.Component) *prjpb.Component {
		if len(ids) == 0 {
			return c
		}
		if pruns, modified := c.COWPRuns(delIfFinished, nil); modified {
			return &prjpb.Component{
				Pruns:        pruns,
				Clids:        c.GetClids(),
				DecisionTime: c.GetDecisionTime(),
				TriageRequired:        true,
			}
		}
		return c
	}

	s.PB.CreatedPruns, _ = s.PB.COWCreatedRuns(delIfFinished, nil)
	stillTrackedRuns := len(s.PB.GetCreatedPruns())
	var modified bool
	s.PB.Components, modified = s.PB.COWComponents(func(c *prjpb.Component) *prjpb.Component {
		c = removeFromComponent(c)
		stillTrackedRuns += len(c.GetPruns())
		return c
	}, nil)
	if modified {
		// Removing usually changes components and/or pruning of PCLs.
		s.PB.RepartitionRequired = true
	}
	return stillTrackedRuns
}

// loadRuns loads Runs from Datastore corresponding to given Run IDs.
//
// Returns slice of Runs, error.MultiError slice corresponding to
// per-Run errors *(always exists and has same length as Runs)*, and a
// top level error if it can't be attributed to any Run.
//
// *each error in per-Run errors* is not annotated and is nil if Run was loaded
// successfully.
func loadRuns(ctx context.Context, ids map[common.RunID]struct{}) ([]*run.Run, errors.MultiError, error) {
	runs := make([]*run.Run, 0, len(ids))
	for id := range ids {
		runs = append(runs, &run.Run{ID: id})
	}
	err := datastore.Get(ctx, runs)
	switch merr, ok := err.(errors.MultiError); {
	case err == nil:
		return runs, make(errors.MultiError, len(runs)), nil
	case ok:
		return runs, merr, nil
	default:
		return nil, nil, errors.Annotate(err, "failed to load %d Runs", len(runs)).Tag(transient.Tag).Err()
	}
}
