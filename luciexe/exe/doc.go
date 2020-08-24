// Copyright 2019 The LUCI Authors.
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

// Package exe implements a client for the LUCI Executable ("luciexe") protocol.
//
// The simplest luciexe is:
//
//   import (
//     "context"
//
//     "go.chromium.org/luci/luciexe/exe"
//     "go.chromium.org/luci/luciexe/exe/build"
//   )
//
//   func main() {
//     exe.Run(func(ctx context.Context, state *build.State, userArgs []string) error {
//       ... do whatever you want here ...
//       return nil // nil error indicates successful build.
//     })
//   }
//
// See Also: https://go.chromium.org/luci/luciexe
package exe

import (
	"context"
	"os"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/luciexe/exe/build"
)

// MainFn is the function signature you must implement in your callback to Run.
//
// Args:
//  - ctx: The context will be canceled when the program receives the os
//   Interrupt or SIGTERM (on unix) signal. The context has the following
//   libraries enabled:
//      * go.chromium.org/luci/common/logging
//      * go.chromium.org/luci/common/system/environ
//      * The Build modification functions in this package (i.e. everything
//        enabled by SinkBuildUpdates).
//  - build: The Build state, initialized with the Build read from stdin.
//  - userArgs: All command line arguments supplied after first `--`.
//
// Note: You MUST use the environment from `environ.FromCtx` when inspecting the
// environment or launching subprocesses. In order to ensure this, the process
// environment is cleared from the time that MainFn starts.
type MainFn func(ctx context.Context, build *build.State, userargs []string) error

// Run executes the `main` callback.
//
// To implement the luciexe protocol:
//
//   func main() {
//     exe.Run(func(ctx context.Context, input *exe.Build, userArgs []string) error {
//       ... do whatever you want here ...
//       return nil // nil error indicates successful build.
//     })
//   }
//
// This calls os.Exit on completion of `main`, or panics if something went
// wrong.
//
// If main panics, this is converted to an INFRA_FAILURE. Otherwise main's
// returned error is converted to a build status with GetErrorStatus.
func Run(main MainFn, options ...RunOption) {
	os.Exit(runImpl(os.Args, options, mkRun(main)))
}

// MainRawFn is the function signature you must implement in your callback to
// RunRaw.
//
// Args:
//  - ctx: The context will be canceled when the program receives the os
//   Interrupt or SIGTERM (on unix) signal. The context has the following
//   libraries enabled:
//      * go.chromium.org/luci/common/logging
//      * go.chromium.org/luci/common/system/environ
//      * The Build modification functions in this package (i.e. everything
//        enabled by SinkBuildUpdates).
//  - build: The Build state, initialized with the Build read from stdin.
//  - userArgs: All command line arguments supplied after first `--`.
//
// Note: You MUST use the environment from `environ.FromCtx` when inspecting the
// environment or launching subprocesses. In order to ensure this, the process
// environment is cleared from the time that MainFn starts.
type MainRawFn func(ctx context.Context, build *bbpb.Build, userargs []string, sendFn func()) error

// RunRaw executes the `mainRaw` callback.
//
// Unless you know what you're doing, you want Run instead.
// If you are unsure if you know what you're doing, you want Run instead.
//
// This grants you a context with everything Run does except for `exe/build`,
// and requires you to manage ALL synchronization around the `Build` proto
// message.
//
// If you manually assign build.Status, you must also assign build.EndTime.
// Otherwise, both will be filled in according to build.GetErrorStatus. If your
// callback panics, it's equivalent to returning build.ErrCallbackPaniced.
//
// This calls os.Exit on completion of `mainRaw`, or panics if something went
// wrong.
func RunRaw(mainRaw MainRawFn, options ...RunOption) {
	os.Exit(runImpl(os.Args, options, mkRawRun(mainRaw)))
}
