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
	"context"
	"os"
	"sync"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/exec/internal/execmockctx"
	"go.chromium.org/luci/common/system/environ"
)

type usage interface {
	setOutput(any, error)
}

type uses interface {
	len() int
	addUsage(*execmockctx.MockCriteria, **os.Process) usage
}

// Uses is used to collect uses of a particular mock (Runner + input data).
//
// Your test code can interrogate this object after running your code-under-test
// to determine how many times the corresponding mock entry was used, what
// sub-processes were actually launched (and handles to those processes for your
// test to signal/read/etc.), and, if those sub-processes finished, what `Out`
// data did they return.
type Uses[Out any] struct {
	mu sync.Mutex

	uses []*Usage[Out]
}

func (u *Uses[Out]) len() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.uses)
}

func (u *Uses[Out]) addUsage(mc *execmockctx.MockCriteria, proc **os.Process) usage {
	u.mu.Lock()
	defer u.mu.Unlock()
	ret := &Usage[Out]{
		Args: mc.Args,
		Env:  mc.Env,

		proc:          proc,
		outputWritten: make(chan struct{}),
	}
	u.uses = append(u.uses, ret)
	return ret
}

// Snapshot retrieves a snapshot of the current Usages.
func (u *Uses[Out]) Snapshot() []*Usage[Out] {
	u.mu.Lock()
	defer u.mu.Unlock()
	ret := make([]*Usage[Out], len(u.uses))
	copy(ret, u.uses)
	return ret
}

var ErrNoOutput = errors.New("sub-process did not write an output")
var ErrProcessNotStarted = errors.New("sub-process did not start yet")

// Usage represents a single `hit` for a given mock.
type Usage[Out any] struct {
	// Args and Env are the exact Args and Env of the Cmd which matched this mock.
	Args []string
	Env  environ.Env

	proc **os.Process

	mu            sync.Mutex
	outputWritten chan struct{}
	output        Out
	err           error
}

// GetPID returns the process ID associated with this usage (i.e. the mock
// Process)
//
// NOTE: This is NOT thread-safe; Due to the way that the stdlib exec library
// works with regard to populating Cmd.Process, you must ensure that you only
// call this from a thread which was sequential with the thread which called
// Cmd.Start() (or Run() or CombinedOutput()).
//
// If this Usage is from a MockError invocation, this will always return nil.
//
// Returns 0 if the process is not started.
func (u *Usage[Out]) GetPID() int {
	p := *u.proc
	if p != nil {
		return p.Pid
	}
	return 0
}

// Signal sends a signal to the mock process.
//
// NOTE: This is NOT thread-safe; Due to the way that the stdlib exec library
// works with regard to populating Cmd.Process, you must ensure that you only
// call this from a thread which was sequential with the thread which called
// Cmd.Start() (or Run() or CombinedOutput()).
//
// If this Usage is from a MockError invocation, this will always return an
// error.
func (u *Usage[Out]) Signal(sig os.Signal) error {
	p := *u.proc
	if p != nil {
		return p.Signal(sig)
	}
	return ErrProcessNotStarted
}

// Kill kills mock process (usually by sending SIGKILL).
//
// NOTE: This is NOT thread-safe; Due to the way that the stdlib exec library
// works with regard to populating Cmd.Process, you must ensure that you only
// call this from a thread which was sequential with the thread which called
// Cmd.Start() (or Run() or CombinedOutput()).
//
// If this Usage is from a MockError invocation, this will always return an
// error.
func (u *Usage[Out]) Kill() error {
	p := *u.proc
	if p != nil {
		return p.Kill()
	}
	return ErrProcessNotStarted
}

func (u *Usage[Out]) setOutput(data any, err error) {
	u.mu.Lock()
	defer u.mu.Unlock()

	u.output = data.(Out)
	u.err = err
	close(u.outputWritten)
}

// GetOutput will block until the process writes output data (or until the
// provided context ends), and return the Out value written by the sub-process
// (if any).
//
// If this is Usage[None] then this returns (nil, nil)
//
// Possible errors:
//   - ErrNoOutput if the sub-process did not write an Out value.
//   - ctx.Err() if `ctx` is Done.
//   - errors which occured when reading the output from the sub-process.
//   - errors which the mock itself (i.e. the RunnerFunction) returned.
func (u *Usage[Out]) GetOutput(ctx context.Context) (value Out, err error) {
	select {
	case <-ctx.Done():
		err = ctx.Err()
		return
	case <-u.outputWritten:
	}

	u.mu.Lock()
	defer u.mu.Unlock()
	return u.output, u.err
}
