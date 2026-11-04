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

package artifact

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/maruel/subcommands"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/client/cmd/luci/ids"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

// maxInlineAncestorArtifacts is the maximum number of artifacts printed inline
// for each ancestor work unit in PrintAncestorWorkUnitNotices.
const maxInlineAncestorArtifacts = 15

// DefaultMaxOutputBytes is the default overall stdout byte cap (9.5 KB / 9,500 B)
// for `luci artifact list` responses so output stays within Jetski's 10 KB
// (9,728 B usable stdout) tool truncation threshold without middle-truncation.
const DefaultMaxOutputBytes = 9500

func ListCmd(af *base.AuthFlags, parentType ParentType) *subcommands.Command {
	var usage, desc, longDesc string
	switch parentType {
	case ParentTypeTestResult:
		usage = "list [-invocationid <invocation_id> -testid <test_id> -resultid <result_id>] [<target>]"
		desc = "List artifacts for a test result"
		longDesc = desc + " in ResultDB by a positional URL/ID or explicit ID flags.\n\n" +
			"Ancestor work units are also checked and a notice is displayed if artifacts exist."
	case ParentTypeWorkUnit:
		usage = "list [-invocationid <invocation_id> -workunitid <work_unit_id>] [<target>]"
		desc = "List artifacts for a work unit"
		longDesc = desc + " in ResultDB by a positional URL/ID or explicit ID flags.\n\n" +
			"Ancestor work units are also checked and a notice is displayed if artifacts exist."
	default:
		usage = "list [-invocationid <invocation_id>] [-workunitid <work_unit_id> | -testid <test_id> -resultid <result_id>] [<target>]"
		desc = "List artifacts for a work unit or test result"
		longDesc = desc + " in ResultDB by a positional URL/ID or explicit ID flags.\n\n" +
			"Specify -invocationid to list invocation-level artifacts,\n" +
			"-invocationid and -workunitid for work unit artifacts,\n" +
			"or -invocationid, -testid, and -resultid for test result artifacts.\n\n" +
			"Ancestor work units or parent invocations are also checked and a notice is displayed if artifacts exist."
	}

	return &subcommands.Command{
		UsageLine: usage,
		ShortDesc: desc,
		LongDesc:  longDesc,
		CommandRun: func() subcommands.CommandRun {
			r := &artifactListRun{}
			r.registerTargetFlags(af, parentType)
			r.Flags.StringVar(&r.filter, "filter", "", "Filter artifacts by case-insensitive regular expression or substring (RE2)")
			r.Flags.StringVar(&r.glob, "glob", "", "Filter artifacts by case-insensitive shell glob pattern (e.g. *-logcat-*.txt)")
			r.Flags.IntVar(&r.maxArtifacts, "max-artifacts", DefaultMaxArtifacts, "Maximum number of artifacts to display (default: 100, 0 for all)")
			r.Flags.BoolVar(&r.allArtifacts, "all", false, "Show all artifacts without truncation")
			return r
		},
	}
}

type artifactListRun struct {
	artifactCommonRun
	filter       string
	glob         string
	maxArtifacts int
	allArtifacts bool
}

func (r *artifactListRun) validateListParentType() (ParentType, bool) {
	effectiveParentType := r.parentType
	if effectiveParentType == ParentTypeUnknown {
		var err error
		effectiveParentType, err = ResolveParentType(r.invocationID, r.workUnitID, r.testID, r.resultID)
		if err != nil {
			ids.PrintMissingFlagsError(os.Stderr, r.extracted, err.Error())
			return ParentTypeUnknown, false
		}
	} else if effectiveParentType == ParentTypeTestResult {
		if r.invocationID == "" || r.testID == "" || r.resultID == "" {
			ids.PrintMissingFlagsError(os.Stderr, r.extracted, "flags -invocationid, -testid, and -resultid (or a positional URL/ID) are required")
			return ParentTypeUnknown, false
		}
	} else if effectiveParentType == ParentTypeWorkUnit {
		if r.invocationID == "" || r.workUnitID == "" {
			ids.PrintMissingFlagsError(os.Stderr, r.extracted, "flags -invocationid and -workunitid (or a positional URL/ID) are required")
			return ParentTypeUnknown, false
		}
	} else if effectiveParentType == ParentTypeInvocation {
		if r.invocationID == "" {
			ids.PrintMissingFlagsError(os.Stderr, r.extracted, "flag -invocationid (or a positional URL/ID) are required")
			return ParentTypeUnknown, false
		}
	}
	return effectiveParentType, true
}

