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

	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

// Supporter provides limited access to resources of PM state for ease of
// testing and correctness.
type Supporter interface {
	// PCL provides access to State.PB.Pcls w/o exposing entire state.
	//
	// Returns nil if clid refers to a CL not known to PM's State.
	PCL(clid int64) *prjpb.PCL

	// PurgingCL provides access to State.PB.PurgingCLs w/o exposing entire state.
	//
	// Returns nil if given CL isn't being purged.
	PurgingCL(clid int64) *prjpb.PurgingCL

	// ConfigGroup returns a ConfigGroup for a given index of the current LUCI
	// project config version.
	ConfigGroup(index int32) *config.ConfigGroup
}

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

	// runBuilders are prepared by NextActionTime() and executed in Act().
	runBuilders []*prjmanager.RunBuilder
	// toPurge are clids of CLs which should be newly purged.
	toPurge map[int64]struct{}
}

// New returns new Actor.
func New(c *prjpb.Component, s Supporter) *Actor {
	return &Actor{c: c, s: supporterWrapper{s}}
}

// NextActionTime implements componentActor.
func (a *Actor) NextActionTime(ctx context.Context, now time.Time) (time.Time, error) {
	a.triageCLs()
	return a.stageNewRuns(ctx)
}

// Act implements state.componentActor.
func (a *Actor) Act(ctx context.Context) (*prjpb.Component, []*prjpb.PurgeCLTask, error) {
	c := a.c.CloneShallow()
	c.Dirty = false

	switch newPruns, err := a.createRuns(ctx); {
	case err != nil:
		return nil, nil, err
	case len(newPruns) > 0:
		c.Pruns, _ = c.COWPRuns(nil, newPruns)
	}

	// TODO: cancelations
	// TODO: purges
	return c, nil, nil
}

func (a *Actor) createRuns(ctx context.Context) ([]*prjpb.PRun, error) {
	// TODO: implement.
	return nil, nil
}

type supporterWrapper struct {
	Supporter
}

// MustPCL is panics if clid doesn't exist.
//
// Exists primarily for readability.
func (s supporterWrapper) MustPCL(clid int64) *prjpb.PCL {
	if p := s.PCL(clid); p != nil {
		return p
	}
	panic(fmt.Errorf("MustPCL: clid %d not known", clid))
}
