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
	"sort"
	"strconv"
	"strings"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	bbutil "go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/eventpb"
	"go.chromium.org/luci/cv/internal/run/impl/state"
	"go.chromium.org/luci/cv/internal/tryjob"
)

const (
	// maxTryjobExecutorDuration is the max time that the tryjob executor
	// can process.
	maxTryjobExecutorDuration = 8 * time.Minute
)

// OnTryjobsUpdated implements Handler interface.
func (impl *Impl) OnTryjobsUpdated(ctx context.Context, rs *state.RunState, tryjobs common.TryjobIDs) (*Result, error) {
	if hasExecuteTryjobLongOp(rs) {
		// Process this event after the current tryjob executor finishes running.
		return &Result{State: rs, PreserveEvents: true}, nil
	}
	tryjobs.Dedupe()
	rs = rs.ShallowCopy()
	enqueueTryjobsUpdatedTask(ctx, rs, tryjobs)
	return &Result{State: rs}, nil
}

func (impl *Impl) onCompletedExecuteTryjobs(ctx context.Context, rs *state.RunState, _ *run.OngoingLongOps_Op, opCompleted *eventpb.LongOpCompleted) (*Result, error) {
	opID := opCompleted.GetOperationId()
	rs = rs.ShallowCopy()
	rs.RemoveCompletedLongOp(opID)
	if rs.Status != run.Status_RUNNING {
		// just simply update the execution state to the latest.
		if err := loadAndUpdateExecutionState(ctx, rs); err != nil {
			return nil, err
		}

		// Case #1 - the Run status is RUNNING, and all critical jobs are done.
		// Do nothing; the run will be marked as done in the next block, and
		// PostAction will credit the quota.
		//
		// Case #2 - it's RUNNING, and some critical tryjobs are not done.
		// Do nothing
		//
		// Case #3 - it's ENDED, but all critical jobs are done.
		// Credit it!
		//
		// Case #4 - it's ENDED, but some are still running.
		// Do nothing
		if shouldCreditRunQuota(rs) {
			logging.Infof(ctx, "onCompletedExecuteTryjobs: all critical tryjobs are finalized; enqueuing a long-op to credit the run quota.")
			enqueueCreditRunQuotaTask(ctx, rs)
		}
		return &Result{State: rs}, nil
	}
	var runStatus run.Status
	switch opCompleted.GetStatus() {
	case eventpb.LongOpCompleted_EXPIRED:
		// Tryjob executor timeout.
		fallthrough
	case eventpb.LongOpCompleted_FAILED:
		// normally indicates tryjob executor itself encounters error (e.g. failed
		// to read from datastore).
		runStatus = run.Status_FAILED
	case eventpb.LongOpCompleted_SUCCEEDED:
		if err := loadAndUpdateExecutionState(ctx, rs); err != nil {
			return nil, err
		}
		switch executionStatus := rs.Tryjobs.GetState().GetStatus(); {
		case executionStatus == tryjob.ExecutionState_SUCCEEDED && run.ShouldSubmit(&rs.Run):
			rs.Status = run.Status_WAITING_FOR_SUBMISSION
			return impl.OnReadyForSubmission(ctx, rs)
		case executionStatus == tryjob.ExecutionState_SUCCEEDED:
			runStatus = run.Status_SUCCEEDED
		case executionStatus == tryjob.ExecutionState_FAILED:
			runStatus = run.Status_FAILED
		case executionStatus == tryjob.ExecutionState_RUNNING:
			// Tryjobs are still running. No change to run status.
		case executionStatus == tryjob.ExecutionState_STATUS_UNSPECIFIED:
			return nil, errors.New("execution status is not specified")
		default:
			return nil, fmt.Errorf("unknown tryjob execution status %s", executionStatus)
		}
	default:
		return nil, fmt.Errorf("unknown LongOpCompleted status: %s", opCompleted.GetStatus())
	}

	if run.IsEnded(runStatus) {
		cls, err := run.LoadRunCLs(ctx, rs.ID, rs.CLs)
		if err != nil {
			return nil, err
		}
		executionFailures := rs.Tryjobs.GetState().GetFailures()
		var failedTryjobs []*tryjob.Tryjob
		if len(executionFailures.GetUnsuccessfulResults()) > 0 {
			ids := make(common.TryjobIDs, len(executionFailures.GetUnsuccessfulResults()))
			for i, r := range executionFailures.GetUnsuccessfulResults() {
				ids[i] = common.TryjobID(r.GetTryjobId())
			}
			var err error
			failedTryjobs, err = tryjob.LoadTryjobsByIDs(ctx, ids)
			if err != nil {
				return nil, err
			}
		}

		metas := make(map[common.CLID]reviewInputMeta, len(rs.CLs))
		for _, cl := range cls {
			if rs.HasRootCL() && cl.ID != rs.RootCL {
				continue
			}
			var meta reviewInputMeta
			switch {
			case runStatus == run.Status_SUCCEEDED && rs.Mode == run.NewPatchsetRun:
				meta = reviewInputMeta{} // silent
			case runStatus == run.Status_SUCCEEDED:
				meta = reviewInputMeta{
					message: "This CL has passed the run",
					notify:  rs.Mode.GerritNotifyTargets(),
				}
			case runStatus == run.Status_FAILED && executionFailures != nil:
				meta = reviewInputMeta{
					notify:         rs.Mode.GerritNotifyTargets(),
					addToAttention: rs.Mode.GerritNotifyTargets(),
					reason:         "Tryjobs failed",
				}
				switch {
				case len(executionFailures.GetUnsuccessfulResults()) > 0:
					meta.message = "This CL has failed the run. Reason:\n\n" + composeTryjobsResultFailureReason(cl, failedTryjobs)
				case len(executionFailures.GetLaunchFailures()) > 0:
					meta.message = composeLaunchFailureReason(executionFailures.GetLaunchFailures())
				default:
					return nil, errors.New("Bug: execution state reports failure, but no detailed failure specified")
				}
			default:
				meta = reviewInputMeta{
					message:        "Unexpected error when processing Tryjobs. Please retry. If retry continues to fail, please contact LUCI team.\n\n" + cvBugLink,
					notify:         rs.Mode.GerritNotifyTargets(),
					addToAttention: rs.Mode.GerritNotifyTargets(),
					reason:         "Run failed",
				}
			}
			metas[cl.ID] = meta
		}
		scheduleTriggersReset(ctx, rs, metas, runStatus)
	}

	return &Result{
		State: rs,
	}, nil
}

