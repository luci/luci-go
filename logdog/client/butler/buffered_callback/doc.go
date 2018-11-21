// Copyright 2018 The LUCI Authors.
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

// Package buffered_callback provides functionality to wrap around LogEntry
// callbacks to guarantee calling only on complete LogEntries, because the
// LogDog bundler produces fragmented LogEntries under normal operation, in
// order to meet time or buffer size requirements. The wrapped callbacks will
// not split otherwise contiguous LogEntries (e.g. if a bundler produces a
// LogEntry with multiple complete lines, that will get passed along as-is, not
// called once per line).
//
// Expects:
// - LogEntry to be read-only, so the callback and others must take care not to
//   modify the LogEntry and should be able to assume that keeping the reference
//   remains safe;
// - callback to return quickly as it will block the stream until it completes.
package buffered_callback
