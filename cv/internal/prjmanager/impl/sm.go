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

	"go.chromium.org/luci/cv/internal/config"
	"go.chromium.org/luci/cv/internal/prjmanager"
	"go.chromium.org/luci/cv/internal/run"
)

// state stores serializable state of a ProjectManager.
//
// The state is currently stored entirely in Project entity.
//
// NOTE: this could have been a proto message stored in a single Project's
// entity field. I chose not to do this *yet* because then it'd be impossible to
// read just the ConfigHash or IncompleteRuns w/o paying proto deserialization
// cost, which could be substantial. So, pay the cost of 2 boilerplaty
// toProjectEntity/fromProjectEntity methods to make it easy to optimize reading
// w/o deserialization in the future if necessary.
type state struct {
	Status         prjmanager.Status
	ConfigHash     string
	IncompleteRuns []run.ID
	// TODO(tandrii): add CL grouping state.
}

func fromProjectEntity(p *prjmanager.Project) *state {
	return &state{
		Status:         p.Status,
		ConfigHash:     p.ConfigHash,
		IncompleteRuns: p.IncompleteRuns,
	}
}

func (s *state) toProjectEntity(p *prjmanager.Project) {
	p.Status = s.Status
	p.ConfigHash = s.ConfigHash
	p.IncompleteRuns = s.IncompleteRuns
}

func (s *state) clone() *state {
	return &state{
		Status:         s.Status,
		ConfigHash:     s.ConfigHash,
		IncompleteRuns: s.IncompleteRuns[:],
	}
}

// machine advanced the state of a ProjectManager.
type machine struct {
	luciProject string
	state       *state
	dirty       bool // if true, state was modified.
}

func (m *machine) updateConfig(ctx context.Context) (mutation, error) {
	meta, err := config.GetLatestMeta(ctx, m.luciProject)
	if err != nil {
		return noop, err
	}
	s := m.state

	switch meta.Status {
	case config.StatusEnabled:
		if s.Status == prjmanager.Status_STARTED && meta.Hash() == s.ConfigHash {
			return noop, nil // already up-to-date.
		}
		m.dirty = true
		s.ConfigHash = meta.Hash()
		// NOTE: we may be in STOPPING phase, and some Runs are now finalizing
		// themselves, while others haven't yet even noticed the stopping.
		// The former will eventually be removed from sm.pe.IncompleteRuns,
		// while the latter will continue running.
		s.Status = prjmanager.Status_STARTED
		// TODO(tandrii): re-evaluate pending CLs.
		return mutation{
			apply: chain(notifyGoBPoller(m.luciProject), notifyIncompleteRuns),
		}, nil

	case config.StatusDisabled, config.StatusNotExists:
		logging.Debugf(ctx, "config.Status %s", meta.Status)
		// NOTE: we are intentionally not catching up with new ConfigHash (if any),
		// since it's not actionable.
		switch s.Status {
		case prjmanager.Status_STOPPED:
			// There is no reason to continue processing events.
			return noop, nil
		case prjmanager.Status_STARTED:
			m.dirty = true
			s.Status = prjmanager.Status_STOPPING
			fallthrough
		case prjmanager.Status_STOPPING:
			if len(s.IncompleteRuns) == 0 {
				m.dirty = true
				s.Status = prjmanager.Status_STOPPED
			}
			return mutation{
				apply: chain(notifyGoBPoller(m.luciProject), notifyIncompleteRuns),
			}, nil
		}
	default:
		panic(fmt.Errorf("unknown state: %d", meta.Status))
	}
	panic("unreachable")
}

///////////////////////////////////////////////////////////////////////////////
// helpers

type applyFunc func(context.Context) error

func chain(fs ...applyFunc) applyFunc {
	return func(ctx context.Context) error {
		for _, f := range fs {
			if err := f(ctx); err != nil {
				return err
			}
		}
		return nil
	}
}

// mutation is executed during transaction to modify datastore entities other
// than prjmanager.Project itself.
type mutation struct {
	apply applyFunc
	// TODO(tandrii): keep track of 1) atomicity 2) the resulting state,
	// such that partial progress can be saved.
}

var noop = mutation{apply: func(_ context.Context) error { return nil }}

func notifyGoBPoller(luciProject string) applyFunc {
	return func(_ context.Context) error {
		// TODO(tandrii): notify Gerrit Poller.
		return nil
	}
}

func notifyIncompleteRuns(ctx context.Context) error {
	// TODO(tandrii): notify IncompleteRuns.
	return nil
}
