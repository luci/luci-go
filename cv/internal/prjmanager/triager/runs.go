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

package triager

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
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/prjmanager/itriager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/runcreator"
)

// stageNewRuns returns Run Creators for immediate Run creation or the earliest
// time for the next Run to be created.
//
// Guarantees that returned Run Creators are CL-wise disjoint, and thus can be
// created totally independently.
//
// In exceptional cases, also marks some CLs for purging if their trigger
// matches the existing finalized Run.
func stageNewRuns(ctx context.Context, c *prjpb.Component, cls map[int64]*clInfo, pm pmState) ([]*runcreator.Creator, time.Time, error) {
	var next time.Time
	var out []*runcreator.Creator

	rs := runStage{
		pm:         pm,
		c:          c,
		cls:        cls,
		visitedCLs: make(map[int64]struct{}, len(cls)),
	}
	// For determinism, iterate in fixed order:
	for _, clid := range c.GetClids() {
		info := cls[clid]
		switch rc, nt, err := rs.stageNewRunsFrom(ctx, clid, info); {
		case err != nil:
			return nil, time.Time{}, err
		case rc != nil:
			out = append(out, rc)
		default:
			next = earliest(next, nt)
		}
	}
	return out, next, nil
}

type runStage struct {
	// immutable

	pm  pmState
	c   *prjpb.Component
	cls map[int64]*clInfo

	// mutable

	// visitedCLs tracks CLs already considered. Ensures that 1 CL can appear in
	// at most 1 new Run.
	visitedCLs map[int64]struct{}
	// cachedReverseDeps maps clid to clids of CLs which depend on it.
	// lazily-initialized.
	cachedReverseDeps map[int64][]int64
}

