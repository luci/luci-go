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

package verdict

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func Cmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "verdict <subcommand>",
		ShortDesc: "Manage invocation test verdicts (combined variant outcomes)",
		LongDesc: "Manage invocation test verdicts. An 'invocation test verdict' (or simply test verdict) represents the combined outcome of all test results (retries) for a test variant within an invocation, evaluated alongside any test exonerations. For individual execution attempts, see 'luci test-result'.\n\n" +
			"Available subcommands:\n" +
			"  get   Get details of an invocation test verdict (combined runs and exonerations)",
		CommandRun: func() subcommands.CommandRun {
			return &verdictRun{af: af}
		},
	}
}

type verdictRun struct {
	subcommands.CommandRunBase
	af *base.AuthFlags
}

func (r *verdictRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return base.RunSubcommandApp(a, "luci verdict", "Test verdicts management", []*subcommands.Command{
		GetCmd(r.af),
		subcommands.CmdHelp,
	}, args)
}

func GetCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get -invocationid <invocation_id> -testid <test_id> [-varianthash <hash>]",
		ShortDesc: "Get a test verdict",
		LongDesc:  "Get details of an invocation test verdict by its invocation ID and test ID. Displays combined retry history and exonerations.",
		CommandRun: func() subcommands.CommandRun {
			r := &verdictGetRun{af: af, maxResults: 10}
			r.af.Register(&r.Flags)
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.StringVar(&r.invocationID, "invocationid", "", "Invocation ID (e.g. build-867... or ants-i...)")
			r.Flags.StringVar(&r.testID, "testid", "", "Test ID (e.g. :module!junit:pkg.Class#Method)")
			r.Flags.StringVar(&r.variantHash, "varianthash", "", "Variant hash (optional, e.g. b7de9035241e76cc)")
			r.Flags.IntVar(&r.maxResults, "max-results", 10, "Maximum number of results to display (default: 10, 0 for all)")
			r.Flags.BoolVar(&r.allResults, "all", false, "Show all results without truncation")
			r.Flags.BoolVar(&r.showArtifacts, "show-artifacts", false, "Print content of artifacts embedded in run summary_html")
			r.Flags.BoolVar(&r.showArtifacts, "artifacts", false, "Alias for -show-artifacts")
			r.Flags.BoolVar(&r.showMetadata, "show-metadata", false, "Additionally print test metadata, tags, and properties")
			r.Flags.BoolVar(&r.showMetadata, "metadata", false, "Alias for -show-metadata")
			r.Flags.BoolVar(&r.legacy, "legacy", false, "Query as legacy invocation instead of root invocation")
			return r
		},
	}
}

type verdictGetRun struct {
	subcommands.CommandRunBase
	af            *base.AuthFlags
	host          string
	invocationID  string
	testID        string
	variantHash   string
	maxResults    int
	allResults    bool
	showArtifacts bool
	showMetadata  bool
	legacy        bool
}

func ParseVerdictName(name string) (invName, variantHash, testIDRegexp string, matchFunc func(*pb.TestResult) bool, err error) {
	name = base.TrimResourceURL(name)

	varIdx := strings.Index(name, "/variants/")
	if varIdx == -1 {
		return "", "", "", nil, errors.New("invalid verdict name: missing /variants/")
	}

	afterVar := name[varIdx+len("/variants/"):]
	parts := strings.SplitN(afterVar, "/", 2)
	variantHash = parts[0]

	beforeVar := name[:varIdx]
	if modIdx := strings.Index(beforeVar, "/modules/"); modIdx != -1 {
		invName = beforeVar[:modIdx]
		caseIdx := strings.Index(name, "/cases/")
		if caseIdx != -1 {
			afterCase := name[caseIdx+len("/cases/"):]
			caseParts := strings.Split(afterCase, "/")
			caseName, err := url.PathUnescape(caseParts[0])
			if err != nil {
				return "", "", "", nil, errors.Fmt("failed to unescape case name %q: %w", caseParts[0], err)
			}
			testIDRegexp = ".*" + regexp.QuoteMeta(caseName)
			matchFunc = func(tr *pb.TestResult) bool {
				if tr.VariantHash != variantHash {
					return false
				}
				if tr.TestIdStructured != nil && tr.TestIdStructured.CaseName == caseName {
					return true
				}
				return strings.HasSuffix(tr.TestId, caseName)
			}
			return invName, variantHash, testIDRegexp, matchFunc, nil
		}
	}

	if testIdx := strings.Index(beforeVar, "/tests/"); testIdx != -1 {
		invName = beforeVar[:testIdx]
		encodedTestID := beforeVar[testIdx+len("/tests/"):]
		testID, err := url.PathUnescape(encodedTestID)
		if err != nil {
			return "", "", "", nil, errors.Fmt("failed to unescape test ID %q: %w", encodedTestID, err)
		}
		testIDRegexp = regexp.QuoteMeta(testID)
		matchFunc = func(tr *pb.TestResult) bool {
			return tr.VariantHash == variantHash && tr.TestId == testID
		}
		return invName, variantHash, testIDRegexp, matchFunc, nil
	}

	return "", "", "", nil, errors.New("invalid verdict name format: could not parse test ID or case name")
}

