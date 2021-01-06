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
	"sync"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	"go.chromium.org/luci/logdog/common/types"
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
	ctx       context.Context
	ctxCloser func()

	// NOTE: copyExclusionMu is "backwards", intentionally.
	//
	// The lock is held in "WRITE" mode in order to do `proto.Clone` on buildPb,
	// since the Clone operation actually can write metadata to the struct, and is
	// not safe with concurrent writes to the proto message.
	//
	// The lock is held in "READ" mode for all other mutations to buildPb; The
	// library has other mutexes to protect indivitual portions of the buildPb
	// from concurrent modification.
	//
	// This is done to allow e.g. multiple Steps to be mutated concurrently, but
	// allow `proto.Clone` to proceed safely.

	copyExclusionMu sync.RWMutex
	buildPb         *bbpb.Build

	logsink    *streamclient.Client
	logNames   nameTracker
	logClosers map[string]func() error

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
	if initial == nil {
		initial = &bbpb.Build{}
	}

	ret := &State{
		buildPb:    proto.Clone(initial).(*bbpb.Build),
		logClosers: map[string]func() error{},
	}
	ret.ctx, ret.ctxCloser = context.WithCancel(ctx)

	for _, opt := range opts {
		opt(ret)
	}

	// initialize proto sections which other code in this module assumes exist.
	proto.Merge(ret.buildPb, &bbpb.Build{
		Output: &bbpb.Build_Output{},
	})

	// in case our buildPb is unstarted, start it now.
	if ret.buildPb.StartTime == nil {
		ret.buildPb.StartTime = timestamppb.New(clock.Now(ctx))
		ret.buildPb.Status = bbpb.Status_STARTED
	}

	// initialize all log names already in ret.buildPb; likely this includes
	// stdout/stderr which may already be populated by our parent process, such as
	// `bbagent`.
	for _, l := range ret.buildPb.Output.Logs {
		ret.logNames.resolveName(l.Name)
	}

	return ret, setState(ctx, ctxState{ret, nil})
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
func (s *State) End(err error) {
	var message string
	s.mutate(func() {
		s.buildPb.Status, message = computePanicStatus(err)
		s.buildPb.EndTime = timestamppb.New(clock.Now(s.ctx))

		// TODO(iannucci): handle closing logs
	})
	// buildPb is immutable after mutate ends, so we should be fine to access it
	// outside the locks.

	logStatus(s.ctx, s.buildPb.Status, message, s.buildPb.SummaryMarkdown)

	s.ctxCloser()
}

// addLog adds a new Log entry to this Step.
//
// `name` is the user-provided name for the log.
//
// `openStream` is a callback which takes
//   * `dedupedName` - the deduplicated version of `name`
//   * `relLdName` - The logdog stream name, relative to this process'
//     LOGDOG_NAMESPACE, suitable for use with s.state.logsink.
func (s *State) addLog(name string, openStream func(dedupedName string, relLdName types.StreamName) io.Closer) {
	relLdName := ""
	s.mutate(func() {
		name = s.logNames.resolveName(name)
		relLdName = fmt.Sprintf("log/%d", len(s.buildPb.Output.Logs))
		s.buildPb.Output.Logs = append(s.buildPb.Output.Logs, &bbpb.Log{
			Name: name,
			Url:  relLdName,
		})
		if closer := openStream(name, types.StreamName(relLdName)); closer != nil {
			s.logClosers[relLdName] = closer.Close
		}
	})
}

// Log creates a new build-level line-oriented text log stream with the given name.
//
// You must close the stream when you're done with it.
func (s *State) Log(name string, opts ...streamclient.Option) io.Writer {
	var ret io.WriteCloser

	if ls := s.logsink; ls != nil {
		s.addLog(name, func(name string, relLdName types.StreamName) io.Closer {
			var err error
			ret, err = ls.NewStream(s.ctx, relLdName, opts...)
			if err != nil {
				panic(err)
			}
			return ret
		})
	}

	return ret
}

// LogDatagram creates a new build-level datagram log stream with the given name.
// Each call to WriteDatagram will produce a single datagram message in the
// stream.
//
// You must close the stream when you're done with it.
func (s *State) LogDatagram(name string, opts ...streamclient.Option) streamclient.DatagramWriter {
	var ret streamclient.DatagramStream

	if ls := s.logsink; ls != nil {
		s.addLog(name, func(name string, relLdName types.StreamName) io.Closer {
			var err error
			ret, err = ls.NewDatagramStream(s.ctx, relLdName, opts...)
			if err != nil {
				panic(err)
			}
			return ret
		})
	}

	return ret
}

// private functions

type ctxState struct {
	state *State
	step  *Step
}

// Returns the step name prefix including terminal "|".
func (c ctxState) stepNamePrefix() string {
	if c.step == nil {
		return ""
	}
	return c.step.name + "|"
}

var contextStateKey = "holds a ctxState"

func setState(ctx context.Context, state ctxState) context.Context {
	return context.WithValue(ctx, &contextStateKey, state)
}

func getState(ctx context.Context) ctxState {
	ret, _ := ctx.Value(&contextStateKey).(ctxState)
	return ret
}

func (s *State) mutate(cb func()) {
	if s != nil {
		s.copyExclusionMu.RLock()
		defer s.copyExclusionMu.RUnlock()

		if protoutil.IsEnded(s.buildPb.Status) {
			panic(errors.New("cannot mutate ended build"))
		}
	}
	cb()
}

func (s *State) registerStep(step *bbpb.Step) (passthrough *bbpb.Step, relLogPrefix, logPrefix string) {
	passthrough = step
	if s == nil {
		return
	}

	s.mutate(func() {
		s.stepsMu.Lock()
		defer s.stepsMu.Unlock()

		step.Name = s.stepNames.resolveName(step.Name)
		s.buildPb.Steps = append(s.buildPb.Steps, step)
		relLogPrefix = fmt.Sprintf("step/%d", len(s.buildPb.Steps)-1)
	})

	logPrefix = relLogPrefix
	if ns := string(s.logsink.GetNamespace()); ns != "" {
		logPrefix = fmt.Sprintf("%s/%s", ns, relLogPrefix)
	}

	return
}
