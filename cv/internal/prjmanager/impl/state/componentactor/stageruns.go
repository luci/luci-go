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
	"strings"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/prjmanager/runcreator"
	"go.chromium.org/luci/cv/internal/run"
)

// stageNewRuns sets .Creators if new Runs have to be created.
//
// Otherwise, returns the time when this could be done.
// NOTE: because this function may take considerable time, the returned time may
// be already in the past.
//
// Guarantees that a CL can be in at most 1 Creator. This avoids conflicts
// whereby 2+ Runs need to modify the same CL entity.
func (a *actor) stageNewRuns(ctx context.Context) (time.Time, error) {
	a.visitedCLs = map[int64]struct{}{}
	defer func() { a.visitedCLs = nil }() // won't be useful afterwards.

	var t time.Time
	for clid, info := range a.cls {
		switch nt, err := a.stageNewRunsFrom(ctx, clid, info); {
		case err != nil:
			return t, err
		case !nt.IsZero() && (t.IsZero() || nt.Before(t)):
			t = nt
		}
	}
	return t, nil
}

func (a *actor) stageNewRunsFrom(ctx context.Context, clid int64, info *clInfo) (time.Time, error) {
	if !a.markVisited(clid) || !info.ready {
		return time.Time{}, nil
	}
	cgIndex := info.pcl.GetConfigGroupIndexes()[0]
	cg := a.s.ConfigGroup(cgIndex)
	if cg.Content.GetCombineCls() == nil {
		return a.stageNewRunsSingle(ctx, info, cg)
	}
	return a.stageNewRunsCombo(ctx, info, cg)
}

func (a *actor) stageNewRunsSingle(ctx context.Context, info *clInfo, cg *config.ConfigGroup) (time.Time, error) {
	if len(info.runIndexes) > 0 {
		// Singular case today doesn't support concurrent runs.
		return time.Time{}, nil
	}
	if len(info.deps.notYetLoaded) > 0 {
		return a.postponeDueToNotYetLoadedDeps(ctx, info)
	}

	combo := combo{}
	combo.add(info)
	rb, err := a.makeCreator(ctx, &combo, cg)
	if err != nil {
		return time.Time{}, err
	}
	a.runCreators = append(a.runCreators, rb)
	return clock.Now(ctx), nil
}

var errUnexpectednotReadyInCombo = errors.New("reached notReady not meeting assumtions")

func (a *actor) stageNewRunsCombo(ctx context.Context, info *clInfo, cg *config.ConfigGroup) (time.Time, error) {
	// First, maximize Run's CL # to include not only all reachable dependencies, but also
	// reachable dependents.
	combo := combo{}
	combo.add(info)
	a.expandComboVisited(info, &combo)

	delay := cg.Content.GetCombineCls().GetStabilizationDelay().AsDuration()
	if next := combo.maxTriggeredTime.Add(delay); next.After(clock.Now(ctx)) {
		return next, nil
	}
	if combo.withNotYetLoadedDeps != nil {
		return a.postponeDueToNotYetLoadedDeps(ctx, combo.withNotYetLoadedDeps)
	}
	if len(combo.notReady) > 0 {
		// Given:
		//  * we started with a ready CL;
		//  * reverseDeps only points to ready CLs;
		//  * no CLs in combo have notYetLoaded deps;
		//  * expandComboVisited doesn't expand notReady CLs.
		//
		// It follows:
		//  * each notReady CL must be a valid dep of an ready CL in combo;
		//  * valid dep means each notReady CL is triggered and in the same group as
		//    ready CL.
		//
		// Hence, each notReady CL either:
		//  * is already being purged,
		//  * has invalid own deps and either one of:
		//    * is already part of a Run, which will be finalized soon;
		//    * should be purged.

		// Check that the conclusions above meet the reality.
		var err error
		clids := make([]int64, len(combo.notReady))
		for i, x := range combo.notReady {
			clids[i] = x.pcl.GetClid()
			if len(x.purgeReasons) == 0 && x.purgingCL == nil && len(x.runIndexes) == 0 {
				// Record error for just 1 CL, but log each one.
				if err == nil {
					err = errors.Annotate(errUnexpectednotReadyInCombo, "started from %v, reached %v", info, x).Err()
				}
				logging.WithError(err).Errorf(ctx, "started from %v, reached %v (combo size %d)", info, x, len(combo.all))
			}
		}
		if err != nil {
			return time.Time{}, err
		}
		return a.postponeDueToNotReadyDeps(ctx, &combo)
	}

	// At this point all CLs in combo are stable, ready and with valid deps.

	// By construction, combo must already contain *all* non-submitted deps.
	// Proof:
	//   Suppose, combo is missing a dep. Then, during expandCombo the dep must
	//   have been considered, but dismissed for either of 2 reasons:
	//     (1) dep doesn't have clInfo, which means dep doesn't belong to this
	//     component.
	//       -> However, component partitioning function guarantees that all
	//       triggered deps are in the same component (unless not yet loaded).
	//     (2) dep was already visited before.
	//       -> However, then dep's reverseDeps would have led to a CL in current
	//       combo.
	// QED.
	if missing := combo.missingDeps(); len(missing) > 0 {
		panic(fmt.Errorf("combo %v has missing deps %s", combo, missing))
	}
	// Furthermore, since all CLs are ready and related, they must belong to
	// the exact same config group as the initial CL.
	if cgIndexes := combo.configGroupsIndexes(); len(cgIndexes) > 1 {
		panic(fmt.Errorf("combo %v has >1 config groups: %v", combo, cgIndexes))
	}

	// TODO(tandrii): support >1 concurrent Run on the same CL.
	if runCounts := combo.runsOverlap(); len(runCounts) > 0 {
		for runIndex, cnt := range runCounts { // take first and only Run
			prun := a.c.GetPruns()[runIndex]
			switch l := len(prun.GetClids()); {
			case l > cnt:
				// Run's scope is larger or different than this combo.  Run Manager will
				// soon be finalizing the Run as not all of its CLs are triggered.
				return a.postponeDueToExistingRunDiffScope(ctx, &combo, prun)

				// cnt > l is impossible, so all cases below have cnt == l.
			case cnt == len(combo.all):
				// The combo scope is exactly the same as Run. This is expected.
				// It's possible that Run's mode is different from the combo, in which
				// case Run Manager will be finalizing the Run soon and we have to wait.
				return time.Time{}, nil
			default:
				// The combo scope is larger than Run.
				return a.postponeAndCancelExistingRun(ctx, &combo, prun)
			}
		}
		panic("unreachable")
	}

	rb, err := a.makeCreator(ctx, &combo, cg)
	if err != nil {
		return time.Time{}, err
	}
	a.runCreators = append(a.runCreators, rb)
	return time.Time{}, nil
}

