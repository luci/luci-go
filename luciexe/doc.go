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
//    responsibilities it's free to do what it wants (i.e. actually do its
//    task).
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
// Host Application is responsible for intercepting and merging all
// 'build.proto' streams emitted within this tree of luciexes (see "Recursive
// Invocation"). The Host Application may choose what happens to these
// intercepted build.proto messages.
//
// The Host Application MUST:
//   * Run a logdog butler service and expose all relevant LOGDOG_* environment
//     variables such that the following client libraries can stream log data:
//       * Golang: go.chromium.org/luci/logdog/client/butlerlib/bootstrap
//       * Python: infra_libs.logdog.bootstrap
//   * Hook the butler to intercept and merge build.proto streams into a single
//     build.proto (zlib-compressed) stream.
//   * Set up a local LUCI ambient authentication service which luciexe's can
//     use to mint auth tokens.
//   * Prepare an empty directory which will house tempdirs and workdirs for
//     all luciexe invocations. The Host Application MAY clean this directory
//     up, but it may be useful to leak it for debugging. It's permissible for
//     the Host Application to defer this cleanup to an external process (e.g.
//     buildbucket's agent may defer this to swarming).
//
// The Host Application MAY hook additional streams for debugging/logging; it is
// frequently convenient to hook the stderr/stdout streams from the top level
// luciexe and tee them to the Host Application's stdout/stderr.
//
// For example: the `go.chromium.org/luci/buildbucket/cmd/agent` binary forwards
// these merged build.proto messages to the Buildbucket service, and also
// uploads all streams to them to the Logdog cloud service. Other host
// implementations may instead choose to write all streams to disk, send them to
// /dev/null or render them as html. However, from the point of view of the
// luciexe that they run, this is transparent.
//
// Host Applications MAY implement 'backpressure' on the luciexe binaries by
// throttling the rate at which the Logdog butler accepts data on its various
// streams. However, doing this could introduce timing issues in the luciexe
// binaries as they try to run, so this should be done thoughtfully.
//
// If a Host Application detects a protocol violation from a luciexe within its
// purview, it SHOULD report the violation (in a manner of its choosing) and
// MUST consider the entire Build status to be INFRA_FAILURE. In addition the
// Host Application SHOULD attempt to kill (via process group SIGTERM/SIGKILL on
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
//   * Set $TEMPDIR, $TMPDIR, $TEMP, $TMP and $MAC_CHROMIUM_TMPDIR to all point
//     to the same, empty directory.
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
//   * Set the Status of initial buildbucket.v2.Build message to `STARTED`.
//
//   * Set the CreateTime and StartTime of initial buildbucket.v2.Build message
//     to the current timestamp.
//
//   * Clear following fields in the initial buildbucket.v2.Build message.
//      - EndTime
//      - Output
//      - StatusDetails
//      - Steps
//      - SummaryMarkdown
//      - UpdateTime
//
// The CWD is up to your application. Some contexts (like Buildbucket) will
// guarantee an empty CWD, but others (like recursive invocation) may explicitly
// share CWD between multiple luciexe's.
//
// The tempdir and workdir paths SHOULD NOT be cleaned up by the invoking
// process. Instead, the invoking process should defer to the Host Application
// to provide this cleanup, since the Host Application may be configured to leak
// these for debugging purposes.
//
// The invoker MUST attach the stdout/stderr to the logdog butler as text
// streams. These MUST be located at `$LOGDOG_NAMESPACE/std{out,err}`.
// Typical luciexe implementations will use these for debug logging and output,
// but are not required to do so.
//
// The invoker MUST write a binary-encoded buildbucket.v2.Build to the stdin of
// the luciexe which contains all the input parameters that the luciexe needs to
// know to run successfully.
//
// The luciexe binary
//
// Once running, the luciexe MUST read a binary-encoded buildbucket.v2.Build
// message from stdin until EOF.
//
// As per the invoker's responsibility, the luciexe binary MAY expect the
// status of the initial Build message to be `STARTED` and CreateTime and
// StartTime are populated. It MAY also expect following fields to be empty.
// It MUST NOT assume other fields in the Build message are set. However, the
// Host Application or invoker MAY fill in other fields they think are useful.
//
//   EndTime
//   Output
//   StatusDetails
//   Steps
//   SummaryMarkdown
//   Tags
//   UpdateTime
//
// As per the Host Application's responsibilities, the luciexe binary MAY expect
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
// Additionally, a build.proto stream MAY append "; encoding=zlib" to the
// Content-Type (and compress each message accordingly). This is useful for when
// you potentially have very large builds.
//
// Each datagram MUST be a valid binary-encoded buildbucket.v2.Build message.
// The state of the build is defined as the last Build message sent on this
// stream. There's no implicit accumulation between sent Build messages.
//
// All Step.Log.Url fields in the emitted Build messages MUST be relative to
// the $LOGDOG_NAMESPACE of the build.proto stream. For example, if the host
// application is parsing a Build.proto in a stream named
// "logdog://host/project/prefix/+/something/build.proto", then a Log with a Url
// of "hello/world/stdout" will be transformed into:
//
//   Url:     logdog://host/project/prefix/+/something/hello/world/stdout
//   ViewUrl: <implementation defined>
//
// The `ViewUrl` field SHOULD be left empty, and will be filled in by the
// host running the luciexe (if supplied it will be overwritten).
//
// The following Build fields will be read from the luciexe-controlled
// build.proto stream:
//
//   EndTime
//   Output
//   Status
//   StatusDetails
//   Steps
//   SummaryMarkdown
//   Tags
//   UpdateTime
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
//   NOTE: JSON outputs SHOULD be written with the original proto field names,
//   not the lowerCamelCase names; downstream users may not be using jsonpb
//   unmarshallers to interpret the JSON data.
//
//   This may need to be revised in a subsequent version of this API
//   specification.
//
// LUCI Executables MAY invoke other LUCI Executables as sub-steps and have the
// Steps from the child luciexe show in the parent's Build updates. This is one
// of the responsibilities of the Host Application.
//
// The parent can achieve this by recording a Step S (with no children), and
// a Step.Log named "$build.proto" which points to a "build.proto" stream (see
// "Updating the Build State"). If step S has multiple logs, the
// "$build.proto" log must be the first one. This is called a "Merge Step", and
// is a directive for the host to merge the Build message located in the
// "$build.proto" log here. Note that the provision that 'Step.Log.Url' fields
// must be relative still applies here. However, it's the invoker's
// responsibility to populate $LOGDOG_NAMESPACE with the new, full, namespace.
// Failure to do this will result in missing logs/step data.
//
// (SIDENOTE: There's an internal proposal to improve the Logdog "butler" API so
// that application code only needs to handle the relative namespaces as well,
// which would make this much less confusing).
//
// The Host Application MUST append all steps from the child build.proto
// stream to the parent build as substeps of step S and copy the following
// fields of the child Build to the equivalent fields of step S *only if*
// step S has *non-final* status. It is the caller's responsibility to populate
// rest of the fields of step S if the caller explicitly marks the step
// status as final.
//
//  SummaryMarkdown
//  Status
//  EndTime
//  Output.Logs (appended)
//
// This rule applies recursively, i.e. the child build MAY have Merge Step(s).
//
// Each luciexe's step names should be emitted as relative names. e.g. say
// a build runs a sub-luciexe with the name "a|b". This sub-luciexe then runs
// a step "x|y|z". The top level build.proto stream will show the step
// "a|b|x|y|z".
//
// The graph of datagram streams MUST be a tree. This follows from the
// namespacing rules of the Log.Url fields; Since Log.Url fields are relative to
// their build's namespace, it's only possible to have a merge step point
// further 'down' the tree, making it impossible to create a cycle.
//
// Related Libraries
//
// For implementation-level details, please refer to the following:
//
//  * "go.chromium.org/luci/luciexe" - low-level protocol details (this module).
//  * "go.chromium.org/luci/luciexe/host" - the Host Application library.
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
