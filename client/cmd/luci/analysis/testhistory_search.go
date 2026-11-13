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

package analysis

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	analysispb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/resultdb/pbutil"
)

const (
	maxTestSubstringLength = 512
	maxMatchingTestsShown  = 20
)

type testSubstringSearchResult struct {
	TestIDs   []string
	Truncated bool
}

// normalizeSubRealm strips an optional "<project>:" prefix from subRealmFlag so it
// matches LUCI Analysis's bare sub_realm expectation across QueryTests, QueryVariants,
// QueryStats, and Query.
func normalizeSubRealm(project, subRealmFlag string) string {
	subRealm := strings.TrimSpace(subRealmFlag)
	if idx := strings.IndexByte(subRealm, ':'); idx >= 0 && subRealm[:idx] == project {
		subRealm = subRealm[idx+1:]
	}
	return subRealm
}

// validateTestIDSubstring checks that val is a non-empty, <= 512-byte printable UTF-8 NFC
// string accepted by LUCI Analysis's TestHistory.QueryTests RPC.
func validateTestIDSubstring(flagName, val string) error {
	if val == "" {
		return errors.Fmt("%s cannot be empty", flagName)
	}
	if err := pbutil.ValidateUTF8Printable(val, maxTestSubstringLength, pbutil.ValidationModeLoose); err != nil {
		return errors.Fmt("invalid %s: %w", flagName, err)
	}
	return nil
}

// queryMatchingTests searches LUCI Analysis's 90-day test_realms index via TestHistory.QueryTests,
// retrying once with CaseInsensitive: true if an exact-case query returns zero matches.
func queryMatchingTests(ctx context.Context, client analysispb.TestHistoryClient, project, subRealm, substring string, caseInsensitive bool) (*testSubstringSearchResult, error) {
	req := &analysispb.QueryTestsRequest{
		Project:         project,
		TestIdSubstring: substring,
		SubRealm:        subRealm,
		PageSize:        testHistoryPageSize,
		CaseInsensitive: caseInsensitive,
	}
	resp, err := client.QueryTests(ctx, req)
	if err != nil {
		return nil, errors.Fmt("TestHistory.QueryTests RPC failed for %s / %q: %w", project, substring, err)
	}
	if len(resp.GetTestIds()) == 0 && !caseInsensitive {
		req.CaseInsensitive = true
		retryResp, retryErr := client.QueryTests(ctx, req)
		if retryErr != nil {
			return nil, errors.Fmt("TestHistory.QueryTests RPC failed for %s / %q: %w", project, substring, retryErr)
		}
		if len(retryResp.GetTestIds()) > 0 {
			resp = retryResp
		}
	}
	return &testSubstringSearchResult{
		TestIDs:   resp.GetTestIds(),
		Truncated: resp.GetNextPageToken() != "",
	}, nil
}

// rankAndResolveSubstringCandidates deduplicates and ranks candidate test IDs (structured ':'
// IDs before legacy IDs, then alphabetical) and returns an auto-resolved canonical test ID when
// there is a single unique match and the response is not page-truncated.
func rankAndResolveSubstringCandidates(candidates []string, truncated bool) (resolvedID string, ranked []string) {
	if len(candidates) == 0 {
		return "", nil
	}
	seen := make(map[string]struct{}, len(candidates))
	deduped := make([]string, 0, len(candidates))
	for _, id := range candidates {
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		deduped = append(deduped, id)
	}
	if len(deduped) == 0 {
		return "", nil
	}

	sort.Slice(deduped, func(i, j int) bool {
		si, sj := pbutil.IsStructuredTestID(deduped[i]), pbutil.IsStructuredTestID(deduped[j])
		if si != sj {
			return si
		}
		return deduped[i] < deduped[j]
	})

	if !truncated && len(deduped) == 1 {
		return deduped[0], deduped
	}
	return "", deduped
}

func formatTestHistoryReRunCommand(
	project, testID, subRealm string,
	variants []string,
	daysExplicit bool,
	days int,
	limitExplicit bool,
	limit int,
) string {
	parts := []string{"luci", "analysis", "test-history", "-project", base.ShellQuoteIfNeeded(project)}
	if subRealm != "" {
		parts = append(parts, "-subrealm", base.ShellQuoteIfNeeded(subRealm))
	}
	for _, v := range variants {
		parts = append(parts, "-variant", base.ShellQuoteIfNeeded(v))
	}
	if daysExplicit {
		parts = append(parts, "-days", fmt.Sprintf("%d", days))
	}
	if limitExplicit {
		parts = append(parts, "-limit", fmt.Sprintf("%d", limit))
	}
	parts = append(parts, "-testid", base.ShellQuoteIfNeeded(testID))
	return strings.Join(parts, " ")
}

func printSubstringSearchHeader(out io.Writer, project, subRealm, query string) {
	fmt.Fprintf(out, "Project:   %s\n", project)
	if subRealm != "" {
		fmt.Fprintf(out, "SubRealm:  %s\n", subRealm)
	}
	fmt.Fprintf(out, "Query:     %q\n", query)
}

func printNoSubstringMatches(out io.Writer, project, subRealm, query string) {
	printSubstringSearchHeader(out, project, subRealm, query)
	fmt.Fprintf(out, "\nNo matching test IDs found in the last 90 days.\n")
}

func printSubstringDisambiguation(
	out io.Writer,
	project, subRealm, query string,
	ranked []string,
	truncated bool,
	variants []string,
	daysExplicit bool,
	days int,
	limitExplicit bool,
	limit int,
) {
	if len(ranked) == 0 {
		return
	}
	printSubstringSearchHeader(out, project, subRealm, query)

	countDesc := fmt.Sprintf("%d", len(ranked))
	if truncated {
		countDesc = fmt.Sprintf("%d+, page cap reached", len(ranked))
	}
	fmt.Fprintf(out, "\nMultiple matching test IDs found (%s); specify exact -testid to view history:\n", countDesc)

	shown := ranked
	extra := 0
	if len(shown) > maxMatchingTestsShown {
		extra = len(shown) - maxMatchingTestsShown
		shown = shown[:maxMatchingTestsShown]
	}
	for _, id := range shown {
		fmt.Fprintf(out, "- %s\n", id)
	}
	if extra > 0 {
		fmt.Fprintf(out, "  ... +%d more matching test IDs (pass a more specific -test-substring to narrow)\n", extra)
	}

	nextCmd := formatTestHistoryReRunCommand(project, ranked[0], subRealm, variants, daysExplicit, days, limitExplicit, limit)
	fmt.Fprintf(out, "\nNext step:\n  %s\n", nextCmd)
}
