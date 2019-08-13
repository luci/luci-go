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
//  * Asynchronously update the state of the build as it runs.
//
// This protocol is recursive; A luciexe can run another luciexe such that the
// child's updates are reflected on the parent's output.
//
// The protocol has 3 parts:
//
//  * Host Application - This sits at the top of the luciexe process
//    hierarchy and sets up singleton environmental requirements for the whole
//    tree.
//  * Invocation of a luciexe binary - This invocation process occurs both
//    for the topmost luciexe, as well as all internal invocations of other
//    luciexe's within the process hierarchy.
//  * The luciexe binary - The binary has a couple of responsibilities to be
//    compatible with this protocol. Once the binary has fulfilled it's
//    responsibilities it's free to do what it wants (i.e. actually do its task).
//
// In general, we strive where possible to minimize the complexity of the
// luciexe binary. This is because we expect to have a small number of 'host
// application' implementations, and a relatively large number of 'luciexe'
// implementations.
//
// The Host Application
//
// At the root of every tree of luciexe invocations there is a 'host'
// application which sets up an manages all environmental singletons (like the
// Logdog 'butler' service, the LUCI ambient authentication service, etc.). This
// host application is responsible for intercepting and merging all
// 'build.proto' streams emitted within this tree of luciexes (see "Recursive
// Invocation"). The host application may choose what happens to these
// intercepted build.proto messages.
//
// The host application MUST:
//   * Run a logdog butler service and expose all relevant LOGDOG_* environment
//     variables such that the following client libraries can stream log data:
//       * Golang: go.chromium.org/luci/logdog/client/butlerlib/bootstrap
//       * Python: infra_libs.logdog.bootstrap
//   * Hook the butler to intercept and merge build.proto streams into a single
//     build.proto stream.
//   * Set up a local LUCI ambient authentication service which luciexe's can
//     use to mint auth tokens.
//
// The host application MAY hook additional streams for debugging/logging; it is
// frequently convenient to hook the stderr/stdout streams from the top level
// luciexe and tee them to the host application's stdout/stderr.
//
// For example: the `go.chromium.org/luci/buildbucket/cmd/agent` binary forwards
// these merged build.proto messages to the Buildbucket service, and also
// uploads all streams to them to the Logdog cloud service. Other host
// implementations may instead choose to write all streams to disk, send them to
// /dev/null or render them as html. However, from the point of view of the
// luciexe that they run, this is transparent.
//
// Host applications MAY implement 'backpressure' on the luciexe binaries by
// throttling the rate at which the Logdog butler accepts data on its various
// streams. However, doing this could introduce timing issues in the luciexe
// binaries as they try to run, so this should be done thoughtfully.
//
// If a host application detects a protocol violation from a luciexe within its
// purview, it SHOULD report the violation (in a manner of its choosing) and
// MUST consider the entire Build status to be INFRA_FAILURE. In addition the
// host application SHOULD attempt to kill (via process group SIGTERM/SIGKILL on
// *nix, and CTRL+BREAK/Terminate on windows) the luciexe hierarchy. The host
// application MAY provide a window of time between the initial "please stop"
// signal and the "you die now" signal, but this comes with the usual caveats of
// cleanups and deadlines (notably: best-effort clean up is just that:
// best-effort. It cannot be relied on to run to completion (or to run
// completely)).
//
// Invocation
//
// When invoking a luciexe, the parent process has a couple responsibilities. It
// must:
//
//   * Start the luciexe with an empty current working directory (CWD).
//     This directory MUST be removed in between invocations of luciexe's.
//
//   * Set $TEMPDIR, $TMPDIR, $TEMP, $TMP and $MAC_CHROMIUM_TMPDIR all point to
//     the same, empty directory.
//     This directory MUST be removed in between invocations of luciexe's.
//     This directory MUST be located on the same file system as CWD.
//     This directory MUST NOT be the same as CWD.
//
//   * Set $LUCI_CONTEXT["luciexe"]["cache_dir"] to a cache dir which makes sense
//     for the luciexe.
//     The cache dir MAY persist/be shared between luciexe invocations.
//     The cache dir MAY NOT be on the same filesystem as CWD.
//
//   * Set the $LOGDOG_NAMESPACE to a prefix which namespaces all logdog streams
//     generated from the luciexe.
//
// The invoker MUST attach the stdout/stderr to the logdog butler as text
// streams. Typical luciexe implementations will use these for debug logging and
// output, but are not required to do so.
//
// The invoker MUST write a binary-encoded buildbucket.v2.Build to the stdin of
// the luciexe which contains all the input parameters that the luciexe needs to
// know to run successfully. No fields are required, but the invoker SHOULD fill
// in as much of the Build proto as appropriate.
//
// The luciexe binary
//
// Once running, the luciexe MUST read a binary-encoded buildbucket.v2.Build
// message from stdin until EOF. It MUST NOT assume that any particular fields
// in the Build message are set. However, the host application MAY fill in any
// fields it thinks are useful.
//
// As per the host application's responsibilities, the luciexe binary MAY expect
// the "luciexe" and "local_auth" sections of LUCI_CONTEXT to be filled. Other
// sections of LUCI_CONTEXT MAY also be filled. See the LUCI_CONTEXT docs:
// https://chromium.googlesource.com/infra/luci/luci-py/+/HEAD/client/LUCI_CONTEXT.md
//
//   !!NOTE!! The paths supplied to the luciexe MUST NOT be considered stable
//   across invocations. Do not hard-code these, and try not to rely on their
//   consistency (e.g. for build reproducibility).
//
// The luciexe binary - Updating the Build state
//
// A luciexe MAY update the Build state by writing to a "build.proto" Logdog
// stream named "$LOGDOG_NAMESPACE/build.proto". A "build.proto" Logdog stream
// is defined as:
//
//   Content-Type: "application/luci+proto; message=buildbucket.v2.Build"
//   Type: Datagram
//
// Each datagram MUST be a valid binary-encoded buildbucket.v2.Build message.
// The state of the build is defined as the last Build message sent on this
// stream. There's no implicit accumulation between sent Build messages.
//
// All Step.Log urls in the emitted Build messages MUST start with
// $LOGDOG_NAMESPACE. For example, if $LOGDOG_STREAM_PREFIX == "infra/prefix"
// and $LOGDOG_NAMESPACE = "u", then a Step.Log entry with a Url field of
// "u/hello/world" would refer to "logdog.host/infra/prefix/+/u/hello/world".
// The `ViewUrl` field SHOULD be left empty, and will be filled in by the
// host running the luciexe (if supplied it will be overwritten).
//
// The luciexe binary - Reporting final status
//
// A luciexe MUST report its success/failure by sending a Build message with
// a terminal `status` value before exiting. If the luciexe exits before sending
// a Build message with a terminal Status, the invoking application MUST
// interpret this as an INFRA_FAILURE status. The exit code of the luciexe
// SHOULD be ignored, except for advisory (logging/reporting) purposes. The host
// application MUST detect this case and fill in a final status of
// INFRA_FAILURE, but MUST NOT terminate the process hierarchy in this case.
//
// Recursive Invocation
//
// To support recursive invocation, a luciexe MUST accept the flag:
//
//   --output=path/to/file.{pb,json,textpb}
//
// The value of this flag MUST be an absolute path to a non-existent file in
// an existing directory. The extension of the file dictates the data format
// (binary, json or text protobuf). The luciexe MUST write it's final Build
// message to this file in the correct format. If `--output` is specified,
// but no Build message (or an invalid/improperly formatted Build message)
// is written, the caller MUST interpret this as an INFRA_FAILURE status.
//
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
// Each luciexe's step names should be emitted as relative names. e.g. say
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
// Related Libraries
//
// For implementation-level details, please refer to the following:
//
//  * "go.chromium.org/luci/luciexe" - low-level protocol details (this module).
//  * "go.chromium.org/luci/luciexe/host" - the host application library.
//  * "go.chromium.org/luci/luciexe/invoke" - luciexe invocation.
//  * "go.chromium.org/luci/luciexe/exe" - luciexe binary helper library.
//
// Other Client Implementations
//
// Python Recipes (https://chromium.googlesource.com/infra/luci/recipes-py)
// implement the LUCI Executable protocol using the "luciexe" subcommand.
//
//   TODO(iannucci): Implement a luciexe binary helper in `infra_libs` analogous
//   to go.chromium.org/luci/luciexe/exe and implement Recipes' support in terms
//   of this.
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
