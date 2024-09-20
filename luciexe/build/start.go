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

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
)

// Start is the 'inner' entrypoint to this library.
//
// If you're writing a standalone luciexe binary, see `Main` and
// `MainWithOutput`.
//
// This function clones `initial` as the basis of all state updates (see
// OptSend) and RegisterProperty declarations. This also initializes the build
// State and properties.State in `ctx` and returns the manipulable State object.
//
// You must End the returned State. To automatically map errors and panics to
// their correct visual representation, End the State like:
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
func Start(ctx context.Context, initial *bbpb.Build, opts ...StartOption) (*State, context.Context, error) {
	if initial == nil {
		initial = &bbpb.Build{}
	}
	initial = proto.Clone(initial).(*bbpb.Build)
	// initialize proto sections which other code in this module assumes exist.
	proto.Merge(initial, &bbpb.Build{
		Output: &bbpb.Build_Output{},
		Input:  &bbpb.Build_Input{},
	})

	logClosers := map[string]func() error{}
	ret := newState(initial, logClosers)

	ret.ctx, ret.ctxCloser = context.WithCancel(ctx)

	for _, opt := range opts {
		opt(ret)
	}

	// in case our buildPb is unstarted, start it now.
	if ret.buildPb.StartTime == nil {
		ret.buildPb.StartTime = timestamppb.New(clock.Now(ctx))
		ret.buildPb.Status = bbpb.Status_STARTED
		ret.buildPb.Output.Status = bbpb.Status_STARTED
	}

	// initialize all log names already in ret.buildPb; likely this includes
	// stdout/stderr which may already be populated by our parent process, such as
	// `bbagent`.
	for _, l := range ret.buildPb.Output.Logs {
		ret.logNames.resolveName(l.Name)
	}

	err := func() (err error) {
		var notifyFn func(version int64)
		if ret.sendCh.C != nil {
			notifyFn = ret.notifyPropertyChange
		}

		pstate, err := Properties.Instantiate(
			ctx,
			initial.GetInput().GetProperties(),
			notifyFn)

		if err != nil {
			return errors.Annotate(err, "build.Start").Err()
		}
		ret.propertyState = pstate
		ret.ctx, err = pstate.SetInContext(ctx)
		return
	}()
	if err != nil {
		err = AttachStatus(err, bbpb.Status_INFRA_FAILURE, nil)
		ret.SetSummaryMarkdown(fmt.Sprintf("fatal error starting build: %s", err))
		ret.End(err)
		return nil, ret.ctx, err
	}

	return ret, setState(ret.ctx, ret), nil
}