func (r *artifactListRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if handled, exitCode := r.checkHelpAndPositionalArgs(args); handled {
		return exitCode
	}
	if r.maxArtifacts < 0 {
		fmt.Fprintf(os.Stderr, "flag -max-artifacts must be non-negative\n")
		return 1
	}
	serverRegexp, filterRe, err := CompileArtifactMatcher(r.filter, r.glob)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	ctx := cli.GetContext(a, r, env)
	ctx = format.WithDiscoveryCache(ctx)
	if !r.resolvePositionalTarget(ctx) {
		return 1
	}
	effectiveParentType, ok := r.validateListParentType()
	if !ok {
		return 1
	}
	if !r.ensureClients(ctx, false) {
		return 1
	}
	client := r.client

	resolved, err := base.ResolveInvocation(ctx, client, r.invocationID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list artifacts: %s\n", err)
		return 1
	}
	format.CacheResolvedInvocation(ctx, resolved)
	if r.extracted != nil {
		r.extracted.Legacy = resolved.Legacy
	}
	if resolved.Legacy && effectiveParentType == ParentTypeWorkUnit {
		fmt.Fprintf(os.Stderr, "work unit artifacts are only supported for root invocations (invocation %q is a legacy invocation)\n", resolved.ID)
		return 1
	}

	cleanTarget, err := ResolveTargetResourceName(ctx, client, effectiveParentType, resolved.ID, r.workUnitID, r.testID, r.resultID, resolved.Legacy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	maxArtifacts := r.maxArtifacts
	maxOutputBytes := DefaultMaxOutputBytes
	if r.allArtifacts {
		maxArtifacts = 0
		maxOutputBytes = 0
	}

	out := r.out
	if out == nil {
		out = os.Stdout
	}

	if effectiveParentType == ParentTypeTestResult {
		trArtifacts, err := queryParentArtifacts(ctx, client, cleanTarget, serverRegexp)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to list artifacts for test result %q: %s\n", cleanTarget, err)
			return 1
		}
		trArtifacts = FilterArtifacts(trArtifacts, filterRe)
		format.SortArtifacts(trArtifacts)

		r.extracted.EnrichFromArtifactName(cleanTarget, "")
		var derivedBuf bytes.Buffer
		ids.PrintDerivedIDs(&derivedBuf, r.extracted)
		fmt.Fprint(out, derivedBuf.String())
		if maxOutputBytes > 0 {
			maxOutputBytes = max(maxOutputBytes-derivedBuf.Len(), 0)
		}

		totalCount := len(trArtifacts)
		displayed := trArtifacts
		if maxArtifacts > 0 && len(displayed) > maxArtifacts {
			displayed = displayed[:maxArtifacts]
		}

		if resolved.Legacy {
			var parentBuf bytes.Buffer
			PrintImmediateParentInvocationNotice(ctx, client, cleanTarget, &parentBuf)
			primaryBudget := maxOutputBytes
			if primaryBudget > 0 && parentBuf.Len() > 0 {
				primaryBudget = max(primaryBudget-parentBuf.Len(), 0)
			}
			fmt.Fprint(out, renderPrimaryArtifactSection("Test Result", format.FormatTestResultBreadcrumb(cleanTarget), cleanTarget, displayed, totalCount, false, maxArtifacts, false, primaryBudget))
			fmt.Fprint(out, parentBuf.String())
		} else {
			writePrimaryAndAncestorNotices(ctx, client, "Test Result", format.FormatTestResultBreadcrumb(cleanTarget), cleanTarget, displayed, totalCount, maxArtifacts, effectiveParentType, maxOutputBytes, out)
		}
		return 0
	}

	if effectiveParentType == ParentTypeInvocation {
		if resolved.Legacy {
			invArtifacts, err := queryParentArtifacts(ctx, client, cleanTarget, serverRegexp)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to list artifacts for invocation %q: %s\n", cleanTarget, err)
				return 1
			}
			invArtifacts = FilterArtifacts(invArtifacts, filterRe)
			format.SortArtifacts(invArtifacts)

			var derivedBuf bytes.Buffer
			ids.PrintDerivedIDs(&derivedBuf, r.extracted)
			fmt.Fprint(out, derivedBuf.String())
			if maxOutputBytes > 0 {
				maxOutputBytes = max(maxOutputBytes-derivedBuf.Len(), 0)
			}

			totalCount := len(invArtifacts)
			displayed := invArtifacts
			if maxArtifacts > 0 && len(displayed) > maxArtifacts {
				displayed = displayed[:maxArtifacts]
			}
			fmt.Fprint(out, renderPrimaryArtifactSection("Invocation", format.FormatInvocationBreadcrumb(cleanTarget), cleanTarget, displayed, totalCount, false, maxArtifacts, false, maxOutputBytes))
			return 0
		}

		invArtifacts, totalFetched, hasMorePages, err := queryInvocationArtifactsSorted(ctx, client, cleanTarget, serverRegexp, maxArtifacts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to list artifacts for invocation %q: %s\n", cleanTarget, err)
			return 1
		}

		var derivedBuf bytes.Buffer
		ids.PrintDerivedIDs(&derivedBuf, r.extracted)
		fmt.Fprint(out, derivedBuf.String())
		if maxOutputBytes > 0 {
			maxOutputBytes = max(maxOutputBytes-derivedBuf.Len(), 0)
		}

		fmt.Fprint(out, renderPrimaryArtifactSection("Invocation", format.FormatInvocationBreadcrumb(cleanTarget), cleanTarget, invArtifacts, totalFetched, hasMorePages, maxArtifacts, true, maxOutputBytes))
		return 0
	}

	// Work unit
	wuArtifacts, err := queryParentArtifacts(ctx, client, cleanTarget, serverRegexp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list artifacts for work unit %q: %s\n", cleanTarget, err)
		return 1
	}
	wuArtifacts = FilterArtifacts(wuArtifacts, filterRe)
	format.SortArtifacts(wuArtifacts)

	var derivedBuf bytes.Buffer
	ids.PrintDerivedIDs(&derivedBuf, r.extracted)
	fmt.Fprint(out, derivedBuf.String())
	if maxOutputBytes > 0 {
		maxOutputBytes = max(maxOutputBytes-derivedBuf.Len(), 0)
	}

	totalCount := len(wuArtifacts)
	displayed := wuArtifacts
	if maxArtifacts > 0 && len(displayed) > maxArtifacts {
		displayed = displayed[:maxArtifacts]
	}

	writePrimaryAndAncestorNotices(ctx, client, "Work Unit", format.FormatWorkUnitBreadcrumb(cleanTarget), cleanTarget, displayed, totalCount, maxArtifacts, effectiveParentType, maxOutputBytes, out)
	return 0
}

