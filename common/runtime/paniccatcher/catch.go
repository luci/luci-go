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
	"runtime"
)

// The maximum stack buffer size (64K). This is the same value used by
// net.Conn's serve() method.
const maxStackBufferSize = (64 << 10)

// Panic is a snapshot of a panic, containing both the panic's reason and the
// system stack.
type Panic struct {
	// Reason is the value supplied to the recover function.
	Reason interface{}
	// Stack is a stack dump at the time of the panic.
	Stack string
}

// Catch recovers from panic. It should be used as a deferred call.
//
// If the supplied panic callback is nil, the panic will be silently discarded.
// Otherwise, the callback will be invoked with the panic's information.
func Catch(cb func(p *Panic)) {
	if reason := recover(); reason != nil && cb != nil {
		stack := make([]byte, maxStackBufferSize)
		count := runtime.Stack(stack, true)
		cb(&Panic{
			Reason: reason,
			Stack:  string(stack[:count]),
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
