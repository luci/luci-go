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

package execute

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	bbutil "go.chromium.org/luci/buildbucket/protoutil"

	cfgpb "go.chromium.org/luci/cv/api/config/v2"
	"go.chromium.org/luci/cv/internal/tryjob"
)

type intSet map[int]struct{}

func (is intSet) has(value int) bool {
	_, ret := is[value]
	return ret
}

// composeReason make a human-readable string explaining about failing Tryjobs.
func composeReason(tryjobs []*tryjob.Tryjob) string {
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
				sb.WriteString(" with summary:\n\n")
				sb.WriteString("---\n")
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
					sb.WriteString(". Summary:\n\n")
					sb.WriteString("---\n")
					sb.WriteString(sm)
					sb.WriteString("\n\n---")
				}
			}
		}
		return sb.String()
	}
}

// composeLaunchFailureReason makes a string explaining tryjob launch failures.
func composeLaunchFailureReason(launchFailures map[*tryjob.Definition]string) string {
	if len(launchFailures) == 0 {
		panic(fmt.Errorf("expected non-empty launch failures"))
	}
	if len(launchFailures) == 1 { // optimize for most common case
		for def, reason := range launchFailures {
			switch {
			case def.GetBuildbucket() == nil:
				panic(fmt.Errorf("non Buildbucket backend is not supported. got %T", def.GetBackend()))
			case def.GetResultVisibility() == cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED:
				// TODO(crbug/1302119): Replace terms like "Project admin" with
				// dedicated contact sourced from Project Config.
				return "Failed to launch one tryjob. The tryjob name can't be shown due to configuration. Please contact your Project admin for help."
			default:
				builderName := bbutil.FormatBuilderID(def.GetBuildbucket().GetBuilder())
				return fmt.Sprintf("Failed to launch tryjob `%s`. Reason: %s", builderName, reason)
			}
		}
	}

	var sb strings.Builder
	sb.WriteString("Failed to launch the following tryjobs:")
	var restrictedCnt int
	lines := make([]string, 0, len(launchFailures))
	for def, reason := range launchFailures {
		if def.GetResultVisibility() == cfgpb.CommentLevel_COMMENT_LEVEL_RESTRICTED {
			restrictedCnt++
			continue
		}
		lines = append(lines, fmt.Sprintf("* `%s`; Failure reason: %s", bbutil.FormatBuilderID(def.GetBuildbucket().GetBuilder()), reason))
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

func makeLogTryjobSnapshot(def *tryjob.Definition, tj *tryjob.Tryjob, reused bool) *tryjob.ExecutionLogEntry_TryjobSnapshot {
	return &tryjob.ExecutionLogEntry_TryjobSnapshot{
		Definition: def,
		Id:         int64(tj.ID),
		ExternalId: string(tj.ExternalID),
		Status:     tj.Status,
		Result:     tj.Result,
		Reused:     reused,
	}
}

func makeLogTryjobSnapshotFromAttempt(def *tryjob.Definition, attempt *tryjob.ExecutionState_Execution_Attempt) *tryjob.ExecutionLogEntry_TryjobSnapshot {
	return &tryjob.ExecutionLogEntry_TryjobSnapshot{
		Definition: def,
		Id:         attempt.GetTryjobId(),
		ExternalId: attempt.GetExternalId(),
		Status:     attempt.GetStatus(),
		Result:     attempt.GetResult(),
		Reused:     attempt.GetReused(),
	}
}