func writePrimaryAndAncestorNotices(ctx context.Context, client rdbpb.ResultDBClient, category, breadcrumb, cleanTarget string, artifacts []*rdbpb.Artifact, totalCount, maxArtifacts int, parentType ParentType, totalBudget int, out io.Writer) {
	withArtifacts, targetTestID, targetResultID := fetchAncestorWorkUnitArtifacts(ctx, client, cleanTarget, parentType)
	ancFull := formatAncestorWorkUnitNotices(withArtifacts, targetTestID, targetResultID, parentType, 0)

	primaryBudget := totalBudget
	if totalBudget > 0 && len(ancFull) > 0 {
		ancReserve := min(len(ancFull), totalBudget/2)
		primaryBudget = max(totalBudget-ancReserve, 0)
	}

	primaryOut := renderPrimaryArtifactSection(category, breadcrumb, cleanTarget, artifacts, totalCount, false, maxArtifacts, false, primaryBudget)
	fmt.Fprint(out, primaryOut)

	if len(ancFull) == 0 {
		return
	}
	ancOut := ancFull
	if totalBudget > 0 {
		remaining := max(totalBudget-len(primaryOut), 0)
		if len(ancOut) > remaining {
			ancOut = formatAncestorWorkUnitNotices(withArtifacts, targetTestID, targetResultID, parentType, remaining)
		}
	}
	fmt.Fprint(out, ancOut)
}

