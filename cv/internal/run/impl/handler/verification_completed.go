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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/protobuf/types/known/timestamppb"

	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/usertext"
)

const (
	cqdVerifiedLabel           = "CQD Verified"
	cqdVerificationFailedLabel = "CQD Verification Failed"
)

// OnCQDVerificationCompleted implements Handler interface.
func (impl *Impl) OnCQDVerificationCompleted(ctx context.Context, rs *state.RunState) (*Result, error) {
	switch status := rs.Run.Status; {
	case run.IsEnded(status):
		logging.Debugf(ctx, "Ignoring CQDVerificationCompleted event because Run is %s", status)
		return &Result{State: rs}, nil
	case status == run.Status_WAITING_FOR_SUBMISSION || status == run.Status_SUBMITTING:
		// Run probably entered submission phase due to previously received
		// CQDVerificationCompleted event. Delay processing this event
		// until submission completes.
		return &Result{State: rs, PreserveEvents: true}, nil
	case status != run.Status_RUNNING:
		return nil, errors.Reason("expected RUNNING status, got %s", status).Err()
	}

	rid := rs.Run.ID
	vr := migration.VerifiedCQDRun{ID: rid}
	switch err := datastore.Get(ctx, &vr); {
	case err == datastore.ErrNoSuchEntity:
		return nil, errors.New("received CQDVerificationCompleted event but VerifiedRun entity doesn't exist")
	case err != nil:
		return nil, errors.Annotate(err, "failed to load VerifiedRun").Tag(transient.Tag).Err()
	}

	rs = rs.ShallowCopy()
	switch vr.Payload.Action {
	case migrationpb.ReportVerifiedRunRequest_ACTION_SUBMIT:
		rs.Run.Status = run.Status_WAITING_FOR_SUBMISSION
		rs.LogEntries = append(rs.LogEntries, &run.LogEntry{
			Time: timestamppb.New(clock.Now(ctx)),
			Kind: &run.LogEntry_Info_{
				Info: &run.LogEntry_Info{
					Label:   cqdVerifiedLabel,
					Message: usertext.OnFullRunSucceeded(rs.Run.Mode),
				},
			},
		})
		return impl.OnReadyForSubmission(ctx, rs)
	case migrationpb.ReportVerifiedRunRequest_ACTION_DRY_RUN_OK:
		msg := usertext.OnRunSucceeded(rs.Run.Mode)
		if err := impl.cancelTriggers(ctx, rs, msg); err != nil {
			return nil, err
		}
		rs.LogEntries = append(rs.LogEntries, &run.LogEntry{
			Time: timestamppb.New(clock.Now(ctx)),
			Kind: &run.LogEntry_Info_{
				Info: &run.LogEntry_Info{
					Label:   cqdVerifiedLabel,
					Message: msg,
				},
			},
		})
		se := impl.endRun(ctx, rs, run.Status_SUCCEEDED)
		return &Result{State: rs, SideEffectFn: se}, nil
	case migrationpb.ReportVerifiedRunRequest_ACTION_FAIL:
		if err := impl.cancelTriggers(ctx, rs, vr.Payload.FinalMessage); err != nil {
			return nil, err
		}
		rs.LogEntries = append(rs.LogEntries, &run.LogEntry{
			Time: timestamppb.New(clock.Now(ctx)),
			Kind: &run.LogEntry_Info_{
				Info: &run.LogEntry_Info{
					Label:   cqdVerificationFailedLabel,
					Message: vr.Payload.FinalMessage,
				},
			},
		})
		se := impl.endRun(ctx, rs, run.Status_FAILED)
		return &Result{State: rs, SideEffectFn: se}, nil
	default:
		return nil, errors.Reason("unknown action %s", vr.Payload.Action).Err()
	}
}

func (impl *Impl) cancelTriggers(ctx context.Context, rs *state.RunState, msg string) error {
	runCLs, err := run.LoadRunCLs(ctx, rs.Run.ID, rs.Run.CLs)
	if err != nil {
		return err
	}
	runCLExternalIDs := make([]changelist.ExternalID, len(runCLs))
	for i, cl := range runCLs {
		runCLExternalIDs[i] = cl.ExternalID
	}
	cg, err := rs.LoadConfigGroup(ctx)
	if err != nil {
		return err
	}
	return impl.cancelCLTriggers(ctx, rs.Run.ID, runCLs, runCLExternalIDs, msg, cg)
}
