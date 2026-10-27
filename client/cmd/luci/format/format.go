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
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"text/tabwriter"

	"golang.org/x/net/html"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/errors"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

// HierarchyAggregations holds test verdict counts at each level of the ResultDB
// test hierarchy for inline display alongside Invocation and Test ID components.
type HierarchyAggregations struct {
	Invocation *rdbpb.TestAggregation_VerdictCounts
	Module     *rdbpb.TestAggregation_VerdictCounts
	Coarse     *rdbpb.TestAggregation_VerdictCounts
	Fine       *rdbpb.TestAggregation_VerdictCounts
}

// GetInvocation returns the invocation-level verdict counts, or nil if receiver is nil.
func (h *HierarchyAggregations) GetInvocation() *rdbpb.TestAggregation_VerdictCounts {
	if h == nil {
		return nil
	}
	return h.Invocation
}

// GetModule returns the module-level verdict counts, or nil if receiver is nil.
func (h *HierarchyAggregations) GetModule() *rdbpb.TestAggregation_VerdictCounts {
	if h == nil {
		return nil
	}
	return h.Module
}

// GetCoarse returns the coarse-level verdict counts, or nil if receiver is nil.
func (h *HierarchyAggregations) GetCoarse() *rdbpb.TestAggregation_VerdictCounts {
	if h == nil {
		return nil
	}
	return h.Coarse
}

// GetFine returns the fine-level verdict counts, or nil if receiver is nil.
func (h *HierarchyAggregations) GetFine() *rdbpb.TestAggregation_VerdictCounts {
	if h == nil {
		return nil
	}
	return h.Fine
}

// FormatVerdictCountsInline formats verdict counts as a compact inline bracketed
// summary, e.g. "[245 failed, 218 passed, 7 skipped]". Returns "" if vc is nil
// or all counts are zero.
func FormatVerdictCountsInline(vc *rdbpb.TestAggregation_VerdictCounts) string {
	if vc == nil {
		return ""
	}
	hasFailures := vc.Failed > 0 || vc.ExecutionErrored > 0 || vc.Precluded > 0 || vc.Flaky > 0 || vc.Exonerated > 0
	total := vc.Failed + vc.ExecutionErrored + vc.Precluded + vc.Flaky + vc.Exonerated + vc.Passed + vc.Skipped
	if total == 0 {
		return ""
	}
	var parts []string
	if vc.Failed > 0 {
		parts = append(parts, fmt.Sprintf("%d failed", vc.Failed))
	}
	if vc.ExecutionErrored > 0 {
		parts = append(parts, fmt.Sprintf("%d execution_errored", vc.ExecutionErrored))
	}
	if vc.Precluded > 0 {
		parts = append(parts, fmt.Sprintf("%d precluded", vc.Precluded))
	}
	if vc.Flaky > 0 {
		parts = append(parts, fmt.Sprintf("%d flaky", vc.Flaky))
	}
	if vc.Exonerated > 0 {
		parts = append(parts, fmt.Sprintf("%d exonerated", vc.Exonerated))
	}
	if vc.Passed > 0 || hasFailures {
		parts = append(parts, fmt.Sprintf("%d passed", vc.Passed))
	}
	if vc.Skipped > 0 {
		parts = append(parts, fmt.Sprintf("%d skipped", vc.Skipped))
	}
	if len(parts) == 0 {
		return ""
	}
	return fmt.Sprintf("[%s]", strings.Join(parts, ", "))
}

// FetchArtifactContent downloads the artifact content from fetchURL using an HTTP request with context.
func FetchArtifactContent(ctx context.Context, httpClient *http.Client, fetchURL string, out io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, "GET", fetchURL, nil)
	if err != nil {
		return errors.Fmt("failed to create HTTP request: %w", err)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return errors.Fmt("HTTP GET failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errors.Fmt("HTTP GET failed with status %d: %s", resp.StatusCode, resp.Status)
	}
	_, err = io.Copy(out, resp.Body)
	return err
}

