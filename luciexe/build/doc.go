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

// Package build implements a minimal, safe, easy-to-use, hard-to-use-wrong,
// 'luciexe binary' implementation in Go.
//
// In particular:
//   * Can be used in library code which is used in both luciexe and non-luciexe
//     contexts. You won't need "two versions" of your library just to give it
//     the appearance of steps in a LUCI build context, or to have it read or
//     write build properties.
//   * Goroutine-safe API.
//   * Handles all interactions with the log sink (e.g. LogDog), and
//     automatically reroutes the logging library if a log sink is configured
//     (so all log messages will be preserved in a build context).
//   * Handles coalescing all build mutations from multiple goroutines.
//     Efficient throttling for outgoing updates.
//   * Automatically upholds all luciexe protocol invariants; Avoids invalid
//     Build states by construction.
//   * Automatically maps errors and panics to their obvious visual
//     representation in the build (and allows manual control in case you want
//     something else).
//   * Fully decoupled from LUCI service implementation; You can use this API
//     with no external services (e.g. for a CLI application), backed with the
//     real LUCI implementation (e.g. when running under BuildBucket), and in
//     tests (you can observe all outputs from this API without any live
//     services or heavy mocks).
//
// See `Start` for the entrypoint to this API.
package build
