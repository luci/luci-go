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
//
// The state is modified via copy-on-write a.k.a. COW
// (https://en.wikipedia.org/wiki/Copy-on-write).
type state struct {
	luciProject string

	// Serializable state.

	status         prjmanager.Status // stored in a ProjectStateOffload entity.
	configHash     string            // stored in a ProjectStateOffload entity.
	incompleteRuns common.RunIDs     // sorted; stored in a Project entity.
	pendingCLs     *pendingCLs       // stored in a Project entity.
}

func (s *state) cloneShallow() *state {
	ret := &state{}
	*ret = *s
	return ret
}

// Top-level transitions funcs.

func (s *state) updateConfig(ctx context.Context) (*state, eventbox.SideEffectFn, error) {
	meta, err := config.GetLatestMeta(ctx, s.luciProject)
	if err != nil {
		return nil, nil, err
	}

	switch meta.Status {
	case config.StatusEnabled:
		if s.status == prjmanager.Status_STARTED && meta.Hash() == s.configHash {
			return s, nil, nil // already up-to-date.
		}
		s = s.cloneShallow()
		s.configHash = meta.Hash()
		// NOTE: we may be in STOPPING phase, and some Runs are now finalizing
		// themselves, while others haven't yet even noticed the stopping.
		// The former will eventually be removed from s.IncompleteRuns,
		// while the latter will continue running.
		s.status = prjmanager.Status_STARTED

		if err := poller.Poke(ctx, s.luciProject); err != nil {
			return nil, nil, err
		}
		p, sideEffectFn, err := s.pendingCLs.updateConfig(ctx, meta)
		if err != nil {
			return nil, nil, err
		}
		s.pendingCLs = p
		return s, eventbox.Chain(sideEffectFn, s.updateRunsConfigFactory(meta)), nil

	case config.StatusDisabled, config.StatusNotExists:
		// NOTE: we are intentionally not catching up with new ConfigHash (if any),
		// since it's not actionable.
		switch s.status {
		case prjmanager.Status_STATUS_UNSPECIFIED:
			// Project entity doesn't exist. No need to create it.
			return s, nil, nil
		case prjmanager.Status_STOPPED:
			return s, nil, nil
		case prjmanager.Status_STARTED:
			s = s.cloneShallow()
			s.status = prjmanager.Status_STOPPING
			fallthrough
		case prjmanager.Status_STOPPING:
			if len(s.incompleteRuns) == 0 {
				s = s.cloneShallow()
				s.status = prjmanager.Status_STOPPED
			}
			if err := poller.Poke(ctx, s.luciProject); err != nil {
				return nil, nil, err
			}
			return s, s.cancelRuns, nil
		default:
			panic(fmt.Errorf("unexpected project status: %d", s.status))
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
	p, sideEffectFn, err := s.pendingCLs.poke(ctx)
	if err != nil {
		return nil, nil, err
	}
	s.pendingCLs = p
	return s, sideEffectFn, nil
}

func (s *state) onRunsCreated(ctx context.Context, created common.RunIDs) (*state, eventbox.SideEffectFn, error) {
	mutated := false
	for _, id := range created {
		if !mutated {
			if s.incompleteRuns.ContainsSorted(id) {
				continue
			}
			mutated = true
			s = s.cloneShallow()
			cpy := make(common.RunIDs, len(s.incompleteRuns), len(s.incompleteRuns)+1)
			copy(cpy, s.incompleteRuns)
			s.incompleteRuns = cpy
		}
		s.incompleteRuns.InsertSorted(id)
	}
	if !mutated {
		return s, nil, nil
	}
	switch s.status {
	case prjmanager.Status_STOPPED:
		// This must not happen. Log, but do nothing.
		logging.Errorf(ctx, "CRITICAL: RunCreated %s events on STOPPED Project Manager", created)
		return s, nil, nil

	case prjmanager.Status_STOPPING:
		// No need to update pendingCLs since no actions will be taken anyhow.
		return s, nil, nil

	case prjmanager.Status_STARTED:
		p, sideEffectFn, err := s.pendingCLs.onRunsCreated(ctx, created)
		if err != nil {
			return nil, nil, err
		}
		s.pendingCLs = p
		return s, sideEffectFn, nil

	default:
		panic(fmt.Errorf("unexpected project status: %d", s.status))
	}
}

func (s *state) onRunsFinished(ctx context.Context, finished common.RunIDs) (*state, eventbox.SideEffectFn, error) {
	remaining := s.incompleteRuns.WithoutSorted(finished)
	if len(remaining) == len(s.incompleteRuns) {
		return s, nil, nil // noop.
	}
	s = s.cloneShallow()
	s.incompleteRuns = remaining

	switch s.status {
	case prjmanager.Status_STOPPED:
		// This must not happen, but handle it anyway.
		logging.Errorf(ctx, "CRITICAL: RunFinished %s events on STOPPED Project Manager", finished)
		return s, nil, nil

	case prjmanager.Status_STOPPING:
		if len(remaining) == 0 {
			s.status = prjmanager.Status_STOPPED
		}
		return s, nil, nil

	case prjmanager.Status_STARTED:
		p, sideEffectFn, err := s.pendingCLs.onRunsFinished(ctx, finished)
		if err != nil {
			return nil, nil, err
		}
		s.pendingCLs = p
		return s, sideEffectFn, nil

	default:
		panic(fmt.Errorf("unexpected project status: %d", s.status))
	}
}

func (s *state) onCLsUpdated(ctx context.Context, cls []*internal.CLUpdated) (*state, eventbox.SideEffectFn, error) {
	switch p, sideEffectFn, err := s.pendingCLs.updateCLs(ctx, cls); {
	case err != nil:
		return nil, nil, err
	case p == s.pendingCLs:
		return s, sideEffectFn, nil // no state change
	default:
		s = s.cloneShallow()
		s.pendingCLs = p
		return s, sideEffectFn, nil
	}
}

func (s *state) execDeferred(ctx context.Context) (*state, eventbox.SideEffectFn, error) {
	switch p, sideEffectFn, err := s.pendingCLs.execDeferred(ctx); {
	case err != nil:
		return nil, nil, err
	case p == s.pendingCLs:
		return s, sideEffectFn, nil // no state change
	default:
		s = s.cloneShallow()
		s.pendingCLs = p
		return s, sideEffectFn, nil
	}
}
