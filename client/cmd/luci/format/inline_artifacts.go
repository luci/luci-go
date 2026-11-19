// Copyright 2026 The LUCI Authors.
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

package format

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"go.chromium.org/luci/client/cmd/luci/base"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	// DefaultMaxOutputBytes is the default overall stdout byte cap (9.5 KB / 9,500 B)
	// across LUCI CLI commands so output stays within agent 10 KB tool output limits
	// without middle-truncation.
	DefaultMaxOutputBytes = 9500

	// DefaultInlineGetArtifactBudget is the maximum byte budget (1.5 KB / 1,536 B)
	// for inline failure-artifact summaries rendered inside `get` subcommands
	// (`test-result get`, `verdict get`, `work-unit get`, and `module get`).
	DefaultInlineGetArtifactBudget = 1536

	// MaxInspectedWorkUnits bounds the number of target work units queried for
	// direct and ancestor artifacts in a single call.
	MaxInspectedWorkUnits = 20

	maxInlineAncestorArtifacts           = 15
	defaultMaxListedArtifacts            = 100
	maxVerdictFailedAttemptsForArtifacts = 3
	minShrunkInlineBudget                = 512
)

// TestResultArtifactFormatter formats prefetched inline direct and parent/ancestor
// artifacts for a test result within maxBytes (0 for unlimited, <0 for empty),
// excluding any direct artifact IDs in excludedArtIDs.
type TestResultArtifactFormatter func(excludedArtIDs map[string]bool, maxBytes int) string

// VerdictArtifactFormatter formats prefetched inline failure artifacts for a
// test verdict within maxBytes (0 for unlimited, <0 for empty).
type VerdictArtifactFormatter func(maxBytes int) string

// WorkUnitArtifactsInfo holds a work unit resource name and its direct artifacts.
type WorkUnitArtifactsInfo struct {
	Name      string
	Artifacts []*rdbpb.Artifact
	More      bool
}

// ParentInvocationNoticeInfo holds artifact count metadata for a legacy parent
// invocation (e.g. a Swarming task invocation).
type ParentInvocationNoticeInfo struct {
	LeafInvID string
	Count     int
	More      bool
}

// isNonPassingTestResult reports whether tr represents a failed, execution-errored,
// or precluded test execution (excluding PASSED and SKIPPED results).
func isNonPassingTestResult(tr *rdbpb.TestResult) bool {
	if tr == nil {
		// Avoid falling through to STATUS_UNSPECIFIED (!tr.GetExpected()) for nil.
		return false
	}
	switch tr.GetStatusV2() {
	case rdbpb.TestResult_PASSED, rdbpb.TestResult_SKIPPED:
		return false
	case rdbpb.TestResult_STATUS_UNSPECIFIED:
		return !tr.GetExpected() && tr.GetStatus() != rdbpb.TestStatus_PASS && tr.GetStatus() != rdbpb.TestStatus_SKIP
	default:
		return true
	}
}

// FitPrimaryAndInlineArtifacts combines primary command output with an optional
// inline artifact block (capped at DefaultInlineGetArtifactBudget). When
// capTotalOutput is true, the combined stdout is strictly bounded by
// DefaultMaxOutputBytes (9,500 B).
func FitPrimaryAndInlineArtifacts(primary string, formatInline func(maxBytes int) string, capTotalOutput bool) string {
	if primary != "" && !strings.HasSuffix(primary, "\n") {
		primary += "\n"
	}

	inline := ""
	if formatInline != nil {
		inline = trimTrailingExtraNewlines(formatInline(DefaultInlineGetArtifactBudget))
	}

	sep := "\n"
	if primary == "" || strings.HasSuffix(primary, "\n\n") {
		sep = ""
	}

	if !capTotalOutput {
		if inline == "" {
			return primary
		}
		return primary + sep + inline
	}

	if inline == "" {
		if len(primary) <= DefaultMaxOutputBytes {
			return primary
		}
		return ensureTrailingNewline(TruncateOutputBytes(primary, DefaultMaxOutputBytes-1, "pass -show-metadata to view full output"))
	}

	if len(primary)+len(sep)+len(inline) <= DefaultMaxOutputBytes {
		return primary + sep + inline
	}

	if remaining := DefaultMaxOutputBytes - len(primary) - len(sep); remaining >= minShrunkInlineBudget && formatInline != nil {
		if shrunk := trimTrailingExtraNewlines(formatInline(remaining)); shrunk != "" && len(primary)+len(sep)+len(shrunk) <= DefaultMaxOutputBytes {
			return primary + sep + shrunk
		}
	}

	maxPrimary := DefaultMaxOutputBytes - len(inline) - 2
	if maxPrimary <= 0 {
		return TruncateUTF8Prefix(inline, DefaultMaxOutputBytes)
	}
	truncPrimary := ensureTrailingNewline(TruncateOutputBytes(primary, maxPrimary, "pass -show-metadata to view full output"))
	truncSep := "\n"
	if strings.HasSuffix(truncPrimary, "\n\n") || len(truncPrimary)+1+len(inline) > DefaultMaxOutputBytes {
		truncSep = ""
	}
	return truncPrimary + truncSep + inline
}

func queryWorkUnitArtifactsConcurrently(ctx context.Context, client rdbpb.ResultDBClient, wuNames []string) []WorkUnitArtifactsInfo {
	if len(wuNames) == 0 {
		return nil
	}
	fetched := make([]WorkUnitArtifactsInfo, len(wuNames))
	var eg errgroup.Group
	eg.SetLimit(8)
	for idx, wuName := range wuNames {
		eg.Go(func() error {
			arts, more, err := QueryArtifactsForParent(ctx, client, wuName)
			if err == nil && len(arts) > 0 {
				fetched[idx] = WorkUnitArtifactsInfo{
					Name:      wuName,
					Artifacts: arts,
					More:      more,
				}
			}
			return nil
		})
	}
	_ = eg.Wait()

	withArtifacts := make([]WorkUnitArtifactsInfo, 0, len(wuNames))
	for _, info := range fetched {
		if len(info.Artifacts) > 0 {
			withArtifacts = append(withArtifacts, info)
		}
	}
	return withArtifacts
}