func formatOmittedArtifactsLine(out io.Writer, cleanTarget string, omitted int, hasMore bool) {
	meta := base.ExtractArtifactMetadata(cleanTarget)
	if meta.InvocationID == "" {
		return
	}
	quotedInv := base.ShellQuoteIfNeeded(meta.InvocationID)
	flags := meta.FormatFlags()
	if flags != "" {
		flags = " " + flags
	}
	viewAllCmd := fmt.Sprintf("luci artifact list -invocationid=%s%s -all [-filter=...]", quotedInv, flags)
	inWUPart := ""
	if meta.WorkUnitID != "" && meta.TestID == "" {
		inWUPart = fmt.Sprintf(" in work unit %s", meta.WorkUnitID)
	}
	if omitted > 0 {
		omittedStr := fmt.Sprintf("%d", omitted)
		otherLabel := "artifacts"
		if hasMore {
			omittedStr += "+"
		} else if omitted == 1 {
			otherLabel = "artifact"
		}
		fmt.Fprintf(out, "  ... and %s other %s%s; run '%s' to view all\n", omittedStr, otherLabel, inWUPart, viewAllCmd)
	} else if hasMore {
		fmt.Fprintf(out, "  ... and more artifacts%s; run '%s' to view all\n", inWUPart, viewAllCmd)
	}
}

func renderPrimaryArtifactSection(category, breadcrumb, cleanTarget string, artifacts []*rdbpb.Artifact, totalCount int, hasMore bool, maxArtifacts int, includeMetadata bool, maxBytes int) string {
	if len(artifacts) == 0 {
		var buf bytes.Buffer
		if includeMetadata {
			printInvocationArtifactHeader(&buf, breadcrumb, 0, hasMore)
		} else {
			printArtifactHeaderTo(&buf, category, breadcrumb, 0, maxArtifacts)
		}
		fmt.Fprintf(&buf, "  (no artifacts)\n\n")
		return buf.String()
	}

	build := func(k int) string {
		var b bytes.Buffer
		truncated := k < totalCount || hasMore
		if includeMetadata {
			if truncated && !hasMore && totalCount > k {
				fmt.Fprintf(&b, "Invocation Artifacts (%s) (showing %d of %d, use -all to see all):\n", breadcrumb, k, totalCount)
			} else {
				printInvocationArtifactHeader(&b, breadcrumb, k, truncated)
			}
		} else {
			if truncated {
				fmt.Fprintf(&b, "%s Artifacts (%s) (showing %d of %d, use -all to see all):\n", category, breadcrumb, k, totalCount)
			} else {
				fmt.Fprintf(&b, "%s Artifacts (%s) (%d):\n", category, breadcrumb, totalCount)
			}
		}
		if k > 0 {
			format.PrintArtifactTableTo(&b, artifacts[:k], includeMetadata)
		}
		if truncated {
			formatOmittedArtifactsLine(&b, cleanTarget, totalCount-k, hasMore)
		}
		fmt.Fprintln(&b)
		if k > 0 {
			if hint := format.FormatArtifactFetchHint(artifacts[0].GetName(), includeMetadata); hint != "" {
				fmt.Fprintf(&b, "To fetch an artifact:\n  %s\n\n", hint)
			}
		}
		return b.String()
	}

	full := build(len(artifacts))
	if maxBytes <= 0 || len(full) <= maxBytes {
		return full
	}

	lo, hi := 1, len(artifacts)-1
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
	return build(0)
}

