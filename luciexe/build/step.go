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
	"fmt"
	"io"
	"strings"
	"sync"

	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
)

// StepState represents the state of a single step.
//
// This is properly initialized by the Step and ScheduleStep functions.
type StepState struct {
	ctx   context.Context
	state *State

	stepPbMu sync.Mutex
	stepPb   *bbpb.Step
}

var _ Loggable = (*StepState)(nil)

// Step adds a new step to the build.
//
// The step will have a "STARTED" status with a StartTime.
//
// The returned context is updated so that calling Step/ScheduleStep on it will create sub-steps.
//
// If `name` contains `|` this function will panic, since this is a reserved
// character for delimiting hierarchy in steps.
//
// Duplicate step names will be disambiguated by appending " (N)" for the 2nd,
// 3rd, etc. duplicate.
//
// The returned context will have `name` embedded in it; Calling Step or
// ScheduleStep with this context will generate a sub-step.
//
// You MUST call StepState.End. To automatically map errors and panics to their
// correct visual representation, End the Step like:
//
//    var err error
//    step, ctx := build.Step(ctx, "Step name")
//    defer func() { step.End(err) }()
//
//    err = opThatErrsOrPanics(ctx)
//
// NOTE: A panic will still crash the program as usual. This does NOT
// `recover()` the panic. Please use conventional Go error handling and control
// flow mechanisms.
func Step(ctx context.Context, name string) (*StepState, context.Context) {
	ret, ctx := ScheduleStep(ctx, name)
	ret.Start()
	return ret, ctx
}

// ScheduleStep is like Step, except that it leaves the new step in the
// SCHEDULED status, and does not set a StartTime.
//
// The step will move to STARTED when calling any other methods on
// the StepState, when creating a sub-Step, or if you explicitly call
// StepState.Start().
func ScheduleStep(ctx context.Context, name string) (*StepState, context.Context) {
	if strings.Contains(name, "|") {
		panic(errors.Reason("step name %q contains reserved character `|`", name).Err())
	}

	cstate := getState(ctx)

	ret := &StepState{
		ctx:   ctx,
		state: cstate.state,
		stepPb: cstate.state.registerStep(&bbpb.Step{
			Name:   cstate.stepPrefix + name,
			Status: bbpb.Status_SCHEDULED,
		}),
	}

	if ret.noopMode() /* || no logsink */ {
		ctx = logging.SetField(ctx, "build.step", ret.stepPb.Name)
		logging.Infof(ctx, "set status: %s", ret.stepPb.Status)
	} else {
		// TODO(iannucci): set up logging redirection
	}
	ret.ctx = ctx

	cstate.stepPrefix = ret.stepPb.Name + "|"
	return ret, setState(ctx, cstate)
}

// End sets the step's final status, according to `err` (See ExtractStatus).
//
// End will also be able to set INFRA_FAILURE status and log additional
// information if the program is panic'ing.
//
// End must be invoked like:
//
//    var err error
//    step, ctx := build.Step(ctx, ...)  // or build.ScheduleStep
//    defer func() { step.End(err) }()
//
//    err = opThatErrsOrPanics()
//
// NOTE: A panic will still crash the program as usual. This does NOT
// `recover()` the panic. Please use conventional Go error handling and control
// flow mechanisms.
func (s *StepState) End(err error) {
	var message string
	s.mutate(func() {
		if errors.IsPanicking(1) {
			message = "PANIC"
			// TODO(iannucci): include details of panic in SummaryMarkdown or log?
			// How to prevent panic dump from showing up at every single step on the
			// stack?
			s.stepPb.Status = bbpb.Status_INFRA_FAILURE
		} else {
			s.stepPb.Status, _ = ExtractStatus(err)
			if err != nil {
				message = err.Error()
			}
		}
		s.stepPb.EndTime = timestamppb.New(clock.Now(s.ctx))
	})
	// stepPb is immutable after mutate ends, so we should be fine to access it
	// outside the locks.

	logf := logging.Errorf
	switch s.stepPb.Status {
	case bbpb.Status_SUCCESS:
		logf = logging.Infof
	case bbpb.Status_CANCELED:
		logf = logging.Warningf
	}
	logMsg := fmt.Sprintf("set status: %s", s.stepPb.Status)
	if len(message) > 0 {
		logMsg += ": " + message
	}
	if len(s.stepPb.SummaryMarkdown) > 0 {
		logMsg += "\n  with SummaryMarkdown:\n" + s.stepPb.SummaryMarkdown
	}
	logf(s.ctx, "%s", logMsg)

	if s.noopMode() {
		return
	}

	panic("not implemented")
}

// Log creates a new step-level line-oriented text log stream with the given name.
//
// You must close the stream when you're done with it.
func (*StepState) Log(name string, opts ...streamclient.Option) (io.WriteCloser, error) {
	panic("implement")
}

// LogDatagram creates a new step-level datagram log stream with the given name.
// Each call to WriteDatagram will produce a single datagram message in the
// stream.
//
// You must close the stream when you're done with it.
func (*StepState) LogDatagram(name string, opts ...streamclient.Option) (streamclient.DatagramStream, error) {
	panic("implement")
}

func (s *StepState) noopMode() bool {
	return s.state == nil
}

// mutate gives exclusive access to read+write stepPb
//
// Will panic if stepPb is in a terminal (ended) state.
func (s *StepState) mutate(cb func()) {
	s.state.excludeCopy(func() {
		s.stepPbMu.Lock()
		defer s.stepPbMu.Unlock()
		if protoutil.IsEnded(s.stepPb.Status) {
			panic(errors.Reason("cannot mutate ended step %q", s.stepPb.Name).Err())
		}
		cb()
	})
}
