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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	commonpb "go.chromium.org/luci/cv/api/common/v1"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
)

// OnCQDTryjobsUpdated implements Handler interface.
func (impl *Impl) OnCQDTryjobsUpdated(ctx context.Context, rs *state.RunState) (*Result, error) {
	switch status := rs.Run.Status; {
	case run.IsEnded(status):
		logging.Debugf(ctx, "Ignoring CQDTryjobsUpdated event because Run is %s", status)
		return &Result{State: rs}, nil
	case status == commonpb.Run_WAITING_FOR_SUBMISSION || status == commonpb.Run_SUBMITTING:
		// Delay processing this event until submission completes.
		return &Result{State: rs, PreserveEvents: true}, nil
	case status != commonpb.Run_RUNNING:
		return nil, errors.Reason("expected RUNNING status, got %s", status).Err()
	}

	// Limit loading reported tryjobs to 128 latest only.
	// If there is a malfunction in CQDaemon, ignoring earlier reports is fine.
	switch latest, err := migration.ListReportedTryjobs(ctx, rs.Run.ID, time.Time{}, 128); {
	case err != nil:
		return nil, errors.Annotate(err, "failed to load latest reported Tryjobs").Err()
	case len(latest) == 0:
		logging.Errorf(ctx, "received CQDTryjobsUpdated event, but couldn't find any reported tryjobs")
		return &Result{State: rs}, nil
	default:
		logging.Debugf(ctx, "received CQDTryjobsUpdated event, read %d latest tryjob reports", len(latest))
		// TODO(crbug/1232158): implement.
		return &Result{State: rs}, nil
	}
}
