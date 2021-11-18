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
	buildPb   *bbpb.Build
	// buildPbVers updated/read with sync/atomic while holding buildPbMu in
	// either WRITE/READ mode.
	buildPbVers int64
	// buildPbVersSent only updated when buildPbMu is held in WRITE mode.
	buildPbVersSent int64

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

var _ Loggable = (*State)(nil)

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
	s.mutate(func() bool {
		s.buildPb.Status, message = computePanicStatus(err)
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
func (s *State) addLog(name string, openStream func(dedupedName string, relLdName ldTypes.StreamName) io.Closer) {
	relLdName := ""
	s.mutate(func() bool {
		name = s.logNames.resolveName(name)
		relLdName = fmt.Sprintf("log/%d", len(s.buildPb.Output.Logs))
		s.buildPb.Output.Logs = append(s.buildPb.Output.Logs, &bbpb.Log{
			Name: name,
			Url:  relLdName,
		})
		if closer := openStream(name, ldTypes.StreamName(relLdName)); closer != nil {
			s.logClosers[relLdName] = closer.Close
		}
		return true
	})
}

// Log creates a new build-level line-oriented text log stream with the given name.
//
// You must close the stream when you're done with it.
func (s *State) Log(name string, opts ...streamclient.Option) io.Writer {
	var ret io.WriteCloser

	if ls := s.logsink; ls != nil {
		s.addLog(name, func(name string, relLdName ldTypes.StreamName) io.Closer {
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

// Infra returns a clone of the Build.Infra submessage.
func (s *State) Infra() *bbpb.BuildInfra {
	s.buildPbMu.RLock()
	defer s.buildPbMu.RUnlock()
	if s.buildPb.Infra == nil {
		return nil
	}
	return proto.Clone(s.buildPb.Infra).(*bbpb.BuildInfra)
}

// SynthesizeIOProto synthesizes a `.proto` file from the input and ouptut
// property messages declared at Start() time.
func (s *State) SynthesizeIOProto(o io.Writer) error {
	_, err := iotools.WriteTracker(o, func(o io.Writer) error {
		_ = func(format string, a ...interface{}) { fmt.Fprintf(o, format, a...) }
		// TODO(iannucci): implement
		return nil
	})
	return err
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

// Allows reads from buildPb and also must be held when sub-messages within
// buildPb are being written to.
//
// cb returns true if some portion of buildPB was mutated.
func (s *State) excludeCopy(cb func() bool) {
	if s != nil {
		s.buildPbMu.RLock()
		defer s.buildPbMu.RUnlock()

		if protoutil.IsEnded(s.buildPb.Status) {
			panic(errors.New("cannot mutate ended build"))
		}
	}
	changed := cb()
	if changed && s != nil && s.sendCh.C != nil {
		s.sendCh.C <- atomic.AddInt64(&s.buildPbVers, 1)
	}
}

// cb returns true if some portion of buildPB was mutated.
//
// Allows writes to s.buildPb
func (s *State) mutate(cb func() bool) {
	if s != nil {
		s.buildPbMu.Lock()
		defer s.buildPbMu.Unlock()

		if protoutil.IsEnded(s.buildPb.Status) {
			panic(errors.New("cannot mutate ended build"))
		}
	}
	changed := cb()
	if changed && s != nil && s.sendCh.C != nil {
		s.sendCh.C <- atomic.AddInt64(&s.buildPbVers, 1)
	}
}

func (s *State) registerStep(step *bbpb.Step) (passthrough *bbpb.Step, relLogPrefix, logPrefix string) {
	passthrough = step
	if s == nil {
		return
	}

	s.mutate(func() bool {
		step.Name = s.stepNames.resolveName(step.Name)
		s.buildPb.Steps = append(s.buildPb.Steps, step)
		relLogPrefix = fmt.Sprintf("step/%d", len(s.buildPb.Steps)-1)

		return true
	})

	logPrefix = relLogPrefix
	if ns := string(s.logsink.GetNamespace()); ns != "" {
		logPrefix = fmt.Sprintf("%s/%s", ns, relLogPrefix)
	}

	return
}
