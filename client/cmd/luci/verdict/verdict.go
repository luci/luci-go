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
		UsageLine: "get <name>",
		ShortDesc: "Get a test verdict",
		LongDesc:  "Get details of an invocation test verdict by its resource name or UI URL path. Displays combined retry history and exonerations.",
		CommandRun: func() subcommands.CommandRun {
			r := &verdictGetRun{af: af}
			r.af.Register(&r.Flags)
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.BoolVar(&r.showArtifacts, "show-artifacts", false, "Print content of artifacts embedded in run summary_html")
			r.Flags.BoolVar(&r.showMetadata, "show-metadata", false, "Additionally print test metadata, tags, and properties")
			r.Flags.BoolVar(&r.legacy, "legacy", false, "Query as legacy invocation instead of root invocation")
			return r
		},
	}
}

type verdictGetRun struct {
	subcommands.CommandRunBase
	af            *base.AuthFlags
	host          string
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

// ResolveVerdictResults parses a target string (which may be a verdict URL, optionally containing
// a ?result=<id> parameter) and returns the verdict's test results and the specific matched test result if any.
func ResolveVerdictResults(ctx context.Context, client pb.ResultDBClient, target string, legacy bool) (results []*pb.TestResult, matched *pb.TestResult, err error) {
	clean := target
	resultID := ""
	if u, err := url.Parse(target); err == nil && u.Query().Get("result") != "" {
		resultID = u.Query().Get("result")
		q := u.Query()
		q.Del("result")
		u.RawQuery = q.Encode()
		clean = u.String()
	}

	clean = base.TrimResourceURL(clean)

	invName, variantHash, testIDRegexp, matchFunc, err := ParseVerdictName(clean)
	if err != nil {
		return nil, nil, err
	}

	results, _, err = QueryVerdictResultsAndExonerations(ctx, client, invName, variantHash, testIDRegexp, matchFunc, legacy)
	if err != nil {
		return nil, nil, err
	}
	if len(results) == 0 {
		return nil, nil, errors.Fmt("no results found for verdict %q", clean)
	}

	if resultID == "" {
		return results, results[0], nil
	}

	for _, tr := range results {
		if tr.ResultId == resultID {
			return results, tr, nil
		}
	}
	return nil, nil, errors.Fmt("result ID %q not found in verdict (%d results available)", resultID, len(results))
}

// QueryVerdictResultsAndExonerations queries test results and exonerations for a verdict.
// Defaults to querying as a root invocation using QueryTestVerdicts unless legacy is true.
func QueryVerdictResultsAndExonerations(ctx context.Context, client pb.ResultDBClient, invName, variantHash, testIDRegexp string, matchFunc func(*pb.TestResult) bool, legacy bool) ([]*pb.TestResult, []*pb.TestExoneration, error) {
	if legacy {
		return queryLegacyInvocationVerdicts(ctx, client, invName, variantHash, testIDRegexp, matchFunc)
	}
	return queryRootInvocationVerdicts(ctx, client, invName, variantHash, matchFunc)
}

func queryRootInvocationVerdicts(ctx context.Context, client pb.ResultDBClient, invName, variantHash string, matchFunc func(*pb.TestResult) bool) ([]*pb.TestResult, []*pb.TestExoneration, error) {
	rootInvName := invName
	if idx := strings.Index(rootInvName, "/workUnits/"); idx != -1 {
		rootInvName = rootInvName[:idx]
	}
	rootInvName = "rootInvocations/" + base.NormalizeInvocation(rootInvName)

	var verdictResults []*pb.TestResult
	var exList []*pb.TestExoneration
	pageToken := ""
	for {
		req := &pb.QueryTestVerdictsRequest{
			Parent:    rootInvName,
			View:      pb.TestVerdictView_TEST_VERDICT_VIEW_FULL,
			PageSize:  1000,
			PageToken: pageToken,
		}
		res, err := client.QueryTestVerdicts(ctx, req)
		if err != nil {
			return nil, nil, errors.Fmt("QueryTestVerdicts RPC failed for %s (if this is a legacy invocation, try with -legacy): %w", rootInvName, err)
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
		if res.NextPageToken == "" {
			break
		}
		pageToken = res.NextPageToken
	}
	return verdictResults, exList, nil
}

func queryLegacyInvocationVerdicts(ctx context.Context, client pb.ResultDBClient, invName, variantHash, testIDRegexp string, matchFunc func(*pb.TestResult) bool) ([]*pb.TestResult, []*pb.TestExoneration, error) {
	legacyInvName := "invocations/" + base.NormalizeInvocation(invName)

	var verdictResults []*pb.TestResult
	pageToken := ""
	for {
		req := &pb.QueryTestResultsRequest{
			Invocations: []string{legacyInvName},
			PageSize:    1000,
			PageToken:   pageToken,
		}
		if testIDRegexp != "" {
			req.Predicate = &pb.TestResultPredicate{
				TestIdRegexp: testIDRegexp,
			}
		}
		res, err := client.QueryTestResults(ctx, req)
		if err != nil {
			return nil, nil, errors.Fmt("QueryTestResults RPC failed for %s: %w", legacyInvName, err)
		}
		for _, tr := range res.TestResults {
			if matchFunc(tr) {
				verdictResults = append(verdictResults, tr)
			}
		}
		if res.NextPageToken == "" {
			break
		}
		pageToken = res.NextPageToken
	}

	var exList []*pb.TestExoneration
	pageToken = ""
	for {
		reqEx := &pb.QueryTestExonerationsRequest{
			Invocations: []string{legacyInvName},
			PageSize:    1000,
			PageToken:   pageToken,
		}
		if testIDRegexp != "" {
			reqEx.Predicate = &pb.TestExonerationPredicate{
				TestIdRegexp: testIDRegexp,
			}
		}
		resEx, err := client.QueryTestExonerations(ctx, reqEx)
		if err != nil || resEx == nil {
			break
		}
		for _, ex := range resEx.TestExonerations {
			if ex.VariantHash == variantHash {
				exList = append(exList, ex)
			}
		}
		if resEx.NextPageToken == "" {
			break
		}
		pageToken = resEx.NextPageToken
	}

	return verdictResults, exList, nil
}

func (r *verdictGetRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			r.Flags.Usage()
			return 0
		}
	}
	ctx := cli.GetContext(a, r, env)
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: luci verdict get <name>\n")
		return 1
	}
	name := strings.TrimSpace(args[0])
	if name == "-" {
		cd, err := base.LoadCache()
		if err == nil && cd != nil && cd.Verdict != "" {
			name = cd.Verdict
		} else {
			fmt.Fprintf(os.Stderr, "no previous verdict found in cache; please specify a verdict name or URL\n")
			return 1
		}
	}

	invName, variantHash, testIDRegexp, matchFunc, err := ParseVerdictName(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse verdict name %q: %s\n", name, err)
		return 1
	}
	base.RecordVerdict(invName, name)

	if err := r.af.Parse(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
		return 1
	}

	ctx = format.WithDiscoveryCache(ctx)
	client, schemasClient, httpClient, err := r.af.NewResultDBClient(ctx, r.host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create resultdb client: %s\n", err)
		return 1
	}

	verdictResults, exList, err := QueryVerdictResultsAndExonerations(ctx, client, invName, variantHash, testIDRegexp, matchFunc, r.legacy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to query verdict: %s\n", err)
		return 1
	}
	if len(verdictResults) == 0 {
		fmt.Fprintf(os.Stderr, "no results found for verdict %q\n", name)
		return 1
	}

	vg := &VerdictGroup{
		Key:          VerdictKey{TestID: verdictResults[0].TestId, VariantHash: variantHash},
		Variant:      verdictResults[0].Variant,
		Results:      verdictResults,
		Exonerations: exList,
	}

	printVerdictSummary(ctx, schemasClient, client, httpClient, name, vg, r.showArtifacts, r.showMetadata)
	return 0
}

func printVerdictSummary(ctx context.Context, schemasClient pb.SchemasClient, rdbClient pb.ResultDBClient, httpClient *http.Client, name string, g *VerdictGroup, showArtifacts, showMetadata bool) {
	status := g.DisplayStatus()

	fmt.Printf("Verdict Name: %s\n", name)
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

	fmt.Printf("Runs (%d):\n", len(g.Results))
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