// FetchAncestorWorkUnitArtifacts queries parent/ancestor work units of target
// and returns those that have artifacts along with targetTestID and targetResultID.
func FetchAncestorWorkUnitArtifacts(ctx context.Context, client rdbpb.ResultDBClient, target string, ancestorOnly bool) ([]WorkUnitArtifactsInfo, string, string) {
	ctx = WithDiscoveryCache(ctx)
	var wuToCheck []string
	visited := make(map[string]bool)
	var targetTestID, targetResultID string

	if ancestorOnly {
		visited[target] = true
		for _, anc := range QueryAncestorWorkUnits(ctx, client, target) {
			if anc.GetName() != "" && !visited[anc.GetName()] {
				visited[anc.GetName()] = true
				wuToCheck = append(wuToCheck, anc.GetName())
			}
		}
	} else {
		idx := strings.Index(target, "/tests/")
		if idx == -1 {
			return nil, "", ""
		}
		_, targetTestID, targetResultID = base.ExtractTestResultComponents(target)
		parentWU := target[:idx]
		visited[parentWU] = true
		wuToCheck = append(wuToCheck, parentWU)

		for _, anc := range QueryAncestorWorkUnits(ctx, client, parentWU) {
			if anc.GetName() != "" && !visited[anc.GetName()] {
				visited[anc.GetName()] = true
				wuToCheck = append(wuToCheck, anc.GetName())
			}
		}
	}

	if len(wuToCheck) == 0 {
		return nil, targetTestID, targetResultID
	}

	return queryWorkUnitArtifactsConcurrently(ctx, client, wuToCheck), targetTestID, targetResultID
}

// FetchWorkUnitAndAncestorNoticeFormatterExcluding queries direct artifacts on
// up to MaxInspectedWorkUnits of targetWorkUnits (excluding IDs in excludedByWU)
// and artifacts on their unique ancestor work units, returning a budget-aware
// formatter and direct artifact IDs keyed by work unit resource name.
func FetchWorkUnitAndAncestorNoticeFormatterExcluding(ctx context.Context, client rdbpb.ResultDBClient, targetWorkUnits []string, excludedByWU map[string]map[string]bool) (func(maxBytes int) string, map[string][]string) {
	if client == nil || len(targetWorkUnits) == 0 {
		return nil, nil
	}
	ctx = WithDiscoveryCache(ctx)

	visited := make(map[string]bool, len(targetWorkUnits))
	var uniqueTargets []string
	for _, wuName := range targetWorkUnits {
		if wuName == "" || visited[wuName] {
			continue
		}
		visited[wuName] = true
		uniqueTargets = append(uniqueTargets, wuName)
	}
	if len(uniqueTargets) == 0 {
		return nil, nil
	}

	inspectedTargets := uniqueTargets
	omittedDirectWUs := 0
	if len(inspectedTargets) > MaxInspectedWorkUnits {
		omittedDirectWUs = len(inspectedTargets) - MaxInspectedWorkUnits
		inspectedTargets = inspectedTargets[:MaxInspectedWorkUnits]
	}

	rawDirect := queryWorkUnitArtifactsConcurrently(ctx, client, inspectedTargets)
	directWithArtifacts := make([]WorkUnitArtifactsInfo, 0, len(rawDirect))
	directArtifactIDs := make(map[string][]string, len(rawDirect))

	for _, info := range rawDirect {
		artIDs := make([]string, 0, len(info.Artifacts))
		for _, a := range info.Artifacts {
			if a.GetArtifactId() != "" {
				artIDs = append(artIDs, a.GetArtifactId())
			}
		}
		directArtifactIDs[info.Name] = artIDs

		arts := info.Artifacts
		if excluded := excludedByWU[info.Name]; len(excluded) > 0 {
			filtered := make([]*rdbpb.Artifact, 0, len(arts))
			for _, a := range arts {
				if !excluded[a.GetArtifactId()] {
					filtered = append(filtered, a)
				}
			}
			arts = filtered
		}
		if len(arts) > 0 || info.More {
			directWithArtifacts = append(directWithArtifacts, WorkUnitArtifactsInfo{
				Name:      info.Name,
				Artifacts: arts,
				More:      info.More,
			})
		}
	}

	var ancToCheck []string
	for _, wuName := range inspectedTargets {
		for _, anc := range QueryAncestorWorkUnits(ctx, client, wuName) {
			if anc.GetName() != "" && !visited[anc.GetName()] {
				visited[anc.GetName()] = true
				ancToCheck = append(ancToCheck, anc.GetName())
			}
		}
	}

	ancestorsWithArtifacts := queryWorkUnitArtifactsConcurrently(ctx, client, ancToCheck)

	if len(directWithArtifacts) == 0 && len(ancestorsWithArtifacts) == 0 {
		return nil, directArtifactIDs
	}

	renderDirect := func(budget int) string {
		return FormatAncestorWorkUnitNoticesWithOmitted(directWithArtifacts, omittedDirectWUs, "", "", false, budget)
	}
	renderAnc := func(budget int) string {
		return FormatAncestorWorkUnitNoticesWithOmitted(ancestorsWithArtifacts, 0, "", "", true, budget)
	}

	formatter := func(maxBytes int) string {
		if maxBytes < 0 {
			return ""
		}
		return combineDirectAndAncestorSections(renderDirect, renderAnc, maxBytes)
	}

	return formatter, directArtifactIDs
}

