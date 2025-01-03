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

	"go.chromium.org/luci/server/quota/quotapb"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/common/eventbox"
	"go.chromium.org/luci/cv/internal/common/tree"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/bq"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/run/pubsub"
	"go.chromium.org/luci/cv/internal/run/rdb"
	"go.chromium.org/luci/cv/internal/tryjob"
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
	Cancel(context.Context, *state.RunState, []string) (*Result, error)

	// OnCLsUpdated decides whether to cancel a Run based on changes to the CLs.
	OnCLsUpdated(context.Context, *state.RunState, common.CLIDs) (*Result, error)

	// UpdateConfig updates Run's config if possible.
	//
	// If Run is no longer viable, cancels the Run.
	UpdateConfig(context.Context, *state.RunState, string) (*Result, error)

	// OnReadyForSubmission acquires a slot in Submit Queue and makes sure this
	// Run is not currently being submitted by another RM task. If all succeeded,
	// returns a PostProcessFn for submission.
	OnReadyForSubmission(context.Context, *state.RunState) (*Result, error)

	// OnCLsSubmitted records provided CLs have been submitted.
	OnCLsSubmitted(context.Context, *state.RunState, common.CLIDs) (*Result, error)

	// OnSubmissionCompleted acts on the submission result.
	//
	// If submission succeeds, mark run as `SUCCEEDED`. Otherwise, decides whether
	// to retry submission or fail the run depending on the submission result.
	OnSubmissionCompleted(ctx context.Context, rs *state.RunState, sc *eventpb.SubmissionCompleted) (*Result, error)

	// OnLongOpCompleted processes results of the completed long operation.
	OnLongOpCompleted(ctx context.Context, rs *state.RunState, result *eventpb.LongOpCompleted) (*Result, error)

	// OnTryjobsUpdated decides the next step for the given tryjobs and the run.
	OnTryjobsUpdated(context.Context, *state.RunState, common.TryjobIDs) (*Result, error)

	// TryResumeSubmission resumes not-yet-expired submission if the current task
	// is a retry of the submission task.
	//
	// Fail the Run if the submission deadline has been exceeded.
	TryResumeSubmission(context.Context, *state.RunState) (*Result, error)

	// Poke checks current Run state and takes actions to progress the Run.
	Poke(context.Context, *state.RunState) (*Result, error)

	// OnParentRunCompleted checks decides how to handle the run based on its completed parent run.
	OnParentRunCompleted(context.Context, *state.RunState) (*Result, error)
}

// PM encapsulates interaction with Project Manager by the Run events handler.
type PM interface {
	NotifyRunFinished(ctx context.Context, runID common.RunID, status run.Status) error
	NotifyCLsUpdated(ctx context.Context, luciProject string, cls *changelist.CLUpdatedEvents) error
}

// RM encapsulates interaction with Run Manager by the Run events handler.
type RM interface {
	Invoke(ctx context.Context, runID common.RunID, eta time.Time) error
	PokeAfter(ctx context.Context, runID common.RunID, after time.Duration) error
	NotifyReadyForSubmission(ctx context.Context, runID common.RunID, eta time.Time) error
	NotifyCLsSubmitted(ctx context.Context, runID common.RunID, clids common.CLIDs) error
	NotifySubmissionCompleted(ctx context.Context, runID common.RunID, sc *eventpb.SubmissionCompleted, invokeRM bool) error
	Start(ctx context.Context, runID common.RunID) error
	NotifyParentRunCompleted(ctx context.Context, runID common.RunID) error
}

// QM manages run and tryjob quotas.
type QM interface {
	DebitRunQuota(ctx context.Context, r *run.Run) (*quotapb.OpResult, *cfgpb.UserLimit, error)
}

// CLUpdater encapsulates interaction with CL Updater by the Run events handler.
type CLUpdater interface {
	ScheduleBatch(ctx context.Context, luciProject string, cls []*changelist.CL, requester changelist.UpdateCLTask_Requester) error
}

// TryjobNotifier encapsulates interaction with Tryjob components by the Run
// events handler.
type TryjobNotifier interface {
	ScheduleUpdate(ctx context.Context, id common.TryjobID, eid tryjob.ExternalID) error
}

// Impl is a prod implementation of Handler interface.
type Impl struct {
	PM          PM
	RM          RM
	TN          TryjobNotifier
	QM          QM
	GFactory    gerrit.Factory
	CLUpdater   CLUpdater
	CLMutator   *changelist.Mutator
	BQExporter  *bq.Exporter
	RdbNotifier *rdb.Notifier
	TreeFactory tree.ClientFactory
	Publisher   *pubsub.Publisher
	Env         *common.Env
}

var _ Handler = (*Impl)(nil)
