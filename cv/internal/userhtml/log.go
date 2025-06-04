// Copyright 2022 The LUCI Authors.
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

package userhtml

import (
	"fmt"
	"strings"
	"time"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"

	bbutil "go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

type uiLogEntry struct {
	runLog    *run.LogEntry
	tryjobLog *tryjob.ExecutionLogEntry

	// Context info to help render message
	run *run.Run
	cls []*run.RunCL
}

func (ul *uiLogEntry) HasTryjobChips() bool {
	switch {
	case ul.runLog != nil: // legacy CQDaemon way
		return ul.runLog.GetTryjobsUpdated() != nil
	case ul.tryjobLog != nil:
		switch ul.tryjobLog.GetKind().(type) {
		case *tryjob.ExecutionLogEntry_TryjobsLaunched_:
		case *tryjob.ExecutionLogEntry_TryjobsReused_:
		case *tryjob.ExecutionLogEntry_TryjobsEnded_:
		case *tryjob.ExecutionLogEntry_TryjobDiscarded_:
		case *tryjob.ExecutionLogEntry_RetryDenied_:
		default:
			return false
		}
		return true
	default:
		panic(fmt.Errorf("either Run Log or Tryjob Execution Log must be present"))
	}
}

func (ul *uiLogEntry) Time() time.Time {
	switch {
	case ul.runLog != nil:
		return ul.runLog.GetTime().AsTime().UTC()
	case ul.tryjobLog != nil:
		return ul.tryjobLog.GetTime().AsTime().UTC()
	default:
		panic(fmt.Errorf("either Run Log or Tryjob Execution Log must be present"))
	}
}

func (ul *uiLogEntry) EventType() string {
	switch {
	case ul.runLog != nil:
		return runLogEventName(ul.runLog)
	case ul.tryjobLog != nil:
		return tryjobLogEventName(ul.tryjobLog)
	default:
		panic(fmt.Errorf("either Run Log or Tryjob Execution Log must be present"))
	}
}

func runLogEventName(rle *run.LogEntry) string {
	switch v := rle.GetKind().(type) {
	case *run.LogEntry_TryjobsUpdated_:
		return "Tryjob Updated"
	case *run.LogEntry_ConfigChanged_:
		return "Config Changed"
	case *run.LogEntry_Started_:
		return "Run Started"
	case *run.LogEntry_Created_:
		return "Run Created"
	case *run.LogEntry_TreeChecked_:
		return "Tree Status Check"
	case *run.LogEntry_Info_:
		return v.Info.Label
	case *run.LogEntry_AcquiredSubmitQueue_:
		return "Acquired Submit Queue"
	case *run.LogEntry_ReleasedSubmitQueue_:
		return "Released Submit Queue"
	case *run.LogEntry_Waitlisted_:
		return "Waiting For Submission"
	case *run.LogEntry_SubmissionFailure_:
		return "CL Submission Failed"
	case *run.LogEntry_ClSubmitted:
		return "CL Submission"
	case *run.LogEntry_RunEnded_:
		return "Run Ended"
	default:
		panic(fmt.Errorf("unknown Run log kind %T", v))
	}
}

func tryjobLogEventName(tle *tryjob.ExecutionLogEntry) string {
	switch v := tle.GetKind().(type) {
	case *tryjob.ExecutionLogEntry_RequirementChanged_:
		return "Tryjob Requirement Changed"
	case *tryjob.ExecutionLogEntry_TryjobsLaunched_:
		return "Tryjob Launched"
	case *tryjob.ExecutionLogEntry_TryjobsLaunchFailed_:
		return "Tryjob Launch Failure"
	case *tryjob.ExecutionLogEntry_TryjobsReused_:
		return "Tryjob Reused"
	case *tryjob.ExecutionLogEntry_TryjobsEnded_:
		return "Tryjob Ended"
	case *tryjob.ExecutionLogEntry_TryjobDiscarded_:
		return "Tryjob Discarded"
	case *tryjob.ExecutionLogEntry_RetryDenied_:
		return "Retry Denied"
	default:
		panic(fmt.Errorf("unknown Tryjob execution log kind %T", v))
	}
}

func (ul *uiLogEntry) Message() string {
	switch {
	case ul.runLog != nil:
		return ul.runLogMessage(ul.runLog)
	case ul.tryjobLog != nil:
		return tryjobLogMessage(ul.tryjobLog)
	default:
		panic(fmt.Errorf("either Run Log or Tryjob Execution Log must be present"))
	}
}

func (ul *uiLogEntry) runLogMessage(rle *run.LogEntry) string {
	switch v := rle.GetKind().(type) {
	case *run.LogEntry_Info_:
		return v.Info.Message
	case *run.LogEntry_ClSubmitted:
		return StringifySubmissionSuccesses(ul.cls, v.ClSubmitted)
	case *run.LogEntry_SubmissionFailure_:
		return StringifySubmissionFailureReason(ul.cls, v.SubmissionFailure.Event)
	case *run.LogEntry_RunEnded_:
		// This assumes the status of a Run won't change after transitioning to
		// one of the terminal statuses. If the assumption is no longer valid.
		// End status and cancellation reason need to be stored explicitly in
		// the log entry.
		if ul.run.Status == run.Status_CANCELLED {
			switch len(ul.run.CancellationReasons) {
			case 0:
			case 1:
				return fmt.Sprintf("Run is cancelled. Reason: %s", ul.run.CancellationReasons[0])
			default:
				var sb strings.Builder
				sb.WriteString("Run is cancelled. Reasons:")
				for _, reason := range ul.run.CancellationReasons {
					sb.WriteRune('\n')
					sb.WriteString("  * ")
					sb.WriteString(strings.TrimSpace(reason))
				}
				return sb.String()
			}
		}
		return ""
	default:
		return ""
	}
}

func tryjobLogMessage(tle *tryjob.ExecutionLogEntry) string {
	switch v := tle.GetKind().(type) {
	case *tryjob.ExecutionLogEntry_RequirementChanged_:
		// TODO(yiwzhang): describe what has changed.
		return ""
	case *tryjob.ExecutionLogEntry_TryjobsLaunchFailed_:
		var sb strings.Builder
		for i, tj := range v.TryjobsLaunchFailed.GetTryjobs() {
			if i != 0 {
				sb.WriteString("\n")
			}
			sb.WriteString("* ")
			sb.WriteString(bbutil.FormatBuilderID(tj.Definition.GetBuildbucket().GetBuilder()))
			sb.WriteString(": ")
			sb.WriteString(tj.GetReason())
		}
		return sb.String()
	case *tryjob.ExecutionLogEntry_TryjobDiscarded_:
		return fmt.Sprintf("Reason: %s", v.TryjobDiscarded.GetReason())
	case *tryjob.ExecutionLogEntry_RetryDenied_:
		return fmt.Sprintf("Can't retry following tryjob(s) because %s", v.RetryDenied.GetReason())
	default:
		return ""
	}
}

func (ul *uiLogEntry) LegacyTryjobsByStatus() map[string][]*uiTryjob {
	if ul.runLog.GetTryjobsUpdated() == nil {
		return nil
	}
	tjs := ul.runLog.GetTryjobsUpdated().GetTryjobs()
	ret := make(map[string][]*uiTryjob, len(tjs))
	for _, tj := range tjs {
		caser := cases.Title(language.English)
		k := caser.String(strings.ToLower(tj.Status.String()))
		ret[k] = append(ret[k], &uiTryjob{
			ExternalID: tryjob.ExternalID(tj.ExternalId),
			Definition: tj.Definition,
			Status:     tj.Status,
			Result:     tj.Result,
			Reused:     tj.Reused,
		})
	}
	return ret
}

func (ul *uiLogEntry) Tryjobs() []*uiTryjob {
	if !ul.HasTryjobChips() {
		panic(errors.Fmt("requested tryjobs for log entry that doesn't have tryjob chip %T", ul.tryjobLog.GetKind()))
	}
	switch v := ul.tryjobLog.GetKind().(type) {
	case *tryjob.ExecutionLogEntry_TryjobsLaunched_:
		return makeUITryjobsFromSnapshots(v.TryjobsLaunched.GetTryjobs())
	case *tryjob.ExecutionLogEntry_TryjobsReused_:
		return makeUITryjobsFromSnapshots(v.TryjobsReused.GetTryjobs())
	case *tryjob.ExecutionLogEntry_TryjobsEnded_:
		return makeUITryjobsFromSnapshots(v.TryjobsEnded.GetTryjobs())
	case *tryjob.ExecutionLogEntry_TryjobDiscarded_:
		return makeUITryjobsFromSnapshots([]*tryjob.ExecutionLogEntry_TryjobSnapshot{v.TryjobDiscarded.GetSnapshot()})
	case *tryjob.ExecutionLogEntry_RetryDenied_:
		return makeUITryjobsFromSnapshots(v.RetryDenied.GetTryjobs())
	default:
		panic(fmt.Errorf("not supported tryjob log kind %T", v))
	}
}
