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
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// OnCQDVerificationCompleted implements Handler interface.
func (impl *Impl) OnCQDVerificationCompleted(ctx context.Context, rs *state.RunState) (*Result, error) {
	switch status := rs.Run.Status; {
	case run.IsEnded(status):
		logging.Debugf(ctx, "Received CQDVerificationCompleted event after Run is ended")
		return &Result{State: rs}, nil
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
		return impl.OnReadyForSubmission(ctx, rs)
	case migrationpb.ReportVerifiedRunRequest_ACTION_DRY_RUN_OK:
		return nil, errors.New("not implemented")
	case migrationpb.ReportVerifiedRunRequest_ACTION_FAIL:
		return nil, errors.New("not implemented")
	default:
		return nil, errors.Reason("unknown action %s", vr.Payload.Action).Err()
	}
}
