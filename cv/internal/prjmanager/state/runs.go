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
	// Each newly created Run relates to the State in one of 4 ways:
	//   (0) Run has already finished.
	//
	//   (1) Already tracked in State. This method assumes its caller,
	//   OnRunsCreated, has already filtered these Runs out.
	//
	//   (2) Most likely, there is already a fitting component s.t. all Run's CLs
	//   are in the component. Then, we just append Run to the component's Runs.
	//
	//   (3) But, it's also possible that component doesn't yet exist, e.g. if Run
	//   was just created via API call. Then, we add their CLs to tracked CLs, but
	//   store these Runs separately from components until components are
	//   re-computed.

	// Approach: load Runs and filter out already finished. Then, after loading CL
	// IDs for each Run, loop over all existing components and add all Runs of
	// type (2). All remaining Runs are thus of type (3).
	runs, errs, err := loadRuns(ctx, ids)
	if err != nil {
		return err
	}
	// maps CL ID to index of Run(s) in runs slice.
	// Allocate 2x runs, because most projects have 1..2 CLs per Run on average.
	clToRunIndex := make(map[common.CLID][]int, 2*len(runs))
	// processed keeps track of already processed Runs.
	processed := make([]bool, len(runs))
	for i, r := range runs {
		switch err := errs[i]; {
		case err == datastore.ErrNoSuchEntity:
			logging.Errorf(ctx, "Newly created Run %s not found", r.ID)
			processed[i] = true
		case err != nil:
			return errors.Annotate(err, "failed to load Run %s", r.ID).Tag(transient.Tag).Err()
		case run.IsEnded(r.Status):
			logging.Warningf(ctx, "Newly created Run %s is already finished %s", r.ID, r.Status)
			processed[i] = true
		default:
			for _, clid := range r.CLs {
				clToRunIndex[clid] = append(clToRunIndex[clid], i)
			}
		}
	}

	// Add Runs of type (2) to existing components.
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
				processed[idx] = true
				toAdd = append(toAdd, prjpb.MakePRun(runs[idx]))
			}
		}
		if len(toAdd) == 0 {
			return c
		}
		if pruns, modified := c.COWPRuns(nil, toAdd); modified {
			return &prjpb.Component{
				Clids:          c.GetClids(),
				DecisionTime:   c.GetDecisionTime(),
				Pruns:          pruns,
				TriageRequired: true,
			}
		}
		return c
	}, nil)
	if modified {
		s.PB.RepartitionRequired = true
	}

	// Add remaining Runs are of type (3) to CreatedPruns for later processing.
	var toAdd []*prjpb.PRun
	for i, r := range runs {
		if !processed[i] {
			toAdd = append(toAdd, prjpb.MakePRun(r))
		}
	}
	s.PB.CreatedPruns, _ = s.PB.COWCreatedRuns(nil, toAdd)
	return nil
}

// removeFinishedRuns removes known runs and returns number of the still tracked
// runs.
func (s *State) removeFinishedRuns(ids map[common.RunID]run.Status, removeCB func(r *prjpb.PRun)) int {
	delIfFinished := func(r *prjpb.PRun) *prjpb.PRun {
		id := common.RunID(r.GetId())
		if _, ok := ids[id]; ok {
			removeCB(r)
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
				Pruns:          pruns,
				Clids:          c.GetClids(),
				DecisionTime:   c.GetDecisionTime(),
				TriageRequired: true,
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

func incompleteRuns(ctx context.Context, ids map[common.RunID]struct{}) (common.RunIDs, error) {
	runs, errs, err := loadRuns(ctx, ids)
	if err != nil {
		return nil, err
	}
	var incomplete common.RunIDs
	for i, r := range runs {
		switch {
		case errs[i] != nil:
			return nil, transient.Tag.Apply(errors.Fmt("failed to load Run %s: %w", r.ID, errs[i]))
		case !run.IsEnded(r.Status):
			incomplete = append(incomplete, r.ID)
		}
	}
	return incomplete, err
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

// maybeMCERun returns whether a given Run could be part of MCE Run.
// That is, whether the Run was triggered by chained CQ votes.
func maybeMCERun(ctx context.Context, s *State, r *prjpb.PRun) bool {
	if run.Mode(r.GetMode()) != run.FullRun {
		return false
	}
	pcl := s.PB.GetPCL(r.GetClids()[0])
	if pcl == nil {
		return false
	}
	switch idxs := pcl.GetConfigGroupIndexes(); {
	case len(idxs) != 1:
		// In case of misconfiguration, PCL may be involved with 0 or 2+
		// applicable ConfigGroups, and evalcl uses an empty ConfigGroup
		// to remove such CLs.
		//
		// Such PCL should not be a valid MCE run.
		return false
	case len(s.configGroups) == 0:
		// This may be a bug in CV, but ok. It is not CombineCl.
		return true
	case s.configGroups[idxs[0]].Content.GetCombineCls() != nil:
		return false
	}
	return true
}
