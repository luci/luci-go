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
	"regexp"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/prjcfg"
	"go.chromium.org/luci/cv/internal/metrics"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/tryjob/requirement"
)

const (
	// maxPostStartMessageDuration is the max time that a Run will be waiting for
	// the starting message to be posted on every CL.
	maxPostStartMessageDuration = 8 * time.Minute

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

	if reasons, isValid := validateRunOptions(rs); !isValid {
		rs = rs.ShallowCopy()
		var msgBuilder strings.Builder
		msgBuilder.WriteString("Failed to start the Run. Reason:")
		switch len(reasons) {
		case 0:
			panic(fmt.Errorf("must provide reason"))
		case 1:
			msgBuilder.WriteRune(' ')
			msgBuilder.WriteString(reasons[0])
		default:
			msgBuilder.WriteString("\n")
			for _, reason := range reasons {
				msgBuilder.WriteString("\n* ")
				msgBuilder.WriteString(reason)
			}
		}
		whoms := rs.Mode.GerritNotifyTargets()
		meta := reviewInputMeta{
			message:        msgBuilder.String(),
			notify:         whoms,
			addToAttention: whoms,
			reason:         "Failed to start the run",
		}
		metas := make(map[common.CLID]reviewInputMeta, len(rs.CLs))
		for _, cl := range rs.CLs {
			metas[cl] = meta
		}
		scheduleTriggersReset(ctx, rs, metas, run.Status_FAILED)
		return &Result{State: rs}, nil
	}

	rs = rs.ShallowCopy()
	cg, runCLs, cls, err := loadCLsAndConfig(ctx, rs, rs.CLs)
	if err != nil {
		return nil, err
	}
	rs = rs.ShallowCopy()
	switch ok, err := checkRunCreate(ctx, rs, cg, runCLs, cls); {
	case err != nil:
		return nil, err
	case !ok:
		return &Result{State: rs}, nil
	}

	switch result, err := requirement.Compute(ctx, requirement.Input{
		ConfigGroup: cg.Content,
		RunOwner:    rs.Owner,
		CLs:         runCLs,
		RunOptions:  rs.Options,
		RunMode:     rs.Mode,
	}); {
	case err != nil:
		return nil, err
	case result.OK():
		rs.Tryjobs = &run.Tryjobs{
			Requirement:           result.Requirement,
			RequirementVersion:    1,
			RequirementComputedAt: timestamppb.New(clock.Now(ctx).UTC()),
		}
		rs.LogInfof(ctx, "Tryjob", "LUCI CV is managing the Tryjobs for this Run")
		enqueueRequirementChangedTask(ctx, rs)
	default:
		whoms := rs.Mode.GerritNotifyTargets()
		meta := reviewInputMeta{
			message:        fmt.Sprintf("Failed to compute tryjob requirement. Reason:\n\n%s", result.ComputationFailure.Reason()),
			notify:         whoms,
			addToAttention: whoms,
			reason:         "Computing tryjob requirement failed",
		}
		metas := make(map[common.CLID]reviewInputMeta, len(cls))
		for _, cl := range rs.CLs {
			metas[cl] = meta
		}
		scheduleTriggersReset(ctx, rs, metas, run.Status_FAILED)
		rs.LogInfof(ctx, "Tryjob Requirement Computation", "Failed to compute tryjob requirement. Reason: %s", result.ComputationFailure.Reason())
		return &Result{State: rs}, nil
	}

	// New patchset runs should be quiet.
	if rs.Mode != run.NewPatchsetRun {
		rs.EnqueueLongOp(&run.OngoingLongOps_Op{
			Deadline: timestamppb.New(clock.Now(ctx).UTC().Add(maxPostStartMessageDuration)),
			Work: &run.OngoingLongOps_Op_PostStartMessage{
				PostStartMessage: true,
			},
		})
	}
	// Note that it is inevitable that duplicate pickup latency metric maybe
	// emitted for the same Run if the state transition fails later that
	// causes a retry.
	rs.Status = run.Status_RUNNING
	rs.StartTime = datastore.RoundTime(clock.Now(ctx).UTC())
	rs.LogEntries = append(rs.LogEntries, &run.LogEntry{
		Time: timestamppb.New(rs.StartTime),
		Kind: &run.LogEntry_Started_{
			Started: &run.LogEntry_Started{},
		},
	})

	return &Result{
		State: rs,
		SideEffectFn: func(ctx context.Context) error {
			txndefer.Defer(ctx, func(ctx context.Context) {
				reportStartMetrics(ctx, rs, cg)
			})
			return nil
		},
	}, nil
}