// QueryVerdictResultsAndExonerations queries test results and exonerations for a verdict by invocation ID and test ID.
// Defaults to querying as a root invocation using QueryTestVerdicts unless legacy is true.
func QueryVerdictResultsAndExonerations(ctx context.Context, client pb.ResultDBClient, invID, testID, variantHash string, legacy bool, maxFetch int) ([]*pb.TestResult, []*pb.TestExoneration, bool, error) {
	if legacy {
		return queryLegacyInvocationVerdicts(ctx, client, invID, testID, variantHash, maxFetch)
	}
	return queryRootInvocationVerdicts(ctx, client, invID, testID, variantHash, maxFetch)
}

func queryRootInvocationVerdicts(ctx context.Context, client pb.ResultDBClient, invID, testID, variantHash string, maxFetch int) ([]*pb.TestResult, []*pb.TestExoneration, bool, error) {
	rootInvName := "rootInvocations/" + base.NormalizeInvocation(invID)

	var verdictResults []*pb.TestResult
	var exList []*pb.TestExoneration
	pageToken := ""
	hasMore := false

	for {
		pageSize := int32(1000)
		if maxFetch > 0 && int32(maxFetch) < pageSize {
			pageSize = int32(maxFetch)
		}
		req := &pb.QueryTestVerdictsRequest{
			Parent:    rootInvName,
			View:      pb.TestVerdictView_TEST_VERDICT_VIEW_FULL,
			PageSize:  pageSize,
			PageToken: pageToken,
			OrderBy:   "ui_priority",
		}
		if testID != "" {
			req.Predicate = &pb.TestVerdictPredicate{
				ContainsTestResultFilter: fmt.Sprintf("test_id = %q", testID),
			}
		}
		res, err := client.QueryTestVerdicts(ctx, req)
		if err != nil {
			return nil, nil, false, errors.Fmt("QueryTestVerdicts RPC failed for %s (if this is a legacy invocation, try with -legacy): %w", rootInvName, err)
		}
		for _, tv := range res.TestVerdicts {
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
				if testID != "" && tr.TestId != testID {
					continue
				}
				if variantHash != "" && tr.VariantHash != variantHash {
					continue
				}
				verdictResults = append(verdictResults, tr)
			}
			for _, ex := range tv.Exonerations {
				if ex.VariantHash == "" && tv.TestIdStructured != nil {
					ex.VariantHash = tv.TestIdStructured.ModuleVariantHash
				}
				if ex.TestId == "" {
					ex.TestId = tv.TestId
				}
				if testID != "" && ex.TestId != testID {
					continue
				}
				if variantHash != "" && ex.VariantHash != variantHash {
					continue
				}
				exList = append(exList, ex)
			}
		}
		if maxFetch > 0 && len(verdictResults) >= maxFetch {
			if res.NextPageToken != "" || len(verdictResults) > maxFetch {
				hasMore = true
				verdictResults = verdictResults[:maxFetch]
			}
			break
		}
		if res.NextPageToken == "" {
			break
		}
		pageToken = res.NextPageToken
	}
	return verdictResults, exList, hasMore, nil
}

