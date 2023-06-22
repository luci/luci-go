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
	"encoding/gob"
	"reflect"
	"runtime"
	"sync"

	"go.chromium.org/luci/common/errors"
)

// None can be used as an In or Out type for a runner function which doesn't
// need an input or an output.
type None struct{}

type runnerMeta struct {
	file string
	line int
}

// Because it is required to call Register from inside a module or init()
// function (or, I guess, TestMain), and https://go.dev/ref/spec#Package_initialization
// specifies that module initialization is deterministic for the same binary
// (i.e. since we only need the ids here to be stable for two invocations of the
// exact same compiled output), we use a simple integer here to track calls to
// Register, and embed this inside of the returned ExecMocker.
var (
	runnerMu              sync.Mutex
	runnerID              uint64
	runnerRegistry        map[uint64]reflect.Value
	runnerRegistryMeta    map[uint64]runnerMeta
	runnerRegistryMutable = true
)

// RunnerFunction is the implementation for your mocked execution; It will be
// invoked with `indata` value as input, and can return an `outdata` value,
// along with the exit code for the process.
//
// When your code under test does `exec` and matches a MockMockor using this
// function, a subprocess will be spawned to run this function. When this
// function returns, the `outdata` and `err` will be communicated back to the
// ExecMocker, and then the process will exit with the returned `exitcode`.
//
// If the function panics, execmock will generate an exit code of 1 and an error
// of ErrPanicedRunner.
//
// Typically, `err` should be used when your runner detects that it was invoked
// incorrectly (e.g. asserting that certain arguments were present, or an input
// file exists, etc.). The error is only passed back from the application as
// a string.
//
// All standard `os` functions and variables (e.g. environ, args, standard IO
// handles, etc.) will reflect the that the code under test set when running the
// mocked executable. Additionally `flag.CommandLine` is reset to a pristine
// state during the invocation to make it easier to parse input flags without
// picking up global flag registration from the Go "testing" library, which
// could interfere with flags that the RunnerFunction needs to parse.
type RunnerFunction[In any, Out any] func(indata In) (outdata Out, exitcode int, err error)

// Register must be called for all Runner functions in order to
// allow their inputs and outputs to be GoB-encoded.
//
// Should be called in an `init()` function, or as a module level assignment in
// the module where the function is implemented.
//
// In particular this must be called in such a way that other instances
// of this executable register the same types.
//
// Example:
//
//	func exitCoder(retcode int) (None, int) {
//	  return None{}, retcode
//	}
//
//	var gitMocker := Register(exitCoder)
//	gitMocker.WithArgs("^", "git").Mock(ctx, 100)
func Register[In any, Out any](runnerFn RunnerFunction[In, Out]) Mocker[In, Out] {
	var inExample In
	gob.RegisterName(gobName(inExample), inExample)

	var outExample Out
	gob.RegisterName(gobName(outExample), outExample)

	runnerMu.Lock()
	defer runnerMu.Unlock()

	if !runnerRegistryMutable {
		panic(errors.New("execmock.Register: RunnerFunction registry is already closed. Register all functions at init() time."))
	}

	runnerID++
	id := runnerID

	if runnerRegistry == nil {
		runnerRegistry = make(map[uint64]reflect.Value)
		runnerRegistryMeta = make(map[uint64]runnerMeta)
	}

	runnerRegistry[id] = reflect.ValueOf(runnerFn)
	if _, file, line, ok := runtime.Caller(1); ok {
		runnerRegistryMeta[id] = runnerMeta{
			file: file,
			line: line,
		}
	}

	return &execMocker[In, Out]{runnerID: id}
}
