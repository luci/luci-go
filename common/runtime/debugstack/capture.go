// Copyright 2025 The LUCI Authors.
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

package debugstack

import (
	"runtime/debug"
)

// Capture returns a debug.Stack() output after dropping `skip` number of
// additional frames.
//
// A skip value of 0 means that the caller of Capture will be the top
// frame.
func Capture(skip int) string {
	return Parse(debug.Stack()).Filter(CompileRules(
		Rule{ApplyTo: StackFrameKind, DropN: 2 + skip},
	), false).String()
}
