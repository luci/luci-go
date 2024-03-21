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
	"sync/atomic"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/iotools"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/sync/dispatcher"
	"go.chromium.org/luci/logdog/client/butlerlib/streamclient"
	ldTypes "go.chromium.org/luci/logdog/common/types"
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

	// inputBuildPb represents the build when state was created.
	// This is used to provide client access to input build.
	inputBuildPb *bbpb.Build
	// buildPbMu is held in "WRITE" mode whenever buildPb may be directly written
	// to, or in order to do `proto.Clone` on buildPb (since the Clone operation
	// actually can write metadata to the struct), and is not safe with concurrent
	// writes to the proto message.
	//
	// buildPbMu is held in "READ" mode for all other reads of the buildPb; The
	// library has other mutexes to protect indivitual portions of the buildPb
	// from concurrent modification.
	//
	// This is done to allow e.g. multiple Steps to be mutated concurrently, but
	// allow `proto.Clone` to proceed safely.
	buildPbMu sync.RWMutex
	// buildPb represents the live build.
	buildPb *bbpb.Build
	// buildPbVers updated/read while holding buildPbMu in either WRITE/READ mode.
	buildPbVers atomic.Int64
	// buildPbVersSent only updated when buildPbMu is held in WRITE mode.
	buildPbVersSent atomic.Int64

	sendCh dispatcher.Channel

	logsink    *streamclient.Client
	logNames   nameTracker
	logClosers map[string]func() error

	strictParse bool

	reservedInputProperties map[string]proto.Message
	topLevelInputProperties proto.Message

	// Note that outputProperties is statically allocated at Start time; No keys
	// are added/removed for the duration of the Build.
	outputProperties map[string]*outputPropertyState
	topLevelOutput   *outputPropertyState

	stepNames nameTracker
}

// newState creates a new state.
func newState(inputBuildPb *bbpb.Build, logClosers map[string]func() error, outputProperties map[string]*outputPropertyState) *State {
	state := &State{
		buildPb:          inputBuildPb,
		logClosers:       logClosers,
		outputProperties: outputProperties,
	}

	if inputBuildPb != nil {
		state.inputBuildPb = proto.Clone(inputBuildPb).(*bbpb.Build)
	}

	return state
}

var _ Loggable = (*State)(nil)

// End sets the build's final status, according to `err` (See ExtractStatus).
//
// End will also be able to set INFRA_FAILURE status and log additional
// information if the program is panic'ing.
//
// End must be invoked like:
//
//	var err error
//	state, ctx := build.Start(ctx, initialBuild, ...)
//	defer func() { state.End(err) }()
//
//	err = opThatErrsOrPanics(ctx)
//
// NOTE: A panic will still crash the program as usual. This does NOT
// `recover()` the panic. Please use conventional Go error handling and control
// flow mechanisms.
func (s *State) End(err error) {
	var message string
	s.mutate(func() bool {
		s.buildPb.Output.Status, message = computePanicStatus(err)
		s.buildPb.Status = s.buildPb.Output.Status
		s.buildPb.EndTime = timestamppb.New(clock.Now(s.ctx))

		for logName, closer := range s.logClosers {
			if err := closer(); err != nil {
				logging.Warningf(s.ctx, "error closing log %q: %s", logName, err)
			}
		}
		s.logClosers = nil

		return true
	})
	// buildPb is immutable after mutate ends, so we should be fine to access it
	// outside the locks.

	if s.sendCh.C != nil {
		s.sendCh.CloseAndDrain(s.ctx)
	}

	if s.logsink == nil || s.buildPb.Output.Status != bbpb.Status_SUCCESS {
		// If we're panicking, we need to log. In a situation where we have a log
		// sink (i.e. a real build), all other information is already reflected via
		// the Build message itself.
		logStatus(s.ctx, s.buildPb.Output.Status, message, s.buildPb.SummaryMarkdown)
	}

	s.ctxCloser()
}

// addLog adds a new Log entry to this Step.
//
// `name` is the user-provided name for the log.
//
// `openStream` is a callback which takes
//   - `dedupedName` - the deduplicated version of `name`
//   - `relLdName` - The logdog stream name, relative to this process'
//     LOGDOG_NAMESPACE, suitable for use with s.state.logsink.
func (s *State) addLog(name string, openStream func(dedupedName string, relLdName ldTypes.StreamName) io.Closer) *bbpb.Log {
	var logRef *bbpb.Log
	s.mutate(func() bool {
		name = s.logNames.resolveName(name)
		relLdName := fmt.Sprintf("log/%d", len(s.buildPb.Output.Logs))
		logRef = &bbpb.Log{
			Name: name,
			Url:  relLdName,
		}
		s.buildPb.Output.Logs = append(s.buildPb.Output.Logs, logRef)
		if closer := openStream(name, ldTypes.StreamName(relLdName)); closer != nil {
			s.logClosers[relLdName] = closer.Close
		}
		return true
	})
	return logRef
}

