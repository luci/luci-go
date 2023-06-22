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

// Package execmock allows mocking exec commands using the
// go.chromium.org/luci/common/exec ("luci exec") library, which is nearly
// a drop-in replacement for the "os/exec" stdlib library.
//
// The way this package works is by registering `RunnerFunction`s, and then
// adding references to those inside of the Context ('mocks'). The LUCI exec
// library will then detect these in any constructed Command's and run the
// RunnerFunction in a real sub-process.
//
// The sub-process is always an instance of the test binary itself; You hook
// execution of these RunnerFunctions by calling the Intercept() function in
// your TestMain function. TestMain will then have two modes. For the main test
// execution, the test binary will run an http server to serve and retrieve data
// from 'invocations'. For the mock sub-processes, a special environment
// variable will be set, and the Intercept() function will effectively hijack
// the test execution before the `testing.Main` function. The hijack will reach
// out to the http server and retrieve information about this specific
// invocation, including inputs tot he RunnerFunction, and execute the
// RunnerFunction with the provided input. Once the function is done, its
// output will be POST'd back to the http server, and the process will exit with
// the return code from the runner function.
//
// Running these as real sub-processes has a number of advantages; In
// particular, application code which is written to interact with a real
// sub-process (e.g. passing file descriptors, sending signals, reading/writing
// from Stdio pipes, calling system calls like Wait() on the process, etc.) will
// be interacting with a 100% real process, not an emulation in a goroutine.
// Additionally, the RunnerFunction will get to use all `os` level functions (to
// read environment, working directory, look for parent process ID, etc.) and
// they will work correctly.
//
// As a convenience, this library includes a `Simple` mock which can cover many
// very basic execution scenarios.
//
// By default, if the Intercept() function has been called in the process, ALL
// commands using the luci exec library will need to have a matching mock. You
// can explicitly enable 'passthrough' for some executions (with the Passthrough
// mock).
//
// Finally, you can simulate any error from the Start function using the
// StartError mock; This will not run a sub-process, but will instead produce
// an error from Start and other functions which call Start.
//
// # Debugging Runners
//
// Occassionally you have a Runner which is sufficiently complex that you would
// need to debug it (e.g. with delve). execmock supports this by doing the
// following:
//
// First, run `go test . -execmock.list`, which will print out all registered
// runners at the point that your test calls Intercept().
package execmock

import (
	"context"
	"reflect"
)

// A Mocker allows you to create mocks for a RunnerFunction, or to create mocks
// for a 'Start error' (via StartError).
//
// Mockers are immutable, and by default a Mock would apply to ALL commands. You
// can create derivative Mockers using the With methods, which will only apply
// to commands matching that criteria.
//
// Once you have constructed the filter you want by chaining With calls, call
// Mock to actually add a filtered mock into the context, supplying any input
// data your RunnerFunction needs (in the case of a Start error, this would be
// the error that will be returned from Command.Start()).
type Mocker[In any, Out any] interface {
	// WithArgs will return a new Mocker which only applies to commands whose
	// argument list matches `argPattern`.
	//
	// Using this multiple times will require a command to match ALL given patterns.
	//
	// `argPattern` follows the same rules that
	// go.chromium.org/luci/common/data/text/sequence.NewPattern uses, namely:
	//
	// Tokens can be:
	//   - "/a regex/" - A regular expression surrounded by slashes.
	//   - "..." - An Ellipsis which matches any number of sequence entries.
	//   - "^" at index 0 - Zero-width matches at the beginning of the sequence.
	//   - "$" at index -1 - Zero-width matches at the end of the sequence.
	//   - "=string" - Literally match anything after the "=". Allows escaping
	//     special strings, e.g. "=/regex/", "=...", "=^", "=$", "==something".
	//   - "any other string" - Literally match without escaping.
	//
	// Panics if `argPattern` is invalid.
	WithArgs(argPattern ...string) Mocker[In, Out]

	// WithEnv will return a new Mocker which only applies to commands which
	// include an environment variable `varName` which matches `valuePattern`.
	//
	// Using this multiple times will require a command to match ALL the given
	// restrictions (even within the same `varName`).
	//
	// varName is the environment variable name (which will be matched exactly), and
	// valuePattern is either:
	//   - "/a regex/" - A regular expression surrounded by slashes.
	//   - "!" - An indicator that this envvar should be unset.
	//   - "=string" - Literally match anything after the "=". Allows escaping
	//     regex strings, e.g. "=/regex/", "==something".
	//   - "any other string" - Literally match without escaping.
	WithEnv(varName, valuePattern string) Mocker[In, Out]

	// WithLimit will restrict the number of times a Mock from this Mocker can
	// be used.
	//
	// After that many usages, the Mock will become inactive and won't match any
	// new executions.
	//
	// Calling WithLimit replaces the current limit.
	// Calling WithLimit(0) will remove the limit.
	WithLimit(limit uint64) Mocker[In, Out]

	// Mock adds a new mocker in the context, and returns the corresponding
	// Uses struct which will allow your test to see how many times this mock
	// was used, and what outputs it produced.
	//
	// Any execution which matches this Entry will run this Mocker's associated
	// RunnerFunction in a subprocess with the data `indata`.
	//
	// Supplying multiple input values will panic.
	// Supplying no input values will use a default-constructed In (especially
	// useful for None{}).
	//
	// Mocks are ordered based on the filter (i.e. what WithXXX calls in the chain
	// prior to calling Mock) once the first exec.Command/CommandContext is
	// Start()'d. The ordering criteria is as follows:
	//   * Number of LiteralMatchers in WithArgs patterns (more LiteralMatchers
	//     will be tried earlier).
	//   * Number of all matchers in WithArgs patterns (more matchers will be
	//     tried earlier).
	//   * Mocks with lower limits are tried before mocks with higher limits.
	//   * Mocks with more WithEnv entries are tried before mocks with fewer
	//     WithEnv.
	//   * Finally, mocks are tried in the order they are created (i.e. the order
	//     that Mock() here is called.
	//
	// Lastly, once `ctx` has been used to start executing commands (e.g.
	// `exec.Command(ctx, ...)`), the state in that context is `sealed`, and
	// calling Mock on that context again will panic. The state can be unsealed by
	// calling ResetState on the context.
	Mock(ctx context.Context, indata ...In) *Uses[Out]
}

// getOne is a little helper function for WithArgs implementations.
func getOne[In any](indatas []In) In {
	if len(indatas) > 1 {
		panic("Mock only accepts a single input")
	}

	var ret In
	inType := reflect.TypeOf(&ret).Elem()
	if inType.Kind() == reflect.Pointer {
		ret = reflect.New(inType.Elem()).Interface().(In)
	}
	if len(indatas) > 0 {
		ret = indatas[0]
	}
	return ret
}
