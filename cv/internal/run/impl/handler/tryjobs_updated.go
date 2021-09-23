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
	"fmt"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/migration"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/tryjob"
)

// OnCQDTryjobsUpdated implements Handler interface.
func (impl *Impl) OnCQDTryjobsUpdated(ctx context.Context, rs *state.RunState) (*Result, error) {
	switch status := rs.Status; {
	case run.IsEnded(status):
		logging.Debugf(ctx, "Ignoring CQDTryjobsUpdated event because Run is %s", status)
		return &Result{State: rs}, nil
	case status == run.Status_WAITING_FOR_SUBMISSION || status == run.Status_SUBMITTING:
		// Delay processing this event until submission completes.
		return &Result{State: rs, PreserveEvents: true}, nil
	case status != run.Status_RUNNING:
		return nil, errors.Reason("expected RUNNING status, got %s", status).Err()
	}

	var lastSeen time.Time
	if t := rs.Tryjobs.GetCqdUpdateTime(); t != nil {
		lastSeen = t.AsTime()
	}

	// Limit loading reported tryjobs to 128 latest only.
	// If there is a malfunction in CQDaemon, ignoring earlier reports is fine.
	switch latest, err := migration.ListReportedTryjobs(ctx, rs.ID, lastSeen, 128); {
	case err != nil:
		return nil, errors.Annotate(err, "failed to load latest reported Tryjobs").Err()
	case len(latest) == 0:
		logging.Warningf(ctx, "received CQDTryjobsUpdated event, but couldn't find any new reports")
		return &Result{State: rs}, nil
	default:
		logging.Debugf(ctx, "received CQDTryjobsUpdated event, read %d latest tryjob reports", len(latest))
		rs = rs.ShallowCopy()
		if rs.Tryjobs == nil {
			rs.Tryjobs = &run.Tryjobs{}
		} else {
			rs.Tryjobs = proto.Clone(rs.Tryjobs).(*run.Tryjobs)
		}
		// `latest` are ordered newest to oldest, so process them in reverse order
		// such the newest report is ultimately stored in the Run.Tryjobs.
		for i := len(latest) - 1; i > -1; i-- {
			updateTryjobsFromCQD(ctx, rs, latest[i])
		}
		return &Result{State: rs}, nil
	}
}

func updateTryjobsFromCQD(ctx context.Context, rs *state.RunState, rep *migration.ReportedTryjobs) {
	before := rs.Tryjobs.GetTryjobs()
	after := make([]*run.Tryjob, 0, len(rep.Payload.GetTryjobs()))
	for _, cqd := range rep.Payload.GetTryjobs() {
		if cqd.GetBuilder() == nil {
			logging.Warningf(ctx, "Skipping old Tryjob report from CQD without builder information: %q", rep.ID)
			return
		}
		switch t, err := toRunTryjob(cqd); {
		case err != nil:
			logging.Errorf(ctx, "Failed to convert CQD tryjob to run.Tryjob: %s\nTryjob details:\n%s", err, cqd)
		default:
			after = append(after, t)
		}
	}
	run.SortTryjobs(after)
	rs.Tryjobs.CqdUpdateTime = timestamppb.New(rep.ReportTime())
	rs.Tryjobs.Tryjobs = after
	if updated := run.DiffTryjobsForReport(before, after); len(updated) > 0 {
		rs.LogEntries = append(rs.LogEntries, &run.LogEntry{
			Time: timestamppb.New(rep.ReportTime()),
			Kind: &run.LogEntry_TryjobsUpdated_{
				TryjobsUpdated: &run.LogEntry_TryjobsUpdated{
					Tryjobs: updated,
				},
			},
		})
	}
}

// toRunTryjob constructs CV representation of a Tryjob based on CQDaemon's
// input.
//
// Best effort. Some information loss expected. Intended use for UI, not for
// decision making.
func toRunTryjob(in *migrationpb.Tryjob) (*run.Tryjob, error) {
	eid, err := tryjob.BuildbucketID(in.GetBuild().GetHost(), in.GetBuild().GetId())
	if err != nil {
		return nil, err
	}

	tjStatus := tryjob.Status_PENDING
	resStatus := tryjob.Result_RESULT_STATUS_UNSPECIFIED
	bbStatus := buildbucketpb.Status_STATUS_UNSPECIFIED
	switch in.GetStatus() {
	case migrationpb.TryjobStatus_NOT_TRIGGERED:
		tjStatus = tryjob.Status_PENDING
	case migrationpb.TryjobStatus_PENDING:
		// CQD's PENDING means build is scheduled but isn't yet running,
		// but CV doesn't differentiate them.
		tjStatus = tryjob.Status_TRIGGERED
		bbStatus = buildbucketpb.Status_SCHEDULED
	case migrationpb.TryjobStatus_RUNNING:
		tjStatus = tryjob.Status_TRIGGERED
		bbStatus = buildbucketpb.Status_STARTED
	case migrationpb.TryjobStatus_FAILED:
		tjStatus = tryjob.Status_ENDED
		resStatus = tryjob.Result_FAILED_PERMANENTLY
		bbStatus = buildbucketpb.Status_FAILURE
	case migrationpb.TryjobStatus_SUCCEEDED:
		tjStatus = tryjob.Status_ENDED
		resStatus = tryjob.Result_SUCCEEDED
		bbStatus = buildbucketpb.Status_SUCCESS
	case migrationpb.TryjobStatus_TIMED_OUT:
		tjStatus = tryjob.Status_ENDED
		resStatus = tryjob.Result_FAILED_TRANSIENTLY
		bbStatus = buildbucketpb.Status_INFRA_FAILURE
	default:
		panic(fmt.Errorf("unhandled CQD status %s", in.GetStatus()))
	}

	return &run.Tryjob{
		Id:         0,
		ExternalId: string(eid),
		Eversion:   0,
		Definition: &tryjob.Definition{
			Backend: &tryjob.Definition_Buildbucket_{Buildbucket: &tryjob.Definition_Buildbucket{
				Host:    in.GetBuild().GetHost(),
				Builder: in.GetBuilder(),
			}},
		},
		Reused: in.GetBuild().GetOrigin() == cvbqpb.Build_REUSED,
		Status: tjStatus,
		Result: &tryjob.Result{
			Status:     resStatus,
			CreateTime: in.GetCreateTime(),
			UpdateTime: nil, // CQD doesn't track this.
			Output:     nil, // recipe Output is not relevant to the UI.
			Backend: &tryjob.Result_Buildbucket_{Buildbucket: &tryjob.Result_Buildbucket{
				Id:     in.GetBuild().GetId(),
				Status: bbStatus,
			}},
		},
		CqdDerived: true,
	}, nil
}