func (rs *runStage) stageNewRunsFrom(ctx context.Context, clid int64, info *clInfo) (*runcreator.Creator, time.Time, error) {
	// Only start with ready CLs. Non-ready ones can't form new Runs anyway.
	if !info.ready {
		return nil, time.Time{}, nil
	}

	if !rs.markVisited(clid) {
		return nil, time.Time{}, nil
	}

	combo := combo{}
	combo.add(info)

	cgIndex := info.pcl.GetConfigGroupIndexes()[0]
	cg := rs.pm.ConfigGroup(cgIndex)

	if cg.Content.GetCombineCls() != nil {
		// Maximize Run's CL # to include not only all reachable dependencies, but
		// also reachable dependents, recursively.
		rs.expandComboVisited(info, &combo)

		// Shall the decision be delayed?
		delay := cg.Content.GetCombineCls().GetStabilizationDelay().AsDuration()
		if next := combo.maxTriggeredTime.Add(delay); next.After(clock.Now(ctx)) {
			return nil, next, nil
		}
	}

	if combo.withNotYetLoadedDeps != nil {
		return rs.postponeDueToNotYetLoadedDeps(ctx, &combo)
	}
	if len(combo.notReady) > 0 {
		return rs.postponeDueToNotReadyCLs(ctx, &combo)
	}

	// At this point all CLs in combo are stable, ready and with valid deps.
	if cg.Content.GetCombineCls() != nil {
		// For multi-CL runs, this means all non-submitted deps are already inside
		// combo.
		if missing := combo.missingDeps(); len(missing) > 0 {
			panic(fmt.Errorf("%s has missing deps %s", combo, missing))
		}
	}
	// Furthermore, since all CLs are ready and related, they must belong to
	// the exact same config group as the initial CL.
	if cgIndexes := combo.configGroupsIndexes(); len(cgIndexes) > 1 {
		panic(fmt.Errorf("%s has >1 config groups: %v", combo, cgIndexes))
	}

	// Check whether combo overlaps with any existing Runs.
	// TODO(tandrii): support >1 concurrent Run on the same CL(s).
	if runs := combo.overlappingRuns(); len(runs) > 0 {
		for runIndex, sharedCLsCount := range runs { // take the first and only Run
			prun := rs.c.GetPruns()[runIndex]
			switch l := len(prun.GetClids()); {
			case l < sharedCLsCount:
				panic("impossible")
			case l > sharedCLsCount:
				// Run's scope is larger or different than this combo. Run Manager will
				// soon be finalizing the Run as not all of its CLs are triggered.
				//
				// This may happen in a many cases of multi-CL Runs, for example:
				//  * during submitted: some CLs have already been submitted;
				//  * during cancellation: some CLs' votes have already been removed;
				//  * a newly ingested LUCI project config splits Run across multiple
				//    ConfigGroups or even makes one CL unwatched by the project.
				return rs.postponeDueToExistingRunDiffScope(ctx, &combo, prun)
			case sharedCLsCount == len(combo.all):
				// The combo scope is exactly the same as Run. This is the most likely
				// situation -- there is nothing for PM to do but wait.
				// Note, that it's possible that Run's mode is different from the combo,
				// in which case Run Manager will be finalizing the Run soon, so wait
				// for notification from Run Manager anyway.
				return nil, time.Time{}, nil
			case sharedCLsCount > len(combo.all):
				panic("impossible")
			default:
				// The combo scope is larger than this Run.
				//
				// CQDaemon in this case aborts existing Run **without** removing the
				// triggering CQ votes, and then immediately starts working on the
				// larger-scoped Run. This isn't what user usually want though, as this
				// usually means re-running all the tryjobs from scratch.
				//
				// TODO(tandrii): decide if it's OK to just purge a CL which isn't in
				// active Run once CV is in charge AND just waiting if CV isn't in
				// charge. This is definitely easier to implement.
				// However, the problem with this potential approach is that if user
				// really wants to stop existing Run of N CLs and start a larger Run on
				// N+1 CLs instead, then user has to first remove all existing CQ votes,
				// and then re-vote on all CLs from scratch. Worse, during the removal,
				// CV/CQDaemon may temporarily see CQ votes on K < N CLs, and since
				// these CQ votes are >> stabilization delay, CV/CQDaemon will happily
				// start a spurious Run on K CLs, and even potentially trigger redundant
				// tryjobs, which won't even be cancelled. Grrr.
				// TODO(tandrii): alternatively, consider canceling the existing Run,
				// similar to CQDaemon.
				return rs.postponeExpandingExistingRunScope(ctx, &combo, prun)
			}
		}
		panic("unreachable")
	}

	rc, err := rs.makeCreator(ctx, &combo, cg)
	if err != nil {
		return nil, time.Time{}, err
	}

	// Check if Run about to be created already exists in order to detect avoid
	// infinite retries if CL triggers are somehow re-used.
	existing := run.Run{ID: rc.ExpectedRunID()}
	switch err := datastore.Get(ctx, &existing); {
	case err == datastore.ErrNoSuchEntity:
		// This is the expected case.
		// NOTE: actual creation may still fail due to a race, and that's fine.
		return rc, time.Time{}, nil
	case err != nil:
		return nil, time.Time{}, errors.Annotate(err, "failed to check for existing Run %q", existing.ID).Tag(transient.Tag).Err()
	case !run.IsEnded(existing.Status):
		// The Run already exists. Most likely another triager called from another
		// TQ was first. Check again in a few seconds, at which point PM should
		// incorporate existing Run into its state.
		logging.Warningf(ctx, "Run %q already exists. If this warning persists, there is a bug in PM which appears to not see this Run", existing.ID)
		return nil, clock.Now(ctx).Add(5 * time.Second), nil
	default:
		since := clock.Since(ctx, existing.EndTime)
		if since < time.Minute {
			logging.Warningf(ctx, "Recently finalized Run %q already exists, will check later", existing.ID)
			return nil, existing.EndTime.Add(time.Minute), nil
		}
		logging.Warningf(ctx, "Run %q already exists, finalized %s ago; will purge CLs with reused triggers", existing.ID, since)
		for _, info := range combo.all {
			info.addPurgeReason(info.pcl.Triggers.GetCqVoteTrigger(), &changelist.CLError{
				Kind: &changelist.CLError_ReusedTrigger_{
					ReusedTrigger: &changelist.CLError_ReusedTrigger{
						Run: string(existing.ID),
					},
				},
			})
		}
		return nil, time.Time{}, nil
	}
}

