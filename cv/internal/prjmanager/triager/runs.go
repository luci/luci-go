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
	"sort"
	"strings"
	"time"

	"go.chromium.org/luci/auth/identity"
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
	var candidates []*runcreator.Creator

	rs := runStage{
		pm:         pm,
		c:          c,
		cls:        cls,
		visitedCLs: make(map[int64]struct{}, len(cls)),
	}
	// For determinism, iterate in fixed order:
	for _, clid := range c.GetClids() {
		info := cls[clid]
		switch rcs, nt, err := rs.stageNewRunsFrom(ctx, clid, info); {
		case err != nil:
			return nil, time.Time{}, err
		case len(rcs) != 0:
			candidates = append(candidates, rcs...)
		default:
			next = earliest(next, nt)
		}
	}
	if len(candidates) == 0 {
		return nil, next, nil
	}
	final := make([]*runcreator.Creator, 0, len(candidates))
	for _, rc := range candidates {
		if shouldCreateNow, depRuns := rs.resolveDepRuns(ctx, rc); shouldCreateNow {
			rc.DepRuns = append(rc.DepRuns, depRuns...)
			final = append(final, rc)
		}
	}
	return final, next, nil
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

func (rs *runStage) stageNewRunsFrom(ctx context.Context, clid int64, info *clInfo) ([]*runcreator.Creator, time.Time, error) {
	if !info.cqReady && !info.nprReady {
		return nil, time.Time{}, nil
	}
	var runs []*runcreator.Creator
	var retTime time.Time
	if info.cqReady {
		switch cqRuns, t, err := rs.stageNewCQVoteRunsFrom(ctx, clid, info); {
		case err != nil:
			return nil, time.Time{}, err
		default:
			runs = append(runs, cqRuns...)
			retTime = earliest(t, retTime)
		}
	}
	if info.nprReady {
		switch nprRuns, t, err := rs.stageNewPatchsetRunsFrom(ctx, clid, info); {
		case err != nil:
			return nil, time.Time{}, err
		default:
			runs = append(runs, nprRuns...)
			retTime = earliest(t, retTime)
		}
	}
	return runs, retTime, nil
}

func (rs *runStage) stageNewPatchsetRunsFrom(ctx context.Context, clid int64, info *clInfo) ([]*runcreator.Creator, time.Time, error) {
	cgIndex := info.pcl.GetConfigGroupIndexes()[0]
	cg := rs.pm.ConfigGroup(cgIndex)

	combo := &combo{}
	combo.add(info, useNewPatchsetTrigger)

	if runs := combo.overlappingRuns(); len(runs) > 0 {
		for idx := range runs {
			if rs.c.Pruns[idx].Mode == string(run.NewPatchsetRun) {
				// A run exists with the same mode and CL, wait for it to end.
				return nil, time.Time{}, nil
			}
		}
	}

	rc, err := rs.makeCreator(ctx, combo, cg, useNewPatchsetTrigger)
	if err != nil {
		return nil, time.Time{}, err
	}

	switch exists, nextCheck, err := checkExisting(ctx, rc, *combo, useNewPatchsetTrigger); {
	case err != nil:
		return nil, time.Time{}, err
	case !exists:
		return []*runcreator.Creator{rc}, time.Time{}, nil
	default:
		return nil, nextCheck, nil
	}
}

