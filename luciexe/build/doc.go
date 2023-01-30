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
// See `Start` for the entrypoint to this API.
//
// Features:
//   - Can be used in library code which is used in both luciexe and non-luciexe
//     contexts. You won't need "two versions" of your library just to give it
//     the appearance of steps in a LUCI build context, or to have it read or
//     write build properties.
//   - Goroutine-safe API.
//   - Handles all interactions with the log sink (e.g. LogDog), and
//     automatically reroutes the logging library if a log sink is configured
//     (so all log messages will be preserved in a build context).
//   - Handles coalescing all build mutations from multiple goroutines.
//     Efficient throttling for outgoing updates.
//   - Automatically upholds all luciexe protocol invariants; Avoids invalid
//     Build states by construction.
//   - Automatically maps errors and panics to their obvious visual
//     representation in the build (and allows manual control in case you want
//     something else).
//   - Fully decoupled from LUCI service implementation; You can use this API
//     with no external services (e.g. for a CLI application), backed with the
//     real LUCI implementation (e.g. when running under BuildBucket), and in
//     tests (you can observe all outputs from this API without any live
//     services or heavy mocks).
//
// # No-Op Mode
//
// When no Build is in use in the context, the library behaves in 'no-op' mode.
// This should enable libraries to add `build` features which gracefully degrade
// into pure terminal output via logging.
//
//   - MakePropertyReader functions will return empty messages. Well-behaved
//     libraries should handle having no configuration in the context if this is
//     possible.
//   - There will be no *State object, because there is no Start call.
//   - StartStep/ScheduleStep will return a *Step which is detached. Step
//     namespacing will still work in context (but name deduplication will not).
//   - The result of State.Modify/Step.Modify (and Set) calls will be
//     logged at DEBUG.
//   - Step scheduled/started/ended messages will be logged at INFO.
//     Ended log messages will include the final summary markdown as well.
//   - Text logs will be logged line-by-line at INFO with fields set indicating
//     which step and log they were emitted from. Debug text logs (those whose
//     log names start with "$") will be logged at DEBUG level.
//   - Non-text logs will be dropped with a WARNING indicating that they're
//     being dropped.
//   - MakePropertyReader property readers will return empty message objects.
//   - MakePropertyModifier property manipulators will log their emitted
//     properties at INFO.
package build

// BUG(iannucci): When OptLogsink is used and `logging` output is redirected to
// a Step log entitled "log", the current log format is reset to
// `gologger.StdFormat` instead of preserving the current log format from the
// context.
