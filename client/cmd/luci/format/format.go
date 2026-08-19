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
	"regexp"
	"sort"
	"strings"

	"golang.org/x/net/html"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/errors"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

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
func FetchArtifact(ctx context.Context, rdbClient pb.ResultDBClient, httpClient *http.Client, artName string) ([]byte, error) {
	art, err := rdbClient.GetArtifact(ctx, &pb.GetArtifactRequest{Name: artName})
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

func PrintTestID(ctx context.Context, schemasClient pb.SchemasClient, res *pb.TestResult) {
	st := res.TestIdStructured
	if st != nil && st.ModuleScheme != "" && st.ModuleScheme != "legacy" {
		fmt.Println("Test ID:")
		scheme, err := schemasClient.GetScheme(ctx, &pb.GetSchemeRequest{Name: "schema/schemes/" + st.ModuleScheme})
		if err == nil && scheme != nil {
			modLabel := "Module"
			if scheme.HumanReadableName != "" {
				modLabel = fmt.Sprintf("Module (%s)", scheme.HumanReadableName)
			}
			fmt.Printf("  %s: %s\n", modLabel, st.ModuleName)
			if st.CoarseName != "" {
				label := "Coarse"
				if scheme.Coarse != nil && scheme.Coarse.HumanReadableName != "" {
					label = scheme.Coarse.HumanReadableName
				}
				fmt.Printf("  %s: %s\n", label, st.CoarseName)
			}
			if st.FineName != "" {
				label := "Fine"
				if scheme.Fine != nil && scheme.Fine.HumanReadableName != "" {
					label = scheme.Fine.HumanReadableName
				}
				fmt.Printf("  %s: %s\n", label, st.FineName)
			}
			if st.CaseName != "" {
				label := "Case"
				if scheme.Case != nil && scheme.Case.HumanReadableName != "" {
					label = scheme.Case.HumanReadableName
				}
				fmt.Printf("  %s: %s\n", label, st.CaseName)
			}
		} else {
			fmt.Printf("  Module:    %s (scheme: %s)\n", st.ModuleName, st.ModuleScheme)
			if st.CoarseName != "" {
				fmt.Printf("  Coarse:    %s\n", st.CoarseName)
			}
			if st.FineName != "" {
				fmt.Printf("  Fine:      %s\n", st.FineName)
			}
			if st.CaseName != "" {
				fmt.Printf("  Case:      %s\n", st.CaseName)
			}
		}
	} else {
		fmt.Printf("Test ID:   %s\n", res.TestId)
	}
}

func FormatSummaryHTML(ctx context.Context, rdbClient pb.ResultDBClient, httpClient *http.Client, resName, htmlStr string, showArtifacts bool) string {
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
		if !showArtifacts {
			return fmt.Sprintf("[Embedded Artifact: %s (pass --show-artifacts to view)]", artID)
		}
		isInvLevel := strings.Contains(attrs, "inv-level")
		var artName string
		if isInvLevel {
			idx := strings.Index(resName, "/tests/")
			if idx != -1 {
				artName = resName[:idx] + "/artifacts/" + artID
			} else {
				artName = resName + "/artifacts/" + artID
			}
		} else {
			artName = resName + "/artifacts/" + artID
		}
		body, err := FetchArtifact(ctx, rdbClient, httpClient, artName)
		if err != nil {
			return fmt.Sprintf("[Failed to fetch embedded artifact: %s]", artID)
		}
		return fmt.Sprintf("\n--- Embedded Artifact: %s ---\n%s\n--- End Artifact ---", artID, string(body))
	})
	htmlStr = strings.ReplaceAll(htmlStr, "<br>", "\n")
	htmlStr = strings.ReplaceAll(htmlStr, "<br/>", "\n")
	htmlStr = strings.ReplaceAll(htmlStr, "<br />", "\n")
	htmlStr = strings.ReplaceAll(htmlStr, "</p>", "\n")
	return StripHTML(htmlStr)
}

