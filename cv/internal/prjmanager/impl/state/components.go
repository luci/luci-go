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
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/prjmanager/impl/state/componentactor"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

// earliestDecisionTime returns the earliest decision time of all components.
//
// Returns the same time as time.Time and proto.
//
// Re-uses DecisionTime of one of the components, assuming that components are
// modified copy-on-write.
func earliestDecisionTime(cs []*prjpb.Component) (time.Time, *timestamppb.Timestamp) {
	var ret time.Time
	var retPB *timestamppb.Timestamp
	for _, c := range cs {
		if dt := c.GetDecisionTime(); dt != nil {
			if t := dt.AsTime(); ret.IsZero() || ret.After(t) {
				ret = t
				retPB = dt
			}
		}
	}
	return ret, retPB
}

// cAction is a component action to be taken during current PM mutation.
//
// An action may involve creating one or more new Runs,
// or removing CQ votes from CL(s) which can't form a Run for some reason.
type cAction struct {
	componentIndex int
	actor          componentActor
}

const concurrentComponentProcessing = 16

// scanComponents checks if any immediate actions have be taken on components.
//
// Doesn't modify state itself. If any components are modified or actions are to
// be taken, then allocates a new component slice.
// Otherwise, returns nil, nil, nil.
func (s *State) scanComponents(ctx context.Context) ([]cAction, []*prjpb.Component, error) {
	supporter, err := s.makeActorSupporter(ctx)
	if err != nil {
		return nil, nil, err
	}

	out := make([]*prjpb.Component, len(s.PB.GetComponents()))
	var modified int32
	var mutex sync.Mutex
	var actions []cAction
	now := clock.Now(ctx)

	poolSize := concurrentComponentProcessing
	if n := len(s.PB.GetComponents()); n < poolSize {
		poolSize = n
	}
	errs := parallel.WorkPool(poolSize, func(work chan<- func() error) {
		for i, oldC := range s.PB.GetComponents() {
			out[i] = oldC
			var oldWhen time.Time
			canSkip := !oldC.GetDirty()
			if t := oldC.GetDecisionTime(); t != nil {
				oldWhen = t.AsTime()
				if !oldWhen.After(now) {
					canSkip = false
				}
			}
			if canSkip {
				continue
			}

			i, oldC, oldWhen := i, oldC, oldWhen
			work <- func() error {
				var actor componentActor = componentactor.New(oldC, supporter)
				if s.testComponentActorFactory != nil {
					actor = s.testComponentActorFactory(oldC, supporter)
				}
				switch when, err := actor.NextActionTime(ctx, now); {
				case err != nil:
					// Ensure this component is reconsidered during then next PM mutation.
					atomic.AddInt32(&modified, 1)
					out[i] = oldC.CloneShallow()
					out[i].DecisionTime = timestamppb.New(now)
					return err

				case when == now:
					mutex.Lock()
					actions = append(actions, cAction{i, actor})
					mutex.Unlock()

				case when != oldWhen || oldC.GetDirty():
					atomic.AddInt32(&modified, 1)
					out[i] = oldC.CloneShallow()
					out[i].Dirty = false
					if when.IsZero() {
						out[i].DecisionTime = nil
					} else {
						out[i].DecisionTime = timestamppb.New(when)
					}
				}
				return nil
			}
		}
	})

	if len(actions) == 0 && modified == 0 {
		out = nil // no mutations necessary
	}
	if errs == nil {
		return actions, out, nil
	}
	sharedMsg := fmt.Sprintf("scanComponents(%d): %d errors, %d actions now, %d modified",
		len(s.PB.GetComponents()), len(errs.(errors.MultiError)), len(actions), modified)
	severe := common.MostSevereError(errs)
	if len(actions) == 0 {
		return nil, nil, errors.Annotate(severe, sharedMsg+", keeping the most severe error").Err()
	}
	// Components are independent, so proceed since partial progress is better
	// than none.
	logging.Warningf(ctx, "%s (most severe error %s), proceeding to act", sharedMsg, severe)
	return actions, out, nil
}

