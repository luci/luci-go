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
// Summary
//
// A LUCI Executable ("luciexe") is a binary which implements a protocol to:
//
//  * Pass the initial state of the 'build' from a parent process to the luciexe.
//  * Understand the build's local system contracts (like the location of cached data).
//  * Update the state of the build as it runs.
//
// This protocol is recursive; A luciexe can run another luciexe such that the
// child's updates are reflected on the parent's output.
//
// The Host Application
//
// At the root of every tree of luciexe invocations there is a 'host'
// application which sets up an manages all environmental singletons (like the
// Logdog 'butler' service, the luci-auth service, etc.). This host application
// is responsible for intercepting and merging all 'build.proto' streams emitted
// within this tree of luciexes (see "Recursive Invocation"). The host
// application may choose what happens to these intercepted build.proto
// messages.
//
// For example: the `go.chromium.org/luci/buildbucket/cmd/agent` binary forwards
// these merged build.proto messages to the Buildbucket service, and also
// uploads all streams to them to the Logdog cloud service.
//
// Other host implementations may instead choose to write all streams to disk,
// send them to /dev/null or render them as html. However, from the point of
// view of the luciexe that they run, this is transparent.
//
// Host applications MAY implement 'backpressure' on the luciexe binaries by
// throttling the rate at which the Logdog butler accepts data on its various
// streams. However, doing this could introduce timing issues in the luciexe
// binaries as they try to run, so this should be done thoughtfully.
//
// If a host application detects a protocol violation from a luciexe within its
// purview, it SHOULD report the violation (in a manner of its chosing) and MUST
// consider the entire Build status to be INFRA_FAILURE.
//
// Initial State
//
// The luciexe MUST read a binary-encoded buildbucket.v2.Build message from
// stdin until EOF. It MUST NOT assume that any particular fields in the Build
// message are set. However, the host application SHOULD fill in as much of the
// Build proto (particularly the 'infra' section) as appropriate.
//
// The host process MUST collect the stdout/stderr of the luciexe and treat it
// as opaque streams. Typical luciexe implementations will use these for debug
// logging and output, but are not required to do so.
//
// Local System Environment
//
// Every luciexe:
//
//   * MUST be started with an empty current working directory (CWD).
//     This directory MUST be removed in between invocations of luciexe's.
//   * MUST have $TMPDIR, $TEMP, $TMP all point to the same, empty directory.
//     This directory MUST be removed in between invocations of luciexe's.
//     This directory MUST be located on the same file system as CWD.
//     This directory MUST NOT be the same as CWD.
//   * MUST be invoked with all Logdog butler environment variables set such
//     that the following libraries can be used to stream log data:
//       * Golang: go.chromium.org/luci/logdog/client/butlerlib/bootstrap
//       * Python: infra_libs.logdog.bootstrap
//     * The LOGDOG_NAMESPACE will be set to a prefix which accumulates as
//       luciexe's invoke sub-luciexes.
//
// The CWD and TMPDIR directory requirements apply to sub-luciexe's as well! If
// the parent process would like the sub-luciexe to interact with the parent's
// working or temporary directories, this can be communicated by explicitly
// passing some property (in the input Build message) from parent to child.
//
// In addition, the "luciexe" and "local_auth" sections of LUCI_CONTEXT MUST be
// filled, as documented at
// https://chromium.googlesource.com/infra/luci/luci-py/+/HEAD/client/LUCI_CONTEXT.md
//
// Other sections of LUCI_CONTEXT MAY be filled (such as "swarming").
//
//   !!NOTE!! The paths supplied to the luciexe MUST NOT be considered stable
//   across invocations. Do not hard-code these, and try not to rely on their
//   consistency (e.g. for build reproducibility).
//
// Updating the Build State
//
// A luciexe MAY update the Build state by writing to a "build.proto" Logdog
// stream named "$LOGDOG_NAMESPACE/build.proto". A "build.proto" Logdog stream
// is defined as:
//
//   Content-Type: "application/luci+proto; message=buildbucket.v2.Build"
//   Type: Datagram
//
// Each datagram MUST be a valid binary-encoded buildbucket.v2.Build message.
//
// All Step.Log urls in the emitted Build messages MUST start with
// $LOGDOG_NAMESPACE. For example, if LOGDOG_STREAM_PREFIX == "infra/prefix" and
// LOGDOG_NAMESPACE = "u", then a Step.Log entry with a Url field of
// "u/hello/world" would refer to "logdog.host/infra/prefix/+/u/hello/world".
// The `ViewUrl` field SHOULD be left empty, and will be filled in by the
// host running the luciexe (if supplied it will be overwritten).
//
// Reporting final status
//
// A luciexe MUST report its success/failure by sending a Build message with
// a terminal `status` value before exiting. If the luciexe exits before sending
// a Build message with a terminal Status, the host application MUST interpret
// this as an INFRA_FAILURE status. The exit code of the luciexe SHOULD be
// ignored, except for advisory (logging/reporting) purposes.
//
// To support recursive invocation, a luciexe MUST accept the flag:
//
//   --output=path/to/file.{pb,json,textpb}
//
// The value of this flag MUST be an absolute path to a non-existant file in
// an existing directory. The extension of the file dictates the data format
// (binary, json or text protobuf). The luciexe MUST write it's final Build
// message to this file in the correct format. If `--output-path` is specified,
// but no Build message (or an invalid/improperly formatted Build message)
// is written, the caller MUST interpret this as an INFRA_FAILURE status.
//
// Recursive Invocation
//
// LUCI Executables MAY invoke other LUCI Executables as sub-steps and have the
// Steps from the child luciexe show in the parent's Build updates. This is one
// of the responsibilities of the host application.
//
// The parent can achieve this by recording a Step (with no children), and
// a single Step.Log named "$build.proto" which points to a "build.proto" stream
// (see "Updating the Build State"). The steps from the child stream will appear
// as substeps of step S in the parent. This rule applies recursively, i.e.
// leaf step(s) in the child build MAY also have a "$build.proto" log.
//
// Each luciexe's step names should be emited as relative names. e.g. say
// a build runs a sub-luciexe with the name "a|b". This sub-luciexe then runs
// a step "x|y|z". The top level build.proto stream will show the step
// "a|b|x|y|z".
//
// The graph of datagram streams MUST NOT contain cycles. Typically, it should
// be a tree, but having two leaves point to the same substream is not checked
// for/rejected.
//
//   NOTE: Recursive invocation is not yet implemented ("WIP") as of Aug 12, 2019.
//
// Known Client Implementations
//
// Python Recipes (https://chromium.googlesource.com/infra/luci/recipes-py)
// implement the LUCI Executable protocol using the "luciexe" subcommand.
//
// In Go use "go.chromium.org/luci/luciexe/exe", which implements a very simple
// client of this protocol.
//
//   TODO(iannucci): Implement a simple client in `infra_libs` analagous to
//   go.chromium.org/luci/luciexe/exe and implement Recipes' support in terms
//   of this.
//
// Related Libraries
//
// "go.chromium.org/luci/luciexe" - Low-level protocol details (this module).
// "go.chromium.org/luci/luciexe/host" - Implementation to run the top-level
//    process in a tree of luciexe's.
// "go.chromium.org/luci/luciexe/invoke" - Implementation to run
//    a luciexe subprocess in a tree of luciexe's.
//
// LUCI Executables on Buildbucket
//
// Buildbucket accepts LUCI Executables as CIPD packages containing the
// luciexe to run with the fixed name of "luciexe".
//
// On Windows, this may be named "luciexe.exe" or "luciexe.bat". Buildbucket
// will prefer the first of these which it finds.
//
// Note that the `recipes.py bundle` command generates a "luciexe" wrapper
// script for compatibility with Buildbucket.
package luciexe