// FormatAncestorWorkUnitNoticesWithOmitted formats work-unit or ancestor
// work-unit artifact sections within maxBytes (0 for unlimited).
func FormatAncestorWorkUnitNoticesWithOmitted(withArtifacts []WorkUnitArtifactsInfo, extraOmittedWUs int, targetTestID, targetResultID string, ancestorOnly bool, maxBytes int) string {
	if len(withArtifacts) == 0 {
		return ""
	}

	sectionHeader := "Work Unit Artifacts:"
	fetchHeader := "To fetch a work unit artifact:"
	if ancestorOnly {
		sectionHeader = "Ancestor Work Unit Artifacts:"
		fetchHeader = "To fetch an ancestor work unit artifact:"
	}

	candidates := make([][]*rdbpb.Artifact, len(withArtifacts))
	canPrintAll := make([]bool, len(withArtifacts))
	counts := make([]int, len(withArtifacts))

	for i, info := range withArtifacts {
		if !info.More && len(info.Artifacts) <= maxInlineAncestorArtifacts {
			sorted := make([]*rdbpb.Artifact, len(info.Artifacts))
			copy(sorted, info.Artifacts)
			SortArtifacts(sorted)
			candidates[i] = sorted
			canPrintAll[i] = true
		} else {
			recommended := FilterRecommendedDebugArtifacts(info.Artifacts, targetTestID, targetResultID)
			if len(recommended) > maxInlineAncestorArtifacts {
				recommended = recommended[:maxInlineAncestorArtifacts]
			}
			candidates[i] = recommended
			canPrintAll[i] = false
		}
		counts[i] = len(candidates[i])
	}

	renderWithCounts := func(curCounts []int, shownWUs int) string {
		var b bytes.Buffer
		fmt.Fprintln(&b, sectionHeader)

		var inlineWUIDs []string
		var sampleInvID string
		var sampleArtifactName string
		recordInline := func(qInv, qWU, wuName string, art *rdbpb.Artifact) {
			if qWU != "" {
				inlineWUIDs = append(inlineWUIDs, qWU)
			}
			if sampleInvID == "" && qInv != "" {
				sampleInvID = qInv
			}
			if sampleArtifactName == "" && art != nil {
				artName := art.GetName()
				if artName == "" && wuName != "" && art.GetArtifactId() != "" {
					artName = wuName + "/artifacts/" + url.PathEscape(art.GetArtifactId())
				}
				sampleArtifactName = artName
			}
		}

		for i := range shownWUs {
			info := withArtifacts[i]
			invID, wuID := base.ExtractWorkUnitComponents(info.Name)
			quotedInv := ""
			if invID != "" {
				quotedInv = base.ShellQuoteIfNeeded(invID)
			}
			quotedWU := ""
			if wuID != "" {
				quotedWU = base.ShellQuoteIfNeeded(wuID)
			}
			wuLabel := "Root work unit"
			if wuID == "root" {
				wuLabel = "Root work unit (root)"
			} else if wuID != "" {
				wuLabel = fmt.Sprintf("Work unit %s", wuID)
			}
			allFlag := ""
			if info.More || len(info.Artifacts) > defaultMaxListedArtifacts {
				allFlag = " -all"
			}
			viewAllCmd := ""
			if quotedInv != "" && quotedWU != "" {
				viewAllCmd = fmt.Sprintf("luci artifact list -invocationid=%s -workunitid=%s%s [-filter=...]", quotedInv, quotedWU, allFlag)
			} else if quotedInv != "" {
				viewAllCmd = fmt.Sprintf("luci artifact list -invocationid=%s%s [-filter=...]", quotedInv, allFlag)
			}

			showCount := curCounts[i]
			totalStr := fmt.Sprintf("%d", len(info.Artifacts))
			if info.More {
				totalStr = fmt.Sprintf("%d+", len(info.Artifacts))
			}

			if canPrintAll[i] && showCount == len(info.Artifacts) {
				artLabel := "artifacts"
				if len(info.Artifacts) == 1 {
					artLabel = "artifact"
				}
				fmt.Fprintf(&b, "  %s (%d %s):\n", wuLabel, len(info.Artifacts), artLabel)
				PrintArtifactTableTo(&b, candidates[i], false)
				var firstArt *rdbpb.Artifact
				if len(candidates[i]) > 0 {
					firstArt = candidates[i][0]
				}
				recordInline(quotedInv, quotedWU, info.Name, firstArt)
			} else if !canPrintAll[i] && len(candidates[i]) == 0 {
				if viewAllCmd != "" {
					fmt.Fprintf(&b, "  %s (%s artifacts, none matching default diagnostic types); run '%s' to view all\n", wuLabel, totalStr, viewAllCmd)
				} else {
					fmt.Fprintf(&b, "  %s (%s artifacts, none matching default diagnostic types)\n", wuLabel, totalStr)
				}
			} else if showCount == 0 {
				artLabel := "artifacts"
				if !info.More && len(info.Artifacts) == 1 {
					artLabel = "artifact"
				}
				if viewAllCmd != "" {
					fmt.Fprintf(&b, "  %s (%s %s); run '%s' to view all\n", wuLabel, totalStr, artLabel, viewAllCmd)
				} else {
					fmt.Fprintf(&b, "  %s (%s %s)\n", wuLabel, totalStr, artLabel)
				}
			} else {
				fmt.Fprintf(&b, "  %s (showing %d of %s artifacts):\n", wuLabel, showCount, totalStr)
				PrintArtifactTableTo(&b, candidates[i][:showCount], false)
				recordInline(quotedInv, quotedWU, info.Name, candidates[i][0])
				inWUPart := ""
				if wuID != "" {
					inWUPart = fmt.Sprintf(" in work unit %s", wuID)
				}
				omitted := len(info.Artifacts) - showCount
				if omitted > 0 {
					omittedStr := fmt.Sprintf("%d", omitted)
					otherLabel := "artifacts"
					if info.More {
						omittedStr += "+"
					} else if omitted == 1 {
						otherLabel = "artifact"
					}
					if viewAllCmd != "" {
						fmt.Fprintf(&b, "  ... and %s other %s%s; run '%s' to view all\n", omittedStr, otherLabel, inWUPart, viewAllCmd)
					} else {
						fmt.Fprintf(&b, "  ... and %s other %s%s\n", omittedStr, otherLabel, inWUPart)
					}
				} else if info.More {
					if viewAllCmd != "" {
						fmt.Fprintf(&b, "  ... and more artifacts%s; run '%s' to view all\n", inWUPart, viewAllCmd)
					} else {
						fmt.Fprintf(&b, "  ... and more artifacts%s\n", inWUPart)
					}
				}
			}
			fmt.Fprintln(&b)
		}

		omittedWUs := (len(withArtifacts) - shownWUs) + extraOmittedWUs
		if omittedWUs > 0 {
			var invID string
			for _, w := range withArtifacts {
				if id, _ := base.ExtractWorkUnitComponents(w.Name); id != "" {
					invID = id
					break
				}
			}
			wuNoun := "work units"
			if omittedWUs == 1 {
				wuNoun = "work unit"
			}
			if invID != "" {
				quotedInv := base.ShellQuoteIfNeeded(invID)
				fmt.Fprintf(&b, "  ... and %d more %s with artifacts; run 'luci artifact list -invocationid=%s' to view all\n\n", omittedWUs, wuNoun, quotedInv)
			} else {
				fmt.Fprintf(&b, "  ... and %d more %s with artifacts\n\n", omittedWUs, wuNoun)
			}
		}

		if len(inlineWUIDs) > 0 && sampleInvID != "" {
			wuFlagVal := "<WORK_UNIT_ID>"
			if len(inlineWUIDs) == 1 {
				wuFlagVal = inlineWUIDs[0]
			}
			fmt.Fprintf(&b, "%s\n  luci artifact <get|head|tail> -invocationid=%s -workunitid=%s -artifactid=<ARTIFACT_ID>\n",
				fetchHeader, sampleInvID, wuFlagVal)
			if readyCmd := FormatReadyArtifactGetCommand(sampleArtifactName); readyCmd != "" {
				fmt.Fprintf(&b, "  e.g. %s\n", readyCmd)
			}
			fmt.Fprintln(&b)
		} else if hint := FormatArtifactFetchHint(sampleArtifactName, false); hint != "" {
			fmt.Fprintf(&b, "%s\n  %s\n", fetchHeader, hint)
			if readyCmd := FormatReadyArtifactGetCommand(sampleArtifactName); readyCmd != "" {
				fmt.Fprintf(&b, "  e.g. %s\n", readyCmd)
			}
			fmt.Fprintln(&b)
		}
		return b.String()
	}

	shownWUs := len(withArtifacts)
	outStr := renderWithCounts(counts, shownWUs)
	if maxBytes <= 0 || len(outStr) <= maxBytes {
		return outStr
	}

	// Pass 1: trim inline rows from farthest ancestor down to 1 row per ancestor.
	for i := len(counts) - 1; i >= 0 && len(outStr) > maxBytes; i-- {
		for counts[i] > 1 && len(outStr) > maxBytes {
			counts[i]--
			outStr = renderWithCounts(counts, shownWUs)
		}
	}
	// Pass 2: collapse ancestors to 1-line summary from farthest ancestor if still over budget.
	for i := len(counts) - 1; i >= 0 && len(outStr) > maxBytes; i-- {
		if counts[i] > 0 {
			counts[i] = 0
			outStr = renderWithCounts(counts, shownWUs)
		}
	}
	// Pass 3: cap the number of work units rendered and collapse remaining work units into a summary line.
	for shownWUs > 0 && len(outStr) > maxBytes {
		shownWUs--
		outStr = renderWithCounts(counts, shownWUs)
	}
	if len(outStr) > maxBytes {
		return ""
	}
	return outStr
}