func printInvocationArtifactHeader(out io.Writer, breadcrumb string, count int, hasMore bool) {
	if hasMore {
		fmt.Fprintf(out, "Invocation Artifacts (%s) (showing %d, use -max-artifacts or -all to see more):\n", breadcrumb, count)
	} else {
		fmt.Fprintf(out, "Invocation Artifacts (%s) (%d):\n", breadcrumb, count)
	}
}

func printArtifactHeader(category, breadcrumb string, totalCount, maxArtifacts int) int {
	return printArtifactHeaderTo(os.Stdout, category, breadcrumb, totalCount, maxArtifacts)
}

func printArtifactHeaderTo(out io.Writer, category, breadcrumb string, totalCount, maxArtifacts int) int {
	displayedCount := totalCount
	if maxArtifacts > 0 && totalCount > maxArtifacts {
		displayedCount = maxArtifacts
		fmt.Fprintf(out, "%s Artifacts (%s) (showing %d of %d, use -all to see all):\n", category, breadcrumb, displayedCount, totalCount)
	} else {
		fmt.Fprintf(out, "%s Artifacts (%s) (%d):\n", category, breadcrumb, totalCount)
	}
	return displayedCount
}

func PrintImmediateParentInvocationNotice(ctx context.Context, client rdbpb.ResultDBClient, target string, out io.Writer) {
	idx := strings.Index(target, "/tests/")
	if idx == -1 {
		return
	}
	parentInv := target[:idx]
	leafInvID := strings.TrimPrefix(parentInv, "invocations/")

	res, err := client.ListArtifacts(ctx, &rdbpb.ListArtifactsRequest{
		Parent:   parentInv,
		PageSize: 1000,
	})
	if err == nil && len(res.Artifacts) > 0 {
		artLabel := "artifacts"
		countStr := fmt.Sprintf("%d", len(res.Artifacts))
		if res.NextPageToken != "" {
			countStr = fmt.Sprintf("%d+", len(res.Artifacts))
		} else if len(res.Artifacts) == 1 {
			artLabel = "artifact"
		}
		fmt.Fprintln(out, "Invocation Artifacts:")
		fmt.Fprintf(out, "  - Invocation %s contains %s %s. Run 'luci artifact list -invocationid %s' to view.\n", leafInvID, countStr, artLabel, leafInvID)
		fmt.Fprintln(out)
	}
}

type ancestorWUInfo struct {
	name      string
	artifacts []*rdbpb.Artifact
	more      bool
}

func fetchAncestorWorkUnitArtifacts(ctx context.Context, client rdbpb.ResultDBClient, target string, parentType ParentType) ([]ancestorWUInfo, string, string) {
	var wuToCheck []string
	visited := make(map[string]bool)
	var targetTestID, targetResultID string

	if parentType == ParentTypeTestResult {
		idx := strings.Index(target, "/tests/")
		if idx == -1 {
			return nil, "", ""
		}
		_, targetTestID, targetResultID = base.ExtractTestResultComponents(target)
		parentWU := target[:idx]
		visited[parentWU] = true
		wuToCheck = append(wuToCheck, parentWU)

		for _, anc := range format.QueryAncestorWorkUnits(ctx, client, parentWU) {
			if !visited[anc.Name] {
				visited[anc.Name] = true
				wuToCheck = append(wuToCheck, anc.Name)
			}
		}
	} else if parentType == ParentTypeWorkUnit {
		visited[target] = true
		for _, anc := range format.QueryAncestorWorkUnits(ctx, client, target) {
			if !visited[anc.Name] {
				visited[anc.Name] = true
				wuToCheck = append(wuToCheck, anc.Name)
			}
		}
	}

	if len(wuToCheck) == 0 {
		return nil, targetTestID, targetResultID
	}

	var withArtifacts []ancestorWUInfo
	for _, wuName := range wuToCheck {
		res, err := client.ListArtifacts(ctx, &rdbpb.ListArtifactsRequest{
			Parent:   wuName,
			PageSize: 1000,
		})
		if err == nil && len(res.Artifacts) > 0 {
			withArtifacts = append(withArtifacts, ancestorWUInfo{
				name:      wuName,
				artifacts: res.Artifacts,
				more:      res.NextPageToken != "",
			})
		}
	}
	return withArtifacts, targetTestID, targetResultID
}

