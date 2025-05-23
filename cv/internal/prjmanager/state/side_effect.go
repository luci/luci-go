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

	"go.chromium.org/luci/common/sync/parallel"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/eventbox"
	"go.chromium.org/luci/cv/internal/prjmanager/clpurger"
	"go.chromium.org/luci/cv/internal/prjmanager/cltriggerer"
	"go.chromium.org/luci/cv/internal/prjmanager/prjpb"
)

// SideEffect describes action to be done transactionally with updating state in
// Datastore.
//
// It may consist of several `SideEffect`s, which are executed sequentially.
//
// Semantically, this is translatable to eventbox.SideEffectFn,
// but is easy to assert for in tests of this package.
type SideEffect interface {
	// Do is the eventbox.SideEffectFn.
	Do(context.Context) error
}

func SideEffectFn(s SideEffect) eventbox.SideEffectFn {
	if s == nil {
		return nil
	}
	return s.Do
}

// SideEffects combines 2+ `SideEffect`s.
type SideEffects struct {
	items []SideEffect
}

// Do implements SideEffect interface.
func (s SideEffects) Do(ctx context.Context) error {
	for _, it := range s.items {
		if err := it.Do(ctx); err != nil {
			return err
		}
	}
	return nil
}

// NewSideEffects returns composite SideEffect.
//
// At least 2 items must be provided.
// Provided arg slice must not be mutated.
func NewSideEffects(items ...SideEffect) SideEffect {
	if len(items) < 2 {
		panic("at least 2 required")
	}
	se := &SideEffects{}
	for _, item := range items {
		if item != nil {
			se.items = append(se.items, item)
		}
	}
	if len(se.items) == 0 {
		return nil
	}
	return se
}

// concurrency is how many goroutines may an individual SideEffect run at the
// same time.
const concurrency = 16

// UpdateIncompleteRunsConfig sends UpdateConfig events to incomplete Runs.
type UpdateIncompleteRunsConfig struct {
	RunNotifier RunNotifier
	RunIDs      common.RunIDs
	Hash        string
	EVersion    int64
}

// Do implements SideEffect interface.
func (u *UpdateIncompleteRunsConfig) Do(ctx context.Context) error {
	err := parallel.WorkPool(concurrency, func(work chan<- func() error) {
		for _, id := range u.RunIDs {
			work <- func() error {
				return u.RunNotifier.UpdateConfig(ctx, id, u.Hash, u.EVersion)
			}
		}
	})
	return common.MostSevereError(err)
}

// CancelIncompleteRuns sends Cancel event to incomplete Runs.
type CancelIncompleteRuns struct {
	RunNotifier RunNotifier
	RunIDs      common.RunIDs
}

// Do implements SideEffect interface.
func (c *CancelIncompleteRuns) Do(ctx context.Context) error {
	err := parallel.WorkPool(concurrency, func(work chan<- func() error) {
		for _, id := range c.RunIDs {
			work <- func() error {
				return c.RunNotifier.Cancel(ctx, id, fmt.Sprintf("CV is disabled for LUCI Project %q", id.LUCIProject()))
			}
		}
	})
	return common.MostSevereError(err)
}

// TriggerPurgeCLTasks triggers PurgeCLTasks via TQ.
type TriggerPurgeCLTasks struct {
	payloads []*prjpb.PurgeCLTask
	clPurger *clpurger.Purger
}

// Do implements SideEffect interface.
func (t *TriggerPurgeCLTasks) Do(ctx context.Context) error {
	err := parallel.WorkPool(concurrency, func(work chan<- func() error) {
		for _, p := range t.payloads {
			work <- func() error {
				return t.clPurger.Schedule(ctx, p)
			}
		}
	})
	return common.MostSevereError(err)
}

// ScheduleTriggeringCLDepsTasks schedules TriggeringCLDepsTask(s) via TQ.
type ScheduleTriggeringCLDepsTasks struct {
	payloads    []*prjpb.TriggeringCLDepsTask
	clTriggerer *cltriggerer.Triggerer
}

// Do implements SideEffect interface.
func (t *ScheduleTriggeringCLDepsTasks) Do(ctx context.Context) error {
	if len(t.payloads) == 0 {
		return nil
	}
	err := parallel.WorkPool(concurrency, func(work chan<- func() error) {
		for _, p := range t.payloads {
			work <- func() error {
				return t.clTriggerer.Schedule(ctx, p)
			}
		}
	})
	return common.MostSevereError(err)
}