// FormatTestResultInlineArtifacts fetches and formats direct test-result
// artifacts and parent/ancestor work-unit (or legacy invocation) artifacts
// within maxBytes (<= DefaultInlineGetArtifactBudget by default).
func FormatTestResultInlineArtifacts(ctx context.Context, client rdbpb.ResultDBClient, invID string, res *rdbpb.TestResult, excludedArtIDs map[string]bool, legacy bool, maxBytes int) string {
	formatter := FetchTestResultInlineArtifactFormatter(ctx, client, invID, res, legacy)
	if formatter == nil {
		return ""
	}
	return formatter(excludedArtIDs, maxBytes)
}

// FetchTestResultInlineArtifactFormatter concurrently fetches direct test-result
// artifacts and parent/ancestor work-unit (or legacy invocation) artifacts,
// returning a formatter that applies excludedArtIDs and maxBytes.
func FetchTestResultInlineArtifactFormatter(ctx context.Context, client rdbpb.ResultDBClient, invID string, res *rdbpb.TestResult, legacy bool) TestResultArtifactFormatter {
	if client == nil || res.GetName() == "" {
		return nil
	}
	ctx = WithDiscoveryCache(ctx)

	var (
		directArts     []*rdbpb.Artifact
		directMore     bool
		directErr      error
		ancInfos       []WorkUnitArtifactsInfo
		targetTestID   string
		targetResultID string
		legacyInfos    []ParentInvocationNoticeInfo
		wg             sync.WaitGroup
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		directArts, directMore, directErr = QueryArtifactsForParent(ctx, client, res.GetName())
	}()
	go func() {
		defer wg.Done()
		if legacy {
			if idx := strings.Index(res.GetName(), "/tests/"); idx > 0 {
				legacyInfos = FetchParentInvocationNotices(ctx, client, []string{res.GetName()[:idx]})
			}
		} else {
			ancInfos, targetTestID, targetResultID = FetchAncestorWorkUnitArtifacts(ctx, client, res.GetName(), false)
		}
	}()
	wg.Wait()

	if targetTestID == "" {
		targetTestID = res.GetTestId()
	}
	if targetResultID == "" {
		targetResultID = res.GetResultId()
	}

	return func(excludedArtIDs map[string]bool, maxBytes int) string {
		if maxBytes < 0 {
			return ""
		}

		var filteredDirect []*rdbpb.Artifact
		if directErr == nil && len(directArts) > 0 {
			filteredDirect = make([]*rdbpb.Artifact, 0, len(directArts))
			for _, a := range directArts {
				if len(excludedArtIDs) > 0 && excludedArtIDs[a.GetArtifactId()] {
					continue
				}
				filteredDirect = append(filteredDirect, a)
			}
		}

		renderAnc := func(budget int) string {
			if legacy {
				return FormatParentInvocationNotices(legacyInfos, budget)
			}
			return FormatAncestorWorkUnitNoticesWithOmitted(ancInfos, 0, targetTestID, targetResultID, false, budget)
		}

		renderDirect := func(budget int) string {
			if len(filteredDirect) == 0 && !directMore {
				return ""
			}
			return formatSingleResultDirectArtifacts("Test Result Artifacts", res, invID, filteredDirect, directMore, budget)
		}

		return combineDirectAndAncestorSections(renderDirect, renderAnc, maxBytes)
	}
}

