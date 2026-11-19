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
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/client/cmd/luci/ids"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

// maxInspectedWorkUnits bounds the number of target work units queried for
// direct and ancestor artifacts in a single FetchWorkUnitAndAncestorNotices call.
const maxInspectedWorkUnits = format.MaxInspectedWorkUnits

// DefaultMaxOutputBytes is the default overall stdout byte cap (9.5 KB / 9,500 B)
// for `luci artifact list` responses so output stays within agent 10 KB tool
// output limits without middle-truncation.
const DefaultMaxOutputBytes = format.DefaultMaxOutputBytes

func ListCmd(af *base.AuthFlags, parentType ParentType) *subcommands.Command {
	var usage, desc, longDesc string
	switch parentType {
	case ParentTypeTestResult:
		usage = "list [-invocationid <invocation_id> -testid <test_id> -resultid <result_id>] [<target>]"
		desc = "List artifacts for a test result"
		longDesc = desc + " in ResultDB by a positional URL/ID or explicit ID flags.\n\n" +
			"Default output is capped within 9.5 KB (pass -max-artifacts or -all to see more).\n\n" +
			"Ancestor work units are also checked and a notice is displayed if artifacts exist."
	case ParentTypeWorkUnit:
		usage = "list [-invocationid <invocation_id> -workunitid <work_unit_id>] [<target>]"
		desc = "List artifacts for a work unit"
		longDesc = desc + " in ResultDB by a positional URL/ID or explicit ID flags.\n\n" +
			"Default output is capped within 9.5 KB (pass -max-artifacts or -all to see more).\n\n" +
			"Ancestor work units are also checked and a notice is displayed if artifacts exist."
	default:
		usage = "list [-invocationid <invocation_id>] [-workunitid <work_unit_id> | -testid <test_id> -resultid <result_id>] [<target>]"
		desc = "List artifacts for a work unit or test result"
		longDesc = desc + " in ResultDB by a positional URL/ID or explicit ID flags.\n\n" +
			"Specify -invocationid to list invocation-level artifacts,\n" +
			"-invocationid and -workunitid for work unit artifacts,\n" +
			"or -invocationid, -testid, and -resultid for test result artifacts.\n\n" +
			"Default output is capped within 9.5 KB (pass -max-artifacts or -all to see more).\n\n" +
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
			r.Flags.IntVar(&r.maxArtifacts, "max-artifacts", DefaultMaxArtifacts, "Maximum number of artifacts to display (default: 100, 0 for all; setting explicitly bypasses 9.5 KB cap)")
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

func (r *artifactListRun) hasExplicitLimitFlag() bool {
	if r.maxArtifacts > 0 && r.maxArtifacts != DefaultMaxArtifacts {
		return true
	}
	explicit := false
	r.Flags.Visit(func(f *flag.Flag) {
		if f.Name == "max-artifacts" {
			explicit = true
		}
	})
	return explicit
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
	capOutput := !r.allArtifacts && !r.hasExplicitLimitFlag()
	maxOutputBytes := 0
	if capOutput {
		maxOutputBytes = DefaultMaxOutputBytes
	}
	if r.allArtifacts {
		maxArtifacts = 0
	}

	out := r.out
	if out == nil && a != nil {
		out = a.GetOut()
	}
	if out == nil {
		out = os.Stdout
	}
	filterSuffix := r.formatActiveFilterSuffix()

	var outBuf bytes.Buffer
	consumeDerivedBudget := func() {
		var derivedBuf bytes.Buffer
		ids.PrintDerivedIDs(&derivedBuf, r.extracted)
		outBuf.Write(derivedBuf.Bytes())
		if maxOutputBytes > 0 {
			maxOutputBytes = max(maxOutputBytes-derivedBuf.Len(), 1)
		}
	}
	flushOut := func() int {
		if capOutput && outBuf.Len() > DefaultMaxOutputBytes {
			capped := format.TruncateOutputBytes(outBuf.String(), DefaultMaxOutputBytes-1, "pass -all to view full output")
			if capped != "" && !strings.HasSuffix(capped, "\n") {
				capped += "\n"
			}
			fmt.Fprint(out, capped)
			return 0
		}
		fmt.Fprint(out, outBuf.String())
		return 0
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
		consumeDerivedBudget()

		totalCount := len(trArtifacts)
		displayed := trArtifacts
		if maxArtifacts > 0 && len(displayed) > maxArtifacts {
			displayed = displayed[:maxArtifacts]
		}

		if resolved.Legacy {
			var parentBuf bytes.Buffer
			PrintImmediateParentInvocationNotice(ctx, client, cleanTarget, &parentBuf)
			parentFull := parentBuf.String()
			primaryBudget := maxOutputBytes
			if primaryBudget > 0 && len(parentFull) > 0 {
				reserve := min(len(parentFull), maxOutputBytes/2)
				primaryBudget = max(maxOutputBytes-reserve, 1)
			}
			primaryOut := renderPrimaryArtifactSection("Test Result", format.FormatTestResultBreadcrumb(cleanTarget), cleanTarget, displayed, totalCount, false, maxArtifacts, false, filterSuffix, primaryBudget)
			parentOut := parentFull
			if maxOutputBytes > 0 && len(primaryOut)+len(parentOut) > maxOutputBytes {
				parentOut = ""
			}
			if parentOut == "" && primaryBudget < maxOutputBytes {
				primaryOut = renderPrimaryArtifactSection("Test Result", format.FormatTestResultBreadcrumb(cleanTarget), cleanTarget, displayed, totalCount, false, maxArtifacts, false, filterSuffix, maxOutputBytes)
			}
			fmt.Fprint(&outBuf, primaryOut)
			fmt.Fprint(&outBuf, parentOut)
		} else {
			writePrimaryAndAncestorNotices(ctx, client, "Test Result", format.FormatTestResultBreadcrumb(cleanTarget), cleanTarget, displayed, totalCount, maxArtifacts, filterSuffix, effectiveParentType, maxOutputBytes, &outBuf)
		}
		return flushOut()
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

			consumeDerivedBudget()

			totalCount := len(invArtifacts)
			displayed := invArtifacts
			if maxArtifacts > 0 && len(displayed) > maxArtifacts {
				displayed = displayed[:maxArtifacts]
			}
			fmt.Fprint(&outBuf, renderPrimaryArtifactSection("Invocation", format.FormatInvocationBreadcrumb(cleanTarget), cleanTarget, displayed, totalCount, false, maxArtifacts, false, filterSuffix, maxOutputBytes))
			return flushOut()
		}

		invArtifacts, totalFetched, hasMorePages, err := queryInvocationArtifactsSorted(ctx, client, cleanTarget, serverRegexp, maxArtifacts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to list artifacts for invocation %q: %s\n", cleanTarget, err)
			return 1
		}

		consumeDerivedBudget()

		fmt.Fprint(&outBuf, renderPrimaryArtifactSection("Invocation", format.FormatInvocationBreadcrumb(cleanTarget), cleanTarget, invArtifacts, totalFetched, hasMorePages, maxArtifacts, true, filterSuffix, maxOutputBytes))
		return flushOut()
	}

	// Work unit
	wuArtifacts, err := queryParentArtifacts(ctx, client, cleanTarget, serverRegexp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to list artifacts for work unit %q: %s\n", cleanTarget, err)
		return 1
	}
	wuArtifacts = FilterArtifacts(wuArtifacts, filterRe)
	format.SortArtifacts(wuArtifacts)

	consumeDerivedBudget()

	totalCount := len(wuArtifacts)
	displayed := wuArtifacts
	if maxArtifacts > 0 && len(displayed) > maxArtifacts {
		displayed = displayed[:maxArtifacts]
	}

	writePrimaryAndAncestorNotices(ctx, client, "Work Unit", format.FormatWorkUnitBreadcrumb(cleanTarget), cleanTarget, displayed, totalCount, maxArtifacts, filterSuffix, effectiveParentType, maxOutputBytes, &outBuf)
	return flushOut()
}

func (r *artifactListRun) formatActiveFilterSuffix() string {
	var parts []string
	if r.filter != "" {
		parts = append(parts, "-filter="+base.ShellQuoteIfNeeded(r.filter))
	}
	if r.glob != "" {
		parts = append(parts, "-glob="+base.ShellQuoteIfNeeded(r.glob))
	}
	if len(parts) == 0 {
		return "[-filter=...]"
	}
	return strings.Join(parts, " ")
}

func writePrimaryAndAncestorNotices(ctx context.Context, client rdbpb.ResultDBClient, category, breadcrumb, cleanTarget string, artifacts []*rdbpb.Artifact, totalCount, maxArtifacts int, filterSuffix string, parentType ParentType, totalBudget int, out io.Writer) {
	withArtifacts, targetTestID, targetResultID := fetchAncestorWorkUnitArtifacts(ctx, client, cleanTarget, parentType)
	ancFull := formatAncestorWorkUnitNotices(withArtifacts, targetTestID, targetResultID, parentType, 0)

	primaryBudget := totalBudget
	if totalBudget > 0 && len(ancFull) > 0 {
		ancReserve := min(len(ancFull), totalBudget/2)
		primaryBudget = max(totalBudget-ancReserve, 1)
	}

	primaryOut := renderPrimaryArtifactSection(category, breadcrumb, cleanTarget, artifacts, totalCount, false, maxArtifacts, false, filterSuffix, primaryBudget)
	if len(ancFull) == 0 {
		fmt.Fprint(out, primaryOut)
		return
	}
	ancOut := ancFull
	if totalBudget > 0 {
		remaining := totalBudget - len(primaryOut)
		if remaining <= 0 {
			ancOut = ""
		} else if len(ancOut) > remaining {
			ancOut = formatAncestorWorkUnitNotices(withArtifacts, targetTestID, targetResultID, parentType, remaining)
		}
	}
	if ancOut == "" && primaryBudget < totalBudget {
		primaryOut = renderPrimaryArtifactSection(category, breadcrumb, cleanTarget, artifacts, totalCount, false, maxArtifacts, false, filterSuffix, totalBudget)
	}
	fmt.Fprint(out, primaryOut)
	fmt.Fprint(out, ancOut)
}

func formatOmittedArtifactsLine(out io.Writer, cleanTarget string, omitted int, hasMore bool, filterSuffix string) {
	meta := base.ExtractArtifactMetadata(cleanTarget)
	if meta.InvocationID == "" {
		return
	}
	quotedInv := base.ShellQuoteIfNeeded(meta.InvocationID)
	flags := meta.FormatFlags()
	if flags != "" {
		flags = " " + flags
	}
	if filterSuffix == "" {
		filterSuffix = "[-filter=...]"
	}
	viewAllCmd := fmt.Sprintf("luci artifact list -invocationid=%s%s -all %s", quotedInv, flags, filterSuffix)
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

func renderPrimaryArtifactSection(category, breadcrumb, cleanTarget string, artifacts []*rdbpb.Artifact, totalCount int, hasMore bool, maxArtifacts int, includeMetadata bool, filterSuffix string, maxBytes int) string {
	if len(artifacts) == 0 {
		var buf bytes.Buffer
		if includeMetadata {
			printInvocationArtifactHeader(&buf, breadcrumb, 0, hasMore)
		} else {
			printArtifactHeaderTo(&buf, category, breadcrumb, 0, maxArtifacts)
		}
		fmt.Fprintf(&buf, "  (no artifacts)\n\n")
		s := buf.String()
		if maxBytes > 0 && len(s) > maxBytes {
			return format.TruncateUTF8Prefix(s, maxBytes)
		}
		return s
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
			formatOmittedArtifactsLine(&b, cleanTarget, totalCount-k, hasMore, filterSuffix)
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
	summary := build(0)
	if maxBytes > 0 && len(summary) > maxBytes {
		totalStr := fmt.Sprintf("%d", totalCount)
		if hasMore {
			totalStr += "+"
		}
		fallback := fmt.Sprintf("%s Artifacts (%s omitted to stay within output limit; pass -all to view)\n", category, totalStr)
		return format.TruncateUTF8Prefix(fallback, maxBytes)
	}
	return summary
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
	format.PrintImmediateParentInvocationNotice(ctx, client, target, out)
}

func fetchAncestorWorkUnitArtifacts(ctx context.Context, client rdbpb.ResultDBClient, target string, parentType ParentType) ([]format.WorkUnitArtifactsInfo, string, string) {
	if parentType != ParentTypeTestResult && parentType != ParentTypeWorkUnit {
		return nil, "", ""
	}
	return format.FetchAncestorWorkUnitArtifacts(ctx, client, target, parentType == ParentTypeWorkUnit)
}

// PrintAncestorWorkUnitNotices queries ancestor work units of target and prints
// their artifacts inline (all artifacts when <= 15, or up to 15 high-priority
// diagnostic artifacts plus a 1-line summary when > 15), bounded by
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

// PrintWorkUnitAndAncestorNotices queries direct artifacts on targetWorkUnits
// and artifacts on their ancestor work units, printing inline artifact notices
// bounded by DefaultMaxOutputBytes.
func PrintWorkUnitAndAncestorNotices(ctx context.Context, client rdbpb.ResultDBClient, targetWorkUnits []string, out io.Writer) {
	PrintWorkUnitAndAncestorNoticesWithBudget(ctx, client, targetWorkUnits, DefaultMaxOutputBytes, out)
}

// PrintWorkUnitAndAncestorNoticesWithBudget queries direct artifacts on
// targetWorkUnits and artifacts on their ancestor work units, printing inline
// artifact notices bounded by maxBytes (0 for unlimited).
func PrintWorkUnitAndAncestorNoticesWithBudget(ctx context.Context, client rdbpb.ResultDBClient, targetWorkUnits []string, maxBytes int, out io.Writer) {
	if maxBytes < 0 {
		return
	}
	notices, _ := FetchWorkUnitAndAncestorNotices(ctx, client, targetWorkUnits, maxBytes)
	if notices == "" {
		return
	}
	fmt.Fprintln(out)
	fmt.Fprint(out, notices)
}

// FetchWorkUnitAndAncestorNotices queries direct artifacts on targetWorkUnits
// and artifacts on their ancestor work units, returning the formatted notice
// sections (bounded by maxBytes; 0 for unlimited) and a map of direct artifact
// IDs keyed by work unit resource name.
func FetchWorkUnitAndAncestorNotices(ctx context.Context, client rdbpb.ResultDBClient, targetWorkUnits []string, maxBytes int) (string, map[string][]string) {
	formatter, directArtifactIDs := FetchWorkUnitAndAncestorNoticeFormatter(ctx, client, targetWorkUnits)
	if formatter == nil {
		return "", directArtifactIDs
	}
	return formatter(maxBytes), directArtifactIDs
}

// FetchWorkUnitAndAncestorNoticeFormatter queries direct artifacts on up to
// maxInspectedWorkUnits of targetWorkUnits and artifacts on their unique
// ancestor work units, returning a function that formats the notice sections
// within a given maxBytes budget (0 for unlimited, <0 for 0 bytes remaining)
// and a map of direct artifact IDs keyed by work unit resource name.
func FetchWorkUnitAndAncestorNoticeFormatter(ctx context.Context, client rdbpb.ResultDBClient, targetWorkUnits []string) (func(maxBytes int) string, map[string][]string) {
	return format.FetchWorkUnitAndAncestorNoticeFormatterExcluding(ctx, client, targetWorkUnits, nil)
}

func formatAncestorWorkUnitNotices(withArtifacts []format.WorkUnitArtifactsInfo, targetTestID, targetResultID string, parentType ParentType, maxBytes int) string {
	return format.FormatAncestorWorkUnitNoticesWithOmitted(withArtifacts, 0, targetTestID, targetResultID, parentType == ParentTypeWorkUnit, maxBytes)
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
			ReadMask:  format.ArtifactMetadataReadMask(),
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
	artifacts, hasMorePages, err := queryInvocationArtifacts(ctx, client, cleanTarget, predicate, format.ArtifactMetadataReadMask(), fetchLimit)
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
