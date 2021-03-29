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
	"go.chromium.org/luci/cv/internal/prjmanager/runcreator"
	"go.chromium.org/luci/cv/internal/run"
)

// stageNewRuns sets .runBuilders if new Runs have to be created.
//
// Otherwise, returns the time when this could be done.
// NOTE: because this function may take considerable time, the returned time may
// be already in the past.
//
// Guarantees that a CL can be in at most 1 RunBuilder. This avoids conflicts
// whereby 2+ Runs need to modify the same CL entity.
func (a *Actor) stageNewRuns(ctx context.Context) (time.Time, error) {
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

func (a *Actor) stageNewRunsFrom(ctx context.Context, clid int64, info *clInfo) (time.Time, error) {
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

func (a *Actor) stageNewRunsSingle(ctx context.Context, info *clInfo, cg *config.ConfigGroup) (time.Time, error) {
	if len(info.runIndexes) > 0 {
		// Singular case today doesn't support concurrent runs.
		return time.Time{}, nil
	}
	if len(info.deps.notYetLoaded) > 0 {
		return a.postponeDueNotYetLoadedDeps(ctx, info)
	}

	combo := combo{}
	combo.add(info)
	rb, err := a.makeRunBuilder(ctx, &combo, cg)
	if err != nil {
		return time.Time{}, err
	}
	a.runBuilders = append(a.runBuilders, rb)
	return clock.Now(ctx), nil
}

func (a *Actor) stageNewRunsCombo(ctx context.Context, info *clInfo, cg *config.ConfigGroup) (time.Time, error) {
	//TODO(tandrii): implement
	return time.Time{}, nil
}

func (a *Actor) postponeDueNotYetLoadedDeps(ctx context.Context, info *clInfo) (time.Time, error) {
	// TODO(tandrii): for safety, this should not wait forever.
	sb := strings.Builder{}
	fmt.Fprintf(&sb, "combo with %v waiting on %d deps:", info, len(info.deps.notYetLoaded))
	for _, d := range info.deps.notYetLoaded {
		fmt.Fprintf(&sb, " %d", d.GetClid())
	}
	logging.Warningf(ctx, sb.String())
	return time.Time{}, nil
}

func (a *Actor) makeRunBuilder(ctx context.Context, combo *combo, cg *config.ConfigGroup) (*runcreator.RunBuilder, error) {
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

	bcls := make([]runcreator.RunBuilderCL, len(cls))
	for i, cl := range cls {
		pcl := combo.all[i].pcl
		bcls[i] = runcreator.RunBuilderCL{
			ID:               common.CLID(pcl.GetClid()),
			ExpectedEVersion: int(pcl.GetEversion()),
			TriggerInfo:      pcl.GetTrigger(),
			Snapshot:         cl.Snapshot,
		}
	}

	return &runcreator.RunBuilder{
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
func (a *Actor) markVisited(clid int64) bool {
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
		c.clids = map[int64]struct{}{info.pcl.GetClid(): struct{}{}}
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
