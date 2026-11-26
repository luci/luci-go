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
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/client/cmd/luci/ids"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func Cmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "verdict <subcommand>",
		ShortDesc: "Manage invocation test verdicts (combined variant outcomes)",
		LongDesc: "Manage invocation test verdicts. An 'invocation test verdict' (or simply test verdict) represents the combined outcome of all test results (retries) for a test variant within an invocation, evaluated alongside any test exonerations. For individual execution attempts, see 'luci test-result'.\n\n" +
			"Available subcommands:\n" +
			"  get   Get details of an invocation test verdict (combined runs and exonerations)\n" +
			"  list  List test verdicts in an invocation",
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
		ListCmd(r.af),
		subcommands.CmdHelp,
	}, args)
}

const (
	defaultMaxVerdictResults  = 10
	verdictRunCountFetchLimit = 1000
	expectedResultRetention   = 60 * 24 * time.Hour
)

func formatExpiredExpectedResultsNotice(invID string, resolved *base.ResolvedInvocation) string {
	if resolved == nil {
		return ""
	}
	var createTime time.Time
	if resolved.Legacy {
		if ts := resolved.Invocation.GetCreateTime(); ts != nil {
			createTime = ts.AsTime()
		}
	} else {
		if ts := resolved.RootInvocation.GetCreateTime(); ts != nil {
			createTime = ts.AsTime()
		}
	}
	if createTime.IsZero() {
		return ""
	}
	age := time.Since(createTime)
	if age <= expectedResultRetention {
		return ""
	}
	days := int(age / (24 * time.Hour))
	return fmt.Sprintf("Note: Invocation %q is %d days old; ResultDB retains expected/passing test results for 60 days (use 'luci analysis test-history' to check longer-term pass history).\n", invID, days)
}

func GetCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get [-invocationid <invocation_id> -testid <test_id>] [-varianthash <hash>] [<target>]",
		ShortDesc: "Get a test verdict",
		LongDesc: "Get details of an invocation test verdict by a positional URL/ID or by its invocation ID and test ID. Displays combined retry history and exonerations.\n\n" +
			"Examples:\n" +
			"  $ luci verdict get https://ci.chromium.org/ui/test-investigate/invocations/build-123/tests/ninja:%2F%2Fchrome%2Ftest/variants/b7de9035241e76cc\n" +
			"  $ luci verdict get -invocationid build-123 -testid ninja://chrome/test",
		CommandRun: func() subcommands.CommandRun {
			r := &verdictGetRun{af: af, maxResults: defaultMaxVerdictResults}
			if r.af != nil {
				r.af.Register(&r.Flags)
			}
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.StringVar(&r.invocationID, "invocationid", "", "Invocation ID (e.g. build-867... or ants-i...)")
			r.Flags.StringVar(&r.invocationID, "invocation-id", "", "Alias for -invocationid")
			r.Flags.StringVar(&r.testID, "testid", "", "Test ID (e.g. :module!junit:pkg.Class#Method)")
			r.Flags.StringVar(&r.testID, "test-id", "", "Alias for -testid")
			r.Flags.StringVar(&r.variantHash, "varianthash", "", "Variant hash (optional, e.g. b7de9035241e76cc)")
			r.Flags.StringVar(&r.variantHash, "variant-hash", "", "Alias for -varianthash")
			r.Flags.IntVar(&r.maxResults, "max-results", defaultMaxVerdictResults, "Maximum number of results to display (default: 10, 0 for all; setting explicitly bypasses 9.5 KB cap)")
			r.Flags.IntVar(&r.maxResults, "limit", defaultMaxVerdictResults, "Alias for -max-results")
			r.Flags.IntVar(&r.maxResults, "n", defaultMaxVerdictResults, "Alias for -max-results")
			r.Flags.BoolVar(&r.allResults, "all", false, "Show all results without truncation")
			r.Flags.BoolVar(&r.showMetadata, "show-metadata", false, "Additionally print test metadata, tags, and properties")
			// -legacy is now unused and will be removed in the future.
			r.Flags.BoolVar(&r.legacy, "legacy", false, "Query as legacy invocation instead of root invocation")
			return r
		},
	}
}

type verdictGetRun struct {
	subcommands.CommandRunBase
	af           *base.AuthFlags
	client       pb.ResultDBClient
	out          io.Writer
	errOut       io.Writer
	host         string
	invocationID string
	testID       string
	variantHash  string
	maxResults   int
	allResults   bool
	showMetadata bool
	legacy       bool // Unused; will be removed in the future.
}

