// Copyright 2020 The LUCI Authors.
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

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/gerrit/cfgmatcher"
	"go.chromium.org/luci/cv/internal/gerrit/poller"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/internal"
)

// State is a state of Project Manager.
//
// The state object must not be re-used except for serializing public state
// after its public methods returned a modified State or an error.
// This allows for efficient evolution of cached helper datastructures which
// would other have to be copied, too.
//
// To illustrate correct and incorrect usages:
//     s0 := NewExisting(...)
//     s1, _, err := s0.Mut1()
//     if err != nil {
//       // ... := s0.Mut2()             // NOT OK, 2nd call on s0
//       return proto.Marshal(s0.PB)     // OK
//     }
//     //  ... := s0.Mut2()              // NOT OK, 2nd call on s0
//     s2, _, err := s1.Mut2()           // OK, s1 may be s0 if Mut1() was noop
//     if err != nil {
//       // return proto.Marshal(s0.PB)  // OK
//       return proto.Marshal(s0.PB)     // OK
//     }
type State struct {

	// Serializable part mutated using copy-on-write approach
	// https://en.wikipedia.org/wiki/Copy-on-write

	Status prjmanager.Status
	PB     *internal.PState

	// Helper private fields used during mutations.

	// alreadyCloned is set to true after state is cloned to prevent incorrect
	// usage.
	alreadyCloned bool

	// cfgMatcher is lazily created, cached, and passed on to State clones.
	cfgMatcher *cfgmatcher.Matcher

	// pclIndex provides O(1) check if PCL exists for a CL.
	//
	// lazily created, see ensurePCLIndex().
	pclIndex pclIndex // CLID => index.
}

// NewInitial returns initial state at the start of PM's lifetime.
func NewInitial(luciProject string) *State {
	return &State{
		Status: prjmanager.Status_STATUS_UNSPECIFIED,
		PB:     &internal.PState{LuciProject: luciProject},
	}
}

// NewExisting returns state from its parts.
func NewExisting(status prjmanager.Status, pb *internal.PState) *State {
	return &State{
		Status: status,
		PB:     pb,
	}
}

// UpdateConfig updates PM to latest config version.
func (s *State) UpdateConfig(ctx context.Context) (*State, SideEffect, error) {
	s.ensureNotYetCloned()

	meta, err := config.GetLatestMeta(ctx, s.PB.GetLuciProject())
	if err != nil {
		return nil, nil, err
	}

	switch meta.Status {
	case config.StatusEnabled:
		if s.Status == prjmanager.Status_STARTED && meta.Hash() == s.PB.GetConfigHash() {
			return s, nil, nil // already up-to-date.
		}

		// Tell poller to update ASAP. It doesn't need to wait for a transaction as
		// it's OK for poller to be temporarily more up-to-date than PM.
		if err := poller.Poke(ctx, s.PB.GetLuciProject()); err != nil {
			return nil, nil, err
		}

		s = s.cloneShallow()
		s.Status = prjmanager.Status_STARTED
		s.PB.ConfigHash = meta.Hash()
		s.PB.ConfigGroupNames = meta.ConfigGroupNames
		if s.cfgMatcher, err = cfgmatcher.LoadMatcherFrom(ctx, meta); err != nil {
			return nil, nil, err
		}
		if err = s.reevalPCLs(ctx); err != nil {
			return nil, nil, err
		}

		// We may have been in STOPPING phase, in which case incomplete runs may
		// still be finalizing themselves after receiving Cancel event from us.
		// It's harmless to send them UpdateConfig message, too. Eventually, they'll
		// complete finalization, send us OnRunFinished event and then we'll remove
		// them from the state anyway.
		return s, &UpdateIncompleteRunsConfig{
			EVersion: meta.EVersion,
			Hash:     meta.Hash(),
			RunIDs:   s.PB.IncompleteRuns(),
		}, err

	case config.StatusDisabled, config.StatusNotExists:
		// Intentionally not catching up with new ConfigHash (if any),
		// since it's not actionable and also simpler.
		switch s.Status {
		case prjmanager.Status_STATUS_UNSPECIFIED:
			// Project entity doesn't exist. No need to create it.
			return s, nil, nil
		case prjmanager.Status_STOPPED:
			return s, nil, nil
		case prjmanager.Status_STARTED:
			s = s.cloneShallow()
			s.Status = prjmanager.Status_STOPPING
			fallthrough
		case prjmanager.Status_STOPPING:
			if err := poller.Poke(ctx, s.PB.GetLuciProject()); err != nil {
				return nil, nil, err
			}
			runs := s.PB.IncompleteRuns()
			if len(runs) == 0 {
				s = s.cloneShallow()
				s.Status = prjmanager.Status_STOPPED
				return s, nil, nil
			}
			return s, &CancelIncompleteRuns{RunIDs: s.PB.IncompleteRuns()}, nil
		default:
			panic(fmt.Errorf("unexpected project status: %d", s.Status))
		}
	default:
		panic(fmt.Errorf("unexpected config status: %d", meta.Status))
	}
}