// FetchArtifact fetches the full content of an artifact by its ResultDB resource name.
func FetchArtifact(ctx context.Context, rdbClient rdbpb.ResultDBClient, httpClient *http.Client, artName string) ([]byte, error) {
	art, err := rdbClient.GetArtifact(ctx, &rdbpb.GetArtifactRequest{Name: artName})
	if err != nil {
		return nil, errors.Fmt("GetArtifact RPC failed: %w", err)
	}
	if art.FetchUrl == "" {
		return nil, errors.Fmt("artifact %q has no fetch URL", artName)
	}
	var buf bytes.Buffer
	if err := FetchArtifactContent(ctx, httpClient, art.FetchUrl, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// PrintTestID prints the structured or flat test identifier of a test result to stdout.
func PrintTestID(ctx context.Context, schemasClient rdbpb.SchemasClient, res *rdbpb.TestResult) {
	PrintTestIDWithAggregations(ctx, schemasClient, res, nil)
}

// FprintTestID prints the structured or flat test identifier of a test result to out.
func FprintTestID(out io.Writer, ctx context.Context, schemasClient rdbpb.SchemasClient, res *rdbpb.TestResult) {
	FprintTestIDWithAggregations(out, ctx, schemasClient, res, nil)
}

// PrintTestIDWithAggregations prints the structured or flat test identifier of a
// test result, optionally annotating the module, coarse, and fine hierarchy levels
// with inline verdict counts from aggs.
func PrintTestIDWithAggregations(ctx context.Context, schemasClient rdbpb.SchemasClient, res *rdbpb.TestResult, aggs *HierarchyAggregations) {
	FprintTestIDWithAggregations(os.Stdout, ctx, schemasClient, res, aggs)
}

// FprintTestIDWithAggregations prints the structured or flat test identifier of a
// test result to out, optionally annotating the module, coarse, and fine hierarchy
// levels with inline verdict counts from aggs.
func FprintTestIDWithAggregations(out io.Writer, ctx context.Context, schemasClient rdbpb.SchemasClient, res *rdbpb.TestResult, aggs *HierarchyAggregations) {
	appendCounts := func(val string, vc *rdbpb.TestAggregation_VerdictCounts) string {
		if c := FormatVerdictCountsInline(vc); c != "" {
			return val + " " + c
		}
		return val
	}

	st := res.TestIdStructured
	if st != nil && st.ModuleScheme != "" && st.ModuleScheme != "legacy" {
		fmt.Fprintln(out, "Test ID:")
		var scheme *rdbpb.Scheme
		var err error
		if schemasClient != nil {
			scheme, err = schemasClient.GetScheme(ctx, &rdbpb.GetSchemeRequest{Name: "schema/schemes/" + st.ModuleScheme})
		}
		if err == nil && scheme != nil {
			modLabel := "Module"
			if scheme.HumanReadableName != "" {
				modLabel = fmt.Sprintf("Module (%s)", scheme.HumanReadableName)
			}
			fmt.Fprintf(out, "  %s: %s\n", modLabel, appendCounts(st.ModuleName, aggs.GetModule()))
			if st.CoarseName != "" {
				label := "Coarse"
				if scheme.Coarse != nil && scheme.Coarse.HumanReadableName != "" {
					label = scheme.Coarse.HumanReadableName
				}
				fmt.Fprintf(out, "  %s: %s\n", label, appendCounts(st.CoarseName, aggs.GetCoarse()))
			}
			if st.FineName != "" {
				label := "Fine"
				if scheme.Fine != nil && scheme.Fine.HumanReadableName != "" {
					label = scheme.Fine.HumanReadableName
				}
				fmt.Fprintf(out, "  %s: %s\n", label, appendCounts(st.FineName, aggs.GetFine()))
			}
			if st.CaseName != "" {
				label := "Case"
				if scheme.Case != nil && scheme.Case.HumanReadableName != "" {
					label = scheme.Case.HumanReadableName
				}
				fmt.Fprintf(out, "  %s: %s\n", label, st.CaseName)
			}
		} else {
			modVal := fmt.Sprintf("%s (scheme: %s)", st.ModuleName, st.ModuleScheme)
			fmt.Fprintf(out, "  Module:    %s\n", appendCounts(modVal, aggs.GetModule()))
			if st.CoarseName != "" {
				fmt.Fprintf(out, "  Coarse:    %s\n", appendCounts(st.CoarseName, aggs.GetCoarse()))
			}
			if st.FineName != "" {
				fmt.Fprintf(out, "  Fine:      %s\n", appendCounts(st.FineName, aggs.GetFine()))
			}
			if st.CaseName != "" {
				fmt.Fprintf(out, "  Case:      %s\n", st.CaseName)
			}
		}
	} else {
		fmt.Fprintf(out, "Test ID:   %s\n", res.TestId)
	}
}

// maxInlineArtifactBytes is the maximum size (64 KiB) of an embedded
// <text-artifact> in summary_html that will be fetched and expanded inline
// automatically.
const maxInlineArtifactBytes = 64 * 1024

// FormatSummaryHTML converts a test result's summary_html into plain text,
// automatically expanding small embedded <text-artifact> tags inline when
// rdbClient and httpClient are provided.
func FormatSummaryHTML(ctx context.Context, rdbClient rdbpb.ResultDBClient, httpClient *http.Client, resName, htmlStr string) string {
	re := regexp.MustCompile(`<text-artifact\s+([^>]+)>`)
	htmlStr = re.ReplaceAllStringFunc(htmlStr, func(tag string) string {
		match := re.FindStringSubmatch(tag)
		if len(match) < 2 {
			return tag
		}
		attrs := match[1]
		idRe := regexp.MustCompile(`artifact-id="([^"]+)"`)
		idMatch := idRe.FindStringSubmatch(attrs)
		if len(idMatch) < 2 {
			return tag
		}
		artID := idMatch[1]
		if rdbClient == nil || httpClient == nil {
			return fmt.Sprintf("[Embedded Artifact: %s - use 'luci artifact get' to view]", artID)
		}
		isInvLevel := strings.Contains(attrs, "inv-level")
		encodedArtID := url.PathEscape(artID)
		var artName string
		if isInvLevel {
			idx := strings.Index(resName, "/tests/")
			if idx != -1 {
				artName = resName[:idx] + "/artifacts/" + encodedArtID
			} else {
				artName = resName + "/artifacts/" + encodedArtID
			}
		} else {
			artName = resName + "/artifacts/" + encodedArtID
		}
		art, err := rdbClient.GetArtifact(ctx, &rdbpb.GetArtifactRequest{Name: artName})
		if err != nil || art == nil || art.FetchUrl == "" {
			return fmt.Sprintf("[Failed to fetch embedded artifact: %s]", artID)
		}
		if art.SizeBytes > maxInlineArtifactBytes {
			return fmt.Sprintf("[Embedded Artifact: %s (%s) - use 'luci artifact get' to view]", artID, FormatSize(art.SizeBytes))
		}
		var buf bytes.Buffer
		if err := FetchArtifactContent(ctx, httpClient, art.FetchUrl, &buf); err != nil {
			return fmt.Sprintf("[Failed to fetch embedded artifact: %s]", artID)
		}
		return fmt.Sprintf("\n--- Embedded Artifact: %s ---\n%s\n--- End Artifact ---", artID, buf.String())
	})
	htmlStr = strings.ReplaceAll(htmlStr, "<br>", "\n")
	htmlStr = strings.ReplaceAll(htmlStr, "<br/>", "\n")
	htmlStr = strings.ReplaceAll(htmlStr, "<br />", "\n")
	htmlStr = strings.ReplaceAll(htmlStr, "</p>", "\n")
	return StripHTML(htmlStr)
}

func PrintAdditionalMetadata(res *rdbpb.TestResult) {
	FprintAdditionalMetadata(os.Stdout, res)
}

func FprintAdditionalMetadata(out io.Writer, res *rdbpb.TestResult) {
	if tm := res.TestMetadata; tm != nil {
		if loc := tm.Location; loc != nil {
			fmt.Fprintf(out, "Location:  repo: %s, file: %s", loc.Repo, loc.FileName)
			if loc.Line > 0 {
				fmt.Fprintf(out, ", line: %d", loc.Line)
			}
			fmt.Fprintln(out)
		}
		if bc := tm.BugComponent; bc != nil {
			if bc.GetIssueTracker() != nil {
				fmt.Fprintf(out, "Bug Component: issue tracker %d\n", bc.GetIssueTracker().ComponentId)
			} else if bc.GetMonorail() != nil {
				fmt.Fprintf(out, "Bug Component: monorail %s/%s\n", bc.GetMonorail().Project, bc.GetMonorail().Value)
			}
		}
		if tm.PreviousTestId != "" {
			fmt.Fprintf(out, "Prev ID:   %s\n", tm.PreviousTestId)
		}
		if tm.Properties != nil {
			b, _ := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(tm.Properties)
			fmt.Fprintf(out, "Metadata Properties:\n%s\n", string(b))
		}
	}
	if len(res.Tags) > 0 {
		fmt.Fprintln(out, "Tags:")
		for _, tag := range res.Tags {
			fmt.Fprintf(out, "  %s: %s\n", tag.Key, tag.Value)
		}
	}
	if res.Properties != nil {
		b, _ := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(res.Properties)
		fmt.Fprintf(out, "Properties:\n%s\n", string(b))
	}
}

func ParseInvocationContext(resultName string) string {
	parts := strings.Split(resultName, "/")
	if len(parts) < 2 {
		return ""
	}
	invID := parts[1]
	if strings.HasPrefix(invID, "build-") {
		return fmt.Sprintf("build %s", strings.TrimPrefix(invID, "build-"))
	}
	if strings.HasPrefix(invID, "task-") {
		taskParts := strings.Split(strings.TrimPrefix(invID, "task-"), "-")
		if len(taskParts) > 0 {
			return fmt.Sprintf("task %s", taskParts[len(taskParts)-1])
		}
		return fmt.Sprintf("task %s", strings.TrimPrefix(invID, "task-"))
	}
	return fmt.Sprintf("invocation %s", invID)
}

// FormatTestResultBreadcrumb returns a compact human-readable breadcrumb for a test result.
// e.g. "Result 0c30c334-01920 in task 7a07b808bfa95b11"
func FormatTestResultBreadcrumb(resultName string) string {
	invLabel := ParseInvocationContext(resultName)
	var resultID string
	if idx := strings.Index(resultName, "/results/"); idx != -1 {
		rest := resultName[idx+len("/results/"):]
		if slashIdx := strings.Index(rest, "/"); slashIdx != -1 {
			resultID = rest[:slashIdx]
		} else {
			resultID = rest
		}
	}
	if resultID != "" && invLabel != "" {
		return fmt.Sprintf("Result %s in %s", resultID, invLabel)
	}
	if resultID != "" {
		return fmt.Sprintf("Result %s", resultID)
	}
	if invLabel != "" {
		return invLabel
	}
	return resultName
}

// FormatWorkUnitBreadcrumb returns a compact human-readable breadcrumb for a work unit.
// e.g. "Work Unit run-tests in build 8673802696052024673"
func FormatWorkUnitBreadcrumb(wuName string) string {
	invLabel := ParseInvocationContext(wuName)
	var wuID string
	if idx := strings.Index(wuName, "/workUnits/"); idx != -1 {
		rest := wuName[idx+len("/workUnits/"):]
		if slashIdx := strings.Index(rest, "/"); slashIdx != -1 {
			wuID = rest[:slashIdx]
		} else {
			wuID = rest
		}
	}
	if wuID != "" && invLabel != "" {
		return fmt.Sprintf("Work Unit %s in %s", wuID, invLabel)
	}
	if wuID != "" {
		return fmt.Sprintf("Work Unit %s", wuID)
	}
	if invLabel != "" {
		return invLabel
	}
	return wuName
}

// FormatInvocationBreadcrumb returns a compact human-readable breadcrumb for an invocation.
// e.g. "build 8673802696052024673" or "invocation u-root-2026-..."
func FormatInvocationBreadcrumb(invName string) string {
	invLabel := ParseInvocationContext(invName)
	if invLabel != "" {
		return invLabel
	}
	return invName
}

// ParentGroup holds grouping info for test results or artifacts.
type ParentGroup struct {
	Label string // e.g. "Task 7a07b808bfa95b11", "Work Unit run-tests", "build-867380...", "u-foo"
	ID    string // canonical parent resource name or ID
}

// GetParentGroup determines the immediate parent task, work unit, or legacy invocation.
// The label uses "Task" ONLY when the parent is actually a Swarming task; otherwise it
// uses "Work Unit <id>" or the legacy invocation ID.
func GetParentGroup(name string) ParentGroup {
	if strings.Contains(name, "/workUnits/") {
		idx := strings.Index(name, "/workUnits/")
		rest := name[idx+len("/workUnits/"):]
		wuID := rest
		if slash := strings.Index(rest, "/"); slash != -1 {
			wuID = rest[:slash]
		}
		parentName := name[:idx+len("/workUnits/")+len(wuID)]
		return ParentGroup{
			Label: fmt.Sprintf("Work Unit %s", wuID),
			ID:    parentName,
		}
	}

	if strings.HasPrefix(name, "invocations/") {
		parts := strings.Split(name, "/")
		if len(parts) >= 2 {
			invID := parts[1]
			parentName := "invocations/" + invID
			if strings.HasPrefix(invID, "task-") {
				taskParts := strings.Split(strings.TrimPrefix(invID, "task-"), "-")
				taskID := taskParts[len(taskParts)-1]
				return ParentGroup{
					Label: fmt.Sprintf("Task %s", taskID),
					ID:    parentName,
				}
			}
			return ParentGroup{
				Label: invID,
				ID:    parentName,
			}
		}
	}

	return ParentGroup{
		Label: name,
		ID:    name,
	}
}

func FormatVariant(v *rdbpb.Variant) string {
	if v == nil {
		return ""
	}
	keys := make([]string, 0, len(v.GetDef()))
	for k := range v.GetDef() {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v.GetDef()[k]))
	}
	return strings.Join(parts, " ")
}

