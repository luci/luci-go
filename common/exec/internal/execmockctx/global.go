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

// Package execmockctx provides the minimum interface that the
// `go.chromium.org/luci/common/exec` library needs to hook into the mocking
// system provided by `go.chromium.org/luci/common/exec/execmock` without
// needing to actually link the execmock code (including it's registration of
// the Simple runner, and the implementation of the http test server)
// into non-test binaries.
package execmockctx

import (
	"context"
	"os"
	"os/exec"
	"sync"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/system/environ"
)

// MockCriteria is what execmock uses to look up Entries.
type MockCriteria struct {
	Args []string
	Env  environ.Env
}

// NewMockCriteria is a convenience function to return a new MockCriteria from
// a Cmd.
func NewMockCriteria(cmd *exec.Cmd) *MockCriteria {
	return &MockCriteria{
		Args: cmd.Args,
		Env:  environ.New(cmd.Env),
	}
}

var ErrNoMatchingMock = errors.New("execmock: mocking enabled but no mock matches")

type MockInvocation struct {
	// Unique ID of this invocation.
	ID uint64

	// An environment variable ("KEY=Value") which exec should add to
	// the command during invocation.
	//
	// This is "generic" in the sense that it doesn't tie to the specific
	// key/value format that `execmockserver` actually uses, but:
	//   1) this package is internal, so only exec & execmock can use it.
	//   2) this package is separate from execmock specifically to decouple the
	//      need to pull in heavy stuff like "net/http" and "testing" into uses
	//      of "exec".
	//
	// Practically speaking this will always look like
	// "LUCI_EXECMOCK_CTX=localhost:port|invocationID".
	EnvVar string

	// After Wait()'ing for the process, the Cmd runner can call this to get an
	// error (if any) which the Runner returned, as well as the panic stack (if
	// any) from the runner.
	GetErrorOutput func() (panicStack string, err error)
}

// CreateMockInvocation encapsulates the entire functionality which
// "common/exec" needs to call, and which "common/exec/execmock" needs to
// implement.
//
// This function, if set, should evaluate `mc` against state stored by execmock
// in `ctx`, and return a MockInvocation if `mc` matches a mock. Note that this
// will return an error wrapping ErrNoMatchingMock if the context doesn't define
// a matching mock (which could be due to the user forgetting to Init the
// context at all).
//
// `proc` should point to the underlying Cmd.Process field; This will be used to
// expose the Process to the test via Usage.GetProcess().
//
// If this returns (nil, nil) it means that this invocation should be passed
// through (run as normal).
type CreateMockInvocation func(mc *MockCriteria, proc **os.Process) (*MockInvocation, error)

type MockFactory func(ctx context.Context, strict bool) (mocker CreateMockInvocation, chatty bool)

var mockCreator MockFactory
var mockStrict bool
var mockCreatorOnce sync.Once

// EnableMockingForThisProcess is called from execmock to install the mocker service here.
func EnableMockingForThisProcess(mcFactory MockFactory, strict bool) {
	alreadySet := true
	mockCreatorOnce.Do(func() {
		alreadySet = false
		mockStrict = strict
		mockCreator = mcFactory
	})

	if mockCreator == nil {
		panic("EnableMockingForThisProcess called after execmock.Init(ctx)")
	}

	if alreadySet {
		panic("EnableMockingForThisProcess called twice")
	}
}

// GetMockCreator returns the process wide implementation of
// CreateMockInvocation, or nil, if mocking is not enabled for this process.
//
// Once this function has been called (i.e. after the first call of
// ".../common/exec.CommandContext()"), EnableMockingForThisProcess will panic.
//
// If no mocking is configured for this process, returns (nil, false).
func GetMockCreator(ctx context.Context) (mocker CreateMockInvocation, chatty bool) {
	mockCreatorOnce.Do(func() {
		mockCreator = nil
	})
	if mockCreator != nil {
		mocker, chatty = mockCreator(ctx, mockStrict)
	}
	return
}

// MockingEnabled returns true iff mocking is enabled for the current process.
//
// This behaves similarly to GetMockCreator in that calling it will permanently
// cause mocking to be disabled for this process if it wasn't setup prior to
// this.
func MockingEnabled() bool {
	mockCreatorOnce.Do(func() {
		mockCreator = nil
	})
	return mockCreator != nil
}