func queryLegacyInvocationVerdicts(ctx context.Context, client pb.ResultDBClient, invID, testID, variantHash string, maxFetch int) ([]*pb.TestResult, []*pb.TestExoneration, bool, error) {
	legacyInvName := "invocations/" + base.NormalizeInvocation(invID)

	var verdictResults []*pb.TestResult
	pageToken := ""
	hasMore := false

	for {
		pageSize := int32(1000)
		if maxFetch > 0 && int32(maxFetch) < pageSize {
			pageSize = int32(maxFetch)
		}
		req := &pb.QueryTestResultsRequest{
			Invocations: []string{legacyInvName},
			PageSize:    pageSize,
			PageToken:   pageToken,
		}
		if testID != "" {
			req.Predicate = &pb.TestResultPredicate{
				TestIdRegexp: "^" + regexp.QuoteMeta(testID) + "$",
			}
			if variantHash != "" {
				req.Predicate.Variant = &pb.VariantPredicate{
					Predicate: &pb.VariantPredicate_Contains{
						Contains: &pb.Variant{
							Def: map[string]string{},
						},
					},
				}
			}
		}
		res, err := client.QueryTestResults(ctx, req)
		if err != nil {
			return nil, nil, false, errors.Fmt("QueryTestResults RPC failed for %s: %w", legacyInvName, err)
		}
		for _, tr := range res.TestResults {
			if testID != "" && tr.TestId != testID {
				continue
			}
			if variantHash != "" && tr.VariantHash != variantHash {
				continue
			}
			verdictResults = append(verdictResults, tr)
		}
		if maxFetch > 0 && len(verdictResults) >= maxFetch {
			if res.NextPageToken != "" || len(verdictResults) > maxFetch {
				hasMore = true
				verdictResults = verdictResults[:maxFetch]
			}
			break
		}
		if res.NextPageToken == "" {
			break
		}
		pageToken = res.NextPageToken
	}

	var exList []*pb.TestExoneration
	reqEx := &pb.QueryTestExonerationsRequest{
		Invocations: []string{legacyInvName},
		PageSize:    1000,
	}
	if testID != "" {
		reqEx.Predicate = &pb.TestExonerationPredicate{
			TestIdRegexp: "^" + regexp.QuoteMeta(testID) + "$",
		}
	}
	resEx, err := client.QueryTestExonerations(ctx, reqEx)
	if err == nil && resEx != nil {
		for _, ex := range resEx.TestExonerations {
			if testID != "" && ex.TestId != testID {
				continue
			}
			if variantHash != "" && ex.VariantHash != variantHash {
				continue
			}
			exList = append(exList, ex)
		}
	}

	return verdictResults, exList, hasMore, nil
}

func (r *verdictGetRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			r.Flags.Usage()
			return 0
		}
	}
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "unexpected positional arguments; use flags -invocationid and -testid (run 'luci ids <url>' to extract ids)\n")
		return 1
	}
	if r.invocationID == "" || r.testID == "" {
		fmt.Fprintf(os.Stderr, "flags -invocationid and -testid are required (run 'luci ids <url>' to extract ids)\n")
		return 1
	}

	if err := r.af.Parse(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
		return 1
	}

	ctx := cli.GetContext(a, r, env)
	ctx = format.WithDiscoveryCache(ctx)
	client, schemasClient, httpClient, err := r.af.NewResultDBClient(ctx, r.host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create resultdb client: %s\n", err)
		return 1
	}

	maxFetch := 0
	if !r.allResults && r.maxResults > 0 {
		maxFetch = r.maxResults
	}

	verdictResults, exList, hasMore, err := QueryVerdictResultsAndExonerations(ctx, client, r.invocationID, r.testID, r.variantHash, r.legacy, maxFetch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to query verdict: %s\n", err)
		return 1
	}
	if len(verdictResults) == 0 {
		fmt.Fprintf(os.Stderr, "no results found for test %q in invocation %q\n", r.testID, r.invocationID)
		return 1
	}

	vg := &VerdictGroup{
		Key:          VerdictKey{TestID: verdictResults[0].TestId, VariantHash: r.variantHash},
		Variant:      verdictResults[0].Variant,
		Results:      verdictResults,
		Exonerations: exList,
		HasMore:      hasMore,
	}

	printVerdictSummary(ctx, schemasClient, client, httpClient, r.invocationID, vg, r.showArtifacts, r.showMetadata)
	return 0
}

