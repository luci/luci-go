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

	bbpb "go.chromium.org/luci/buildbucket/proto"
)

// State is the state of the current Build.
//
// This is properly initialized with the Start function, and as long as it isn't
// "End"ed, you can manipulate it with the State's various methods.
//
// The State is preserved in the context.Context for use with the ScheduleStep
// and Step functions. These will add a new manipulatable step to the build
// State.
//
// All manipulations to the build State will result in an invocation of the
// configured Send function (see OptSend).
type State struct{}

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
//    state, ctx := build.Start(ctx, ...)
//    defer func() { state.End(err) }()
//
//    err = opThatErrsOrPanics(ctx)
//
// NOTE: A panic will still crash the program as usual. This does NOT
// `recover()` the panic. Please use conventional Go error handling and control
// flow mechanisms.
func Start(ctx context.Context, initial *bbpb.Build, opts ...StartOption) (*State, context.Context) {
	panic("not implemented")
}

// End sets the build's final status, according to `err` (See ExtractStatus).
//
// End will also be able to set INFRA_FAILURE status and log additional
// information if the program is panic'ing.
//
// End must be invoked like:
//
//    var err error
//    state, ctx := build.Start(ctx, ...)
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