func (impl *Impl) onCompletedPostStartMessage(ctx context.Context, rs *state.RunState, op *run.OngoingLongOps_Op, result *eventpb.LongOpCompleted) (*Result, error) {
	opID := result.GetOperationId()
	rs = rs.ShallowCopy()
	rs.RemoveCompletedLongOp(opID)
	if rs.Status != run.Status_RUNNING {
		logging.Warningf(ctx, "Long operation to post start message completed but Run is %s: %s", rs.Status, result)
		return &Result{State: rs}, nil
	}

	switch result.GetStatus() {
	case eventpb.LongOpCompleted_FAILED:
		// NOTE: if there are several CLs across different projects, detailed
		// failure may have sensitive info which shouldn't be shared elsewhere.
		// Future improvement: if posting start message is a common problem,
		// record `result` message and render links to CLs on which CV failed to
		// post starting message.
		rs.LogInfo(ctx, logEntryLabelPostStartMessage, "Failed to post the starting message")
	case eventpb.LongOpCompleted_EXPIRED:
		rs.LogInfo(ctx, logEntryLabelPostStartMessage, fmt.Sprintf("Failed to post the starting message within the %s deadline", maxPostStartMessageDuration))
	case eventpb.LongOpCompleted_SUCCEEDED:
		// TODO(tandrii): simplify once all such events have timestamp.
		if t := result.GetPostStartMessage().GetTime(); t != nil {
			rs.LogInfoAt(t.AsTime(), logEntryLabelPostStartMessage, "posted start message on each CL")
		} else {
			rs.LogInfo(ctx, logEntryLabelPostStartMessage, "posted start message on each CL")
		}
	case eventpb.LongOpCompleted_CANCELLED:
		rs.LogInfo(ctx, logEntryLabelPostStartMessage, "cancelled posting start message on CL(s)")
	default:
		panic(fmt.Errorf("unexpected LongOpCompleted status: %s", result.GetStatus()))
	}

	return &Result{State: rs}, nil
}

const customTryjobTagRe = `^[a-z0-9_\-]+:.+$`

var customTryjobTagReCompiled = regexp.MustCompile(customTryjobTagRe)

func validateRunOptions(rs *state.RunState) (reasons []string, isValid bool) {
	for _, tag := range rs.Options.GetCustomTryjobTags() {
		if !customTryjobTagReCompiled.Match([]byte(strings.TrimSpace(tag))) {
			reasons = append(reasons, fmt.Sprintf("malformed tag: %q; expecting format %q", tag, customTryjobTagRe))
		}
	}
	if len(reasons) == 0 {
		return nil, true
	}
	return reasons, false
}

func reportStartMetrics(ctx context.Context, rs *state.RunState, cg *prjcfg.ConfigGroup) {
	project := rs.ID.LUCIProject()
	metrics.Public.RunStarted.Add(ctx, 1, project, rs.ConfigGroupID.Name(), string(rs.Mode))
	delay := rs.StartTime.Sub(rs.CreateTime)
	metricPickupLatencyS.Add(ctx, delay.Seconds(), project)

	if d := cg.Content.GetCombineCls().GetStabilizationDelay(); d != nil {
		delay -= d.AsDuration()
	}
	metricPickupLatencyAdjustedS.Add(ctx, delay.Seconds(), project)

	if delay >= 1*time.Minute {
		logging.Warningf(ctx, "Too large adjusted pickup delay: %s", delay)
	}
}
