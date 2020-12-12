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
// You must End the returned State. To automatically handle errors and panics,
// End the State like:
//
//    var err error
//    ctx, state := build.Start(ctx, ...)
//    defer state.End(err)
//
//    err = opThatErrsOrPanics(ctx)
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
//    ctx, state := build.Start(ctx, ...)
//    defer state.End(err)
//
//    err = opThatErrsOrPanics(ctx)
func (*State) End(err error) {
	panic("not implemented")
}

// Log creates a new build-level line-oriented text log stream with the given name.
//
// You must close the stream when you're done with it.
func (*State) Log(ctx context.Context, name string) (io.WriteCloser, error) {
	panic("implement")
}

// LogBinary creates a new build-level binary log stream with the given name.
//
// You must close the stream when you're done with it.
func (*State) LogBinary(ctx context.Context, name string) (io.WriteCloser, error) {
	panic("implement")
}

// LogDatagram creates a new build-level datagram log stream with the given name.
// Each call to WriteDatagram will produce a single datagram message in the
// stream.
//
// You must close the stream when you're done with it.
func (*State) LogDatagram(ctx context.Context, name string) (DatagramWriter, error) {
	panic("implement")
}

// LogFile is a helper method which copies the contents of the file at
// `filepath` to the build-level line-oriented log named `name`.
func (*State) LogFile(ctx context.Context, name, filepath string) error {
	panic("implement")
}

// LogBinaryFile is a helper method which copies the contents of the file at
// `filepath` to the build-level binary log named `name`.
func (*State) LogBinaryFile(ctx context.Context, name, filepath string) error {
	panic("implement")
}
