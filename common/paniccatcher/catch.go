// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

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
	// Reason is the value supplied to the recover funciton.
	Reason interface{}
	// Stack is a stack dump at the time of the panic.
	Stack string
}

// Catch recovers from panic. It should be used as a deferred call.
//
// If the supplied panic callback is nil, the panic will be silently discarded.
// Otherwise, the callback will be invoked with the panic's information.
func Catch(cb func(p *Panic)) {
	p := Panic{}
	if p.Reason = recover(); p.Reason != nil {
		stack := make([]byte, maxStackBufferSize)
		count := runtime.Stack(stack, true)
		p.Stack = string(stack[:count])

		if cb != nil {
			cb(&p)
		}
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