// PrintAncestorWorkUnitNotices queries ancestor work units of target and prints
// their artifacts inline (all artifacts when <= maxInlineAncestorArtifacts, or
// up to maxInlineAncestorArtifacts high-priority diagnostic artifacts plus a
// 1-line summary when > maxInlineAncestorArtifacts), bounded by
// DefaultMaxOutputBytes.
func PrintAncestorWorkUnitNotices(ctx context.Context, client rdbpb.ResultDBClient, target string, parentType ParentType, out io.Writer) {
	PrintAncestorWorkUnitNoticesWithBudget(ctx, client, target, parentType, DefaultMaxOutputBytes, out)
}

// PrintAncestorWorkUnitNoticesWithBudget queries ancestor work units of target
// and prints their artifacts inline within maxBytes (0 for unlimited).
func PrintAncestorWorkUnitNoticesWithBudget(ctx context.Context, client rdbpb.ResultDBClient, target string, parentType ParentType, maxBytes int, out io.Writer) {
	withArtifacts, targetTestID, targetResultID := fetchAncestorWorkUnitArtifacts(ctx, client, target, parentType)
	if len(withArtifacts) == 0 {
		return
	}
	fmt.Fprint(out, formatAncestorWorkUnitNotices(withArtifacts, targetTestID, targetResultID, parentType, maxBytes))
}

