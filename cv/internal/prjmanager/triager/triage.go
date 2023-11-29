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
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/cv/internal/prjmanager/itriager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

// Triage triages a component with 1+ CLs deciding what has to be done now and
// when should the next re-Triage happen.
//
// Meets the `itriager.Triage` requirements.
func Triage(ctx context.Context, c *prjpb.Component, s itriager.PMState) (itriager.Result, error) {
	pm := pmState{s}
	res := itriager.Result{}
	var nextPurge, nextRun time.Time
	var err error

	cls := triageCLs(ctx, c, pm)
	res.RunsToCreate, nextRun, err = stageNewRuns(ctx, c, cls, pm)
	if err != nil {
		return res, err
	}
	res.CLsToPurge, nextPurge = stagePurges(ctx, cls, pm)
	res.CLsToTriggerDeps = stageTriggerCLDeps(ctx, cls, pm)

	if len(res.RunsToCreate) > 0 || len(res.CLsToPurge) > 0 || len(res.CLsToTriggerDeps) > 0 {
		res.NewValue = c.CloneShallow()
		res.NewValue.TriageRequired = false
		// Wait for any of Run creation, CL purging, or CL triggering to finish,
		// which will result in a re-Triage.
		res.NewValue.DecisionTime = nil
		return res, nil
	}

	next := earliest(nextPurge, nextRun)
	if c.GetTriageRequired() || !isSameTime(next, c.GetDecisionTime()) {
		res.NewValue = c.CloneShallow()
		res.NewValue.TriageRequired = false
		res.NewValue.DecisionTime = nil
		if !next.IsZero() {
			res.NewValue.DecisionTime = timestamppb.New(next)
		}
	}
	return res, nil
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
