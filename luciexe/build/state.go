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
	"sync"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"google.golang.org/protobuf/proto"
)

// State is the state of the current Build.
//
// This is properly initialized with the Start function, and as long as it isn't
// "End"ed, you can manipulate it with the State's various methods.
//
// The State is preserved in the context.Context for use with the ScheduleStep
// and StartStep functions. These will add a new manipulatable step to the build
// State.
//
// All manipulations to the build State will result in an invocation of the
// configured Send function (see OptSend).
type State struct {
	copyExclusionMu sync.RWMutex
	buildPb         *bbpb.Build

	stepsMu   sync.Mutex
	stepNames nameTracker
}

var _ Loggable = (*State)(nil)

// Start is the entrypoint to this library.
//
// This function clones `initial` as the basis of all state updates (see
// OptSend) and MakePropertyReader declarations. This also initializes the build
// State in `ctx` and returns the manipulable State object.
//
// Start may print information and exit the program immediately if various
// command-line options, such as `--help`, are passed. Use OptSuppressExit() to
// avoid this.
//
// You must End the returned State. To automatically map errors and panics to
// their correct visual representation, End the State like:
//
//    var err error
//    state, ctx := build.Start(ctx, initialBuild, ...)
//    defer func() { state.End(err) }()
//
//    err = opThatErrsOrPanics(ctx)
//
// NOTE: A panic will still crash the program as usual. This does NOT
// `recover()` the panic. Please use conventional Go error handling and control
// flow mechanisms.
func Start(ctx context.Context, initial *bbpb.Build, opts ...StartOption) (*State, context.Context) {
	ret := &State{
		buildPb: proto.Clone(initial).(*bbpb.Build),
	}
	for _, opt := range opts {
		opt(ret)
	}
	return ret, setState(ctx, ctxState{ret, ""})
}

// End sets the build's final status, according to `err` (See ExtractStatus).
//
// End will also be able to set INFRA_FAILURE status and log additional
// information if the program is panic'ing.
//
// End must be invoked like:
//
//    var err error
//    state, ctx := build.Start(ctx, initialBuild, ...)
//    defer func() { state.End(err) }()
//
//    err = opThatErrsOrPanics(ctx)
//
// NOTE: A panic will still crash the program as usual. This does NOT
// `recover()` the panic. Please use conventional Go error handling and control
// flow mechanisms.
func (*State) End(err error) {
	panic("not implemented")
}

// Log creates a new build-level line-oriented text log stream with the given name.
//
// You must close the stream when you're done with it.
func (*State) Log(name string, opts ...streamclient.Option) (io.WriteCloser, error) {
	panic("implement")
}

// LogDatagram creates a new build-level datagram log stream with the given name.
// Each call to WriteDatagram will produce a single datagram message in the
// stream.
//
// You must close the stream when you're done with it.
func (*State) LogDatagram(name string, opts ...streamclient.Option) (streamclient.DatagramStream, error) {
	panic("implement")
}

// private functions

type ctxState struct {
	state *State

	// stepPrefix is the full step prefix including trailing "|"
	stepPrefix string
}

var contextStateKey = "holds a ctxState"

func setState(ctx context.Context, state ctxState) context.Context {
	return context.WithValue(ctx, &contextStateKey, state)
}

func getState(ctx context.Context) ctxState {
	ret, _ := ctx.Value(&contextStateKey).(ctxState)
	return ret
}

func (s *State) excludeCopy(cb func()) {
	if s != nil {
		s.copyExclusionMu.RLock()
		defer s.copyExclusionMu.RUnlock()
	}
	cb()
}

func (s *State) registerStep(step *bbpb.Step) *bbpb.Step {
	if s == nil {
		return step
	}

	s.excludeCopy(func() {
		s.stepsMu.Lock()
		defer s.stepsMu.Unlock()

		step.Name = s.stepNames.resolveName(step.Name)
		s.buildPb.Steps = append(s.buildPb.Steps, step)
	})

	return step
}