// FormatVerdictInlineFailureArtifacts fetches and formats inline failure
// artifacts across non-passing attempts of a test verdict within maxBytes.
// Returns "" without making any RPCs when all results are PASSED or SKIPPED.
func FormatVerdictInlineFailureArtifacts(ctx context.Context, client rdbpb.ResultDBClient, invID string, results []*rdbpb.TestResult, legacy bool, maxBytes int) string {
	formatter := FetchVerdictInlineFailureArtifactFormatter(ctx, client, invID, results, legacy)
	if formatter == nil {
		return ""
	}
	return formatter(maxBytes)
}

type verdictAttemptArtifacts struct {
	res       *rdbpb.TestResult
	artifacts []*rdbpb.Artifact
	more      bool
}

// FetchVerdictInlineFailureArtifactFormatter concurrently fetches direct
// artifacts on up to maxVerdictFailedAttemptsForArtifacts non-passing test
// results and their parent/ancestor work units (or legacy invocations).
// Returns nil without making any RPCs when results has no non-passing attempts.
func FetchVerdictInlineFailureArtifactFormatter(ctx context.Context, client rdbpb.ResultDBClient, invID string, results []*rdbpb.TestResult, legacy bool) VerdictArtifactFormatter {
	if client == nil || len(results) == 0 {
		return nil
	}

	var failed []*rdbpb.TestResult
	for _, tr := range results {
		if isNonPassingTestResult(tr) && tr.GetName() != "" {
			failed = append(failed, tr)
		}
	}
	if len(failed) == 0 {
		return nil
	}

	ctx = WithDiscoveryCache(ctx)
	inspected := failed
	omittedAttempts := 0
	if len(inspected) > maxVerdictFailedAttemptsForArtifacts {
		omittedAttempts = len(inspected) - maxVerdictFailedAttemptsForArtifacts
		inspected = inspected[:maxVerdictFailedAttemptsForArtifacts]
	}

	fetchedDirect := make([]verdictAttemptArtifacts, len(inspected))
	var (
		ancInfos    []WorkUnitArtifactsInfo
		legacyInfos []ParentInvocationNoticeInfo
		wg          sync.WaitGroup
	)

	for i, tr := range inspected {
		wg.Add(1)
		go func(idx int, res *rdbpb.TestResult) {
			defer wg.Done()
			arts, more, err := QueryArtifactsForParent(ctx, client, res.GetName())
			if err == nil && (len(arts) > 0 || more) {
				fetchedDirect[idx] = verdictAttemptArtifacts{
					res:       res,
					artifacts: arts,
					more:      more,
				}
			}
		}(i, tr)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		if legacy {
			var parentInvs []string
			seenInv := make(map[string]bool)
			for _, tr := range inspected {
				if idx := strings.Index(tr.GetName(), "/tests/"); idx > 0 {
					parentInv := tr.GetName()[:idx]
					if !seenInv[parentInv] {
						seenInv[parentInv] = true
						parentInvs = append(parentInvs, parentInv)
					}
				}
			}
			legacyInfos = FetchParentInvocationNotices(ctx, client, parentInvs)
			return
		}
		var wuToCheck []string
		visited := make(map[string]bool)
		for _, tr := range inspected {
			idx := strings.Index(tr.GetName(), "/tests/")
			if idx == -1 {
				continue
			}
			parentWU := tr.GetName()[:idx]
			if !visited[parentWU] {
				visited[parentWU] = true
				wuToCheck = append(wuToCheck, parentWU)
			}
			for _, anc := range QueryAncestorWorkUnits(ctx, client, parentWU) {
				if anc.GetName() != "" && !visited[anc.GetName()] {
					visited[anc.GetName()] = true
					wuToCheck = append(wuToCheck, anc.GetName())
				}
			}
		}
		ancInfos = queryWorkUnitArtifactsConcurrently(ctx, client, wuToCheck)
	}()
	wg.Wait()

	var attemptsWithDirect []verdictAttemptArtifacts
	for _, item := range fetchedDirect {
		if len(item.artifacts) > 0 || item.more {
			attemptsWithDirect = append(attemptsWithDirect, item)
		}
	}
	if len(attemptsWithDirect) == 0 && len(ancInfos) == 0 && len(legacyInfos) == 0 {
		return nil
	}

	targetTestID := failed[0].GetTestId()
	targetResultID := ""
	if len(failed) == 1 {
		targetResultID = failed[0].GetResultId()
	}

	return func(maxBytes int) string {
		if maxBytes < 0 {
			return ""
		}

		renderAnc := func(budget int) string {
			if legacy {
				return FormatParentInvocationNotices(legacyInfos, budget)
			}
			return FormatAncestorWorkUnitNoticesWithOmitted(ancInfos, 0, targetTestID, targetResultID, false, budget)
		}

		renderDirect := func(budget int) string {
			if len(attemptsWithDirect) == 0 {
				return ""
			}
			if len(attemptsWithDirect) == 1 && omittedAttempts == 0 {
				only := attemptsWithDirect[0]
				title := "Test Result Artifacts"
				if only.res.GetResultId() != "" {
					title = fmt.Sprintf("Test Result Artifacts (result_id: %s)", only.res.GetResultId())
				}
				return formatSingleResultDirectArtifacts(title, only.res, invID, only.artifacts, only.more, budget)
			}
			return formatMultiAttemptDirectArtifacts(invID, attemptsWithDirect, len(failed), omittedAttempts, legacy, budget)
		}

		return combineDirectAndAncestorSections(renderDirect, renderAnc, maxBytes)
	}
}

