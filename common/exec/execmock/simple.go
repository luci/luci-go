// Copyright 2023 The LUCI Authors.
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

package execmock

import (
	"io"
	"os"
	"sync"
)

// SimpleInput is the options you can provide to Simple.Mock.
type SimpleInput struct {
	// If true, fire off a goroutine to consume (and discard) all of Stdin.
	ConsumeStdin bool

	// If non-empty, fire off a goroutine to write this string to Stdout.
	Stdout string

	// If non-empty, fire off a goroutine to write this string to Stderr.
	Stderr string

	// Exit with this return code.
	ExitCode int

	// Emit this error back to the Usage.
	Error error
}

// Simple implements a very basic mock for executables which can read from
// stdin, write to stdout and stderr, and emit an exit code.
//
// Each of the I/O operations operates in a totally independent goroutine,
// and all of them must complete for the Runner to exit.
//
// Omitting the SimpleInput will result in a mock which just exits 0.
//
// The Output for this is a string which is the stdin which the process consumed
// (if ConsumeStdin was set).
var Simple = Register(simpleMocker)

// simpleMocker is the RunnerFunction implementation for Simple.
func simpleMocker(opts SimpleInput) (stdin string, code int, err error) {
	var wg sync.WaitGroup

	if opts.ConsumeStdin {
		wg.Add(1)
		go func() {
			defer wg.Done()
			raw, err := io.ReadAll(os.Stdin)
			if err != nil {
				panic(err)
			}
			stdin = string(raw)
		}()
	}

	if opts.Stdout != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := os.Stdout.WriteString(opts.Stdout); err != nil {
				panic(err)
			}
		}()
	}

	if opts.Stderr != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := os.Stderr.WriteString(opts.Stderr); err != nil {
				panic(err)
			}
		}()
	}

	wg.Wait()
	code = opts.ExitCode
	err = opts.Error
	return
}