func (a *actor) expandComboVisited(info *clInfo, result *combo) {
	if !info.ready {
		return
	}
	info.deps.iterateNotSubmitted(info.pcl, func(dep *changelist.Dep) {
		a.expandCombo(dep.GetClid(), result)
	})
	for _, clid := range a.reverseDeps[info.pcl.GetClid()] {
		a.expandCombo(clid, result)
	}
}

func (a *actor) expandCombo(clid int64, result *combo) {
	info := a.cls[clid]
	if info == nil {
		// Can only happen if clid is a dep that's not yet loaded (otherwise dep
		// would be in this component, and hence info would be set).
		return
	}
	if !a.markVisited(clid) {
		return
	}
	result.add(info)
	a.expandComboVisited(info, result)
}

func (a *actor) postponeDueToNotYetLoadedDeps(ctx context.Context, info *clInfo) (time.Time, error) {
	// TODO(crbug/1211576): this waiting can last forever. Component needs to
	// record how long it has been waiting and abort with clear message to the
	// user.
	sb := strings.Builder{}
	fmt.Fprintf(&sb, "combo with CL %d waiting on %d deps to load: [", info.pcl.GetClid(), len(info.deps.notYetLoaded))
	for _, d := range info.deps.notYetLoaded {
		fmt.Fprintf(&sb, " %d", d.GetClid())
	}
	sb.WriteRune(']')
	logging.Warningf(ctx, sb.String())
	return time.Time{}, nil
}

func (a *actor) postponeDueToNotReadyDeps(ctx context.Context, combo *combo) (time.Time, error) {
	// TODO(tandrii): for safety, this should not wait forever.
	sb := strings.Builder{}
	fmt.Fprintf(
		&sb, "combo from %v of size %d reached %d notReady CLs:",
		combo.all[0], len(combo.all), len(combo.notReady))
	for _, x := range combo.notReady {
		fmt.Fprintf(&sb, " %d", x.pcl.GetClid())
	}
	logging.Warningf(ctx, sb.String())
	return time.Time{}, nil
}

func (a *actor) postponeDueToExistingRunDiffScope(ctx context.Context, combo *combo, r *prjpb.PRun) (time.Time, error) {
	// TODO(tandrii): for safety, this should not wait forever.
	logging.Warningf(ctx, "combo of %d is waiting for differently scoped run %q to finish", len(combo.all), r.GetId())
	return time.Time{}, nil
}

func (a *actor) postponeAndCancelExistingRun(ctx context.Context, combo *combo, r *prjpb.PRun) (time.Time, error) {
	// TODO(tandrii): for safety, this should not wait forever.
	// TODO(tandrii): record desire to cancel and cancel in act().
	logging.Warningf(ctx, "combo of %d is waiting for smaller scoped run %q to finish", len(combo.all), r.GetId())
	return time.Time{}, nil
}