func (rs *runStage) reverseDeps() map[int64][]int64 {
	if rs.cachedReverseDeps != nil {
		return rs.cachedReverseDeps
	}
	rs.cachedReverseDeps = map[int64][]int64{}
	for clid, info := range rs.cls {
		if info.deps == nil {
			// CL is or will be purged, so its deps weren't even triaged.
			continue
		}
		info.deps.iterateNotSubmitted(info.pcl, func(dep *changelist.Dep) {
			did := dep.GetClid()
			rs.cachedReverseDeps[did] = append(rs.cachedReverseDeps[did], clid)
		})
	}
	return rs.cachedReverseDeps
}

func (rs *runStage) expandComboVisited(info *clInfo, result *combo) {
	if info.deps != nil {
		info.deps.iterateNotSubmitted(info.pcl, func(dep *changelist.Dep) {
			rs.expandCombo(dep.GetClid(), result)
		})
	}
	for _, clid := range rs.reverseDeps()[info.pcl.GetClid()] {
		rs.expandCombo(clid, result)
	}
}

func (rs *runStage) expandCombo(clid int64, result *combo) {
	info := rs.cls[clid]
	if info == nil {
		// Can only happen if clid is a dep that's not yet loaded (otherwise dep
		// would be in this component, and hence info would be set).
		return
	}
	if !rs.markVisited(clid) {
		return
	}
	result.add(info)
	rs.expandComboVisited(info, result)
}

func (rs *runStage) postponeDueToNotYetLoadedDeps(ctx context.Context, combo *combo) (*runcreator.Creator, time.Time, error) {
	// TODO(crbug/1211576): this waiting can last forever. Component needs to
	// record how long it has been waiting and abort with clear message to the
	// user.
	logging.Warningf(ctx, "%s waits for not yet loaded deps", combo)
	return nil, time.Time{}, nil
}

func (rs *runStage) postponeDueToNotReadyCLs(ctx context.Context, combo *combo) (*runcreator.Creator, time.Time, error) {
	// TODO(crbug/1211576): for safety, this should not wait forever.
	logging.Warningf(ctx, "%s waits for not yet ready CLs", combo)
	return nil, time.Time{}, nil
}

func (rs *runStage) postponeDueToExistingRunDiffScope(ctx context.Context, combo *combo, r *prjpb.PRun) (*runcreator.Creator, time.Time, error) {
	// TODO(crbug/1211576): for safety, this should not wait forever.
	logging.Warningf(ctx, "%s is waiting for a differently scoped run %q to finish", combo, r.GetId())
	return nil, time.Time{}, nil
}

func (rs *runStage) postponeExpandingExistingRunScope(ctx context.Context, combo *combo, r *prjpb.PRun) (*runcreator.Creator, time.Time, error) {
	// TODO(crbug/1211576): for safety, this should not wait forever.
	logging.Warningf(ctx, "%s is waiting for smaller scoped run %q to finish", combo, r.GetId())
	return nil, time.Time{}, nil
}

