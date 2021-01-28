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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

// cAction is a component action.
//
// An action may involve creating one or more new Runs,
// or removing CQ votes from CL(s) which can't form a Run for some reason.
type cAction struct {
	componentIndex int
	actor          componentActor
}

const concurrentComponentProcessing = 16

// prepareComponentActions returns actions to be taken on components.
//
// Doesn't modify state.
func (s *State) prepareComponentActions(ctx context.Context) ([]cAction, error) {
	// First, do quick serial evaluation of components. Most frequently, this
	// should result in no actions.
	pclGetter := s.makePCLGetter()
	evaluator := s.testComponentPreevaluator
	if evaluator == nil {
		// TODO(tandrii): add production implementation.
		return nil, nil
	}
	var potential []cAction
	now := clock.Now(ctx)
	for i, c := range s.PB.GetComponents() {
		if actor, skip := evaluator.shouldEvaluate(now, pclGetter, c); !skip {
			potential = append(potential, cAction{i, actor})
		}
	}
	if len(potential) == 0 {
		return nil, nil
	}

	var required []cAction
	var mutex sync.Mutex

	poolSize := concurrentComponentProcessing
	if n := len(potential); n < poolSize {
		poolSize = n
	}
	errs := parallel.WorkPool(poolSize, func(work chan<- func() error) {
		for _, a := range potential {
			a := a
			work <- func() error {
				switch shouldAct, err := a.actor.shouldAct(ctx); {
				case err != nil:
					return err
				case shouldAct:
					mutex.Lock()
					required = append(required, a)
					mutex.Unlock()
				}
				return nil
			}
		}
	})
	if errs == nil {
		return required, nil
	}
	sharedMsg := fmt.Sprintf("shouldAct on %d, failed to check %d", len(required), len(errs.(errors.MultiError)))
	severe := common.MostSevereError(errs)
	if len(required) == 0 {
		return nil, errors.Annotate(severe, sharedMsg+", keeping the most severe error").Err()
	}
	// Components are independent, so proceed since partial progress is better
	// than none.
	logging.Warningf(ctx, "%s (most severe error %s), proceeding to act", sharedMsg, severe)
	// TODO(tandrii): ensure PM is re-triggered to reeval components with
	// failures.
	return required, nil
}

// execComponentActions executes actions on components.
func (s *State) execComponentActions(ctx context.Context, actions []cAction) error {
	components := make([]*prjpb.Component, len(s.PB.GetComponents()))
	copy(components, s.PB.GetComponents())
	var modified int32

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
				switch newC, err = a.actor.act(ctx); {
				case err != nil:
				case newC != oldC:
					components[a.componentIndex] = newC
					atomic.AddInt32(&modified, 1)
				}
				return
			}
		}
	})
	s.PB.Components = components
	if errs == nil {
		return nil
	}
	failed := len(errs.(errors.MultiError))
	severe := common.MostSevereError(errs)
	sharedMsg := fmt.Sprintf(
		"acted on components: succeded %d (modified %d), failed %d",
		len(actions)-failed, modified, failed)
	if modified == 0 {
		return errors.Annotate(severe, sharedMsg+", keeping the most severe error").Err()
	}
	// Components are independent, so proceed since partial progress is better
	// than none.
	logging.Warningf(ctx, "%s (most severe error %s)", sharedMsg, severe)
	// TODO(tandrii): ensure PM is re-triggered to reeval components which had
	// failures.
	return nil
}

// componentPreevaluator checks if a component has to be deeply evaluated.
type componentPreevaluator interface {
	// shouldEvaluate quickly checks if action may be necessary on a component.
	//
	// If so, returns a new componentActor and false.
	// Otherwise, returns nil, true to indicate that no action is necessary.
	//
	// The given component may only be modified in copy-on-write way.
	shouldEvaluate(now time.Time, p pclGetter, c *prjpb.Component) (ce componentActor, skip bool)
}

// componentActor evaluates and acts on a single component.
type componentActor interface {
	// shouldAct returns true if component requires an action.
	//
	// Called outside of any Datastore transaction.
	// May perform its own Datastore operations.
	shouldAct(ctx context.Context) (bool, error)

	// act executes the component action.
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
	act(ctx context.Context) (*prjpb.Component, error)
}

// pclGetter provides access to State.PB.Pcls w/o exposing entire state.
//
// Returns nil if clid refers to CL not known to PM's State.
type pclGetter func(clid int64) *prjpb.PCL

// makePCLGetter returns pclGetter for this State.
func (s *State) makePCLGetter() pclGetter {
	s.ensurePCLIndex()
	index := s.pclIndex
	pcls := s.PB.GetPcls()
	return func(clid int64) *prjpb.PCL {
		i, ok := index[common.CLID(clid)]
		if !ok {
			return nil
		}
		return pcls[i]
	}
}