func combineDirectAndAncestorSections(renderDirect, renderAnc func(budget int) string, maxBytes int) string {
	ancFull := renderAnc(0)
	directCheck := renderDirect(0)
	if directCheck == "" && ancFull == "" {
		return ""
	}
	if directCheck == "" {
		if maxBytes == 0 || len(ancFull) <= maxBytes {
			return ancFull
		}
		return renderAnc(maxBytes)
	}
	if ancFull == "" {
		if maxBytes == 0 || len(directCheck) <= maxBytes {
			return directCheck
		}
		return renderDirect(maxBytes)
	}
	if maxBytes == 0 || len(directCheck)+len(ancFull) <= maxBytes {
		return directCheck + ancFull
	}

	ancReserve := min(len(ancFull), maxBytes/2)
	directBudget := max(maxBytes-ancReserve, 1)
	directOut := renderDirect(directBudget)

	remaining := maxBytes - len(directOut)
	ancOut := ""
	if remaining > 0 {
		if len(ancFull) <= remaining {
			ancOut = ancFull
		} else {
			ancOut = renderAnc(remaining)
		}
	}
	if ancOut == "" && directBudget < maxBytes {
		directOut = renderDirect(maxBytes)
	}
	return directOut + ancOut
}

func selectDirectCandidates(artifacts []*rdbpb.Artifact, hasMore bool, testID, resultID string) (candidates []*rdbpb.Artifact, canPrintAll bool) {
	if !hasMore && len(artifacts) <= maxInlineAncestorArtifacts {
		sorted := make([]*rdbpb.Artifact, len(artifacts))
		copy(sorted, artifacts)
		SortArtifacts(sorted)
		return sorted, true
	}
	recommended := FilterRecommendedDebugArtifacts(artifacts, testID, resultID)
	if len(recommended) == 0 {
		sorted := make([]*rdbpb.Artifact, len(artifacts))
		copy(sorted, artifacts)
		SortArtifacts(sorted)
		recommended = sorted
	}
	if len(recommended) > maxInlineAncestorArtifacts {
		recommended = recommended[:maxInlineAncestorArtifacts]
	}
	return recommended, false
}

func formatSingleResultDirectArtifacts(titlePrefix string, res *rdbpb.TestResult, invID string, artifacts []*rdbpb.Artifact, hasMore bool, maxBytes int) string {
	if len(artifacts) == 0 && !hasMore {
		return ""
	}
	parentName := res.GetName()
	meta := base.ExtractArtifactMetadata(parentName)
	listInv := meta.InvocationID
	if listInv == "" {
		listInv = base.NormalizeInvocation(invID)
	}
	testID := res.GetTestId()
	if testID == "" {
		testID = meta.TestID
	}
	resultID := res.GetResultId()
	if resultID == "" {
		resultID = meta.ResultID
	}

	wuFlag := ""
	if meta.WorkUnitID != "" {
		wuFlag = " -workunitid=" + base.ShellQuoteIfNeeded(meta.WorkUnitID)
	}
	allFlag := ""
	if hasMore || len(artifacts) > defaultMaxListedArtifacts {
		allFlag = " -all"
	}
	viewAllCmd := ""
	if listInv != "" && testID != "" && resultID != "" {
		viewAllCmd = fmt.Sprintf("luci test-result artifact list -invocationid=%s%s -testid=%s -resultid=%s%s [-filter=...]",
			base.ShellQuoteIfNeeded(listInv),
			wuFlag,
			base.ShellQuoteIfNeeded(testID),
			base.ShellQuoteIfNeeded(resultID),
			allFlag)
	} else if listInv != "" && meta.WorkUnitID != "" {
		viewAllCmd = fmt.Sprintf("luci work-unit artifact list -invocationid=%s%s%s [-filter=...]",
			base.ShellQuoteIfNeeded(listInv),
			wuFlag,
			allFlag)
	} else if listInv != "" {
		viewAllCmd = fmt.Sprintf("luci artifact list -invocationid=%s%s [-filter=...]",
			base.ShellQuoteIfNeeded(listInv),
			allFlag)
	}

	candidates, canPrintAll := selectDirectCandidates(artifacts, hasMore, testID, resultID)
	totalCount := len(artifacts)
	totalStr := fmt.Sprintf("%d", totalCount)
	if hasMore {
		totalStr += "+"
	}

	headerWithCount := func(detail string) string {
		if strings.HasSuffix(titlePrefix, ")") {
			return strings.TrimSuffix(titlePrefix, ")") + ", " + detail + "):"
		}
		return fmt.Sprintf("%s (%s):", titlePrefix, detail)
	}

	build := func(k int) string {
		var b bytes.Buffer
		if k == 0 {
			artNoun := "artifacts"
			if !hasMore && totalCount == 1 {
				artNoun = "artifact"
			}
			if viewAllCmd != "" {
				fmt.Fprintf(&b, "%s\n  (view all: %s)\n\n", headerWithCount(fmt.Sprintf("%s %s", totalStr, artNoun)), viewAllCmd)
			} else {
				fmt.Fprintf(&b, "%s\n\n", headerWithCount(fmt.Sprintf("%s %s", totalStr, artNoun)))
			}
			return b.String()
		}

		if canPrintAll && k == totalCount {
			artNoun := "artifacts"
			if totalCount == 1 {
				artNoun = "artifact"
			}
			fmt.Fprintln(&b, headerWithCount(fmt.Sprintf("%d %s", totalCount, artNoun)))
		} else {
			fmt.Fprintln(&b, headerWithCount(fmt.Sprintf("showing %d of %s artifacts", k, totalStr)))
		}

		PrintArtifactTableTo(&b, candidates[:k], false)

		omitted := totalCount - k
		if omitted > 0 {
			omittedStr := fmt.Sprintf("%d", omitted)
			otherNoun := "artifacts"
			if hasMore {
				omittedStr += "+"
			} else if omitted == 1 {
				otherNoun = "artifact"
			}
			if viewAllCmd != "" {
				fmt.Fprintf(&b, "  ... and %s other %s (view all: %s)\n", omittedStr, otherNoun, viewAllCmd)
			} else {
				fmt.Fprintf(&b, "  ... and %s other %s\n", omittedStr, otherNoun)
			}
		} else if hasMore {
			if viewAllCmd != "" {
				fmt.Fprintf(&b, "  ... and more artifacts (view all: %s)\n", viewAllCmd)
			} else {
				fmt.Fprintf(&b, "  ... and more artifacts\n")
			}
		}

		sampleName := candidates[0].GetName()
		if sampleName == "" && parentName != "" && candidates[0].GetArtifactId() != "" {
			sampleName = parentName + "/artifacts/" + url.PathEscape(candidates[0].GetArtifactId())
		}
		if hint := FormatArtifactFetchHint(sampleName, false); hint != "" {
			fmt.Fprintf(&b, "\nTo fetch a test result artifact:\n  %s\n", hint)
			if readyCmd := FormatReadyArtifactGetCommand(sampleName); readyCmd != "" {
				fmt.Fprintf(&b, "  e.g. %s\n", readyCmd)
			}
		}
		fmt.Fprintln(&b)
		return b.String()
	}

	return fitArtifactSectionToBudget(len(candidates), maxBytes, build)
}

