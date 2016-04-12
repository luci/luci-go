// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"os"
	"os/signal"
)

// catchInterrupt handles SIGINT and SIGTERM signals.
//
// When caught for the first time, it calls the `handler`, assuming it will
// gracefully shutdown the process.
//
// If called for the second time, it just kills the process right away.
func catchInterrupt(handler func()) {
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, interruptSignals()...)
	go func() {
		stopCalled := false
		for range sig {
			if !stopCalled {
				stopCalled = true
				handler()
			} else {
				os.Exit(2)
			}
		}
	}()
}