func ParseVerdictName(name string) (invName, variantHash, testIDRegexp string, matchFunc func(*pb.TestResult) bool, err error) {
	return base.ParseVerdictName(name)
}

// QueryVerdictResultsAndExonerations queries test results and exonerations for a verdict by invocation ID and test ID.
// Defaults to querying as a root invocation using QueryTestVerdicts unless legacy is true.
func QueryVerdictResultsAndExonerations(ctx context.Context, client pb.ResultDBClient, invID, testID, variantHash string, legacy bool, maxFetch int) ([]*pb.TestResult, []*pb.TestExoneration, bool, error) {
	return base.QueryVerdictResultsAndExonerations(ctx, client, invID, testID, variantHash, legacy, maxFetch)
}

func (r *verdictGetRun) hasExplicitMaxResultsFlag() bool {
	if r.maxResults > 0 && r.maxResults != defaultMaxVerdictResults {
		return true
	}
	explicit := false
	r.Flags.Visit(func(f *flag.Flag) {
		if f.Name == "max-results" || f.Name == "limit" || f.Name == "n" {
			explicit = true
		}
	})
	return explicit
}

func (r *verdictGetRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if base.HasHelpFlag(args) {
		r.Flags.Usage()
		return 0
	}
	errOut := r.errOut
	if errOut == nil {
		errOut = os.Stderr
	}
	target, err := base.ParsePositionalTarget(&r.Flags, args)
	if err != nil {
		fmt.Fprintf(errOut, "%s\n", err)
		return 1
	}
	if target == "" && (r.invocationID == "" || r.testID == "") {
		fmt.Fprintf(errOut, "flags -invocationid and -testid (or a positional URL/ID) are required\n")
		return 1
	}

	ctx := cli.GetContext(a, r, env)
	ctx = format.WithDiscoveryCache(ctx)
	client := r.client
	var schemasClient pb.SchemasClient
	var httpClient *http.Client
	if client == nil && r.af != nil {
		if err := r.af.Parse(); err != nil {
			fmt.Fprintf(errOut, "failed to parse auth flags: %s\n", err)
			return 1
		}
		var err error
		client, schemasClient, httpClient, err = r.af.NewResultDBClient(ctx, r.host)
		if err != nil {
			fmt.Fprintf(errOut, "failed to create resultdb client: %s\n", err)
			return 1
		}
	}

	extracted, err := ids.ExtractWithAuth(ctx, r.af, client, nil, target, ids.TargetBindings{
		InvocationID: &r.invocationID,
		TestID:       &r.testID,
		VariantHash:  &r.variantHash,
	})
	if err != nil {
		fmt.Fprintf(errOut, "failed to extract IDs from %q: %s\n", target, err)
		return 1
	}
	if r.invocationID == "" || r.testID == "" {
		ids.PrintMissingFlagsError(errOut, extracted, "flags -invocationid and -testid (or a positional URL/ID containing them) are required")
		return 1
	}

	resolved, err := base.ResolveInvocation(ctx, client, r.invocationID)
	if err != nil {
		fmt.Fprintf(errOut, "failed to query verdict: %s\n", err)
		return 1
	}
	format.CacheResolvedInvocation(ctx, resolved)
	invID, legacy := resolved.ID, resolved.Legacy
	if extracted != nil {
		extracted.Legacy = legacy
	}

	maxDisplay := 0
	fetchLimit := 0
	if !r.allResults && r.maxResults > 0 {
		maxDisplay = r.maxResults
		fetchLimit = max(r.maxResults, verdictRunCountFetchLimit)
	}

	readMask := base.LegacySummaryReadMask()
	if r.showMetadata {
		readMask = base.LegacyFullReadMask()
	}
	allResults, exList, fetchHasMore, err := base.QueryVerdictResultsAndExonerationsWithMask(ctx, client, invID, r.testID, r.variantHash, legacy, fetchLimit, readMask)
	if err != nil {
		fmt.Fprintf(errOut, "failed to query verdict: %s\n", err)
		return 1
	}
	if len(allResults) == 0 {
		fmt.Fprintf(errOut, "no results found for test %q in invocation %q (try 'luci verdict list -invocationid %s' to list failing verdicts in this invocation)\n", r.testID, invID, invID)
		if notice := formatInvocationInProgressNotice(invID, resolved); notice != "" {
			fmt.Fprint(errOut, notice)
		}
		if notice := formatExpiredExpectedResultsNotice(invID, resolved); notice != "" {
			fmt.Fprint(errOut, notice)
		}
		if !legacy && (strings.HasSuffix(r.testID, "#.") || strings.HasSuffix(r.testID, ".#") || !strings.Contains(r.testID, "#")) {
			modName := strings.TrimSuffix(r.testID, "#.")
			modName = strings.TrimSuffix(modName, ".#")
			modName = strings.TrimPrefix(modName, ":")
			fmt.Fprintf(errOut, "Hint: if this was a module error, try: luci module get -invocationid %s -modulename %s\n", base.ShellQuoteIfNeeded(invID), base.ShellQuoteIfNeeded(modName))
		}
		return 1
	}

	totalResults := len(allResults)
	totalByParent := make(map[string]int)
	for _, tr := range allResults {
		pg := format.GetParentGroup(tr.Name)
		totalByParent[pg.ID]++
	}
	verdictResults := allResults
	hasMore := fetchHasMore
	if maxDisplay > 0 && len(verdictResults) > maxDisplay {
		verdictResults = verdictResults[:maxDisplay]
		hasMore = true
	}

	out := r.out
	if out == nil {
		out = os.Stdout
	}
	var derivedBuf bytes.Buffer
	extracted.EnrichFromTestResult(verdictResults[0])
	ids.PrintDerivedIDs(&derivedBuf, extracted)
	if notice := formatInvocationInProgressNotice(invID, resolved); notice != "" {
		derivedBuf.WriteString(notice)
	}

	vg := &VerdictGroup{
		Key:           VerdictKey{TestID: verdictResults[0].TestId, VariantHash: r.variantHash},
		Variant:       verdictResults[0].Variant,
		Results:       verdictResults,
		AllResults:    allResults,
		Exonerations:  exList,
		HasMore:       hasMore,
		TotalResults:  totalResults,
		TotalMore:     fetchHasMore,
		TotalByParent: totalByParent,
	}

	renderVerdictSummary(ctx, out, vg, verdictSummaryOptions{
		derivedPrefix: derivedBuf.String(),
		schemasClient: schemasClient,
		rdbClient:     client,
		httpClient:    httpClient,
		invID:         invID,
		showMetadata:  r.showMetadata,
		capOutput:     !r.showMetadata && !r.allResults && !r.hasExplicitMaxResultsFlag(),
		legacy:        legacy,
	})
	return 0
}