// Log creates a new step-level line-oriented text log stream with the given name.
// Returns a Log value which can be written to directly, but also provides additional
// information about the log itself.
//
// The stream will close when the state is End'd.
func (s *State) Log(name string, opts ...streamclient.Option) *Log {
	if ls := s.logsink; ls != nil {
		var ret io.WriteCloser
		logRef := s.addLog(name, func(name string, relLdName ldTypes.StreamName) io.Closer {
			var err error
			ret, err = ls.NewStream(s.ctx, relLdName, opts...)
			if err != nil {
				panic(err)
			}
			return ret
		})
		var infra *bbpb.BuildInfra_LogDog
		if b := s.Build(); b != nil {
			infra = b.GetInfra().GetLogdog()
		}
		return &Log{
			Writer:    ret,
			ref:       logRef,
			namespace: s.logsink.GetNamespace().AsNamespace(),
			infra:     infra,
		}
	}
	return nil
}

// LogDatagram creates a new build-level datagram log stream with the given name.
// Each call to WriteDatagram will produce a single datagram message in the
// stream.
//
// You must close the stream when you're done with it.
func (s *State) LogDatagram(name string, opts ...streamclient.Option) streamclient.DatagramWriter {
	var ret streamclient.DatagramStream

	if ls := s.logsink; ls != nil {
		s.addLog(name, func(name string, relLdName ldTypes.StreamName) io.Closer {
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

// Build returns a copy of the initial Build state.
//
// This is useful to access fields such as Infra, Tags, Ancestor ids etc.
//
// Changes to this copy will not reflect anywhere in the live Build state and
// not affect other calls to Build().
//
// NOTE: It is recommended to use the PropertyModifier/PropertyReader functionality
// of this package to interact with Build Input Properties; They are encoded as
// Struct proto messages, which are extremely cumbersome to work with directly.
func (s *State) Build() *bbpb.Build {
	if s.inputBuildPb == nil {
		return nil
	}
	return proto.Clone(s.inputBuildPb).(*bbpb.Build)
}

// SynthesizeIOProto synthesizes a `.proto` file from the input and ouptut
// property messages declared at Start() time.
func (s *State) SynthesizeIOProto(o io.Writer) error {
	_, err := iotools.WriteTracker(o, func(o io.Writer) error {
		_ = func(format string, a ...any) { fmt.Fprintf(o, format, a...) }
		// TODO(iannucci): implement
		return nil
	})
	return err
}

// private functions

var buildStateKey = "holds the *State"

func setState(ctx context.Context, s *State) context.Context {
	return context.WithValue(ctx, &buildStateKey, s)
}

func getState(ctx context.Context) *State {
	ret, _ := ctx.Value(&buildStateKey).(*State)
	return ret
}

var currentStepKey = "holds the current *Step"

func setCurrentStep(ctx context.Context, s *Step) context.Context {
	return context.WithValue(ctx, &currentStepKey, s)
}

// getCurrentStep returns the current Step from the context, if one is set.
//
// If no Step is set in the context, this returns `nil`.
//
// This is non-exported to keep the ownership of Step objects in user code
// clear. Otherwise it would be possible to directly get the current step and
// mutate it at a distance. If user code wants that to happen, it can pass Step
// explicitly.
func getCurrentStep(ctx context.Context) *Step {
	ret, _ := ctx.Value(&currentStepKey).(*Step)
	return ret
}

// Allows reads from buildPb and also must be held when sub-messages within
// buildPb are being written to.
//
// cb returns true if some portion of buildPB was mutated.
func (s *State) excludeCopy(cb func() bool) {
	if s != nil {
		s.buildPbMu.RLock()
		defer s.buildPbMu.RUnlock()

		if protoutil.IsEnded(s.buildPb.Output.Status) {
			panic(errors.New("cannot mutate ended build"))
		}
	}
	changed := cb()
	if changed && s != nil && s.sendCh.C != nil {
		s.sendCh.C <- s.buildPbVers.Add(1)
	}
}

// cb returns true if some portion of buildPB was mutated.
//
// Allows writes to s.buildPb
func (s *State) mutate(cb func() bool) {
	if s != nil {
		s.buildPbMu.Lock()
		defer s.buildPbMu.Unlock()

		if protoutil.IsEnded(s.buildPb.Output.Status) {
			panic(errors.New("cannot mutate ended build"))
		}
	}
	changed := cb()
	if changed && s != nil && s.sendCh.C != nil {
		s.sendCh.C <- s.buildPbVers.Add(1)
	}
}

func (s *State) registerStep(step *bbpb.Step) (passthrough *bbpb.Step, logNamespace, logSuffix string) {
	passthrough = step
	if s == nil {
		return
	}

	s.mutate(func() bool {
		step.Name = s.stepNames.resolveName(step.Name)
		s.buildPb.Steps = append(s.buildPb.Steps, step)
		logSuffix = fmt.Sprintf("step/%d", len(s.buildPb.Steps)-1)

		return true
	})
	logNamespace = string(s.logsink.GetNamespace())

	return
}
