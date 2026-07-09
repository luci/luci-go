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

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

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

func FormatSummaryHTML(ctx context.Context, rdbClient pb.ResultDBClient, httpClient *http.Client, resName, html string, showArtifacts bool) string {
	re := regexp.MustCompile(`<text-artifact\s+([^>]+)>`)
	html = re.ReplaceAllStringFunc(html, func(tag string) string {
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
		art, err := rdbClient.GetArtifact(ctx, &pb.GetArtifactRequest{Name: artName})
		if err != nil || art.FetchUrl == "" {
			return fmt.Sprintf("[Failed to fetch embedded artifact: %s]", artID)
		}
		resp, err := httpClient.Get(art.FetchUrl)
		if err != nil || resp.StatusCode != http.StatusOK {
			if resp != nil {
				resp.Body.Close()
			}
			return fmt.Sprintf("[Failed to download embedded artifact: %s]", artID)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Sprintf("[Failed to read embedded artifact: %s]", artID)
		}
		return fmt.Sprintf("\n--- Embedded Artifact: %s ---\n%s\n--- End Artifact ---", artID, string(body))
	})
	html = strings.ReplaceAll(html, "<br>", "\n")
	html = strings.ReplaceAll(html, "<br/>", "\n")
	html = strings.ReplaceAll(html, "<br />", "\n")
	html = strings.ReplaceAll(html, "</p>", "\n")
	return StripHTML(html)
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
