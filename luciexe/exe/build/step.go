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

package build

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	ldtypes "go.chromium.org/luci/logdog/common/types"
)

// Step tracks the state of a single Build step in this exe.
//
// Add a new step to the Build using `WithStep`.
type Step struct {
	step    *bbpb.Step
	stepIdx int

	build *State

	outputMu sync.Mutex // protects everything except step.Logs
	logsMu   sync.Mutex // protects step.Logs
}

// FullName returns the full step name, as it appears in Build.Steps.
func (s *Step) FullName() string {
	return s.step.Name
}

// EnsureStarted will explicitly modify the status of this Step to STARTED if it
// is still SCHEDULED.
func (s *Step) EnsureStarted(ctx context.Context) {
	s.modLock(ctx, func() error { return nil })
}

// ldPrep adds a new Log to this Steps's Logs and returns a the build-unique
// logdog streamname allocated for this log.
func (s *Step) ldPrep(ctx context.Context, name string) (ldtypes.StreamName, error) {
	return addUniqueLog(s.stepIdx, name, func(cb func(*[]*bbpb.Log) error) error {
		return s.modLock(ctx, func() error {
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
// This specifically EXCLUDES Logs and Status. Use Log* methods to add logs and
// the error returned from your WithStep callback for Status.
//
// Used by Step.Modify.
type StepView struct {
	SummaryMarkdown string
}

// modLock is used to synchronize all access to the underlying Step proto.
//
// Anything which modifies any portion of the Step.step object must do so
// within the modLock callback. When the callback returns (regardless of error)
// the build will be triggered to send an update.
//
// Only one goroutine can be inside modLock at the same time (synchronized on
// outputMu).
func (s *Step) modLock(ctx context.Context, cb func() error) error {
	return s.build.modLock(func() error {
		s.outputMu.Lock()
		defer s.outputMu.Unlock()
		if s.step.Status == bbpb.Status_SCHEDULED {
			s.step.Status = bbpb.Status_STARTED
			s.step.StartTime = google.NewTimestamp(clock.Now(ctx))
		}
		if protoutil.IsEnded(s.step.Status) {
			return ErrStepClosed
		}
		return cb()
	})
}

// Modify runs your callback with process-exclusive access to modify
// this step's modifiable fields.
//
// The Build will be queued for sending on the return of `cb`.
//
// Passes through `cb`'s result.
func (s *Step) Modify(ctx context.Context, cb func(*StepView) error) error {
	return s.modLock(ctx, func() error {
		sv := &StepView{s.step.SummaryMarkdown}
		err := cb(sv)
		s.step.SummaryMarkdown = sv.SummaryMarkdown
		return err
	})
}

// WithStep calls `cb` with an updated context and a mutable Step object.
//
// `ctx` must not be canceled/expired.
//
// `name` is the name of the step and must not be empty or contain "|".
// Duplicate names will be automatically disambiguated by appending " (N)" for
// the Nth duplicate (N > 0).
//
// The context passed to `cb` will contain Step.FullName() as the current step
// namespace. Creating additional steps from this context will create nested
// steps.
//
// The Step's status and EndTime will be set as soon as `cb` returns, and the
// context provide to `cb` will be canceled. This implies that `cb` must wait
// for any child steps/goroutines before returning.
//
// A per-step logging stream will be lazily allocated such that using the
// `go.chromium.org/luci/common/logging` package on the callback's context will
// initialize the `log` stream on the Step and direct all logging there. If you
// have manually opened a log called "log" already, then logging messages will
// be sunk to null.
//
// On return of `cb`, the Step's status will be set according to GetErrorStatus.
// Non-nil errors will be logged at Debug level.
//
// If `cb` panics then the error will be logged to `log` and Step's status
// will be INFRA_FAILURE.
//
// Attempting to modify the Step (with Modify or Log* methods) after the return
// of `cb` will return ErrStepClosed and have no other effect.
//
// Returns errors only if:
//   * `ctx` doesn't contain a Build (via SinkBuildUpdates).
//   * `ctx` is canceled/timed out.
//   * `name` was invalid (empty or contains "|")
//   * cb returns an error
func WithStep(ctx context.Context, name string, cb func(context.Context, *Step) error) (err error) {
	if strings.Contains(name, "|") || name == "" {
		return errors.Reason("invalid name %q", name).Tag(StatusInfraFailure).Err()
	}

	b := getState(ctx)

	fullNS, ctx := b.enterDisambiguatedNamespace(ctx, name)
	select {
	case <-ctx.Done():
		return errors.Annotate(ctx.Err(), "creating step %q", fullNS).Err()
	default:
	}

	step := &Step{build: b, step: &bbpb.Step{
		Name:   fullNS,
		Status: bbpb.Status_SCHEDULED,
	}}

	b.modLock(func() error {
		b.stepsMu.Lock()
		step.stepIdx = len(b.build.Steps)
		b.build.Steps = append(b.build.Steps, step.step)
		b.stepsMu.Unlock()
		return nil
	})

	ctx = synthesizeLogdogNamespace(ctx, step.build.ldClient.Namespace(), step.stepIdx)

	logFactory, closer := lazyLogger(step, fullNS)
	ctx = logging.SetFactory(ctx, logFactory)
	defer closer()

	defer func() {
		status, _ := GetErrorStatus(err)

		perr := recover()
		if perr != nil {
			logging.Errorf(ctx, "step %q panic'd: %s", fullNS, perr)
			status = bbpb.Status_INFRA_FAILURE
		} else if err != nil {
			// do the Level check to avoid allocating `log` if the debug message
			// wouldn't show anyway.
			if logging.GetLevel(ctx) <= logging.Debug {
				logging.Debugf(ctx, "step %q returned error: %s", fullNS, err)
			}
		}

		step.modLock(ctx, func() error {
			step.step.EndTime = google.NewTimestamp(clock.Now(ctx))
			step.step.Status = status
			return nil
		})

		if perr != nil {
			panic(perr)
		}
	}()

	cctx, cancel := context.WithCancel(ctx)
	defer cancel()
	return cb(cctx, step)
}