func FormatDuration(d *durationpb.Duration) string {
	if d == nil {
		return "N/A"
	}
	return d.AsDuration().String()
}

func StripHTML(s string) string {
	var b strings.Builder
	z := html.NewTokenizer(strings.NewReader(s))
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			break
		}
		if tt == html.TextToken {
			b.Write(z.Text())
		}
	}
	return strings.TrimSpace(html.UnescapeString(b.String()))
}

func FormatSize(sizeBytes int64) string {
	if sizeBytes < 0 {
		return "0 B"
	}
	const unit = 1024
	if sizeBytes < unit {
		return fmt.Sprintf("%d B", sizeBytes)
	}
	div, exp := int64(unit), 0
	for n := sizeBytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	prefixes := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	if exp < len(prefixes) {
		return fmt.Sprintf("%.1f %s (%d bytes)", float64(sizeBytes)/float64(div), prefixes[exp], sizeBytes)
	}
	return fmt.Sprintf("%d bytes", sizeBytes)
}

// FormatCompactSize formats a byte count as a compact human-readable string
// (e.g. "671 B", "45.1 KiB", "18.4 MiB") without the raw byte count suffix.
func FormatCompactSize(sizeBytes int64) string {
	if sizeBytes < 0 {
		return "0 B"
	}
	const unit = 1024
	if sizeBytes < unit {
		return fmt.Sprintf("%d B", sizeBytes)
	}
	div, exp := int64(unit), 0
	for n := sizeBytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	prefixes := []string{"KiB", "MiB", "GiB", "TiB", "PiB"}
	if exp < len(prefixes) {
		return fmt.Sprintf("%.1f %s", float64(sizeBytes)/float64(div), prefixes[exp])
	}
	return fmt.Sprintf("%d B", sizeBytes)
}

