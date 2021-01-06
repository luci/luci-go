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

package impl

import (
	"context"
	"fmt"

	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/gerrit/poller"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/prjmanager/internal"
)

// state tracks state of a ProjectManager during its mutation.
type state struct {
	luciProject string

	// Serializable state.

	// TODO(tandrii): make all remaining members private.
	Status         prjmanager.Status // stored in a ProjectStateOffload entity.
	ConfigHash     string            // stored in a ProjectStateOffload entity.
	IncompleteRuns common.RunIDs     // sorted; stored in a Project entity.
}

func (s *state) cloneShallow() *state {
	ret := &state{}
	*ret = *s
	return ret
}

func (s *state) updateConfig(ctx context.Context) (*state, eventbox.SideEffectFn, error) {
	meta, err := config.GetLatestMeta(ctx, s.luciProject)
	if err != nil {
		return nil, nil, err
	}

	switch meta.Status {
	case config.StatusEnabled:
		if s.Status == prjmanager.Status_STARTED && meta.Hash() == s.ConfigHash {
			return s, nil, nil // already up-to-date.
		}
		s = s.cloneShallow()
		s.ConfigHash = meta.Hash()
		// NOTE: we may be in STOPPING phase, and some Runs are now finalizing
		// themselves, while others haven't yet even noticed the stopping.
		// The former will eventually be removed from s.IncompleteRuns,
		// while the latter will continue running.
		s.Status = prjmanager.Status_STARTED

		if err := poller.Poke(ctx, s.luciProject); err != nil {
			return nil, nil, err
		}
		// TODO(tandrii): re-evaluate pending CLs.
		return s, s.updateRunsConfigFactory(meta), nil

	case config.StatusDisabled, config.StatusNotExists:
		// NOTE: we are intentionally not catching up with new ConfigHash (if any),
		// since it's not actionable.
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
			if len(s.IncompleteRuns) == 0 {
				s = s.cloneShallow()
				s.Status = prjmanager.Status_STOPPED
			}
			if err := poller.Poke(ctx, s.luciProject); err != nil {
				return nil, nil, err
			}
			return s, s.cancelRuns, nil
		default:
			panic(fmt.Errorf("unexpected project status: %d", s.Status))
		}
	default:
		panic(fmt.Errorf("unexpected config status: %d", meta.Status))
	}
}

func (s *state) poke(ctx context.Context) (*state, eventbox.SideEffectFn, error) {
	// First, check if updateConfig if necessary.
	switch newState, sideEffect, err := s.updateConfig(ctx); {
	case err != nil:
		return nil, nil, err
	case newState != s:
		// updateConfig noticed a change and its SideEffectFn will propagate it
		// downstream.
		return newState, sideEffect, nil
	}
	// Propagate downstream.
	if err := poller.Poke(ctx, s.luciProject); err != nil {
		return nil, nil, err
	}
	if err := s.pokeRuns(ctx); err != nil {
		return nil, nil, err
	}
	// TODO(tandrii): implement.
	return s, nil, nil
}

func (s *state) runsCreated(ctx context.Context, created common.RunIDs) (*state, eventbox.SideEffectFn, error) {
	mutated := false
	for _, id := range created {
		if !mutated {
			if s.IncompleteRuns.ContainsSorted(id) {
				continue
			}
			mutated = true
			s = s.cloneShallow()
			cpy := make(common.RunIDs, len(s.IncompleteRuns), len(s.IncompleteRuns)+1)
			copy(cpy, s.IncompleteRuns)
			s.IncompleteRuns = cpy
		}
		s.IncompleteRuns.InsertSorted(id)
	}
	if !mutated {
		return s, nil, nil
	}
	if s.Status == prjmanager.Status_STOPPED {
		// This must not happen. Log, but do nothing.
		logging.Errorf(ctx, "CRITICAL: RunCreated %s events on STOPPED Project Manager", created)
		return s, nil, nil
	}
	// TODO(tandrii): re-evaluate pending CLs.
	return s, nil, nil
}

func (s *state) runsFinished(ctx context.Context, finished common.RunIDs) (*state, eventbox.SideEffectFn, error) {
	remaining := s.IncompleteRuns.WithoutSorted(finished)
	if len(remaining) == len(s.IncompleteRuns) {
		return s, nil, nil // no change
	}
	s = s.cloneShallow()
	s.IncompleteRuns = remaining

	if s.Status == prjmanager.Status_STOPPING && len(s.IncompleteRuns) == 0 {
		s.Status = prjmanager.Status_STOPPED
		return s, nil, nil
	}
	// TODO(tandrii): re-evaluate pending CLs.
	return s, nil, nil
}

func (s *state) clsUpdated(ctx context.Context, cls []*internal.CLUpdated) (*state, eventbox.SideEffectFn, error) {
	// TODO(tandrii): implement.
	return s, nil, nil
}
