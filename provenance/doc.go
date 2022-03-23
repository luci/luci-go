// Copyright 2022 The LUCI Authors.
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

// Package provenance provides API definitions and simple libraries to interface
// with the APIs.
//
// It supports all possible provenance information that are supported currently.
//
// Provenance is a metadata about an artifact that provides information to trace
// a binary back to its sources. It also provides a chain-of-custody as the
// artifact traverses through multiple systems.
//
// Summary
//
// Provenance defines APIs for reporting `provenance` metadata, in order to
// establish a traceable lineage for artifacts produced within the LUCI
// ecosystem.
// Artifact provenance is otherwise defined as "metadata which records a
// snapshot of the build-time states that correspond to an artifact."
//
// Usage
//
// Service self-report is used by local processes to insert relevant information
// into provenance. Server side implementation is beyond the scope of this
// package and is Google-internal.
//
// A local git checkout can be reported by:
//  pClient, _ := client.MakeSnooperClient(ctx, "http://localhost:port")
//  reporter := &reporter.Report{RClient: pClient}
//  ok, err := reporter.ReportGitCheckout(ctx, "https://repo.git", "deadbeef", "refs/heads/example")
//  if err != nil & !ok {
//      ...
//  }
//
// This call will return a result back to you, user can implement it in a
// blocking way to ensure the event was recorded. Failure mode here can be
// internal error (that would include invalid input) as well as absence of local
// provenance server.
//
// Results here can be:
//  (true, nil) => report successfully exported
//  (true, ErrServiceUnavailable) => service unavailable
//  (false, err) => all other cases
//
// This allows users to determine how to interpret failure. A concrete example
// would be a workload where security policy is set to "enforce" mode, meaning
// checkout should fail loudly, in this case `ok` status can be ignored.
// For workloads where this is in "audit" mode, API will return a success ok
// status when local server isn't configured/unreachable. This is particularly
// helpful for flexible workloads.
//
// Similarly other interfaces of thi API can be used to report a variety of
// interesting things.
package provenance