// PrintFailureReason prints the failure reason error message(s) and stack trace(s).
func PrintFailureReason(fr *rdbpb.FailureReason, indent string) {
	FprintFailureReason(os.Stdout, fr, indent)
}

// FprintFailureReason prints the failure reason error message(s) and stack trace(s) to out.
func FprintFailureReason(out io.Writer, fr *rdbpb.FailureReason, indent string) {
	if fr == nil {
		return
	}
	if len(fr.Errors) == 0 {
		if fr.PrimaryErrorMessage != "" {
			fmt.Fprintf(out, "%sError:     %s\n", indent, fr.PrimaryErrorMessage)
		}
		return
	}

	for i, errItem := range fr.Errors {
		errMsg := errItem.Message
		if errMsg == "" && i == 0 && fr.PrimaryErrorMessage != "" {
			errMsg = fr.PrimaryErrorMessage
		}

		if errMsg != "" {
			var prefix string
			if len(fr.Errors) > 1 {
				prefix = fmt.Sprintf("%sError #%d: ", indent, i+1)
			} else {
				prefix = fmt.Sprintf("%sError:     ", indent)
			}

			lines := strings.Split(strings.TrimRight(errMsg, "\n"), "\n")
			if len(lines) == 1 {
				fmt.Fprintf(out, "%s%s\n", prefix, lines[0])
			} else {
				fmt.Fprintf(out, "%s\n", strings.TrimRight(prefix, " "))
				for _, l := range lines {
					fmt.Fprintf(out, "%s  %s\n", indent, l)
				}
			}
		}

		if errItem.Trace != "" {
			traceIndent := indent + "  "
			fmt.Fprintf(out, "%sStack Trace:\n", indent)
			for _, l := range strings.Split(strings.TrimRight(errItem.Trace, "\n"), "\n") {
				fmt.Fprintf(out, "%s%s\n", traceIndent, l)
			}
		}
	}
}

