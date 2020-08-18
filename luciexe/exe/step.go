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
	"io"
	"os"
	"strings"
	"sync"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	ldtypes "go.chromium.org/luci/logdog/common/types"
)

// Step represents a synchronized handle to a step attached to the current
// build.
type Step struct {
	statusMu sync.Mutex
	logsMu   sync.Mutex
	step     *bbpb.Step

	build *Build
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
	s.build.modLock(func() error {
		s.statusMu.Lock()
		defer s.statusMu.Unlock()
		s.lockedEnsureStarted()
		return nil
	})
}

func (s *Step) ldPrep(ctx context.Context, name string) (ldtypes.StreamName, error) {
	fullNameToks := append(strings.Split(s.step.Name, "|"), name)

	return ldPrep(ctx, fullNameToks, func(cb func(*[]*bbpb.Log) error) error {
		return s.build.modLock(func() error {
			s.logsMu.Lock()
			defer s.logsMu.Unlock()
			return cb(&s.step.Logs)
		})
	})
}

// Log attaches a new text-mode log to the step with the given name.
//
// Caller must close the log when they're done with it.
func (s *Step) Log(ctx context.Context, name string, opts ...streamclient.Option) (io.WriteCloser, error) {
	fullName, err := s.ldPrep(ctx, name)
	if err != nil {
		return nil, err
	}
	return s.build.ldClient.NewTextStream(ctx, fullName, opts...)
}

// LogFile is a convenience method to attach a new text-mode log to the step
// with the given name and copy the contents of the file at `path` into it.
func (s *Step) LogFile(ctx context.Context, name string, path string, opts ...streamclient.Option) error {
	out, err := s.Log(ctx, name, opts...)
	if err != nil {
		return err
	}
	defer out.Close()

	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()

	_, err = io.Copy(out, in)
	return err
}

// LogBinary attaches a new binary-mode log to the step with the given name.
//
// Caller must close the log when they're done with it.
func (s *Step) LogBinary(ctx context.Context, name string, opts ...streamclient.Option) (io.WriteCloser, error) {
	fullName, err := s.ldPrep(ctx, name)
	if err != nil {
		return nil, err
	}
	return s.build.ldClient.NewBinaryStream(ctx, fullName, opts...)
}

// LogDatagram attaches a new datagram-mode log to the step with the given name.
//
// Caller must close the log when they're done with it.
func (s *Step) LogDatagram(ctx context.Context, name string, opts ...streamclient.Option) (streamclient.DatagramStream, error) {
	fullName, err := s.ldPrep(ctx, name)
	if err != nil {
		return nil, err
	}
	return s.build.ldClient.NewDatagramStream(ctx, fullName, opts...)
}

// StepView is a struct containing the modifiable portions of a Step.
//
// This specifically EXCLUDES Logs and Status.
//
// Use Log* methods to add logs.
// Use the error returned from your WithStep callback for Status.
//
// Used by Step.Modify.
type StepView struct {
	SummaryMarkdown string
}

// Modify runs your callback with process-exclusive access to modify
// this step's modifiable fields.
//
// The Build will be queued for sending on the return of `cb`.
//
// Passes through `cb`'s result.
func (s *Step) Modify(cb func(*StepView) error) error {
	return s.build.modLock(func() error {
		s.statusMu.Lock()
		defer s.statusMu.Unlock()
		s.lockedEnsureStarted()

		sv := &StepView{s.step.SummaryMarkdown}
		err := cb(sv)
		s.step.SummaryMarkdown = sv.SummaryMarkdown
		return err
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

	step := &Step{build: b, step: &bbpb.Step{
		Name:      fullNS,
		StartTime: google.NewTimestamp(clock.Now(ctx)),
		Status:    bbpb.Status_SCHEDULED,
	}}

	b.modLock(func() error {
		b.stepsMu.Lock()
		b.build.Steps = append(b.build.Steps, step.step)
		b.stepsMu.Unlock()
		return nil
	})

	defer b.modLock(func() error {
		step.step.EndTime = google.NewTimestamp(clock.Now(ctx))
		step.step.Status, _ = getErrorStatus(err)
		return nil
	})

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	return cb(cctx, step)
}
