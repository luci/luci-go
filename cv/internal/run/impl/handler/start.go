// Copyright 2020 The LUCI Authors.
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

	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/acls"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/tryjob/requirement"
	"go.chromium.org/luci/cv/internal/usertext"
)

const (
	// maxPostStartMessageDuration is the max time that a Run will be waiting for
	// the starting message to be posted on every CL.
	maxPostStartMessageDuration = 10 * time.Minute

	logEntryLabelPostStartMessage = "Posting Starting Message"
)

// Start implements Handler interface.
func (impl *Impl) Start(ctx context.Context, rs *state.RunState) (*Result, error) {
	switch status := rs.Status; {
	case status == run.Status_STATUS_UNSPECIFIED:
		err := errors.Reason("CRITICAL: can't start a Run %q with unspecified status", rs.ID).Err()
		common.LogError(ctx, err)
		panic(err)
	case status != run.Status_PENDING:
		logging.Debugf(ctx, "Skip starting Run because this Run is %s", status)
		return &Result{State: rs}, nil
	}

	rs = rs.ShallowCopy()
	rs.Status = run.Status_RUNNING
	rs.StartTime = datastore.RoundTime(clock.Now(ctx).UTC())
	rs.LogEntries = append(rs.LogEntries, &run.LogEntry{
		Time: timestamppb.New(rs.StartTime),
		Kind: &run.LogEntry_Started_{
			Started: &run.LogEntry_Started{},
		},
	})

	var cg *prjcfg.ConfigGroup
	var runCLs []*run.RunCL
	eg, ectx := errgroup.WithContext(ctx)
	eg.Go(func() (err error) {
		cg, err = prjcfg.GetConfigGroup(ectx, rs.ID.LUCIProject(), rs.ConfigGroupID)
		if err != nil {
			return err
		}
		return nil
	})
	eg.Go(func() (err error) {
		runCLs, err = run.LoadRunCLs(ectx, rs.ID, rs.CLs)
		return err
	})
	if err := eg.Wait(); err != nil {
		return nil, err
	}

	switch result, err := acls.CheckRunCreate(ctx, cg, runCLs); {
	case err != nil:
		return nil, errors.Annotate(err, "CheckRunCreate").Err()
	case !result.OK:
		// Let the Run move forwards.
		// CQD should reject the Run via the migration steps.
		//
		// TODO(crbug/1268574): cancel the Run, once it's verified that
		// both (CQ and CV) implementations agree with each other.
		if s := result.FailuresSummary; s != "" {
			logging.Infof(ctx, "crbug/1268574: %s", s)
		}
	}

	switch result, err := requirement.Compute(ctx, requirement.Input{
		ConfigGroup: cg,
		RunOwner:    rs.Owner,
		CLs:         runCLs,
		RunOptions:  rs.Options,
	}); {
	case err != nil:
		return nil, err
	case result.OK():
		rs.EnqueueLongOp(&run.OngoingLongOps_Op{
			Deadline: timestamppb.New(clock.Now(ctx).Add(maxPostStartMessageDuration)),
			Work: &run.OngoingLongOps_Op_PostStartMessage{
				PostStartMessage: true,
			},
		})
		rs.Tryjobs = &run.Tryjobs{
			Requirement: result.Requirement,
		}
		// TODO(crbug/1227363): enqueue long op to execute requirement.
	default:
		rs.LogInfof(ctx, "tryjob requirement computation", "failed to compute tryjob requirement. Reason: %s", result.ComputationFailure.Reason())
		scheduleTriggersCancellation(ctx, rs, rs.CLs, reviewInputMeta{
			message:        fmt.Sprintf("LUCI CV failed to compute tryjob requirement. Reason:\n\n  %s", result.ComputationFailure.Reason()),
			notify:         gerrit.Whoms{gerrit.Owner, gerrit.CQVoters},
			addToAttention: gerrit.Whoms{gerrit.Owner, gerrit.CQVoters},
			reason:         "failed to compute tryjob requirement",
		}, run.Status_FAILED)
	}
	// Note that it is inevitable that duplicate pickup latency metric maybe
	// emitted for the same Run if the state transition fails later that
	// causes a retry.
	recordPickupLatency(ctx, &(rs.Run), cg)
	return &Result{State: rs}, nil
}

func (impl *Impl) onCompletedPostStartMessage(ctx context.Context, rs *state.RunState, op *run.OngoingLongOps_Op, result *eventpb.LongOpCompleted) (*Result, error) {
	opID := result.GetOperationId()
	rs = rs.ShallowCopy()
	rs.RemoveCompletedLongOp(opID)
	if rs.Status != run.Status_RUNNING {
		logging.Warningf(ctx, "Long operation to post start message completed but Run is %s: %s", rs.Status, result)
		return &Result{State: rs}, nil
	}

	failRunReason := "Failed to post the starting message"
	switch result.GetStatus() {
	case eventpb.LongOpCompleted_FAILED:
		// NOTE: if there are several CLs across different projects, detailed
		// failure may have sensitive info which shouldn't be shared elsewhere.
		// Future improvement: if posting start message is a common problem,
		// record `result` message and render links to CLs on which CV failed to
		// post starting message.
		rs.LogInfo(ctx, logEntryLabelPostStartMessage, failRunReason)
	case eventpb.LongOpCompleted_EXPIRED:
		failRunReason += fmt.Sprintf(" within the %s deadline", maxPostStartMessageDuration)
		rs.LogInfo(ctx, logEntryLabelPostStartMessage, failRunReason)
	case eventpb.LongOpCompleted_SUCCEEDED:
		// TODO(tandrii): simplify once all such events have timestamp.
		if t := result.GetPostStartMessage().GetTime(); t != nil {
			rs.LogInfoAt(t.AsTime(), logEntryLabelPostStartMessage, "posted start message on each CL")
		} else {
			rs.LogInfo(ctx, logEntryLabelPostStartMessage, "posted start message on each CL")
		}
		return &Result{State: rs}, nil
	case eventpb.LongOpCompleted_CANCELLED:
		rs.LogInfo(ctx, logEntryLabelPostStartMessage, "cancelled posting start message on CL(s)")
		return &Result{State: rs}, nil
	default:
		panic(fmt.Errorf("unexpected LongOpCompleted status: %s", result.GetStatus()))
	}

	msgPrefix, attentionReason := usertext.OnRunFailed(rs.Mode)
	meta := reviewInputMeta{
		notify:         gerrit.Whoms{gerrit.Owner, gerrit.CQVoters},
		addToAttention: gerrit.Whoms{gerrit.Owner, gerrit.CQVoters},
		reason:         attentionReason,
		message:        msgPrefix + "\n\n" + failRunReason,
	}
	if err := impl.cancelTriggers(ctx, rs, meta); err != nil {
		return nil, err
	}
	return &Result{
		State:        rs,
		SideEffectFn: impl.endRun(ctx, rs, run.Status_FAILED),
	}, nil
}

func recordPickupLatency(ctx context.Context, r *run.Run, cg *prjcfg.ConfigGroup) {
	delay := r.StartTime.Sub(r.CreateTime)
	metricPickupLatencyS.Add(ctx, delay.Seconds(), r.ID.LUCIProject())

	if d := cg.Content.GetCombineCls().GetStabilizationDelay(); d != nil {
		delay -= d.AsDuration()
	}
	metricPickupLatencyAdjustedS.Add(ctx, delay.Seconds(), r.ID.LUCIProject())

	if delay >= 1*time.Minute {
		logging.Warningf(ctx, "Too large adjusted pickup delay: %s", delay)
	}
}