// TruncateFirstLine returns the first non-empty line of text, trimmed to maxLen characters with "..." if longer.
// It also returns whether the input text was truncated (e.g. had more lines or exceeded maxLen).
func TruncateFirstLine(s string, maxLen int) (string, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return "", false
	}
	truncated := false
	if idx := strings.Index(s, "\n"); idx != -1 {
		s = strings.TrimSpace(s[:idx])
		truncated = true
	}
	if maxLen > 0 {
		runes := []rune(s)
		if len(runes) > maxLen {
			s = string(runes[:maxLen]) + "..."
			truncated = true
		}
	}
	return s, truncated
}

// FormatFailureReasonFirstLine extracts the first line of a failure reason for compact summary display,
// and returns whether the failure reason contains more details (such as a stack trace or additional lines/errors).
func FormatFailureReasonFirstLine(fr *rdbpb.FailureReason, maxLen int) (string, bool) {
	if fr == nil {
		return "", false
	}
	msg := fr.PrimaryErrorMessage
	hasMore := len(fr.Errors) > 1
	if len(fr.Errors) > 0 {
		if fr.Errors[0].Message != "" {
			msg = fr.Errors[0].Message
		}
		if fr.Errors[0].Trace != "" {
			hasMore = true
		}
	}
	if msg == "" && len(fr.Errors) > 0 && fr.Errors[0].Trace != "" {
		msg = fr.Errors[0].Trace
	}
	res, truncated := TruncateFirstLine(msg, maxLen)
	return res, truncated || hasMore
}

