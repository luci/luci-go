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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// OnCQDFinished implements Handler interface.
func (impl *Impl) OnCQDFinished(ctx context.Context, rs *state.RunState) (*Result, error) {
	return impl.onCQDFinished(ctx, rs, nil)
}

func (impl *Impl) onCQDFinished(ctx context.Context, rs *state.RunState, fr *migration.FinishedCQDRun) (*Result, error) {
	switch status := rs.Run.Status; {
	case run.IsEnded(status):
		logging.Warningf(ctx, "Ignoring OnCQDFinished event because Run is %s", status)
		return &Result{State: rs}, nil
	case status != run.Status_RUNNING:
		return nil, errors.Reason("expected RUNNING status, got %s", status).Err()
	}

	var err error
	if fr == nil {
		fr, err = migration.LoadFinishedCQDRun(ctx, rs.Run.ID)
		if err != nil {
			return nil, err
		}
	}

	rs = rs.ShallowCopy()
	a := fr.Payload.GetAttempt()
	var finalStatus run.Status
	switch a.GetStatus() {
	case cvbqpb.AttemptStatus_ABORTED:
		finalStatus = run.Status_CANCELLED
	case cvbqpb.AttemptStatus_FAILURE, cvbqpb.AttemptStatus_INFRA_FAILURE:
		finalStatus = run.Status_FAILED
	case cvbqpb.AttemptStatus_SUCCESS:
		finalStatus = run.Status_SUCCEEDED
		// On SUCCESS, CQD guarantees all CLs to have the same mode.
		if a.GetGerritChanges()[0].GetMode() == cvbqpb.Mode_FULL_RUN {
			rs.Run.Submission, err = setSubmissionFromCQD(ctx, &rs.Run, a)
			if err != nil {
				return nil, err
			}
			if len(rs.Run.Submission.Cls) != len(rs.Run.Submission.SubmittedCls) {
				// CQD's SUCCESS doesn't guarantee that all CLs were submitted in
				// case of FullRun.
				finalStatus = run.Status_FAILED
			}
		}
	case cvbqpb.AttemptStatus_ATTEMPT_STATUS_UNSPECIFIED, cvbqpb.AttemptStatus_STARTED:
		fallthrough
	default:
		logging.Errorf(ctx, "Unexpected status of FinishedCQDRun %q: %s", rs.Run.ID, a.GetStatus())
		return nil, errors.Reason("unknown status of FinishedCQDRun %q: %s", rs.Run.ID, a.GetStatus()).Err()
	}

	// NOTE: there is no place in CV for tryjobs, yet, so this info is ignored,
	// but it'll be possible to backfill it later if necessary based on
	// FinishedCQDRun entities.
	rs.Run.FinalizedByCQD = true
	se := impl.endRun(ctx, rs, finalStatus)
	rs.Run.EndTime = a.GetEndTime().AsTime()
	return &Result{State: rs, SideEffectFn: se}, nil
}

// setSubmissionFromCQD populates Submission based on what CQD reported.
func setSubmissionFromCQD(ctx context.Context, r *run.Run, a *cvbqpb.Attempt) (*run.Submission, error) {
	runCLs, err := run.LoadRunCLs(ctx, r.ID, r.CLs)
	if err != nil {
		return nil, err
	}
	eidMap := make(map[changelist.ExternalID]int, len(runCLs))
	for i, rcl := range runCLs {
		eidMap[rcl.ExternalID] = i
	}

	s := &run.Submission{}
	for _, cl := range a.GetGerritChanges() {
		eid, err := changelist.GobID(cl.GetHost(), cl.GetChange())
		if err != nil {
			return nil, err
		}
		i, ok := eidMap[eid]
		if !ok {
			return nil, errors.Reason("reported GerritChange %s not found among Run CLs: %v", eid, eidMap).Err()
		}
		clid := int64(runCLs[i].ID)
		// The order of Attempt.GerritChanges isn't guaranteed to be in the submission order,
		// but at this point it doesn't really matter.
		s.Cls = append(s.Cls, clid)
		if cl.GetSubmitStatus() == cvbqpb.GerritChange_SUCCESS {
			s.SubmittedCls = append(s.SubmittedCls, clid)
		}
	}
	return s, nil
}
