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

	"go.chromium.org/luci/cv/internal/prjmanager/impl/state/itriager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
	"go.chromium.org/luci/cv/internal/prjmanager/runcreator"
)

// Triage triages a component with 1+ CLs deciding what has to be done now and
// when should the next re-Triage happen.
//
// Meets the `itriager.Triage` requirements.
func Triage(ctx context.Context, c *prjpb.Component, s itriager.PMState) (itriager.Result, error) {
	res := itriager.Result{}

	a := actor{c: c, s: pmState{s}}
	a.triageCLs()
	whenPurge := a.stagePurges(ctx, clock.Now(ctx))
	whenNewRun, err := a.stageNewRuns(ctx)
	if err != nil {
		return res, err
	}

	if len(a.runCreators) > 0 || len(a.purgeCLtasks) > 0 {
		res.RunsToCreate = a.runCreators
		res.CLsToPurge = a.purgeCLtasks
		res.NewValue = c.CloneShallow()
		res.NewValue.Dirty = false
		// Wait for the Run Creation or the CL Purging to finish, which will result
		// in an event sent to the PM, which will result in a re-Triage.
		res.NewValue.DecisionTime = nil
		return res, nil
	}

	when := earliest(whenPurge, whenNewRun)
	if c.GetDirty() || !isSameTime(when, c.GetDecisionTime()) {
		res.NewValue = c.CloneShallow()
		res.NewValue.Dirty = false
		res.NewValue.DecisionTime = nil
		if !when.IsZero() {
			res.NewValue.DecisionTime = timestamppb.New(when)
		}
	}
	return res, nil
}

type actor struct {
	c *prjpb.Component
	// TODO(tandrii): rename to pm after merging multi-CL Run creation.
	s pmState

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

type pmState struct {
	itriager.PMState
}

// MustPCL panics if clid doesn't exist.
//
// Exists primarily for readability.
func (pm pmState) MustPCL(clid int64) *prjpb.PCL {
	if p := pm.PCL(clid); p != nil {
		return p
	}
	panic(fmt.Errorf("MustPCL: clid %d not known", clid))
}

// earliest returns the earliest of the non-zero time instances.
//
// Returns zero time.Time if no non-zero instances were given.
func earliest(ts ...time.Time) time.Time {
	var res time.Time
	var remaining []time.Time
	// Find first non-zero.
	for i, t := range ts {
		if !t.IsZero() {
			res = t
			remaining = ts[i+1:]
			break
		}
	}
	for _, t := range remaining {
		// res is guaranteed non-zero if this loop iterates.
		if !t.IsZero() && t.Before(res) {
			res = t
		}
	}
	return res
}

func isSameTime(t time.Time, pb *timestamppb.Timestamp) bool {
	if pb == nil {
		return t.IsZero()
	}
	return pb.AsTime().Equal(t)
}