func hasExecuteTryjobLongOp(rs *state.RunState) bool {
	for _, op := range rs.OngoingLongOps.GetOps() {
		if op.GetExecuteTryjobs() != nil {
			return true
		}
	}
	return false
}

func enqueueTryjobsUpdatedTask(ctx context.Context, rs *state.RunState, tryjobs common.TryjobIDs) {
	rs.EnqueueLongOp(&run.OngoingLongOps_Op{
		Deadline: timestamppb.New(clock.Now(ctx).UTC().Add(maxTryjobExecutorDuration)),
		Work: &run.OngoingLongOps_Op_ExecuteTryjobs{
			ExecuteTryjobs: &tryjob.ExecuteTryjobsPayload{
				TryjobsUpdated: tryjobs.ToInt64(),
			},
		},
	})
}

func enqueueRequirementChangedTask(ctx context.Context, rs *state.RunState) {
	rs.EnqueueLongOp(&run.OngoingLongOps_Op{
		Deadline: timestamppb.New(clock.Now(ctx).UTC().Add(maxTryjobExecutorDuration)),
		Work: &run.OngoingLongOps_Op_ExecuteTryjobs{
			ExecuteTryjobs: &tryjob.ExecuteTryjobsPayload{
				RequirementChanged: true,
			},
		},
	})
}

func loadAndUpdateExecutionState(ctx context.Context, rs *state.RunState) error {
	es, _, err := tryjob.LoadExecutionState(ctx, rs.ID)
	switch {
	case err != nil:
		return err
	case es == nil:
		return errors.New("execute Tryjobs task succeeded but ExecutionState was missing") // impossible
	}
	if rs.Tryjobs == nil {
		rs.Tryjobs = &run.Tryjobs{}
	} else {
		rs.Tryjobs = proto.Clone(rs.Tryjobs).(*run.Tryjobs)
	}
	rs.Tryjobs.State = es // Copy the execution state to Run entity
	return nil
}

// composeTryjobsResultFailureReason make a human-readable string explaining
// the failed Tryjob result for the given CL.
func composeTryjobsResultFailureReason(cl *run.RunCL, tryjobs []*tryjob.Tryjob) string {
	switch len(tryjobs) {
	case 0:
		panic(fmt.Errorf("composeReason called without tryjobs"))
	case 1: // Optimize for most common case: one failed tryjob.
		tj := tryjobs[0]
		restricted := tj.Definition.GetResultVisibility() == cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED
		var sb strings.Builder
		if restricted {
			writeMDLink(&sb, "Tryjob", tj.ExternalID.MustURL())
			sb.WriteString(" has failed")
		} else {
			sb.WriteString("Tryjob ")
			writeMDLink(&sb, getBuilderName(tj.Definition, tj.Result), tj.ExternalID.MustURL())
			sb.WriteString(" has failed")
			if sm := tj.Result.GetBuildbucket().GetSummaryMarkdown(); sm != "" {
				sb.WriteString(" with summary")
				if cl.Detail.GetGerrit() != nil {
					fmt.Fprintf(&sb, " ([view all results](%s?checksPatchset=%d&tab=checks))", cl.ExternalID.MustURL(), cl.Detail.GetPatchset())
				}
				sb.WriteString(":\n\n---\n")
				sb.WriteString(sm)
			}
		}
		return sb.String()
	default:
		var sb strings.Builder
		sb.WriteString("Failed Tryjobs:")
		// restrict the result visibility if any tryjob has restricted result
		// visibility
		var restricted bool
		for _, tj := range tryjobs {
			if tj.Definition.GetResultVisibility() == cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED {
				restricted = true
				break
			}
		}
		for _, tj := range tryjobs {
			sb.WriteString("\n* ")
			if restricted {
				sb.WriteString(tj.ExternalID.MustURL())
			} else {
				writeMDLink(&sb, getBuilderName(tj.Definition, tj.Result), tj.ExternalID.MustURL())
				if sm := tj.Result.GetBuildbucket().GetSummaryMarkdown(); sm != "" {
					sb.WriteString(". Summary")
					if cl.Detail.GetGerrit() != nil {
						fmt.Fprintf(&sb, " ([view all results](%s?checksPatchset=%d&tab=checks))", cl.ExternalID.MustURL(), cl.Detail.GetPatchset())
					}
					sb.WriteString(":\n\n---\n")
					sb.WriteString(sm)
					sb.WriteString("\n\n---")
				}
			}
		}
		return sb.String()
	}
}

