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

package errors

import "runtime"

// IsPanicking returns true iff the current goroutine is panicking.
//
// Always returns false when not invoked via a defer'd function.
//
// This should only be used to indicate some best-effort error status, not to
// modify control flow of the program. Panics are still crashes!
//
// HACK: Detection is implemented by looking up the stack at most skip+10 frames
// above IsPanicking to find if the golang panic handler is on the stack. This
// may break when the Go runtime changes!
//
// `skip` indicates how many additional frames of the stack to skip (a value of
// 0 starts the stack at the caller of `IsPanicking`). Clamps to a minimum value
// of 0.
//
// Does NOT invoke `recover()`. WILL detect `panic(nil)`.
func IsPanicking(skip int) bool {
	if skip < 0 {
		skip = 0
	}
	chunk := make([]uintptr, 10)
	chunk = chunk[:runtime.Callers(2+skip, chunk)]
	if len(chunk) == 0 {
		return false
	}
	frames := runtime.CallersFrames(chunk)
	for {
		frame, more := frames.Next()
		if frame.Function == "runtime.gopanic" {
			return true
		}
		if !more {
			return false
		}
	}
}
