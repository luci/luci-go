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

package ids

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/verdict"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// ExtractedIDs holds all extracted resource identifiers.
type ExtractedIDs struct {
	InvocationID string `json:"invocation_id,omitempty"`
	WorkUnitID   string `json:"work_unit_id,omitempty"`
	TestID       string `json:"test_id,omitempty"`
	ResultID     string `json:"result_id,omitempty"`
	ArtifactID   string `json:"artifact_id,omitempty"`
	VariantHash  string `json:"variant_hash,omitempty"`
}

// IsEmpty returns true if no identifiers were extracted.
func (e *ExtractedIDs) IsEmpty() bool {
	return e.InvocationID == "" &&
		e.WorkUnitID == "" &&
		e.TestID == "" &&
		e.ResultID == "" &&
		e.ArtifactID == "" &&
		e.VariantHash == ""
}

// Cmd returns the subcommand for `luci ids`.
func Cmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "ids [-json] <target>",
		ShortDesc: "Extract resource IDs from a URL or resource name",
		LongDesc: "Parse a URL or resource name (including Milo URLs, ResultDB resource names, and AnTS / ATI URLs)\n" +
			"and extract the canonical IDs (-invocationid, -workunitid, -testid, -resultid, -artifactid, -varianthash)\n" +
			"for use with other commands.",
		CommandRun: func() subcommands.CommandRun {
			r := &idsRun{af: af}
			r.af.Register(&r.Flags)
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.BoolVar(&r.jsonOut, "json", false, "Output extracted IDs in JSON format")
			r.Flags.BoolVar(&r.legacy, "legacy", false, "Query as legacy invocation instead of root invocation")
			return r
		},
	}
}

type idsRun struct {
	subcommands.CommandRunBase
	af      *base.AuthFlags
	host    string
	jsonOut bool
	legacy  bool
}

func (r *idsRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			r.Flags.Usage()
			return 0
		}
	}
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: luci ids [-json] <url_or_resource_name>\n")
		return 1
	}

	target := strings.TrimSpace(args[0])
	ctx := cli.GetContext(a, r, env)

	var client pb.ResultDBClient
	if err := r.af.Parse(); err == nil {
		if c, _, _, errClient := r.af.NewResultDBClient(ctx, r.host); errClient == nil {
			client = c
		}
	}

	extracted, err := ExtractIDs(ctx, client, target, r.legacy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to extract IDs: %s\n", err)
		return 1
	}

	if extracted.IsEmpty() {
		fmt.Fprintf(os.Stderr, "no resource IDs could be extracted from %q\n", target)
		return 1
	}

	if r.jsonOut {
		data, err := json.MarshalIndent(extracted, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal JSON: %s\n", err)
			return 1
		}
		fmt.Println(string(data))
		return 0
	}

	// Aligned human-readable output
	if extracted.InvocationID != "" {
		fmt.Printf("Invocation ID: %s\n", extracted.InvocationID)
	}
	if extracted.WorkUnitID != "" {
		fmt.Printf("Work Unit ID:  %s\n", extracted.WorkUnitID)
	}
	if extracted.TestID != "" {
		fmt.Printf("Test ID:       %s\n", extracted.TestID)
	}
	if extracted.ResultID != "" {
		fmt.Printf("Result ID:     %s\n", extracted.ResultID)
	}
	if extracted.ArtifactID != "" {
		fmt.Printf("Artifact ID:   %s\n", extracted.ArtifactID)
	}
	if extracted.VariantHash != "" {
		fmt.Printf("Variant Hash:  %s\n", extracted.VariantHash)
	}

	return 0
}