type verdictSummaryOptions struct {
	derivedPrefix string
	schemasClient pb.SchemasClient
	rdbClient     pb.ResultDBClient
	httpClient    *http.Client
	invID         string
	showMetadata  bool
	capOutput     bool
	legacy        bool
}

func printVerdictSummary(out io.Writer, ctx context.Context, schemasClient pb.SchemasClient, rdbClient pb.ResultDBClient, httpClient *http.Client, invID string, g *VerdictGroup, showMetadata, legacy bool) {
	renderVerdictSummary(ctx, out, g, verdictSummaryOptions{
		schemasClient: schemasClient,
		rdbClient:     rdbClient,
		httpClient:    httpClient,
		invID:         invID,
		showMetadata:  showMetadata,
		capOutput:     !showMetadata,
		legacy:        legacy,
	})
}

func renderVerdictSummary(ctx context.Context, out io.Writer, g *VerdictGroup, opts verdictSummaryOptions) {
	if out == nil {
		out = os.Stdout
	}
	ctx = format.WithDiscoveryCache(ctx)

	var (
		artFormatter format.VerdictArtifactFormatter
		wg           sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		artFormatter = format.FetchVerdictInlineFailureArtifactFormatter(ctx, opts.rdbClient, opts.invID, g.allResults(), opts.legacy)
	}()

	var primaryBuf bytes.Buffer
	if opts.derivedPrefix != "" {
		primaryBuf.WriteString(opts.derivedPrefix)
	}

	status := g.DisplayStatus()

	var firstRes *pb.TestResult
	if len(g.Results) > 0 {
		firstRes = g.Results[0]
	}
	aggs := FetchHierarchyAggregations(ctx, opts.rdbClient, opts.invID, firstRes, opts.legacy)
	invLine := opts.invID
	if c := format.FormatVerdictCountsInline(aggs.GetInvocation()); c != "" {
		invLine += " " + c
	}
	fmt.Fprintf(&primaryBuf, "Invocation:   %s\n", invLine)
	if firstRes != nil {
		format.FprintTestIDWithAggregations(&primaryBuf, ctx, opts.schemasClient, firstRes, aggs)
		if firstRes.TestMetadata != nil && firstRes.TestMetadata.Name != "" {
			fmt.Fprintf(&primaryBuf, "Test Name:    %s\n", firstRes.TestMetadata.Name)
		}
	}
	fmt.Fprintf(&primaryBuf, "Status:       %s\n", status)
	if g.Variant != nil && len(g.Variant.GetDef()) > 0 {
		fmt.Fprintf(&primaryBuf, "Variant:      %s\n", format.FormatVariant(g.Variant))
	}
	if len(g.Exonerations) > 0 {
		fmt.Fprintf(&primaryBuf, "Exonerations:\n")
		for _, ex := range g.Exonerations {
			reasonStr := format.StripHTML(ex.ExplanationHtml)
			if reasonStr == "" {
				reasonStr = "No explanation provided"
			}
			fmt.Fprintf(&primaryBuf, "  - Reason: %s [%s]\n", reasonStr, ex.Reason)
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
		if g.TotalResults > len(g.Results) {
			totalStr := fmt.Sprintf("%d", g.TotalResults)
			if g.TotalMore {
				totalStr += "+"
			}
			fmt.Fprintf(&primaryBuf, "Runs (showing %d of %s results, use -all to see all):\n", len(g.Results), totalStr)
		} else {
			fmt.Fprintf(&primaryBuf, "Runs (showing %d results, use -all to see all):\n", len(g.Results))
		}
	} else {
		fmt.Fprintf(&primaryBuf, "Runs (%d):\n", len(g.Results))
	}
	for _, grp := range groups {
		runCountStr := ""
		parentTotal := g.TotalByParent[grp.parent.ID]
		if parentTotal > len(grp.items) {
			if g.TotalMore {
				runCountStr = fmt.Sprintf("showing %d of %d+ runs", len(grp.items), parentTotal)
			} else {
				runCountStr = fmt.Sprintf("showing %d of %d runs", len(grp.items), parentTotal)
			}
		} else if g.TotalMore {
			runCountStr = fmt.Sprintf("%d+ runs", len(grp.items))
		} else if len(grp.items) == 1 {
			runCountStr = "1 run"
		} else {
			runCountStr = fmt.Sprintf("%d runs", len(grp.items))
		}
		fmt.Fprintf(&primaryBuf, "- %s (%s):\n", grp.parent.Label, runCountStr)
		if !opts.legacy && grp.parent.ID != "" {
			modErr, _ := format.DiscoverWorkUnitError(ctx, opts.rdbClient, opts.httpClient, grp.parent.ID)
			if modErr != nil {
				firstLine, truncated := format.FormatDiscoveredErrorFirstLine(modErr, 120)
				if firstLine != "" {
					label := "Module Error:"
					if truncated {
						label = "Module Error (truncated):"
					}
					fmt.Fprintf(&primaryBuf, "  %s %s\n", label, firstLine)
				}
			}
		}
		for _, tr := range grp.items {
			fmt.Fprintf(&primaryBuf, "  - %s (%s) - result_id: %s\n", tr.StatusV2, format.FormatDuration(tr.Duration), tr.ResultId)
			if tr.FailureReason != nil {
				firstLine, truncated := format.FormatFailureReasonFirstLine(tr.FailureReason, 120)
				if firstLine != "" {
					label := "Error:"
					if truncated {
						label = "Error (truncated):"
					}
					fmt.Fprintf(&primaryBuf, "    %s %s\n", label, firstLine)
				}
			}
			if tr.SummaryHtml != "" {
				firstLine, truncated := format.TruncateFirstLine(format.StripHTML(tr.SummaryHtml), 120)
				if firstLine != "" {
					label := "Summary:"
					if truncated {
						label = "Summary (truncated):"
					}
					fmt.Fprintf(&primaryBuf, "    %s %s\n", label, firstLine)
				}
			}
		}
	}
	if opts.showMetadata {
		if len(g.Results) > 0 {
			fmt.Fprintln(&primaryBuf)
			format.FprintAdditionalMetadata(&primaryBuf, g.Results[0])
		}
		if !opts.legacy {
			if rootInv := format.FetchRootInvocation(ctx, opts.rdbClient, opts.invID); rootInv != nil {
				fmt.Fprintln(&primaryBuf)
				format.FprintRootInvocationMetadata(&primaryBuf, rootInv)
				format.FprintRootInvocationAdditionalMetadata(&primaryBuf, rootInv)
			}
		}
	}

	wg.Wait()
	finalOut := format.FitPrimaryAndInlineArtifacts(primaryBuf.String(), artFormatter, opts.capOutput)
	fmt.Fprint(out, finalOut)
}