// Poke checks PM & world state and acts if necessary.
//
// For example, multi-CL Runs can be created if stabilization delay has passed.
func (s *State) Poke(ctx context.Context) (*State, SideEffect, error) {
	s.ensureNotYetCloned()

	// First, check if UpdateConfig if necessary.
	switch newState, sideEffect, err := s.UpdateConfig(ctx); {
	case err != nil:
		return nil, nil, err
	case newState != s:
		// UpdateConfig noticed a change and its SideEffectFn will propagate it
		// downstream.
		return newState, sideEffect, nil
	}
	// Propagate downstream.
	if err := poller.Poke(ctx, s.PB.GetLuciProject()); err != nil {
		return nil, nil, err
	}
	if err := s.pokeRuns(ctx); err != nil {
		return nil, nil, err
	}
	// TODO(tandrii): implement.
	return s, nil, nil
}

// OnRunsCreated updates state after new Runs were created.
func (s *State) OnRunsCreated(ctx context.Context, created common.RunIDs) (*State, SideEffect, error) {
	s.ensureNotYetCloned()

	existing := s.PB.IncompleteRuns()
	mutated := false
	for _, id := range created {
		if !mutated {
			if existing.ContainsSorted(id) {
				continue
			}
			mutated = true
			s = s.cloneShallow()
			cpy := make(common.RunIDs, len(existing), len(existing)+1)
			copy(cpy, existing)
			existing = cpy
		}
		existing.InsertSorted(id)
	}
	if !mutated {
		return s, nil, nil
	}
	s.tempSetIncompleteRuns(existing)
	if s.Status == prjmanager.Status_STOPPED {
		// This must not happen. Log, but do nothing.
		logging.Errorf(ctx, "CRITICAL: RunCreated %s events on STOPPED Project Manager", created)
		return s, nil, nil
	}
	// TODO(tandrii): re-evaluate pending CLs.
	return s, nil, nil
}

// OnRunsCreated updates state after Runs were finished.
func (s *State) OnRunsFinished(ctx context.Context, finished common.RunIDs) (*State, SideEffect, error) {
	s.ensureNotYetCloned()

	existing := s.PB.IncompleteRuns()
	remaining := existing.WithoutSorted(finished)
	if len(remaining) == len(existing) {
		return s, nil, nil // no change
	}
	s = s.cloneShallow()
	s.tempSetIncompleteRuns(remaining)

	if s.Status == prjmanager.Status_STOPPING && len(remaining) == 0 {
		s.Status = prjmanager.Status_STOPPED
		return s, nil, nil
	}
	// TODO(tandrii): re-evaluate pending CLs.
	return s, nil, nil
}

// OnCLsUpdated updates state as a result of new changes to CLs.
//
// Mutates incoming events slice.
func (s *State) OnCLsUpdated(ctx context.Context, events []*internal.CLUpdated) (*State, SideEffect, error) {
	s.ensureNotYetCloned()

	if s.Status != prjmanager.Status_STARTED {
		// Ignore all incoming CL events. If PM is re-enabled,
		// then first full poll will force re-sending of OnCLsUpdated event for all
		// still interesting CLs.
		return s, nil, nil
	}

	// Avoid doing anything in cases where all CL updates sent due to recent full
	// poll iff we already know about each CL based on its EVersion.
	updated := s.filterOutUpToDate(events)
	if len(updated) == 0 {
		return s, nil, nil
	}

	// Most likely there will be changes to state.
	s = s.cloneShallow()
	if err := s.evalUpdatedCLs(ctx, updated); err != nil {
		return nil, nil, err
	}
	return s, nil, nil
}

func (s *State) tempSetIncompleteRuns(ids common.RunIDs) {
	// TODO(tandrii): implement this properly. This is done only to pass tests.
	pruns := make([]*internal.PRun, len(ids))
	for i, id := range ids {
		pruns[i] = &internal.PRun{Id: string(id)}
	}
	s.PB.Components = []*internal.Component{{Pruns: pruns}}
}