func formatAncestorWorkUnitNotices(withArtifacts []ancestorWUInfo, targetTestID, targetResultID string, parentType ParentType, maxBytes int) string {
	if len(withArtifacts) == 0 {
		return ""
	}

	sectionHeader := "Work Unit Artifacts:"
	fetchHeader := "To fetch a work unit artifact:"
	if parentType == ParentTypeWorkUnit {
		sectionHeader = "Ancestor Work Unit Artifacts:"
		fetchHeader = "To fetch an ancestor work unit artifact:"
	}

	candidates := make([][]*rdbpb.Artifact, len(withArtifacts))
	canPrintAll := make([]bool, len(withArtifacts))
	counts := make([]int, len(withArtifacts))

	for i, info := range withArtifacts {
		if !info.more && len(info.artifacts) <= maxInlineAncestorArtifacts {
			sorted := make([]*rdbpb.Artifact, len(info.artifacts))
			copy(sorted, info.artifacts)
			format.SortArtifacts(sorted)
			candidates[i] = sorted
			canPrintAll[i] = true
		} else {
			recommended := format.FilterRecommendedDebugArtifacts(info.artifacts, targetTestID, targetResultID)
			if len(recommended) > maxInlineAncestorArtifacts {
				recommended = recommended[:maxInlineAncestorArtifacts]
			}
			candidates[i] = recommended
			canPrintAll[i] = false
		}
		counts[i] = len(candidates[i])
	}

	renderWithCounts := func(curCounts []int) string {
		var b bytes.Buffer
		fmt.Fprintln(&b, sectionHeader)

		var inlineWUIDs []string
		var sampleInvID string
		recordInline := func(qInv, qWU string) {
			inlineWUIDs = append(inlineWUIDs, qWU)
			if sampleInvID == "" {
				sampleInvID = qInv
			}
		}

		for i, info := range withArtifacts {
			invID, wuID := base.ExtractWorkUnitComponents(info.name)
			quotedInv := base.ShellQuoteIfNeeded(invID)
			quotedWU := base.ShellQuoteIfNeeded(wuID)
			wuLabel := "Root work unit"
			if wuID == "root" {
				wuLabel = "Root work unit (root)"
			} else if wuID != "" {
				wuLabel = fmt.Sprintf("Work unit %s", wuID)
			}
			allFlag := ""
			if info.more || len(info.artifacts) > DefaultMaxArtifacts {
				allFlag = " -all"
			}
			viewAllCmd := fmt.Sprintf("luci artifact list -invocationid=%s -workunitid=%s%s [-filter=...]", quotedInv, quotedWU, allFlag)

			showCount := curCounts[i]
			totalStr := fmt.Sprintf("%d", len(info.artifacts))
			if info.more {
				totalStr = fmt.Sprintf("%d+", len(info.artifacts))
			}

			if canPrintAll[i] && showCount == len(info.artifacts) {
				artLabel := "artifacts"
				if len(info.artifacts) == 1 {
					artLabel = "artifact"
				}
				fmt.Fprintf(&b, "  %s (%d %s):\n", wuLabel, len(info.artifacts), artLabel)
				format.PrintArtifactTableTo(&b, candidates[i], false)
				recordInline(quotedInv, quotedWU)
			} else if !canPrintAll[i] && len(candidates[i]) == 0 {
				fmt.Fprintf(&b, "  %s (%s artifacts, none matching default diagnostic types); run '%s' to view all\n", wuLabel, totalStr, viewAllCmd)
			} else if showCount == 0 {
				artLabel := "artifacts"
				if !info.more && len(info.artifacts) == 1 {
					artLabel = "artifact"
				}
				fmt.Fprintf(&b, "  %s (%s %s); run '%s' to view all\n", wuLabel, totalStr, artLabel, viewAllCmd)
			} else {
				fmt.Fprintf(&b, "  %s (showing %d of %s artifacts):\n", wuLabel, showCount, totalStr)
				format.PrintArtifactTableTo(&b, candidates[i][:showCount], false)
				recordInline(quotedInv, quotedWU)
				inWUPart := ""
				if wuID != "" {
					inWUPart = fmt.Sprintf(" in work unit %s", wuID)
				}
				omitted := len(info.artifacts) - showCount
				if omitted > 0 {
					omittedStr := fmt.Sprintf("%d", omitted)
					otherLabel := "artifacts"
					if info.more {
						omittedStr += "+"
					} else if omitted == 1 {
						otherLabel = "artifact"
					}
					fmt.Fprintf(&b, "  ... and %s other %s%s; run '%s' to view all\n", omittedStr, otherLabel, inWUPart, viewAllCmd)
				} else if info.more {
					fmt.Fprintf(&b, "  ... and more artifacts%s; run '%s' to view all\n", inWUPart, viewAllCmd)
				}
			}
			fmt.Fprintln(&b)
		}

		if len(inlineWUIDs) > 0 && sampleInvID != "" {
			wuFlagVal := "<WORK_UNIT_ID>"
			if len(inlineWUIDs) == 1 {
				wuFlagVal = inlineWUIDs[0]
			}
			fmt.Fprintf(&b, "%s\n  luci artifact <get|head|tail> -invocationid=%s -workunitid=%s -artifactid=<ARTIFACT_ID>\n\n",
				fetchHeader, sampleInvID, wuFlagVal)
		}
		return b.String()
	}

	outStr := renderWithCounts(counts)
	if maxBytes <= 0 || len(outStr) <= maxBytes {
		return outStr
	}

	// Pass 1: trim inline rows from farthest ancestor down to 1 row per ancestor.
	for i := len(counts) - 1; i >= 0 && len(outStr) > maxBytes; i-- {
		for counts[i] > 1 && len(outStr) > maxBytes {
			counts[i]--
			outStr = renderWithCounts(counts)
		}
	}
	// Pass 2: collapse ancestors to 1-line summary from farthest ancestor if still over budget.
	for i := len(counts) - 1; i >= 0 && len(outStr) > maxBytes; i-- {
		if counts[i] > 0 {
			counts[i] = 0
			outStr = renderWithCounts(counts)
		}
	}
	return outStr
}

func QueryAllArtifacts(ctx context.Context, client rdbpb.ResultDBClient, parent string) ([]*rdbpb.Artifact, error) {
	return queryParentArtifacts(ctx, client, parent, "")
}