func printVerdictSummary(ctx context.Context, schemasClient pb.SchemasClient, rdbClient pb.ResultDBClient, httpClient *http.Client, invID string, g *VerdictGroup, showArtifacts, showMetadata bool) {
	status := g.DisplayStatus()

	fmt.Printf("Invocation:   %s\n", invID)
	if len(g.Results) > 0 {
		format.PrintTestID(ctx, schemasClient, g.Results[0])
		if g.Results[0].TestMetadata != nil && g.Results[0].TestMetadata.Name != "" {
			fmt.Printf("Test Name:    %s\n", g.Results[0].TestMetadata.Name)
		}
	}
	fmt.Printf("Status:       %s\n", status)
	if g.Variant != nil && len(g.Variant.GetDef()) > 0 {
		fmt.Printf("Variant:      %s\n", format.FormatVariant(g.Variant))
	}
	if len(g.Exonerations) > 0 {
		fmt.Printf("Exonerations:\n")
		for _, ex := range g.Exonerations {
			reasonStr := format.StripHTML(ex.ExplanationHtml)
			if reasonStr == "" {
				reasonStr = "No explanation provided"
			}
			fmt.Printf("  - Reason: %s [%s]\n", reasonStr, ex.Reason)
		}
	}
	type groupedRuns struct {
		parent format.ParentGroup
		items  []*pb.TestResult
	}
	var groups []groupedRuns
	groupMap := make(map[string]int)

	for _, tr := range g.Results {
		pg := format.GetParentGroup(tr.Name)
		idx, found := groupMap[pg.ID]
		if !found {
			idx = len(groups)
			groupMap[pg.ID] = idx
			groups = append(groups, groupedRuns{parent: pg})
		}
		groups[idx].items = append(groups[idx].items, tr)
	}

	if g.HasMore {
		fmt.Printf("Runs (showing %d results, use -all to see all):\n", len(g.Results))
	} else {
		fmt.Printf("Runs (%d):\n", len(g.Results))
	}
	for _, grp := range groups {
		runCountStr := ""
		if len(grp.items) == 1 {
			runCountStr = "1 run"
		} else {
			runCountStr = fmt.Sprintf("%d runs", len(grp.items))
		}
		fmt.Printf("- %s (%s):\n", grp.parent.Label, runCountStr)
		if grp.parent.ID != "" {
			modErr, _ := format.DiscoverWorkUnitError(ctx, rdbClient, httpClient, grp.parent.ID)
			if modErr != nil {
				firstLine, truncated := format.FormatDiscoveredErrorFirstLine(modErr, 120)
				if firstLine != "" {
					label := "Module Error:"
					if truncated {
						label = "Module Error (truncated):"
					}
					fmt.Printf("  %s %s\n", label, firstLine)
				}
			}
		}
		for _, tr := range grp.items {
			fmt.Printf("  - %s (%s) - result_id: %s\n", tr.StatusV2, format.FormatDuration(tr.Duration), tr.ResultId)
			if tr.FailureReason != nil {
				firstLine, truncated := format.FormatFailureReasonFirstLine(tr.FailureReason, 120)
				if firstLine != "" {
					label := "Error:"
					if truncated {
						label = "Error (truncated):"
					}
					fmt.Printf("    %s %s\n", label, firstLine)
				}
			}
			if tr.SummaryHtml != "" {
				if showArtifacts {
					summaryText := format.FormatSummaryHTML(ctx, rdbClient, httpClient, tr.Name, tr.SummaryHtml, showArtifacts)
					fmt.Printf("    Summary:\n")
					for _, l := range strings.Split(strings.TrimRight(summaryText, "\n"), "\n") {
						fmt.Printf("      %s\n", l)
					}
				} else {
					firstLine, truncated := format.TruncateFirstLine(format.StripHTML(tr.SummaryHtml), 120)
					if firstLine != "" {
						label := "Summary:"
						if truncated {
							label = "Summary (truncated):"
						}
						fmt.Printf("    %s %s\n", label, firstLine)
					}
				}
			}
		}
	}
	if showMetadata && len(g.Results) > 0 {
		fmt.Println()
		format.PrintAdditionalMetadata(g.Results[0])
	}
}
