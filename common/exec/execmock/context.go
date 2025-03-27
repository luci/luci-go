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
	"sort"
	"sync"
	"sync/atomic"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/exec/internal/execmockctx"
	"go.chromium.org/luci/common/exec/internal/execmockserver"
	"go.chromium.org/luci/common/system/environ"
)

type mockEntry struct {
	uses uses

	inputData *execmockserver.InvocationInput

	f filter

	mockEntryID uint64
}

// mockEntryID is used as a disambiguator for Less. It is used by addMockEntry
// via an atomic.AddUint64 invocation.
var mockEntryID atomic.Uint64

func (me *mockEntry) matches(mc *execmockctx.MockCriteria, proc **os.Process) (match bool, lastUse bool, usage usage) {
	match = me.f.matches(mc)
	if match {
		usage = me.uses.addUsage(mc, proc)
		if me.f.limit > 0 && uint64(me.uses.len()) >= uint64(me.f.limit) {
			lastUse = true
		}
	}
	return
}

func (me *mockEntry) less(other *mockEntry) bool {
	if less, ok := me.f.less(other.f); ok {
		return less
	}
	return me.mockEntryID < other.mockEntryID
}

// mockState is stored in a context; It holds all of the mock entries that the
// user's test set in the context, and it can be interrogated by the user test
// to list all the missed MockCriteria at the end of the test.
//
// Tests will populate the mockState with all of the mock entries.
type mockState struct {
	mu     sync.Mutex
	mocks  []*mockEntry
	misses []*MockCriteria

	chatty bool

	// flipped to `true` once the first mock invocation has happened.
	sealed bool
}

func addMockEntry[Out any](ctx context.Context, f filter, i *execmockserver.InvocationInput) *Uses[Out] {
	u := &Uses[Out]{}

	state := mustGetState(ctx)

	me := &mockEntry{uses: u, inputData: i, f: f}
	me.mockEntryID = mockEntryID.Add(1)

	state.mu.Lock()
	defer state.mu.Unlock()

	if state.sealed {
		panic(errors.New("Cannot add Mock to sealed mocking context. Call ResetState to reset the mocking state on this context."))
	}

	state.mocks = append(state.mocks, me)
	return u
}

func getMocker(ctx context.Context, strict bool) (mocker execmockctx.CreateMockInvocation, chatty bool) {
	state := getState(ctx)
	if state == nil {
		if strict {
			return func(mc *execmockctx.MockCriteria, proc **os.Process) (*execmockctx.MockInvocation, error) {
				return nil, errors.Annotate(execmockctx.ErrNoMatchingMock, "execmock.Init not called on context").Err()
			}, false
		}
		return nil, false
	}
	return state.createMockInvocation, state.chatty
}

// createMockInvocation is the main way that execmock interacts with both the
// global state as well as the state in the context.
func (state *mockState) createMockInvocation(mc *execmockctx.MockCriteria, proc **os.Process) (*execmockctx.MockInvocation, error) {
	state.mu.Lock()
	defer state.mu.Unlock()

	if !state.sealed {
		sort.SliceStable(state.mocks, func(i, j int) bool {
			return state.mocks[i].less(state.mocks[j])
		})
		state.sealed = true
	}

	for i, ent := range state.mocks {
		matches, lastUse, usage := ent.matches(mc, proc)
		if matches {
			if lastUse {
				state.mocks = append(state.mocks[:i], state.mocks[i+1:]...)
			}
			if ent.inputData == nil {
				return nil, nil
			}
			if err := ent.inputData.StartError; err != nil {
				return nil, err
			}
			envvar, id := execmockserver.RegisterInvocation(server, ent.inputData, usage.setOutput)
			return &execmockctx.MockInvocation{
				ID:             id,
				EnvVar:         envvar,
				GetErrorOutput: usage.getErrorOutput,
			}, nil
		}
	}
	state.misses = append(state.misses, mcFromInternal(mc))

	return nil, errors.Annotate(execmockctx.ErrNoMatchingMock, "%s", mc).Err()
}

// MockCriteria are the parameters from a Command used to match an ExecMocker
type MockCriteria struct {
	Args []string
	Env  environ.Env
}

func mcFromInternal(imc *execmockctx.MockCriteria) *MockCriteria {
	return &MockCriteria{
		Args: append([]string(nil), imc.Args...),
		Env:  imc.Env.Clone(),
	}
}

// ResetState returns the list of missed MockCriteria (i.e. commands which
// didn't match any Mocks) and resets the state in `ctx` (wipes all existing
// Mock entries, misses, etc.)
//
// This also `unseals` the state, allowing Mocker.Mock to be called on this
// context again.
func ResetState(ctx context.Context) []*MockCriteria {
	state := mustGetState(ctx)
	state.mu.Lock()
	defer state.mu.Unlock()

	ret := state.misses

	state.mocks = nil
	state.misses = nil
	state.sealed = false

	return ret
}

var stateCtxKey = "context key for holding a *mockState"

func getState(ctx context.Context) *mockState {
	stateI := ctx.Value(&stateCtxKey)
	if stateI == nil {
		return nil
	}

	state, ok := stateI.(*mockState)
	if !ok {
		panic("impossible: execmock context key holds wrong type")
	}
	return state
}

func mustGetState(ctx context.Context) *mockState {
	state := getState(ctx)
	if state == nil {
		panic("execmock: No MockState: Use execmock.Init on context first.")
	}
	return state
}

// Init adds a mockState to the context, which indicates that this context
// should mock new exec calls using it.
//
// If `testing.Verbose()` is true (i.e. `go test -v`), this turns on "chatty
// mode", which will emit a log line for every exec this library intercepts,
// and will also copy and dump any stdout/stderr. This allows easier debugging
// of your RunnerFunctions.
//
// Panics if `ctx` has already been initialized for execmock.
// Panics if you forgot to call execmock.Intercept.
func Init(ctx context.Context) context.Context {
	if getState(ctx) != nil {
		panic(errors.New("execmock.Init: called twice on the same context"))
	}
	if !execmockctx.MockingEnabled() {
		panic(errors.New("execmock.Init: called without running execmock.Intercept in TestMain"))
	}
	return context.WithValue(ctx, &stateCtxKey, &mockState{
		chatty: testing.Verbose(),
	})
}
