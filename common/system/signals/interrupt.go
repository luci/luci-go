// Copyright 2019 The LUCI Authors.
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

// Package signals makes it easier to catch SIGTERM.
package signals

import (
	"fmt"
	"os"
	"os/signal"
)

// HandleInterrupt calls 'fn' in a separate goroutine on SIGTERM or Ctrl+C.
//
// When SIGTERM or Ctrl+C comes for a second time, logs to stderr and kills
// the process immediately via os.Exit(1).
//
// Returns a callback that can be used to remove the installed signal handlers.
func HandleInterrupt(fn func()) (stopper func()) {
	ch := make(chan os.Signal, 2)
	signal.Notify(ch, Interrupts()...)

	go func() {
		handled := false
		for range ch {
			if handled {
				fmt.Fprintf(os.Stderr, "Got second interrupt signal. Aborting.\n")
				os.Exit(1)
			}
			handled = true
			go fn()
		}
	}()

	return func() { signal.Stop(ch) }
}