func (a *actor) makeCreator(ctx context.Context, combo *combo, cg *config.ConfigGroup) (*runcreator.Creator, error) {
	latestIndex := -1
	cls := make([]*changelist.CL, len(combo.all))
	for i, info := range combo.all {
		cls[i] = &changelist.CL{ID: common.CLID(info.pcl.GetClid())}
		if info == combo.latestTriggered {
			latestIndex = i
		}
	}
	if err := datastore.Get(ctx, cls); err != nil {
		// Even if one of errors is EntityNotFound, this is a temporary situation as
		// such CL(s) should be removed from PM state soon.
		return nil, errors.Annotate(err, "failed to load CLs").Tag(transient.Tag).Err()
	}
	for i, cl := range cls {
		exp, act := combo.all[i].pcl.GetEversion(), int64(cl.EVersion)
		if exp != act {
			return nil, errors.Reason("CL %d EVersion changed %d => %d", cl.ID, exp, act).Tag(transient.Tag).Err()
		}
	}

	// Run's owner is whoever owns the latest triggered CL.
	// It's guaranteed to be set because otherwise CL would have been sent for
	// pruning and nor marked as ready.
	owner, err := cls[latestIndex].Snapshot.OwnerIdentity()
	if err != nil {
		return nil, errors.Annotate(err, "failed to get OwnerIdentity of %d", cls[latestIndex].ID).Err()
	}

	bcls := make([]runcreator.CL, len(cls))
	for i, cl := range cls {
		pcl := combo.all[i].pcl
		bcls[i] = runcreator.CL{
			ID:               common.CLID(pcl.GetClid()),
			ExpectedEVersion: int(pcl.GetEversion()),
			TriggerInfo:      pcl.GetTrigger(),
			Snapshot:         cl.Snapshot,
		}
	}

	return &runcreator.Creator{
		ConfigGroupID:            cg.ID,
		LUCIProject:              cg.ProjectString(),
		Mode:                     run.Mode(combo.latestTriggered.pcl.GetTrigger().GetMode()),
		Owner:                    owner,
		ExpectedIncompleteRunIDs: nil, // no Run is expected
		OperationID:              fmt.Sprintf("PM-%d", mathrand.Int63(ctx)),
		InputCLs:                 bcls,
	}, nil
}

// markVisited makes CL visited if not already and returns if action was taken.
func (a *actor) markVisited(clid int64) bool {
	if _, visited := a.visitedCLs[clid]; visited {
		return false
	}
	a.visitedCLs[clid] = struct{}{}
	return true
}

// combo is a set of related CLs that will together form a new Run.
//
// The CLs in a combo are a subset of those from the component.
type combo struct {
	all                  []*clInfo
	clids                map[int64]struct{}
	notReady             []*clInfo
	withNotYetLoadedDeps *clInfo // nil if none; any one otherwise.
	latestTriggered      *clInfo
	maxTriggeredTime     time.Time
}

func (c *combo) add(info *clInfo) {
	c.all = append(c.all, info)
	if c.clids == nil {
		c.clids = map[int64]struct{}{info.pcl.GetClid(): {}}
	} else {
		c.clids[info.pcl.GetClid()] = struct{}{}
	}

	if !info.ready {
		c.notReady = append(c.notReady, info)
	}

	if info.deps != nil && len(info.deps.notYetLoaded) > 0 {
		c.withNotYetLoadedDeps = info
	}

	if pb := info.pcl.GetTrigger().GetTime(); pb != nil {
		t := pb.AsTime()
		if c.maxTriggeredTime.IsZero() || t.After(c.maxTriggeredTime) {
			c.maxTriggeredTime = t
			c.latestTriggered = info
		}
	}
}

func (c *combo) missingDeps() []*changelist.Dep {
	var missing []*changelist.Dep
	for _, info := range c.all {
		info.deps.iterateNotSubmitted(info.pcl, func(dep *changelist.Dep) {
			if _, in := c.clids[dep.GetClid()]; !in {
				missing = append(missing, dep)
			}
		})
	}
	return missing
}

func (c *combo) configGroupsIndexes() []int32 {
	res := make([]int32, 0, 1)
	for _, info := range c.all {
		idx := info.pcl.GetConfigGroupIndexes()[0]
		found := false
		for _, v := range res {
			if v == idx {
				found = true
			}
		}
		if !found {
			res = append(res, idx)
		}
	}
	return res
}

// runsOverlap returns number of CLs shared with each Run identified by its
// index.
func (c *combo) runsOverlap() map[int32]int {
	res := map[int32]int{}
	for _, info := range c.all {
		for _, index := range info.runIndexes {
			res[index]++
		}
	}
	return res
}
