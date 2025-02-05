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

package paniccatcher

import (
	"context"
	"runtime/debug"

	"go.chromium.org/luci/common/logging"
)

// Panic is a snapshot of a panic, containing both the panic's reason and the
// system stack.
type Panic struct {
	// Reason is the value supplied to the recover function.
	Reason any
	// Stack is a stack trace of where the panic happened.
	Stack string
}

// Log logs the given message at error severity with the panic stack trace.
func (p *Panic) Log(ctx context.Context, fmt string, args ...any) {
	logging.ErrorWithStackTrace(ctx, logging.StackTrace{Standard: p.Stack}, fmt, args...)
}

// Catch recovers from panic. It should be used as a deferred call.
//
// If the supplied panic callback is nil, the panic will be silently discarded.
// Otherwise, the callback will be invoked with the panic's information.
func Catch(cb func(p *Panic)) {
	if reason := recover(); reason != nil && cb != nil {
		cb(&Panic{
			Reason: reason,
			Stack:  string(debug.Stack()),
		})
	}
}

// Do executes f. If a panic occurs during execution, the supplied callback will
// be called with the panic's information.
//
// If the panic callback is nil, the panic will be caught and discarded silently.
func Do(f func(), cb func(p *Panic)) {
	defer Catch(cb)
	f()
}
