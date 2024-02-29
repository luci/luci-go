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
	"bytes"
	"context"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"text/template"
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

var (
	LoadCLs = run.LoadRunCLs
)

type tmplInfo struct {
	cl *run.RunCL
}

func (ti tmplInfo) IsGerritCL() bool {
	return ti.cl.Detail.GetGerrit() != nil
}

func (ti tmplInfo) GerritChecksTabMDLink() string {
	patchset := ti.cl.Detail.Patchset
	gerritURL := ti.cl.ExternalID.MustURL()
	return fmt.Sprintf("[(view all results)](%s?checksPatchset=%d&tab=checks)", gerritURL, patchset)
}

// OnTryjobsUpdated implements Handler interface.
func (impl *Impl) OnTryjobsUpdated(ctx context.Context, rs *state.RunState, tryjobs common.TryjobIDs) (*Result, error) {
	switch status := rs.Status; {
	case run.IsEnded(status):
		fallthrough
	case status == run.Status_WAITING_FOR_SUBMISSION || status == run.Status_SUBMITTING:
		logging.Debugf(ctx, "Ignoring Tryjobs event because Run is in status %s", status)
		return &Result{State: rs}, nil
	case status != run.Status_RUNNING:
		return nil, errors.Reason("expected RUNNING status, got %s", status).Err()
	case hasExecuteTryjobLongOp(rs):
		// Process this event after the current tryjob executor finishes running.
		return &Result{State: rs, PreserveEvents: true}, nil
	default:
		tryjobs.Dedupe()
		slices.Sort(tryjobs)
		rs = rs.ShallowCopy()
		enqueueTryjobsUpdatedTask(ctx, rs, tryjobs)
		return &Result{State: rs}, nil
	}
}

func (impl *Impl) onCompletedExecuteTryjobs(ctx context.Context, rs *state.RunState, _ *run.OngoingLongOps_Op, opCompleted *eventpb.LongOpCompleted) (*Result, error) {
	opID := opCompleted.GetOperationId()
	rs = rs.ShallowCopy()
	rs.RemoveCompletedLongOp(opID)
	if rs.Status != run.Status_RUNNING {
		logging.Warningf(ctx, "long operation to execute Tryjobs has completed but Run is %s.", rs.Status)
		return &Result{State: rs}, nil
	}
	var (
		runStatus       run.Status
		msgTmpl         string
		failures        *tryjob.ExecutionState_Failures
		attentionReason string
	)
	switch opCompleted.GetStatus() {
	case eventpb.LongOpCompleted_EXPIRED:
		// Tryjob executor timeout.
		fallthrough
	case eventpb.LongOpCompleted_FAILED:
		// normally indicates tryjob executor itself encounters error (e.g. failed
		// to read from datastore).
		runStatus = run.Status_FAILED
		msgTmpl = "Unexpected error when processing Tryjobs. Please retry. If retry continues to fail, please contact LUCI team.\n\n" + cvBugLink
		attentionReason = "Run failed"
	case eventpb.LongOpCompleted_SUCCEEDED:
		switch es, _, err := tryjob.LoadExecutionState(ctx, rs.ID); {
		case err != nil:
			return nil, err
		case es == nil:
			panic(fmt.Errorf("impossible; Execute Tryjobs task succeeded but ExecutionState was missing"))
		default:
			if rs.Tryjobs == nil {
				rs.Tryjobs = &run.Tryjobs{}
			} else {
				rs.Tryjobs = proto.Clone(rs.Tryjobs).(*run.Tryjobs)
			}
			rs.Tryjobs.State = es // Copy the execution state to Run entity
			switch executionStatus := es.GetStatus(); {
			case executionStatus == tryjob.ExecutionState_SUCCEEDED && run.ShouldSubmit(&rs.Run):
				rs.Status = run.Status_WAITING_FOR_SUBMISSION
				return impl.OnReadyForSubmission(ctx, rs)
			case executionStatus == tryjob.ExecutionState_SUCCEEDED:
				runStatus = run.Status_SUCCEEDED
				if rs.Mode != run.NewPatchsetRun {
					msgTmpl = "This CL has passed the run"
				}
			case executionStatus == tryjob.ExecutionState_FAILED:
				runStatus = run.Status_FAILED
				if es.FailureReasonTmpl != "" {
					msgTmpl = "This CL has failed the run. Reason:\n\n" + es.FailureReasonTmpl
				}
				failures = es.GetFailures()
				attentionReason = "Tryjobs failed"
			case executionStatus == tryjob.ExecutionState_RUNNING:
				// Tryjobs are still running. No change to run status.
			case executionStatus == tryjob.ExecutionState_STATUS_UNSPECIFIED:
				panic(fmt.Errorf("execution status is not specified"))
			default:
				panic(fmt.Errorf("unknown tryjob execution status %s", executionStatus))
			}
		}
	default:
		panic(fmt.Errorf("unknown LongOpCompleted status: %s", opCompleted.GetStatus()))
	}

	if run.IsEnded(runStatus) {
		cls, err := LoadCLs(ctx, rs.ID, rs.CLs)
		if err != nil {
			return nil, err
		}
		var failedTryjobs []*tryjob.Tryjob
		if len(failures.GetUnsuccessfulResults()) > 0 {
			ids := make(common.TryjobIDs, len(failures.GetUnsuccessfulResults()))
			for i, r := range failures.GetUnsuccessfulResults() {
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
			var meta reviewInputMeta
			switch {
			case runStatus == run.Status_SUCCEEDED && rs.Mode == run.NewPatchsetRun:
				meta = reviewInputMeta{} // silent
			case runStatus == run.Status_SUCCEEDED:
				meta = reviewInputMeta{
					message: "This CL has passed the run",
					notify:  rs.Mode.GerritNotifyTargets(),
				}
			case runStatus == run.Status_FAILED && failures != nil:
				meta = reviewInputMeta{
					notify:         rs.Mode.GerritNotifyTargets(),
					addToAttention: rs.Mode.GerritNotifyTargets(),
					reason:         "Tryjobs failed",
				}
				switch {
				case len(failures.GetUnsuccessfulResults()) > 0:
					meta.message = "This CL has failed the run. Reason:\n\n" + composeTryjobsResultFailureReason(cl, failedTryjobs)
				case len(failures.GetLaunchFailures()) > 0:
					meta.message = composeLaunchFailureReason(failures.GetLaunchFailures())
				}
			case msgTmpl != "":
				whoms := rs.Mode.GerritNotifyTargets()
				var msg string
				switch t, err := template.New("FailureReason").Parse(msgTmpl); {
				case err != nil:
					logging.Errorf(ctx, "CL %d: failed to parse FailureReason template %q: %s", cl.ID, msgTmpl, err)
					msg = "This CL has failed the run. LUCI CV is having trouble with generating the reason. Please visit the check result tab for the failed Tryjobs."
				default:
					msgBuf := bytes.Buffer{}
					if err := t.Execute(&msgBuf, tmplInfo{cl: cl}); err != nil {
						logging.Errorf(ctx, "CL %d: failed to execute FailureReason template: %q: %s", cl.ID, msgTmpl, err)
						msg = "This CL has failed the run. LUCI CV is having trouble with generating the reason. Please visit the check result tab for the failed Tryjobs."
						//return nil, errors.Annotate(err, "failed to execute FailureReason template").Err()
					}
					msg = msgBuf.String()
				}
				meta = reviewInputMeta{
					message: msg,
					notify:  whoms,
				}
				if attentionReason != "" {
					meta.addToAttention = whoms
					meta.reason = attentionReason
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