// queryParentArtifacts lists all artifacts directly under parent using
// ResultDB.ListArtifacts, optionally filtered on the server by artifactIDRegexp
// (a case-insensitive RE2 fragment for ArtifactPredicate.ArtifactIdRegexp).
func queryParentArtifacts(ctx context.Context, client rdbpb.ResultDBClient, parent, artifactIDRegexp string) ([]*rdbpb.Artifact, error) {
	var predicate *rdbpb.ArtifactPredicate
	if artifactIDRegexp != "" {
		predicate = &rdbpb.ArtifactPredicate{
			ArtifactIdRegexp: artifactIDRegexp,
		}
	}
	var all []*rdbpb.Artifact
	pageToken := ""
	for {
		res, err := client.ListArtifacts(ctx, &rdbpb.ListArtifactsRequest{
			Parent:    parent,
			PageSize:  1000,
			PageToken: pageToken,
			Predicate: predicate,
		})
		if err != nil {
			if base.IsPermissionDenied(err) {
				return nil, errors.Fmt("ListArtifacts RPC failed (%s): %w", base.ResultDBPermissionReason(parent, err), err)
			}
			return nil, err
		}
		all = append(all, res.Artifacts...)
		if res.NextPageToken == "" {
			break
		}
		pageToken = res.NextPageToken
	}
	return all, nil
}

// QueryInvocationArtifacts queries work-unit-level and test-result-level
// artifacts across a root invocation using ResultDB.QueryArtifacts, optionally
// filtered by artifactIDRegexp (a case-insensitive RE2 fragment for
// ArtifactPredicate.ArtifactIdRegexp), sorts them by Huckle ATI priority, and
// returns up to maxArtifacts (or all artifacts if maxArtifacts <= 0) along with
// whether more artifacts exist beyond maxArtifacts.
//
// When maxArtifacts > 0, up to max(maxArtifacts, maxQueryArtifactsPageSize)
// artifacts are fetched before sorting so that high-priority artifacts (such as
// LOGCAT or BUGREPORT) later in the page are promoted ahead of lower-priority
// artifacts prior to truncation.
func QueryInvocationArtifacts(ctx context.Context, client rdbpb.ResultDBClient, cleanTarget, artifactIDRegexp string, maxArtifacts int) ([]*rdbpb.Artifact, bool, error) {
	artifacts, totalFetched, hasMorePages, err := queryInvocationArtifactsSorted(ctx, client, cleanTarget, artifactIDRegexp, maxArtifacts)
	if err != nil {
		return nil, false, err
	}
	hasMore := hasMorePages || totalFetched > len(artifacts)
	return artifacts, hasMore, nil
}

func queryInvocationArtifactsSorted(ctx context.Context, client rdbpb.ResultDBClient, cleanTarget, artifactIDRegexp string, maxArtifacts int) ([]*rdbpb.Artifact, int, bool, error) {
	var predicate *rdbpb.ArtifactPredicate
	if artifactIDRegexp != "" {
		predicate = &rdbpb.ArtifactPredicate{
			ArtifactIdRegexp: artifactIDRegexp,
		}
	}
	fetchLimit := maxArtifacts
	if maxArtifacts > 0 && fetchLimit < maxQueryArtifactsPageSize {
		fetchLimit = maxQueryArtifactsPageSize
	}
	artifacts, hasMorePages, err := queryInvocationArtifacts(ctx, client, cleanTarget, predicate, &fieldmaskpb.FieldMask{
		Paths: []string{"name", "artifact_id", "content_type", "artifact_type", "size_bytes", "has_lines"},
	}, fetchLimit)
	if err != nil {
		return nil, 0, false, err
	}
	totalFetched := len(artifacts)
	format.SortArtifacts(artifacts)
	if maxArtifacts > 0 && len(artifacts) > maxArtifacts {
		artifacts = artifacts[:maxArtifacts]
	}
	return artifacts, totalFetched, hasMorePages, nil
}