// execComponentActions executes actions on components.
//
// Modifies passed component slice in place.
// Modifies individuals components via copy-on-write.
func (s *State) execComponentActions(ctx context.Context, actions []cAction, components []*prjpb.Component) error {
	var errModified int32
	var okModified int32

	poolSize := concurrentComponentProcessing
	if l := len(actions); l < poolSize {
		poolSize = l
	}
	errs := parallel.WorkPool(poolSize, func(work chan<- func() error) {
		for _, a := range actions {
			a := a
			work <- func() (err error) {
				oldC := components[a.componentIndex]
				var newC *prjpb.Component
				switch newC, err = a.actor.Act(ctx); {
				case err != nil:
					// Ensure this component is reconsidered during then next PM mutation.
					newC = components[a.componentIndex].CloneShallow()
					newC.DecisionTime = timestamppb.New(clock.Now(ctx))
					components[a.componentIndex] = newC
					atomic.AddInt32(&errModified, 1)
				case newC != oldC:
					components[a.componentIndex] = newC
					atomic.AddInt32(&okModified, 1)
				}
				return
			}
		}
	})
	if errs == nil {
		return nil
	}
	severe := common.MostSevereError(errs)
	sharedMsg := fmt.Sprintf(
		"acted on components: succeded %d (ok-modified %d), failed %d",
		int32(len(actions))-errModified, okModified, errModified)
	if okModified == 0 {
		return errors.Annotate(severe, sharedMsg+", keeping the most severe error").Err()
	}
	// Components are independent, so proceed since partial progress is better
	// than none.
	logging.Warningf(ctx, "%s (most severe error %s)", sharedMsg, severe)
	// For failed components, their DecisionTime is set `now` by scanComponents,
	// thus PM will reconsider them as soon possible.
	return nil
}

func (s *State) makeActorSupporter(ctx context.Context) (*actorSupporterImpl, error) {
	if s.configGroups == nil {
		meta, err := config.GetHashMeta(ctx, s.PB.GetLuciProject(), s.PB.GetConfigHash())
		if err != nil {
			return nil, err
		}
		if s.configGroups, err = meta.GetConfigGroups(ctx); err != nil {
			return nil, err
		}
	}
	s.ensurePCLIndex()
	purging := make(map[int64]*prjpb.PurgingCL, len(s.PB.GetPurgingCls()))
	for _, p := range s.PB.GetPurgingCls() {
		purging[p.GetClid()] = p
	}
	return &actorSupporterImpl{
		pcls:         s.PB.GetPcls(),
		pclIndex:     s.pclIndex,
		purging:      purging,
		configGroups: s.configGroups,
	}, nil
}

type actorSupporterImpl struct {
	pcls         []*prjpb.PCL
	pclIndex     map[common.CLID]int
	purging      map[int64]*prjpb.PurgingCL
	configGroups []*config.ConfigGroup
}

func (a *actorSupporterImpl) PCL(clid int64) *prjpb.PCL {
	i, ok := a.pclIndex[common.CLID(clid)]
	if !ok {
		return nil
	}
	return a.pcls[i]
}

func (a *actorSupporterImpl) PurgingCL(clid int64) *prjpb.PurgingCL {
	return a.purging[clid]
}

func (a *actorSupporterImpl) ConfigGroup(index int32) *config.ConfigGroup {
	return a.configGroups[index]
}

// componentActor evaluates and acts on a single component.
type componentActor interface {
	// NextActionTime returns time when the component action has to be taken.
	//
	// If action is necessary now, must return now as passed.
	// If action may be necessary later, must return when.
	// Must return zero value of time.Time to indicate that no action is necessary
	// until an incoming event.
	//
	// Called outside of any Datastore transaction.
	// May perform its own Datastore operations.
	NextActionTime(ctx context.Context, now time.Time) (time.Time, error)

	// Act executes the component action.
	//
	// Called if and only if shouldAct() returned true.
	//
	// Called outside of any Datastore transaction, and notably before the a
	// transaction on PM state.
	//
	// Must return either original component OR copy-on-write modification.
	// If modified, the new component value is **best-effort** saved in the follow
	// up Datastore transaction on PM state, success of which must not be relied
	// upon.
	//
	// If error is not nil, the potentially modified component is ignored.
	//
	// TODO(tandrii): support purge CL and cancel Run actions.
	Act(ctx context.Context) (*prjpb.Component, error)
}