// composeLaunchFailureReason makes a string explaining tryjob launch failures.
func composeLaunchFailureReason(launchFailures []*tryjob.ExecutionState_Failures_LaunchFailure) string {
	if len(launchFailures) == 0 {
		panic(fmt.Errorf("expected non-empty launch failures"))
	}
	if len(launchFailures) == 1 { // optimize for most common case
		for _, failure := range launchFailures {
			switch {
			case failure.GetDefinition().GetBuildbucket() == nil:
				panic(fmt.Errorf("non Buildbucket backend is not supported. got %T", failure.GetDefinition().GetBackend()))
			case failure.GetDefinition().GetResultVisibility() == cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED:
				// TODO(crbug/1302119): Replace terms like "Project admin" with
				// dedicated contact sourced from Project Config.
				return "Failed to launch one tryjob. The tryjob name can't be shown due to configuration. Please contact your Project admin for help."
			default:
				builderName := bbutil.FormatBuilderID(failure.GetDefinition().GetBuildbucket().GetBuilder())
				return fmt.Sprintf("Failed to launch tryjob `%s`. Reason: %s", builderName, failure.GetReason())
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("Failed to launch the following tryjobs:")
	var restrictedCnt int
	lines := make([]string, 0, len(launchFailures))
	for _, failure := range launchFailures {
		if failure.GetDefinition().GetResultVisibility() == cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED {
			restrictedCnt++
			continue
		}
		lines = append(lines, fmt.Sprintf("* `%s`; Failure reason: %s", bbutil.FormatBuilderID(failure.GetDefinition().GetBuildbucket().GetBuilder()), failure.GetReason()))
	}
	sort.Strings(lines) // for determinism
	for _, l := range lines {
		sb.WriteRune('\n')
		sb.WriteString(l)
	}

	switch {
	case restrictedCnt == len(launchFailures):
		// TODO(crbug/1302119): Replace terms like "Project admin" with
		// dedicated contact sourced from Project Config.
		return fmt.Sprintf("Failed to launch %d tryjobs. The tryjob names can't be shown due to configuration. Please contact your Project admin for help.", restrictedCnt)
	case restrictedCnt > 0:
		sb.WriteString("\n\nIn addition to the tryjobs above, failed to launch ")
		sb.WriteString(strconv.Itoa(restrictedCnt))
		sb.WriteString(" tryjob")
		if restrictedCnt > 1 {
			sb.WriteString("s")
		}
		sb.WriteString(". But the tryjob names can't be shown due to configuration. Please contact your Project admin for help.")
	}
	return sb.String()
}

// getBuilderName gets the Buildbucket builder name from Tryjob result or
// Tryjob definition.
//
// Tries to get builder name from the result first as it reflects actual
// builder launched which may or may not be the main builder in the tryjob
// definition.
func getBuilderName(def *tryjob.Definition, result *tryjob.Result) string {
	if result != nil && result.GetBackend() != nil {
		switch result.GetBackend().(type) {
		case *tryjob.Result_Buildbucket_:
			if builder := result.GetBuildbucket().GetBuilder(); builder != nil {
				return bbutil.FormatBuilderID(builder)
			}
		default:
			panic(fmt.Errorf("non Buildbucket tryjob backend is not supported. got %T", result.GetBackend()))
		}
	}
	if def != nil && def.GetBackend() != nil {
		switch def.GetBackend().(type) {
		case *tryjob.Definition_Buildbucket_:
			if builder := def.GetBuildbucket().GetBuilder(); builder != nil {
				return bbutil.FormatBuilderID(builder)
			}
		default:
			panic(fmt.Errorf("non Buildbucket tryjob backend is not supported. got %T", def.GetBackend()))
		}
	}
	panic(fmt.Errorf("impossible; can't get builder name from definition and result. Definition: %s; Result: %s", def, result))
}

func writeMDLink(sb *strings.Builder, text, url string) {
	sb.WriteString("[")
	sb.WriteString(text)
	sb.WriteString("](")
	sb.WriteString(url)
	sb.WriteString(")")
}
