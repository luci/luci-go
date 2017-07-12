// Copyright 2016 The LUCI Authors.
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
