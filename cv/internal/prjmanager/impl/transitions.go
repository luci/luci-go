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

	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/gerrit/poller"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/run"
)

// state stores serializable state of a ProjectManager.
//
// The state is stored in Project entity.
type state struct {
	Status         prjmanager.Status
	ConfigHash     string
	IncompleteRuns []run.ID
	// TODO(tandrii): add CL grouping state.
}

func (s *state) cloneDeep() *state {
	return &state{
		Status:         s.Status,
		ConfigHash:     s.ConfigHash,
		IncompleteRuns: s.IncompleteRuns[:],
	}
}

func (s *state) cloneShallow() *state {
	return &state{
		Status:         s.Status,
		ConfigHash:     s.ConfigHash,
		IncompleteRuns: s.IncompleteRuns,
	}
}

func updateConfig(ctx context.Context, luciProject string, s *state) (
	eventbox.SideEffectFn, *state, error) {
	meta, err := config.GetLatestMeta(ctx, luciProject)
	if err != nil {
		return nil, nil, err
	}

	switch meta.Status {
	case config.StatusEnabled:
		if s.Status == prjmanager.Status_STARTED && meta.Hash() == s.ConfigHash {
			return nil, s, nil // already up-to-date.
		}
		s = s.cloneShallow()
		s.ConfigHash = meta.Hash()
		// NOTE: we may be in STOPPING phase, and some Runs are now finalizing
		// themselves, while others haven't yet even noticed the stopping.
		// The former will eventually be removed from sm.pe.IncompleteRuns,
		// while the latter will continue running.
		s.Status = prjmanager.Status_STARTED

		if err := poller.Poke(ctx, luciProject); err != nil {
			return nil, nil, err
		}
		// TODO(tandrii): re-evaluate pending CLs.
		return notifyIncompleteRuns, s, nil

	case config.StatusDisabled, config.StatusNotExists:
		// NOTE: we are intentionally not catching up with new ConfigHash (if any),
		// since it's not actionable.
		switch s.Status {
		case prjmanager.Status_STATUS_UNSPECIFIED:
			// Project entity doesn't exist. No need to create it.
			return nil, s, nil
		case prjmanager.Status_STOPPED:
			return nil, s, nil
		case prjmanager.Status_STARTED:
			s = s.cloneShallow()
			s.Status = prjmanager.Status_STOPPING
			fallthrough
		case prjmanager.Status_STOPPING:
			if len(s.IncompleteRuns) == 0 {
				s = s.cloneShallow()
				s.Status = prjmanager.Status_STOPPED
			}
			if err := poller.Poke(ctx, luciProject); err != nil {
				return nil, nil, err
			}
			return notifyIncompleteRuns, s, nil
		default:
			panic(fmt.Errorf("unexpected project status: %d", s.Status))
		}
	default:
		panic(fmt.Errorf("unexpected config status: %d", meta.Status))
	}
}

func poke(ctx context.Context, luciProject string, s *state) (
	eventbox.SideEffectFn, *state, error) {
	// First, check if updateConfig if necessary.
	switch sideEffect, newState, err := updateConfig(ctx, luciProject, s); {
	case err != nil:
		return nil, nil, err
	case newState != s:
		// updateConfig noticed a change and its SideEffectFn will propagate it
		// downstream.
		return sideEffect, newState, nil
	}
	// Propagate downstream.
	if err := poller.Poke(ctx, luciProject); err != nil {
		return nil, nil, err
	}
	// TODO(tandrii): implement.
	return nil, s, nil
}
