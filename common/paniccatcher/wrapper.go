// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package paniccatcher

import (
	"fmt"
	"runtime"

	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// The maximum stack buffer size (64K). This is the same value used by
// net.Conn's serve() method.
const maxStackBufferSize = (64 << 10)

// Wrapper is a stateful set of utility functions that support standard panic
// catching and logging.
//
// Note that Wrapper is not goroutine-safe.
type Wrapper struct {
	Panic interface{} // The panic value that was caught.
	Stack []byte      // The stack at the time of the panic.
}

func (w *Wrapper) Catch(ctx context.Context, format string, args ...interface{}) {
	if w.Panic = recover(); w.Panic != nil {
		stack := make([]byte, maxStackBufferSize)
		count := runtime.Stack(stack, true)

		// Copy stack into a different array to reclaim space from stack
		// over-allocation.
		if count < len(stack) {
			w.Stack = make([]byte, count)
			copy(w.Stack, stack)
		} else {
			w.Stack = stack
		}

		log.Fields{
			"panic.error": w.Panic,
		}.Errorf(ctx, "%s\nStack Trace:\n%s", fmt.Sprintf(format, args...), string(w.Stack))
	}
}

// DidPanic returns true if the Wrapper caught a panic.
func (w *Wrapper) DidPanic() bool {
	return w.Panic != nil
}

// Do executes the supplied function, catching and logging any resulting panic.
func Do(ctx context.Context, f func()) (p bool) {
	w := &Wrapper{}
	defer func() {
		p = w.DidPanic()
	}()
	defer w.Catch(ctx, "Caught panic during execution of %+v", f)
	f()
	return
}
