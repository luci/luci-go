// Copyright 2015 The LUCI Authors.
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
// callbacks to guarantee calling only on complete LogEntries, even in cases
// of aggressive flushing in which data might be split among multiple
// LogEntries. The callback is not guaranteed to call on atomic LogEntries
// (consider Text LogEntries with multiple full lines).
//
// Any error handling is the responsibility of the caller/StreamChunkCallback
// implementation.
//
// Expects:
// - LogEntry to be read-only, so the callback and others must take care not to
//   modify the LogEntry and should be able to assume that keeping the reference
//   remains safe;
// - callback to return quickly as it will block the stream until it completes.
package buffered_callback
