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

package exe

import (
	"context"
	"strings"
	"sync"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/google"
)

// Step represents a synchronized handle to a step attached to the current
// build.
type Step struct {
	statusMu sync.Mutex
	step     *bbpb.Step

	modLock func(func() error) error
}

func (s *Step) lockedEnsureStarted() {
	if s.step.Status == bbpb.Status_SCHEDULED {
		s.step.Status = bbpb.Status_STARTED
	}
}

// FullName returns the full step name, as it appears in Build.Steps.
func (s *Step) FullName() string {
	return s.step.Name
}

// EnsureStarted will modify the status of this Step to STARTED if it is still
// SCHEDULED.
func (s *Step) EnsureStarted() {
	s.modLock(func() error {
		s.statusMu.Lock()
		defer s.statusMu.Unlock()
		s.lockedEnsureStarted()
		return nil
	})
}

// StepStatus is a temporary struct for containing a Step's 'status' properties.
//
// Used by Step.ModifyStatus.
type StepStatus struct {
	Status          *bbpb.Status
	SummaryMarkdown *string
}

// ModifyStatus runs your callback with process-exclusive access to modify
// this step's various 'status' fields.
//
// The Build will be queued for sending on the return of `cb`.
//
// You must not retain pointers to anything you assign to the StepStatus
// message after the return of `cb`, or this could lead to tricky race
// conditions.
//
// Passes through `cb`'s result.
func (s *Step) ModifyStatus(cb func(*StepStatus) error) error {
	return s.modLock(func() error {
		s.statusMu.Lock()
		defer s.statusMu.Unlock()
		s.lockedEnsureStarted()
		return cb(&StepStatus{
			&s.step.Status,
			&s.step.SummaryMarkdown,
		})
	})
}

// WithStep calls `cb` with an updated context and a mutable Step object.
//
// `name` must not be empty or contain "|". Duplicate names will be
// automatically disambiguated by appending "(N)" for the Nth duplicate (N > 0).
//
// The context passed to `cb` will contain Step.FullName() as the current step
// namespace. Creating additional steps will be namespaced within this.
//
// The Step's status and EndTime will be set as soon as `cb` returns, and the
// context will be canceled. This implies that `cb` must wait for any child
// goroutines before returning.
//
// The Step's status will be:
//   * left alone if manually set
//   * set to FAILURE if cb returns an untagged error
//   * set to CANCELED if cb returns context.Canceled or
//     context.DeadlineExceeded
//   * set to `status` if cb returns an error tagged with `status` (see Status*
//     in this package)
//
// Returns errors only if:
//   * `ctx` doesn't contain a build
//   * `name` was invalid (empty or contains "|")
//   * cb returns an error
func WithStep(ctx context.Context, name string, cb func(context.Context, *Step) error) (err error) {
	b := getBuild(ctx)

	if strings.Contains(name, "|") || name == "" {
		return errors.Reason("invalid name %q", name).Tag(StatusInfraFailure).Err()
	}

	fullNS, ctx := b.enterDisambiguatedNamespace(ctx, name)

	step := &Step{modLock: b.modLock, step: &bbpb.Step{
		Name:      fullNS,
		StartTime: google.NewTimestamp(clock.Now(ctx)),
		Status:    bbpb.Status_SCHEDULED,
	}}

	b.stepsMu.Lock()
	b.build.Steps = append(b.build.Steps, step.step)
	b.stepsMu.Unlock()

	defer func() {
		b.stepsMu.Lock()
		defer b.stepsMu.Unlock()
		step.step.EndTime = google.NewTimestamp(clock.Now(ctx))
		if (step.step.Status & bbpb.Status_ENDED_MASK) == 0 {
			step.step.Status = getErrorStatus(err)
		}
	}()

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	return cb(cctx, step)
}