// artifactTypeSortOrder defines priority tiers for common debugging artifact
// types, matching Huckle ATI's getRecommendedDebugArtifacts / compareArtifacts
// ordering so the most useful diagnostic logs appear at the top.
var artifactTypeSortOrder = map[string]int{
	"LOGCAT":     0,
	"BUGREPORT":  1,
	"BUGREPORTZ": 1,
	"SCREENSHOT": 2,
	"HOST_LOG":   3,
}

const (
	otherTypedArtifactPriority = 100
	untypedArtifactPriority    = 101
)

func artifactDisplayName(art *rdbpb.Artifact) string {
	if id := art.GetArtifactId(); id != "" {
		return id
	}
	return art.GetName()
}

// effectiveArtifactTypeAndPriority returns the normalized artifact type and
// priority tier for sorting. When ArtifactType is empty, it falls back to
// filename-based inference matching Huckle ATI's getRecommendedDebugArtifacts.
func effectiveArtifactTypeAndPriority(art *rdbpb.Artifact) (string, int) {
	artType := strings.ToUpper(strings.TrimSpace(art.GetArtifactType()))
	if artType != "" {
		if p, ok := artifactTypeSortOrder[artType]; ok {
			return artType, p
		}
		return artType, otherTypedArtifactPriority
	}

	lowerName := strings.ToLower(artifactDisplayName(art))
	switch {
	case strings.Contains(lowerName, "logcat"):
		return "LOGCAT", artifactTypeSortOrder["LOGCAT"]
	case strings.Contains(lowerName, "bugreport"):
		return "BUGREPORT", artifactTypeSortOrder["BUGREPORT"]
	case strings.Contains(lowerName, "screenshot") && strings.HasSuffix(lowerName, ".png"):
		return "SCREENSHOT", artifactTypeSortOrder["SCREENSHOT"]
	case strings.Contains(lowerName, "host_log"):
		return "HOST_LOG", artifactTypeSortOrder["HOST_LOG"]
	default:
		return "", untypedArtifactPriority
	}
}