func (rs *runStage) stageNewCQVoteRunsFrom(ctx context.Context, clid int64, info *clInfo) ([]*runcreator.Creator, time.Time, error) {
	// Only start with ready CLs. Non-ready ones can't form new Runs anyway.

	if !rs.markVisited(clid) {
		return nil, time.Time{}, nil
	}

	combo := combo{}
	combo.add(info, useCQVoteTrigger)

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
		for runIndex, sharedCLsCount := range runs {
			prun := rs.c.GetPruns()[runIndex]
			if run.Mode(prun.Mode) == run.NewPatchsetRun {
				continue
			}

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
		// It is normal that a CQ Run "overlaps" a New Patchset Run, treat it as
		// if there were no overlap.
	}

	rc, err := rs.makeCreator(ctx, &combo, cg, useCQVoteTrigger)
	if err != nil {
		return nil, time.Time{}, err
	}

	switch exists, nextCheck, err := checkExisting(ctx, rc, combo, useCQVoteTrigger); {
	case err != nil:
		return nil, time.Time{}, err
	case !exists:
		return []*runcreator.Creator{rc}, time.Time{}, nil
	default:
		return nil, nextCheck, nil
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
	result.add(info, useCQVoteTrigger)
	rs.expandComboVisited(info, result)
}

func (rs *runStage) postponeDueToNotYetLoadedDeps(ctx context.Context, combo *combo) ([]*runcreator.Creator, time.Time, error) {
	// TODO(crbug/1211576): this waiting can last forever. Component needs to
	// record how long it has been waiting and abort with clear message to the
	// user.
	logging.Warningf(ctx, "%s waits for not yet loaded deps", combo)
	return nil, time.Time{}, nil
}

func (rs *runStage) postponeDueToNotReadyCLs(ctx context.Context, combo *combo) ([]*runcreator.Creator, time.Time, error) {
	// TODO(crbug/1211576): for safety, this should not wait forever.
	logging.Warningf(ctx, "%s waits for not yet ready CLs", combo)
	return nil, time.Time{}, nil
}

func (rs *runStage) postponeDueToExistingRunDiffScope(ctx context.Context, combo *combo, r *prjpb.PRun) ([]*runcreator.Creator, time.Time, error) {
	// TODO(crbug/1211576): for safety, this should not wait forever.
	logging.Warningf(ctx, "%s is waiting for a differently scoped run %q to finish", combo, r.GetId())
	return nil, time.Time{}, nil
}

func (rs *runStage) postponeExpandingExistingRunScope(ctx context.Context, combo *combo, r *prjpb.PRun) ([]*runcreator.Creator, time.Time, error) {
	// TODO(crbug/1211576): for safety, this should not wait forever.
	logging.Warningf(ctx, "%s is waiting for smaller scoped run %q to finish", combo, r.GetId())
	return nil, time.Time{}, nil
}

func (rs *runStage) makeCreator(ctx context.Context, combo *combo, cg *prjcfg.ConfigGroup, chooseTrigger func(ts *run.Triggers) *run.Trigger) (*runcreator.Creator, error) {
	latestIndex := -1
	cls := make([]*changelist.CL, len(combo.all))
	for i, info := range combo.all {
		cls[i] = &changelist.CL{ID: common.CLID(info.pcl.GetClid())}
		if info == combo.latestTriggered {
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
	var incompleteRuns common.RunIDs
	for i, cl := range cls {
		for _, ri := range combo.all[i].runIndexes {
			incompleteRuns = append(incompleteRuns, common.RunID(rs.c.Pruns[ri].Id))
		}
		pcl := combo.all[i].pcl
		exp, act := pcl.GetEversion(), cl.EVersion
		if exp != act {
			return nil, errors.Annotate(itriager.ErrOutdatedPMState, "CL %d EVersion changed %d => %d", cl.ID, exp, act).Err()
		}
		opts = run.MergeOptions(opts, run.ExtractOptions(cl.Snapshot))

		// Restore email, which Project Manager doesn't track inside PCLs.
		tr := chooseTrigger(trigger.Find(&trigger.FindInput{
			ChangeInfo:                   cl.Snapshot.GetGerrit().GetInfo(),
			ConfigGroup:                  cg.Content,
			TriggerNewPatchsetRunAfterPS: cl.TriggerNewPatchsetRunAfterPS,
		}))
		pclT := chooseTrigger(pcl.GetTriggers())
		if tr.GetMode() != pclT.GetMode() {
			panic(fmt.Errorf("inconsistent Trigger in PM (%s) vs freshly extracted (%s)", pclT, tr))
		}

		bcls[i] = runcreator.CL{
			ID:               common.CLID(pcl.GetClid()),
			ExpectedEVersion: pcl.GetEversion(),
			TriggerInfo:      tr,
			Snapshot:         cl.Snapshot,
		}
	}
	t := chooseTrigger(combo.latestTriggered.pcl.GetTriggers())
	triggererIdentity, err := identity.MakeIdentity(fmt.Sprintf("%s:%s", identity.User, t.GetEmail()))
	if err != nil {
		return nil, errors.Annotate(err, "failed to construct triggerer identity of %s", t.GetEmail()).Err()
	}
	sort.Sort(incompleteRuns)
	payer := quotaPayer(cls[latestIndex], owner, triggererIdentity, t)
	return &runcreator.Creator{
		ConfigGroupID:            cg.ID,
		LUCIProject:              cg.ProjectString(),
		Mode:                     run.Mode(t.GetMode()),
		ModeDefinition:           t.GetModeDefinition(),
		CreateTime:               t.GetTime().AsTime(),
		Owner:                    owner,
		CreatedBy:                triggererIdentity,
		BilledTo:                 payer,
		Options:                  opts,
		ExpectedIncompleteRunIDs: incompleteRuns,
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
	all                  []*clInfo
	clids                map[int64]struct{}
	notReady             []*clInfo
	withNotYetLoadedDeps *clInfo // nil if none; any one otherwise.
	latestTriggered      *clInfo
	latestTrigger        *run.Trigger
	maxTriggeredTime     time.Time
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
	if c.latestTriggered != nil {
		t := c.latestTrigger
		fmt.Fprintf(&sb, " latestTriggered=%d at %s", c.latestTriggered.pcl.GetClid(), t.GetTime().AsTime())
	}
	sb.WriteRune(')')
	return sb.String()
}

func (c *combo) add(info *clInfo, chooseTrigger func(ts *run.Triggers) *run.Trigger) {
	c.all = append(c.all, info)
	if c.clids == nil {
		c.clids = map[int64]struct{}{info.pcl.GetClid(): {}}
	} else {
		c.clids[info.pcl.GetClid()] = struct{}{}
	}

	if !info.cqReady {
		c.notReady = append(c.notReady, info)
	}

	if info.deps != nil && len(info.deps.notYetLoaded) > 0 {
		c.withNotYetLoadedDeps = info
	}
	trig := chooseTrigger(info.pcl.GetTriggers())
	if pb := trig.GetTime(); pb != nil {
		t := pb.AsTime()
		if c.maxTriggeredTime.IsZero() || t.After(c.maxTriggeredTime) {
			c.maxTriggeredTime = t
			c.latestTriggered = info
			c.latestTrigger = trig
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

// checkExisting checks whether the run to create already exists.
//
// if it does exist, it decides when to triage again.
func checkExisting(ctx context.Context, rc *runcreator.Creator, combo combo, useTrigger func(*run.Triggers) *run.Trigger) (bool, time.Time, error) {
	// Check if Run about to be created already exists in order to detect avoid
	// infinite retries if CL triggers are somehow re-used.
	existing := run.Run{ID: rc.ExpectedRunID()}
	switch err := datastore.Get(ctx, &existing); {
	case err == datastore.ErrNoSuchEntity:
		// This is the expected case.
		// NOTE: actual creation may still fail due to a race, and that's fine.
		return false, time.Time{}, nil
	case err != nil:
		return false, time.Time{}, errors.Annotate(err, "failed to check for existing Run %q", existing.ID).Tag(transient.Tag).Err()
	case !run.IsEnded(existing.Status):
		// The Run already exists. Most likely another triager called from another
		// TQ was first. Check again in a few seconds, at which point PM should
		// incorporate existing Run into its state.
		logging.Warningf(ctx, "Run %q already exists. If this warning persists, there is a bug in PM which appears to not see this Run", existing.ID)
		return true, clock.Now(ctx).Add(5 * time.Second), nil
	default:
		since := clock.Since(ctx, existing.EndTime)
		if since < time.Minute {
			logging.Warningf(ctx, "Recently finalized Run %q already exists, will check later", existing.ID)
			return true, existing.EndTime.Add(time.Minute), nil
		}
		logging.Warningf(ctx, "Run %q already exists, finalized %s ago; will purge CLs with reused triggers", existing.ID, since)
		for _, info := range combo.all {
			info.addPurgeReason(useTrigger(info.pcl.Triggers), &changelist.CLError{
				Kind: &changelist.CLError_ReusedTrigger_{
					ReusedTrigger: &changelist.CLError_ReusedTrigger{
						Run: string(existing.ID),
					},
				},
			})
		}
		return true, time.Time{}, nil
	}
}

func useNewPatchsetTrigger(ts *run.Triggers) *run.Trigger {
	return ts.GetNewPatchsetRunTrigger()
}

func useCQVoteTrigger(ts *run.Triggers) *run.Trigger {
	return ts.GetCqVoteTrigger()
}

func (rs *runStage) findImmediateHardDeps(pcl *prjpb.PCL) []int64 {
	// TODO: use Snapshot.GitDeps.Immediate to find the immediate hard deps.
	candidates := make(map[int64]struct{})
	notCandidates := make(map[int64]struct{})

	// Iterate the deps in reverse, because child CLs have higher CLIDs than
	// its deps in most cases (but not guaranteed)
	for i := len(pcl.GetDeps()) - 1; i >= 0; i-- {
		dep := pcl.GetDeps()[i]
		if _, exist := notCandidates[dep.GetClid()]; exist {
			continue
		}
		if dep.GetKind() == changelist.DepKind_HARD {
			candidates[dep.GetClid()] = struct{}{}

			var dpcl *prjpb.PCL
			switch info, exist := rs.cls[dep.GetClid()]; {
			case exist:
				dpcl = info.pcl
			default:
				// Panic if the dep PCL is unknown.
				//
				// If a dep is unknown, the dep triager shouldn't mark the CL
				// as cqReady. If it did, there must be a bug.
				dpcl = rs.pm.MustPCL(dep.GetClid())
			}
			// none of its deps can be a candidate.
			for _, depdep := range dpcl.GetDeps() {
				notCandidates[depdep.GetClid()] = struct{}{}
				delete(candidates, depdep.GetClid())
			}
		}
	}

	var clids []int64
	if len(candidates) > 0 {
		clids = make([]int64, 0, len(candidates))
		for clid := range candidates {
			clids = append(clids, clid)
		}
	}
	sort.Slice(clids, func(i, j int) bool {
		return clids[i] < clids[j]
	})
	return clids
}

func (rs *runStage) resolveDepRuns(ctx context.Context, rc *runcreator.Creator) (shouldCreateNow bool, depRuns common.RunIDs) {
	info, ok := rs.cls[int64(rc.InputCLs[0].ID)]
	if !ok {
		panic(fmt.Errorf("resolveDepRuns: rc has a CL %d, not tracked in the component",
			rc.InputCLs[0].ID))
	}
	ctx = logging.SetField(ctx, "cl", info.pcl.GetClid())

	var ideps []int64
	switch cg := rs.pm.ConfigGroup(info.pcl.GetConfigGroupIndexes()[0]); {
	case rc.Mode != run.Mode(run.FullRun), cg.Content.GetCombineCls() != nil:
		// The CL should be cqReady. Otherwise, `rc` wouldn't be created.
		// Unless, it's with combine_cl, this should return true to let the run
		// be created.
		return true, nil
	default:
		ideps = rs.findImmediateHardDeps(info.pcl)
		if len(ideps) == 0 {
			logging.Debugf(ctx, "resolvDepRuns: no immediate hard deps found")
			return true, nil
		}
		logging.Debugf(ctx, "resolveDepRuns: immediate hard-deps %v", ideps)
	}

	shouldCreateNow = true // be optimistic
	for _, idep := range ideps {
		switch dPCL := rs.pm.MustPCL(idep); {
		case dPCL.GetSubmitted():
			continue
		case dPCL.GetStatus() != prjpb.PCL_OK:
			// If the dep status is not PCL_OK, the dep triager should have put
			// it in notYetLoaded or invalidDeps.Unwatched.
			//
			// Therefore, the origin CL must not be cqReady and
			// runcreator.Creator() should have not been created.
			panic(fmt.Errorf("resolveDepRuns: depCL %d has status %q", dPCL.GetClid(), dPCL.GetStatus()))
		}
		switch dinfo, ok := rs.cls[idep]; {
		case !ok:
			// The dep is not in the same component.
			// All hard-deps are supposed to be tracked in the same component,
			// this could still check the other component and the run status.
			//
			// Let's ignore now. The submission will be rejected at the worst
			// case.
			// TODO(ddoman): check if there is a Run triggered from
			// the dep's CQ vote, either completed or not.
			logging.Errorf(ctx, "resolveDepRuns: a HARD dep CL %d is not tracked in the same component", idep)
			continue
		case dinfo.runCountByMode[run.FullRun] > 0:
			for _, idx := range dinfo.runIndexes {
				if prun := rs.c.GetPruns()[idx]; prun.GetMode() == string(run.FullRun) {
					depRuns = append(depRuns, common.RunID(prun.GetId()))
				}
			}
		default:
			shouldCreateNow = false
		}
	}
	return shouldCreateNow, depRuns
}

func quotaPayer(cl *changelist.CL, owner, triggerer identity.Identity, t *run.Trigger) identity.Identity {
	switch owner {
	case "":
		panic(fmt.Errorf("CL %d: empty owner was given: %q", cl.ID, owner))
	case identity.AnonymousIdentity:
		panic(fmt.Errorf("CL %d: the CL owner is anonymous", cl.ID))
	}
	mode := run.Mode(t.GetMode())
	cqv := t.GetModeDefinition().GetCqLabelValue()
	switch {
	case mode != run.FullRun && cqv != trigger.CQVoteByMode(run.FullRun):
		return triggerer
	case trigger.HasAutoSubmit(cl.Snapshot.GetGerrit().GetInfo()):
		return owner
	default:
		return triggerer
	}
}
