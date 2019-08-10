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

// Package luciexe documents the "LUCI Executable" protocol, and contains
// constants which are part of this protocol.
//
// A LUCI Executable ("luciexe") is a binary which implements a protocol to
//
//   * Read the initial state of the build.
//   * Understand the build's local system contracts (like the location of
//     cached data).
//   * Update the state of the build as it runs.
//
// This protocol is recursive; A luciexe can run another luciexe such
// that the child's updates are reflected on the parent's output.
//
// The luciexe MUST NOT assume/rely on anything that is not explicitly
// specified below.
//
// Initial State
//
// The luciexe MUST read a binary-encoded buildbucket.v2.Build message from
// stdin until EOF.
//
// TODO(iannucci): Document what sections of the input Build MUST be filled.
//
// Local System Environment
//
//   - The luciexe will be started with an empty current working directory
//     (CWD). This directory will be removed in between invocations of
//     luciexe's.
//   - $TMPDIR, $TEMP, $TMP: env variables all point to the same, empty
//     directory. This directory is located on the same file system as CWD.
//     This directory will be removed in between invocations of luciexe's.
//   - Logdog butler environment variables will be set such that the following
//     libraries can be used to stream log data:
//       * Golang: go.chromium.org/luci/logdog/client/butlerlib/bootstrap
//       * Python: infra_libs.logdog.bootstrap
//
// In addition, the "luciexe", "local_auth" and "swarming" sections of
// LUCI_CONTEXT will be filled, as documented at:
//   https://chromium.googlesource.com/infra/luci/luci-py/+/HEAD/client/LUCI_CONTEXT.md
//
// Binaries available in $PATH
//
// TODO(iannucci): Document this.
//
// Updating the Build State
//
// A luciexe MUST update the build state at least once (to set the final
// outcome status). This is accomplished by writing to a "build.proto" Logdog
// stream named "$LOGDOG_NAMESPACE/build.proto". A "build.proto" Logdog stream
// is defined as:
//
//   Content-Type: "application/luci+proto; message=buildbucket.v2.Build"
//   Type: Datagram
//
// Each datagram must be a valid binary-encoded buildbucket.v2.Build message.
// Each datagram written to the stream will be used to update the server side
// representation of this build as it runs.
//
// All Step.Log urls in the emitted Build messages MUST start with
// $LOGDOG_NAMESPACE. For example, if LOGDOG_STREAM_PREFIX == "infra/prefix" and
// LOGDOG_NAMESPACE = "u", then a Step.Log entry with a Url field of
// "u/hello/world" would refer to "logdog.host/infra/prefix/+/u/hello/world".
// The `ViewUrl` field SHOULD be left empty, and will be filled in by the
// application running the luciexe.
//
// Recursive Invocation (WIP)
//
// NOTE: This feature is not yet implemented as of 07.08.2019.
//
// LUCI Executables MAY invoke other LUCI Executables as sub-steps and have the
// Steps from the child luciexe show in the parent's Build updates.
//
// The parent can achieve this by recording a Step (with no children), and
// a single Step.Log named "$build.proto" which points to a "build.proto" Logdog
// stream (see above).  The steps from the child stream will appear as substeps
// of step S in the parent. This rule applies recursively, i.e. a leaf step in
// the child build MAY also have a "$build.proto" log.
//
// The graph of datagram streams MUST be acyclic. Typically it should be a tree,
// but having two leaves point to the same substream is not checked
// for/rejected.
//
// Known Implementations
//
// For writing 'raw' executables in Go, use "go.chromium.org/luci/luciexe/exe",
// which implements a very simple client of this protocol.
//
// Low-level are available in "go.chromium.org/luci/luciexe" (this module).
//
// Python Recipes (https://chromium.googlesource.com/infra/luci/recipes-py) also
// implement the LUCI Executable protocol using their "luciexe" subcommand.
//
// LUCI Executables on Buildbucket
//
// Buildbucket accepts LUCI Executables as CIPD packages containing the
// luciexe to run with the fixed name of "luciexe".
//
// On Windows, this may be named "luciexe.exe" or "luciexe.bat". Buildbucket
// will prefer the first of these which it finds.
package luciexe