// ExtractIDs parses target string and extracts all available resource IDs.
func ExtractIDs(ctx context.Context, client pb.ResultDBClient, raw string, legacy bool) (*ExtractedIDs, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, errors.New("empty target")
	}

	extracted := &ExtractedIDs{}

	// Check for query parameters (?artifact=..., ?result=...)
	if u, err := url.Parse(raw); err == nil {
		if art := u.Query().Get("artifact"); art != "" {
			extracted.ArtifactID = art
		}
		if res := u.Query().Get("result"); res != "" {
			extracted.ResultID = res
		}
	}

	// 1. Android Test Investigate (ATI) URL or AnTS TR ID
	if base.IsAntsURL(raw) || base.IsAntsTestResultID(raw) {
		trID := ""
		invID := ""
		if base.IsAntsTestResultID(raw) {
			trID = raw
		} else {
			invID, trID = base.ExtractAntsURLComponents(raw)
		}

		if trID != "" {
			info, err := base.ResolveAntsTestResult(ctx, trID)
			if err != nil {
				return nil, err
			}
			extracted.InvocationID = base.NormalizeInvocation(info.InvocationID)
			if info.WorkUnitID != "" {
				extracted.WorkUnitID = base.NormalizeWorkUnit(info.WorkUnitID)
			}

			// Query ResultDB to resolve canonical TestID, ResultID, VariantHash
			if client != nil && extracted.InvocationID != "" {
				results, _, errQuery := queryAntsResultDBVerdict(ctx, client, extracted.InvocationID, info)
				if errQuery == nil && len(results) > 0 {
					extracted.TestID = results[0].TestId
					extracted.VariantHash = results[0].VariantHash
					if extracted.ResultID == "" {
						extracted.ResultID = results[0].ResultId
					}
					if extracted.WorkUnitID == "" {
						_, wuID := base.ExtractWorkUnitComponents(results[0].Name)
						if wuID != "" {
							extracted.WorkUnitID = base.NormalizeWorkUnit(wuID)
						}
					}
				}
			}
			if extracted.TestID == "" && info.TestCase != "" && !strings.HasPrefix(info.TestCase, "#") {
				extracted.TestID = info.TestCase
			}
			return extracted, nil
		}

		if invID != "" {
			extracted.InvocationID = base.NormalizeInvocation(invID)
			return extracted, nil
		}
	}

	if base.IsAntsInvocationID(raw) {
		extracted.InvocationID = base.NormalizeInvocation(raw)
		return extracted, nil
	}

	if base.IsAntsWorkUnitID(raw) {
		extracted.WorkUnitID = base.NormalizeWorkUnit(raw)
		return extracted, nil
	}

	clean := base.TrimResourceURL(raw)

	// 2. Milo / Chromium structured URL (/modules/.../variants/.../cases/...)
	if strings.Contains(clean, "/modules/") || (strings.Contains(clean, "/variants/") && strings.Contains(clean, "/cases/")) {
		invName, variantHash, _, matchFunc, err := verdict.ParseVerdictName(clean)
		if err == nil {
			extracted.InvocationID = base.NormalizeInvocation(invName)
			extracted.VariantHash = variantHash

			// If ResultDB client available, resolve canonical TestID and ResultID
			if client != nil && extracted.InvocationID != "" {
				results, _, _, errQuery := verdict.QueryVerdictResultsAndExonerations(ctx, client, extracted.InvocationID, "", variantHash, legacy, 1000)
				if errQuery == nil {
					for _, tr := range results {
						if matchFunc == nil || matchFunc(tr) {
							extracted.TestID = tr.TestId
							if extracted.ResultID == "" {
								extracted.ResultID = tr.ResultId
							}
							break
						}
					}
				}
			}
			return extracted, nil
		}
	}

	// 3. Milo Build URL (https://ci.chromium.org/ui/b/<build_id> or .../builders/.../<build_id>)
	if idx := strings.Index(raw, "/ui/b/"); idx != -1 {
		after := raw[idx+len("/ui/b/"):]
		parts := strings.Split(after, "/")
		if len(parts) > 0 && parts[0] != "" {
			extracted.InvocationID = base.NormalizeInvocation(parts[0])
			return extracted, nil
		}
	}
	if idx := strings.Index(raw, "/builders/"); idx != -1 {
		after := raw[idx+len("/builders/"):]
		parts := strings.Split(after, "/")
		if len(parts) >= 3 && parts[len(parts)-1] != "" {
			extracted.InvocationID = base.NormalizeInvocation(parts[len(parts)-1])
			return extracted, nil
		}
	}

	// 4. Canonical artifact path: .../artifacts/<art_id>
	if artIdx := strings.Index(clean, "/artifacts/"); artIdx != -1 {
		artID := clean[artIdx+len("/artifacts/"):]
		if idx := strings.IndexAny(artID, "/?#"); idx != -1 {
			artID = artID[:idx]
		}
		if artID != "" {
			extracted.ArtifactID = artID
		}
		clean = clean[:artIdx]
	}

	// 5. Work unit path: rootInvocations/<root_inv>/workUnits/<wu_id>...
	if wuIdx := strings.Index(clean, "/workUnits/"); wuIdx != -1 {
		rootInv, wuID := base.ExtractWorkUnitComponents(clean)
		if rootInv != "" {
			extracted.InvocationID = base.NormalizeInvocation(rootInv)
		}
		if wuID != "" {
			extracted.WorkUnitID = base.NormalizeWorkUnit(wuID)
		}
	}

	// 6. Test result path: .../tests/<escaped_test_id>/results/<result_id>
	if strings.Contains(clean, "/tests/") && strings.Contains(clean, "/results/") {
		inv, testID, resultID := base.ExtractTestResultComponents(clean)
		if inv != "" {
			extracted.InvocationID = base.NormalizeInvocation(inv)
		}
		if testID != "" {
			extracted.TestID = testID
		}
		if resultID != "" {
			extracted.ResultID = resultID
		}
		return extracted, nil
	}

	// 7. Test path without results: .../tests/<escaped_test_id>
	if testIdx := strings.Index(clean, "/tests/"); testIdx != -1 {
		prefix := clean[:testIdx]
		after := clean[testIdx+len("/tests/"):]
		parts := strings.Split(after, "/")
		if len(parts) > 0 && parts[0] != "" {
			if parts[0] == "view" && (strings.Contains(prefix, "android-build.googleplex.com") || strings.Contains(prefix, "android-build.corp.google.com")) {
				// /builds/tests/view is an Android Build web endpoint, not a test ID.
				return extracted, nil
			}
			if unescaped, err := url.PathUnescape(parts[0]); err == nil {
				extracted.TestID = unescaped
			} else {
				extracted.TestID = parts[0]
			}
		}
		if idx := strings.Index(prefix, "/variants/"); idx != -1 {
			extracted.VariantHash = prefix[idx+len("/variants/"):]
			prefix = prefix[:idx]
		}
		extracted.InvocationID = base.NormalizeInvocation(prefix)
		return extracted, nil
	}

	// 8. Work unit only (already handled above if /workUnits/ was present)
	if extracted.WorkUnitID != "" {
		return extracted, nil
	}

	// 9. Root invocation / invocation only: rootInvocations/<inv> or invocations/<inv>
	if strings.HasPrefix(clean, "rootInvocations/") || strings.HasPrefix(clean, "invocations/") {
		extracted.InvocationID = base.NormalizeInvocation(clean)
		return extracted, nil
	}

	return extracted, nil
}