func isTestResultArtifact(art *rdbpb.Artifact) bool {
	return strings.Contains(art.GetName(), "/tests/")
}

// SortArtifacts sorts artifacts in place using Huckle ATI's priority ordering:
//  1. Priority artifact types first: LOGCAT (0) -> BUGREPORT/BUGREPORTZ (1) ->
//     SCREENSHOT (2) -> HOST_LOG (3) -> other typed artifacts (100, sorted
//     alphabetically by ArtifactType) -> untyped artifacts (101). Untyped
//     artifacts with recognizable names (logcat, bugreport, screenshot*.png,
//     host_log) are promoted to their corresponding priority tier.
//  2. Within the same type tier, test-result-scoped artifacts come before
//     work-unit-scoped artifacts (since test-level logs are more targeted).
//  3. Alphabetical by ArtifactId (case-insensitive, with case-sensitive and
//     resource Name tie-breakers for determinism).
func SortArtifacts(artifacts []*rdbpb.Artifact) {
	sort.SliceStable(artifacts, func(i, j int) bool {
		a, b := artifacts[i], artifacts[j]
		typeA, prioA := effectiveArtifactTypeAndPriority(a)
		typeB, prioB := effectiveArtifactTypeAndPriority(b)
		if prioA != prioB {
			return prioA < prioB
		}
		if prioA == otherTypedArtifactPriority && typeA != typeB {
			return typeA < typeB
		}
		trA, trB := isTestResultArtifact(a), isTestResultArtifact(b)
		if trA != trB {
			return trA
		}
		nameA, nameB := artifactDisplayName(a), artifactDisplayName(b)
		lowerA, lowerB := strings.ToLower(nameA), strings.ToLower(nameB)
		if lowerA != lowerB {
			return lowerA < lowerB
		}
		if nameA != nameB {
			return nameA < nameB
		}
		return a.GetName() < b.GetName()
	})
}