func formatMultiAttemptDirectArtifacts(invID string, attempts []verdictAttemptArtifacts, totalFailedRuns, omittedAttempts int, legacy bool, maxBytes int) string {
	var (
		allNamed   []*rdbpb.Artifact
		totalCount int
		anyMore    bool
		sampleName string
		testID     string
	)
	for _, item := range attempts {
		if testID == "" {
			testID = item.res.GetTestId()
		}
		totalCount += len(item.artifacts)
		if item.more {
			anyMore = true
		}
		for _, art := range item.artifacts {
			named := ensureArtifactResourceName(art, item.res.GetName())
			if sampleName == "" {
				sampleName = named.GetName()
			}
			allNamed = append(allNamed, named)
		}
	}
	if len(allNamed) == 0 && !anyMore {
		return ""
	}

	candidates, canPrintAll := selectDirectCandidates(allNamed, anyMore || omittedAttempts > 0, testID, "")
	normInv := base.NormalizeInvocation(invID)
	if normInv == "" && sampleName != "" {
		normInv = base.ExtractArtifactMetadata(sampleName).InvocationID
	}
	quotedInv := ""
	if normInv != "" {
		quotedInv = base.ShellQuoteIfNeeded(normInv)
	}
	allFlag := ""
	if anyMore || totalCount > defaultMaxListedArtifacts {
		allFlag = " -all"
	}
	var viewAllCmd string
	if quotedInv != "" {
		if legacy && testID != "" {
			viewAllCmd = fmt.Sprintf("luci test-result artifact list -invocationid=%s -testid=%s -resultid=<RESULT_ID>%s [-filter=...]",
				quotedInv, base.ShellQuoteIfNeeded(testID), allFlag)
		} else {
			viewAllCmd = fmt.Sprintf("luci artifact list -invocationid=%s%s [-filter=...]", quotedInv, allFlag)
		}
	}

	totalStr := fmt.Sprintf("%d", totalCount)
	if anyMore || omittedAttempts > 0 {
		totalStr += "+"
	}
	runsCount := max(totalFailedRuns, len(attempts)+omittedAttempts)

	build := func(k int) string {
		var b bytes.Buffer
		if k == 0 {
			if viewAllCmd != "" {
				fmt.Fprintf(&b, "Test Result Artifacts (across %d failed runs, %s artifacts):\n  (view all: %s)\n\n", runsCount, totalStr, viewAllCmd)
			} else {
				fmt.Fprintf(&b, "Test Result Artifacts (across %d failed runs, %s artifacts):\n\n", runsCount, totalStr)
			}
			return b.String()
		}
		if canPrintAll && k == totalCount && omittedAttempts == 0 {
			fmt.Fprintf(&b, "Test Result Artifacts (across %d failed runs, %d artifacts):\n", runsCount, totalCount)
		} else {
			fmt.Fprintf(&b, "Test Result Artifacts (across %d failed runs, showing %d of %s artifacts):\n", runsCount, k, totalStr)
		}
		printArtifactTable(&b, candidates[:k], true, legacy)

		omitted := totalCount - k
		if omitted > 0 {
			omittedStr := fmt.Sprintf("%d", omitted)
			otherNoun := "artifacts"
			if anyMore || omittedAttempts > 0 {
				omittedStr += "+"
			} else if omitted == 1 {
				otherNoun = "artifact"
			}
			if viewAllCmd != "" {
				fmt.Fprintf(&b, "  ... and %s other %s (view all: %s)\n", omittedStr, otherNoun, viewAllCmd)
			} else {
				fmt.Fprintf(&b, "  ... and %s other %s\n", omittedStr, otherNoun)
			}
		} else if anyMore || omittedAttempts > 0 {
			if viewAllCmd != "" {
				fmt.Fprintf(&b, "  ... and more artifacts across failed runs (view all: %s)\n", viewAllCmd)
			} else {
				fmt.Fprintf(&b, "  ... and more artifacts across failed runs\n")
			}
		}

		firstDisplayedName := candidates[0].GetName()
		hint := ""
		if legacy {
			if quotedInv != "" || base.ExtractArtifactMetadata(firstDisplayedName).InvocationID != "" {
				hint = "luci test-result artifact <get|head|tail> -artifactid=<ARTIFACT_ID> [FLAGS]"
			}
		} else if quotedInv != "" {
			hint = fmt.Sprintf("luci artifact <get|head|tail> -invocationid=%s -artifactid=<ARTIFACT_ID> [FLAGS]", quotedInv)
		} else {
			hint = FormatArtifactFetchHint(firstDisplayedName, true)
		}
		if hint != "" {
			fmt.Fprintf(&b, "\nTo fetch a test result artifact:\n  %s\n", hint)
			if readyCmd := FormatReadyArtifactGetCommand(firstDisplayedName); readyCmd != "" {
				fmt.Fprintf(&b, "  e.g. %s\n", readyCmd)
			}
		}
		fmt.Fprintln(&b)
		return b.String()
	}

	return fitArtifactSectionToBudget(len(candidates), maxBytes, build)
}