func PrintAdditionalMetadata(res *pb.TestResult) {
	if tm := res.TestMetadata; tm != nil {
		if loc := tm.Location; loc != nil {
			fmt.Printf("Location:  repo: %s, file: %s", loc.Repo, loc.FileName)
			if loc.Line > 0 {
				fmt.Printf(", line: %d", loc.Line)
			}
			fmt.Println()
		}
		if bc := tm.BugComponent; bc != nil {
			if bc.GetIssueTracker() != nil {
				fmt.Printf("Bug Component: issue tracker %d\n", bc.GetIssueTracker().ComponentId)
			} else if bc.GetMonorail() != nil {
				fmt.Printf("Bug Component: monorail %s/%s\n", bc.GetMonorail().Project, bc.GetMonorail().Value)
			}
		}
		if tm.PreviousTestId != "" {
			fmt.Printf("Prev ID:   %s\n", tm.PreviousTestId)
		}
		if tm.Properties != nil {
			out, _ := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(tm.Properties)
			fmt.Printf("Metadata Properties:\n%s\n", string(out))
		}
	}
	if len(res.Tags) > 0 {
		fmt.Println("Tags:")
		for _, tag := range res.Tags {
			fmt.Printf("  %s: %s\n", tag.Key, tag.Value)
		}
	}
	if res.Properties != nil {
		out, _ := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(res.Properties)
		fmt.Printf("Properties:\n%s\n", string(out))
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

func FormatVariant(v *pb.Variant) string {
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

// PrintFailureReason prints the failure reason error message(s) and stack trace(s).
func PrintFailureReason(fr *pb.FailureReason, indent string) {
	if fr == nil {
		return
	}
	if len(fr.Errors) == 0 {
		if fr.PrimaryErrorMessage != "" {
			fmt.Printf("%sError:     %s\n", indent, fr.PrimaryErrorMessage)
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
				fmt.Printf("%s%s\n", prefix, lines[0])
			} else {
				fmt.Printf("%s\n", strings.TrimRight(prefix, " "))
				for _, l := range lines {
					fmt.Printf("%s  %s\n", indent, l)
				}
			}
		}

		if errItem.Trace != "" {
			traceIndent := indent + "  "
			fmt.Printf("%sStack Trace:\n", indent)
			for _, l := range strings.Split(strings.TrimRight(errItem.Trace, "\n"), "\n") {
				fmt.Printf("%s%s\n", traceIndent, l)
			}
		}
	}
}

// TruncateFirstLine returns the first non-empty line of text, trimmed to maxLen characters with "..." if longer.
func TruncateFirstLine(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if idx := strings.Index(s, "\n"); idx != -1 {
		s = strings.TrimSpace(s[:idx])
	}
	if maxLen > 0 {
		runes := []rune(s)
		if len(runes) > maxLen {
			return string(runes[:maxLen]) + "..."
		}
	}
	return s
}

// FormatFailureReasonFirstLine extracts the first line of a failure reason for compact summary display.
func FormatFailureReasonFirstLine(fr *pb.FailureReason, maxLen int) string {
	if fr == nil {
		return ""
	}
	msg := fr.PrimaryErrorMessage
	if len(fr.Errors) > 0 && fr.Errors[0].Message != "" {
		msg = fr.Errors[0].Message
	}
	if msg == "" && len(fr.Errors) > 0 && fr.Errors[0].Trace != "" {
		msg = fr.Errors[0].Trace
	}
	return TruncateFirstLine(msg, maxLen)
}

// PrintIndentedArtifactList prints a formatted list of artifacts indented by the specified prefix.
func PrintIndentedArtifactList(indent string, artifacts []*pb.Artifact) {
	if len(artifacts) == 0 {
		fmt.Printf("%s  (no artifacts)\n\n", indent)
		return
	}
	for _, art := range artifacts {
		fmt.Printf("%s  - artifact_id: %s\n", indent, art.ArtifactId)
		if art.SizeBytes > 0 {
			fmt.Printf("%s    size:        %s\n", indent, FormatSize(art.SizeBytes))
		}
		if art.ContentType != "" {
			fmt.Printf("%s    type:        %s\n", indent, art.ContentType)
		}
		if art.ArtifactType != "" {
			fmt.Printf("%s    category:    %s\n", indent, art.ArtifactType)
		}
	}
	fmt.Println()
}

// PrintArtifactList prints a formatted list of artifacts.
func PrintArtifactList(artifacts []*pb.Artifact) {
	PrintIndentedArtifactList("", artifacts)
}