// FormatArtifactFetchHint returns a copy-pasteable CLI command template for
// fetching an artifact from the same parent or root invocation as sampleArtifactName.
// Returns "" if sampleArtifactName does not belong to a root invocation.
func FormatArtifactFetchHint(sampleArtifactName string, invocationWide bool) string {
	if !strings.HasPrefix(sampleArtifactName, "rootInvocations/") {
		return ""
	}
	meta := base.ExtractArtifactMetadata(sampleArtifactName)
	if meta.InvocationID == "" {
		return ""
	}
	quotedInv := base.ShellQuoteIfNeeded(meta.InvocationID)
	if invocationWide {
		return fmt.Sprintf("luci artifact <get|head|tail> -invocationid=%s -artifactid=<ARTIFACT_ID> [FLAGS]", quotedInv)
	}
	if meta.TestID != "" && meta.ResultID != "" {
		return fmt.Sprintf("luci test-result artifact <get|head|tail> -invocationid=%s -workunitid=%s -testid=%s -resultid=%s -artifactid=<ARTIFACT_ID>",
			quotedInv,
			base.ShellQuoteIfNeeded(meta.WorkUnitID),
			base.ShellQuoteIfNeeded(meta.TestID),
			base.ShellQuoteIfNeeded(meta.ResultID))
	}
	if meta.WorkUnitID != "" {
		return fmt.Sprintf("luci work-unit artifact <get|head|tail> -invocationid=%s -workunitid=%s -artifactid=<ARTIFACT_ID>",
			quotedInv,
			base.ShellQuoteIfNeeded(meta.WorkUnitID))
	}
	return ""
}

// PrintArtifactList prints a formatted 1-line-per-artifact table to stdout,
// sorted by Huckle ATI artifact priority.
func PrintArtifactList(artifacts []*rdbpb.Artifact) {
	PrintArtifactListTo(os.Stdout, artifacts)
}

// PrintArtifactListTo prints a formatted 1-line-per-artifact table to out,
// sorted by Huckle ATI artifact priority.
func PrintArtifactListTo(out io.Writer, artifacts []*rdbpb.Artifact) {
	printArtifactList(out, artifacts, false)
}

// PrintInvocationArtifactList prints an invocation-wide 1-line-per-artifact
// table to stdout, including copy-pasteable target flags for each row.
func PrintInvocationArtifactList(artifacts []*rdbpb.Artifact) {
	PrintInvocationArtifactListTo(os.Stdout, artifacts)
}

// PrintInvocationArtifactListTo prints an invocation-wide 1-line-per-artifact
// table to out, including copy-pasteable target flags for each row.
func PrintInvocationArtifactListTo(out io.Writer, artifacts []*rdbpb.Artifact) {
	printArtifactList(out, artifacts, true)
}

func printArtifactList(out io.Writer, artifacts []*rdbpb.Artifact, includeMetadata bool) {
	if len(artifacts) == 0 {
		fmt.Fprintf(out, "  (no artifacts)\n\n")
		return
	}

	sorted := make([]*rdbpb.Artifact, len(artifacts))
	copy(sorted, artifacts)
	SortArtifacts(sorted)

	tw := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	if includeMetadata {
		fmt.Fprintln(tw, "  ARTIFACT ID\tTYPE\tSIZE\tCONTENT-TYPE\tFLAGS")
	} else {
		fmt.Fprintln(tw, "  ARTIFACT ID\tTYPE\tSIZE\tCONTENT-TYPE")
	}
	for _, art := range sorted {
		artType := art.GetArtifactType()
		if artType == "" {
			artType = "-"
		}
		sizeStr := "-"
		if art.GetSizeBytes() > 0 {
			sizeStr = FormatCompactSize(art.GetSizeBytes())
		}
		contentType := art.GetContentType()
		if contentType == "" {
			contentType = "-"
		}
		if includeMetadata {
			meta := base.ExtractArtifactMetadata(art.GetName())
			flags := meta.FormatFlags()
			if flags == "" {
				flags = "-"
			}
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\t%s\n", art.GetArtifactId(), artType, sizeStr, contentType, flags)
		} else {
			fmt.Fprintf(tw, "  %s\t%s\t%s\t%s\n", art.GetArtifactId(), artType, sizeStr, contentType)
		}
	}
	_ = tw.Flush()
	fmt.Fprintln(out)

	if hint := FormatArtifactFetchHint(sorted[0].GetName(), includeMetadata); hint != "" {
		fmt.Fprintf(out, "To fetch an artifact:\n  %s\n\n", hint)
	}
}