func (rs *runStage) makeCreator(ctx context.Context, combo *combo, cg *prjcfg.ConfigGroup) (*runcreator.Creator, error) {
	latestIndex := -1
	cls := make([]*changelist.CL, len(combo.all))
	for i, info := range combo.all {
		cls[i] = &changelist.CL{ID: common.CLID(info.pcl.GetClid())}
		if info == combo.latestTriggeredByCQVote {
			latestIndex = i
		}
	}
	if err := datastore.Get(ctx, cls); err != nil {
		// Even if one of errors is ErrEntityNotFound, this is a temporary situation as
		// such CL(s) should be removed from PM state soon.
		return nil, errors.Annotate(err, "failed to load CLs").Tag(transient.Tag).Err()
	}

	// Run's owner is whoever owns the latest triggered CL.
	// It's guaranteed to be set because otherwise CL would have been sent for
	// purging and not marked as ready.
	owner, err := cls[latestIndex].Snapshot.OwnerIdentity()
	if err != nil {
		return nil, errors.Annotate(err, "failed to get OwnerIdentity of %d", cls[latestIndex].ID).Err()
	}

	bcls := make([]runcreator.CL, len(cls))
	var opts *run.Options
	for i, cl := range cls {
		pcl := combo.all[i].pcl
		exp, act := pcl.GetEversion(), cl.EVersion
		if exp != act {
			return nil, errors.Annotate(itriager.ErrOutdatedPMState, "CL %d EVersion changed %d => %d", cl.ID, exp, act).Err()
		}
		opts = run.MergeOptions(opts, run.ExtractOptions(cl.Snapshot))

		// Restore email, which Project Manager doesn't track inside PCLs.
		tr := trigger.Find(&trigger.FindInput{
			ChangeInfo:  cl.Snapshot.GetGerrit().GetInfo(),
			ConfigGroup: cg.Content,
		}).GetCqVoteTrigger()
		pclT := pcl.GetTriggers().GetCqVoteTrigger()
		if tr.GetMode() != pclT.GetMode() {
			panic(fmt.Errorf("inconsistent Trigger in PM (%s) vs freshly extracted (%s)", pcl.GetTriggers().GetCqVoteTrigger(), tr))
		}

		bcls[i] = runcreator.CL{
			ID:               common.CLID(pcl.GetClid()),
			ExpectedEVersion: pcl.GetEversion(),
			TriggerInfo:      tr,
			Snapshot:         cl.Snapshot,
		}
	}
	cqTrigger := combo.latestTriggeredByCQVote.pcl.GetTriggers().GetCqVoteTrigger()
	return &runcreator.Creator{
		ConfigGroupID:            cg.ID,
		LUCIProject:              cg.ProjectString(),
		Mode:                     run.Mode(cqTrigger.GetMode()),
		CreateTime:               cqTrigger.GetTime().AsTime(),
		Owner:                    owner,
		Options:                  opts,
		ExpectedIncompleteRunIDs: nil, // no Run is expected
		OperationID:              fmt.Sprintf("PM-%d", mathrand.Int63(ctx)),
		InputCLs:                 bcls,
	}, nil
}

// markVisited makes CL visited if not already and returns if action was taken.
func (rs *runStage) markVisited(clid int64) bool {
	if _, visited := rs.visitedCLs[clid]; visited {
		return false
	}
	rs.visitedCLs[clid] = struct{}{}
	return true
}

// combo is a set of related CLs that will together form a new Run.
//
// The CLs in a combo are a subset of those from the component.
type combo struct {
	all                     []*clInfo
	clids                   map[int64]struct{}
	notReady                []*clInfo
	withNotYetLoadedDeps    *clInfo // nil if none; any one otherwise.
	latestTriggeredByCQVote *clInfo
	maxTriggeredTime        time.Time
}

func (c combo) String() string {
	sb := strings.Builder{}
	sb.WriteString("combo(CLIDs: [")
	for _, a := range c.all {
		fmt.Fprintf(&sb, "%d ", a.pcl.GetClid())
	}
	sb.WriteRune(']')
	if len(c.notReady) > 0 {
		sb.WriteString(" notReady=[")
		for _, a := range c.notReady {
			fmt.Fprintf(&sb, "%d ", a.pcl.GetClid())
		}
		sb.WriteRune(']')
	}
	if c.withNotYetLoadedDeps != nil {
		fmt.Fprintf(&sb, " notYetLoadedDeps of %d [", c.withNotYetLoadedDeps.pcl.GetClid())
		for _, d := range c.withNotYetLoadedDeps.deps.notYetLoaded {
			fmt.Fprintf(&sb, "%d ", d.GetClid())
		}
		sb.WriteRune(']')
	}
	if c.latestTriggeredByCQVote != nil {
		t := c.latestTriggeredByCQVote.pcl.GetTriggers().GetCqVoteTrigger()
		fmt.Fprintf(&sb, " latestTriggered=%d at %s", c.latestTriggeredByCQVote.pcl.GetClid(), t.GetTime().AsTime())
	}
	sb.WriteRune(')')
	return sb.String()
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
	trig := info.pcl.GetTriggers().GetCqVoteTrigger()
	if pb := trig.GetTime(); pb != nil {
		t := pb.AsTime()
		if c.maxTriggeredTime.IsZero() || t.After(c.maxTriggeredTime) {
			c.maxTriggeredTime = t
			c.latestTriggeredByCQVote = info
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

// overlappingRuns returns number of CLs shared with each Run identified by its
// index.
func (c *combo) overlappingRuns() map[int32]int {
	res := map[int32]int{}
	for _, info := range c.all {
		for _, index := range info.runIndexes {
			res[index]++
		}
	}
	return res
}
