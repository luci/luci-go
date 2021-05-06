// Copyright 2021 The LUCI Authors.
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

package handler

import (
	"context"
	"time"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/eventbox"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// Result is the result of handling the events.
type Result struct {
	// State is the new RunState after handling the events.
	State *state.RunState
	// SideEffectFn is called in a transaction to atomically transition the
	// RunState to the new state and perform side effect.
	SideEffectFn eventbox.SideEffectFn
	// PreserveEvents, if true, instructs RunManager not to consume the events
	// during state transition.
	PreserveEvents bool
	// PostProcessFn is executed by the eventbox user after event processing
	// completes.
	PostProcessFn eventbox.PostProcessFn
}

// Handler is an interface that handles events that RunManager receives.
type Handler interface {
	// Start starts a Run.
	Start(context.Context, *state.RunState) (*Result, error)

	// Cancel cancels a Run.
	Cancel(context.Context, *state.RunState) (*Result, error)

	// OnCLUpdated decides whether to cancel a Run based on changes to the CLs.
	OnCLUpdated(context.Context, *state.RunState, common.CLIDs) (*Result, error)

	// OnCQDVerificationCompleted finalizes the Run according to the verified
	// Run reported by CQDaemon.
	OnCQDVerificationCompleted(context.Context, *state.RunState) (*Result, error)

	// OnReadyForSubmission acquires a slot in Submit Queue and makes sure this
	// Run is not currently being submitted by another RM task. If all succeeded,
	// returns a PostProcessFn for submission.
	OnReadyForSubmission(context.Context, *state.RunState) (*Result, error)

	// OnCLSubmitted records provided CLs have been submitted.
	OnCLSubmitted(context.Context, *state.RunState, common.CLIDs) (*Result, error)

	// OnSubmissionCompleted acts on the submission result.
	//
	// If submission succeeds, mark run as `SUCCEEDED`. Otherwise, decides whether
	// to retry submission or fail the run depending on the submission result and
	// the attempt count.
	OnSubmissionCompleted(ctx context.Context, rs *state.RunState, sr eventpb.SubmissionResult, attempt int32) (*Result, error)
}

// PM encapsulates interaction with Project Manager by the Run events handler.
type PM interface {
	NotifyRunFinished(ctx context.Context, runID common.RunID) error
}

// RM encapsulates interaction with Run Manager by the Run events handler.
type RM interface {
	Invoke(ctx context.Context, runID common.RunID, eta time.Time) error
	PokeAt(ctx context.Context, runID common.RunID, eta time.Time) error
	PokeAfter(ctx context.Context, runID common.RunID, after time.Duration) error
	NotifyReadyForSubmission(ctx context.Context, runID common.RunID, eta time.Time) error
	NotifyCLSubmitted(ctx context.Context, runID common.RunID, clid common.CLID) error
	NotifySubmissionCompleted(ctx context.Context, runID common.RunID, sc *eventpb.SubmissionCompleted, invokeRM bool) error
}

// Impl is a prod implementation of Handler interface.
type Impl struct {
	PM PM
	RM RM
}

var _ Handler = (*Impl)(nil)