func queryAntsResultDBVerdict(ctx context.Context, client pb.ResultDBClient, rootInvID string, info *base.AntsTestResultInfo) ([]*pb.TestResult, []*pb.TestExoneration, error) {
	rootInvName := "rootInvocations/" + base.NormalizeInvocation(rootInvID)

	var filter string
	if info.ModuleName != "" {
		filter = fmt.Sprintf("test_id_structured.module_name = %q", info.ModuleName)
	}
	if info.MethodName != "" {
		if filter != "" {
			filter += " AND "
		}
		filter += fmt.Sprintf("test_id_structured.case_name = %q", info.MethodName)
	}

	matchFunc := func(tr *pb.TestResult) bool {
		if info.MethodName != "" {
			if tr.TestIdStructured != nil && tr.TestIdStructured.CaseName == info.MethodName && tr.TestIdStructured.ModuleName == info.ModuleName {
				return true
			}
			if strings.HasPrefix(tr.TestId, ":"+info.ModuleName+"!") && strings.HasSuffix(tr.TestId, "#"+info.MethodName) {
				return true
			}
			return false
		}
		if info.ModuleName != "" {
			if tr.TestIdStructured != nil && tr.TestIdStructured.ModuleName == info.ModuleName {
				return true
			}
			return strings.HasPrefix(tr.TestId, ":"+info.ModuleName+"!")
		}
		return true
	}

	var verdictResults []*pb.TestResult
	var exList []*pb.TestExoneration
	pageToken := ""

	for {
		req := &pb.QueryTestVerdictsRequest{
			Parent:    rootInvName,
			View:      pb.TestVerdictView_TEST_VERDICT_VIEW_FULL,
			PageSize:  1000,
			PageToken: pageToken,
			OrderBy:   "ui_priority",
		}
		if filter != "" {
			req.Predicate = &pb.TestVerdictPredicate{
				ContainsTestResultFilter: filter,
			}
		}
		res, err := client.QueryTestVerdicts(ctx, req)
		if err != nil {
			return nil, nil, errors.Fmt("QueryTestVerdicts RPC failed for %s: %w", rootInvName, err)
		}
		for _, tv := range res.TestVerdicts {
			matched := false
			for _, tr := range tv.Results {
				if tr.VariantHash == "" && tv.TestIdStructured != nil {
					tr.VariantHash = tv.TestIdStructured.ModuleVariantHash
				}
				if tr.TestId == "" {
					tr.TestId = tv.TestId
				}
				if tr.TestMetadata == nil {
					tr.TestMetadata = tv.TestMetadata
				}
				if matchFunc(tr) {
					verdictResults = append(verdictResults, tr)
					matched = true
				}
			}
			if matched {
				exList = append(exList, tv.Exonerations...)
			}
		}
		if len(verdictResults) > 0 || res.NextPageToken == "" {
			break
		}
		pageToken = res.NextPageToken
	}
	return verdictResults, exList, nil
}
