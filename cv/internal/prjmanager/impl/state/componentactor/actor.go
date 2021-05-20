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

package componentactor

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/migration/migrationcfg"
	"go.chromium.org/luci/cv/internal/prjmanager/impl/state/itriager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/prjmanager/runcreator"
)

// Actor implements PM state.componentActor in production.
//
// Assumptions:
//   for each Component's CL:
//     * there is a PCL via Supporter interface
//     * for each dependency:
//        * it's not yet loaded OR must be itself a component's CL.
//
// The assumptions are in fact guaranteed by PM's State.repartion function.
type Actor struct {
	c *prjpb.Component
	s supporterWrapper

	// cls provides clid -> info for each CL of the component.
	cls map[int64]*clInfo
	// reverseDeps maps dep (as clid) -> which CLs depend on it.
	//
	// Only for CLs with clInfo.ready being true.
	reverseDeps map[int64][]int64

	// visitedCLs tracks clid of visited CLs during stageNewRuns.
	visitedCLs map[int64]struct{}

	// runCreators are prepared by NextActionTime() and executed in Act().
	runCreators []*runcreator.Creator
	// purgeCLtasks for subset of CLs in toPurge which can be purged now.
	purgeCLtasks []*prjpb.PurgeCLTask
}

// newActor returns new Actor.
func newActor(c *prjpb.Component, s itriager.PMState) *Actor {
	return &Actor{c: c, s: supporterWrapper{s}}
}

// NextActionTime implements componentActor.
func (a *Actor) NextActionTime(ctx context.Context, now time.Time) (time.Time, error) {
	a.triageCLs()

	when, err := a.stageNewRuns(ctx)
	switch {
	case err != nil:
		return time.Time{}, err
	case len(a.runCreators) > 0:
		// Required by the componentActor.NextActionTime
		when = now
	}

	if w := a.stagePurges(ctx, now); !w.IsZero() && (when.IsZero() || w.Before(when)) {
		when = w
	}
	return when, nil
}

func Triage(ctx context.Context, c *prjpb.Component, s itriager.PMState) (itriager.Result, error) {
	a := Actor{c: c, s: supporterWrapper{s}}
	res := itriager.Result{}
	now := clock.Now(ctx)
	// TODO(tandrii): refactor Actor into Triager and rewrite this function.
	when, err := a.NextActionTime(ctx, now)
	if err != nil {
		return res, err
	}
	c = c.CloneShallow()
	c.Dirty = false
	res.NewValue = c
	switch {
	case when.IsZero():
		c.DecisionTime = nil
	case when.After(now):
		c.DecisionTime = timestamppb.New(when)
	case when == now: // == is per contract of NextActionTime
		c.DecisionTime = timestamppb.New(when)
		res.RunsToCreate = a.runCreators
		res.CLsTopurge = a.purgeCLtasks
	}
	return res, nil
}

// Act implements state.componentActor.
func (a *Actor) Act(ctx context.Context, pm runcreator.PM, rm runcreator.RM) (*prjpb.Component, []*prjpb.PurgeCLTask, error) {
	c := a.c.CloneShallow()
	c.Dirty = false

	switch newPruns, err := a.createRuns(ctx, pm, rm); {
	case err != nil:
		return nil, nil, err
	case len(newPruns) > 0:
		c.Pruns, _ = c.COWPRuns(nil, newPruns)
	}
	// TODO(tandrii): cancelations
	return c, a.purgeCLtasks, nil
}

func (a *Actor) createRuns(ctx context.Context, pm runcreator.PM, rm runcreator.RM) ([]*prjpb.PRun, error) {
	if len(a.runCreators) == 0 {
		return nil, nil
	}
	switch yes, err := migrationcfg.IsCQDUsingMyRuns(ctx, a.runCreators[0].LUCIProject); {
	case err != nil:
		return nil, err
	case !yes:
		// This a is temporary safeguard against creation of LOTS of Runs,
		// that won't be finalized.
		// TODO(tandrii): delete this check once RunManager cancels Runs based on
		// user actions and finalizes based on CQD reports.
		logging.Debugf(ctx, "would have created %d Runs", len(a.runCreators))
		return nil, nil
	}

	toAdd := make([]*prjpb.PRun, 0, len(a.runCreators))
	var errs errors.MultiError
	for _, rb := range a.runCreators {
		switch r, err := rb.Create(ctx, pm, rm); {
		case err != nil:
			errs = append(errs, err)
		default:
			toAdd = append(toAdd, prjpb.MakePRun(r))
		}
	}
	if len(errs) > 0 {
		err := common.MostSevereError(errs)
		return nil, errors.Annotate(err, "failed to create %d Runs, most severe error:", len(errs)).Err()
	}
	return toAdd, nil
}

type supporterWrapper struct {
	itriager.PMState
}

// MustPCL panics if clid doesn't exist.
//
// Exists primarily for readability.
func (s supporterWrapper) MustPCL(clid int64) *prjpb.PCL {
	if p := s.PCL(clid); p != nil {
		return p
	}
	panic(fmt.Errorf("MustPCL: clid %d not known", clid))
}