func fitArtifactSectionToBudget(candidateCount, maxBytes int, build func(k int) string) string {
	full := build(candidateCount)
	if maxBytes <= 0 || len(full) <= maxBytes {
		return full
	}
	lo, hi := 1, candidateCount-1
	best := ""
	for lo <= hi {
		mid := (lo + hi) / 2
		cand := build(mid)
		if len(cand) <= maxBytes {
			best = cand
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}
	if best != "" {
		return best
	}
	summary := build(0)
	if len(summary) <= maxBytes {
		return summary
	}
	return ""
}

func ensureArtifactResourceName(art *rdbpb.Artifact, parentResName string) *rdbpb.Artifact {
	if art.GetName() != "" || parentResName == "" {
		return art
	}
	return &rdbpb.Artifact{
		Name:         parentResName + "/artifacts/" + url.PathEscape(art.GetArtifactId()),
		ArtifactId:   art.GetArtifactId(),
		ContentType:  art.GetContentType(),
		ArtifactType: art.GetArtifactType(),
		SizeBytes:    art.GetSizeBytes(),
		HasLines:     art.GetHasLines(),
	}
}

// FetchParentInvocationNotices concurrently queries artifacts on up to len(parentInvNames)
// legacy parent invocations (using DiscoveryCache) and returns those with artifacts.
func FetchParentInvocationNotices(ctx context.Context, client rdbpb.ResultDBClient, parentInvNames []string) []ParentInvocationNoticeInfo {
	if client == nil || len(parentInvNames) == 0 {
		return nil
	}
	ctx = WithDiscoveryCache(ctx)

	seen := make(map[string]bool, len(parentInvNames))
	var unique []string
	for _, name := range parentInvNames {
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		unique = append(unique, name)
	}
	if len(unique) == 0 {
		return nil
	}

	fetched := make([]ParentInvocationNoticeInfo, len(unique))
	var eg errgroup.Group
	eg.SetLimit(8)
	for idx, parentInv := range unique {
		eg.Go(func() error {
			arts, more, err := QueryArtifactsForParent(ctx, client, parentInv)
			if err == nil && len(arts) > 0 {
				fetched[idx] = ParentInvocationNoticeInfo{
					LeafInvID: strings.TrimPrefix(parentInv, "invocations/"),
					Count:     len(arts),
					More:      more,
				}
			}
			return nil
		})
	}
	_ = eg.Wait()

	out := make([]ParentInvocationNoticeInfo, 0, len(unique))
	for _, info := range fetched {
		if info.LeafInvID != "" && info.Count > 0 {
			out = append(out, info)
		}
	}
	return out
}

// FormatParentInvocationNotices formats legacy parent invocation artifact notices
// under a single "Invocation Artifacts:" header within maxBytes (0 for unlimited).
func FormatParentInvocationNotices(infos []ParentInvocationNoticeInfo, maxBytes int) string {
	if len(infos) == 0 || maxBytes < 0 {
		return ""
	}

	bullets := make([]string, 0, len(infos))
	for _, info := range infos {
		if info.LeafInvID == "" || info.Count <= 0 {
			continue
		}
		artLabel := "artifacts"
		countStr := fmt.Sprintf("%d", info.Count)
		if info.More {
			countStr = fmt.Sprintf("%d+", info.Count)
		} else if info.Count == 1 {
			artLabel = "artifact"
		}
		quotedID := base.ShellQuoteIfNeeded(info.LeafInvID)
		bullets = append(bullets, fmt.Sprintf("  - Invocation %s contains %s %s. Run 'luci artifact list -invocationid %s' to view.\n",
			info.LeafInvID, countStr, artLabel, quotedID))
	}
	if len(bullets) == 0 {
		return ""
	}

	const header = "Invocation Artifacts:\n"
	full := header + strings.Join(bullets, "") + "\n"
	if maxBytes == 0 || len(full) <= maxBytes {
		return full
	}

	for k := len(bullets) - 1; k >= 1; k-- {
		omitted := len(bullets) - k
		invNoun := "invocations"
		if omitted == 1 {
			invNoun = "invocation"
		}
		footer := fmt.Sprintf("  ... and %d more %s with artifacts\n", omitted, invNoun)
		cand := header + strings.Join(bullets[:k], "") + footer + "\n"
		if len(cand) <= maxBytes {
			return cand
		}
	}

	single := header + bullets[0] + "\n"
	if len(single) <= maxBytes {
		return single
	}
	return ""
}

// PrintImmediateParentInvocationNotice checks the immediate parent invocation of
// a legacy test-result target (before "/tests/") and writes its artifact notice.
func PrintImmediateParentInvocationNotice(ctx context.Context, client rdbpb.ResultDBClient, target string, out io.Writer) {
	if client == nil || out == nil {
		return
	}
	idx := strings.Index(target, "/tests/")
	if idx <= 0 {
		return
	}
	infos := FetchParentInvocationNotices(ctx, client, []string{target[:idx]})
	if notice := FormatParentInvocationNotices(infos, 0); notice != "" {
		fmt.Fprint(out, notice)
	}
}

func trimTrailingExtraNewlines(s string) string {
	if s == "" {
		return ""
	}
	return strings.TrimRight(s, "\n") + "\n"
}

func ensureTrailingNewline(s string) string {
	if s == "" || strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}
