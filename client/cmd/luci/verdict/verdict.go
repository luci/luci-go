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
	"os"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
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

	ctx := cli.GetContext(a, r, env)
	ctx = format.WithDiscoveryCache(ctx)
	client := r.client
	var schemasClient pb.SchemasClient
	var httpClient *http.Client
	if client == nil {
		if err := r.af.Parse(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
			return 1
		}
		var err error
		client, schemasClient, httpClient, err = r.af.NewResultDBClient(ctx, r.host)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create resultdb client: %s\n", err)
			return 1
		}
	}

	resolved, err := base.ResolveInvocation(ctx, client, r.invocationID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to query verdict: %s\n", err)
		return 1
	}
	format.CacheResolvedInvocation(ctx, resolved)
	invID, legacy := resolved.ID, resolved.Legacy

	maxFetch := 0
	if !r.allResults && r.maxResults > 0 {
		maxFetch = r.maxResults
	}

	verdictResults, exList, hasMore, err := QueryVerdictResultsAndExonerations(ctx, client, invID, r.testID, r.variantHash, legacy, maxFetch)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to query verdict: %s\n", err)
		return 1
	}
	if len(verdictResults) == 0 {
		if strings.HasSuffix(r.testID, "#.") || strings.HasSuffix(r.testID, ".#") || !strings.Contains(r.testID, "#") {
			modName := strings.TrimSuffix(r.testID, "#.")
			modName = strings.TrimSuffix(modName, ".#")
			modName = strings.TrimPrefix(modName, ":")
			fmt.Fprintf(os.Stderr, "no results found for test %q in invocation %q (if this was a module error, try: luci module get -invocationid %s -modulename %s)\n", r.testID, invID, invID, modName)
		} else {
			fmt.Fprintf(os.Stderr, "no results found for test %q in invocation %q\n", r.testID, invID)
		}
		return 1
	}

	vg := &VerdictGroup{
		Key:          VerdictKey{TestID: verdictResults[0].TestId, VariantHash: r.variantHash},
		Variant:      verdictResults[0].Variant,
		Results:      verdictResults,
		Exonerations: exList,
		HasMore:      hasMore,
	}

	printVerdictSummary(ctx, schemasClient, client, httpClient, invID, vg, r.showMetadata, legacy)
	return 0
}

func printVerdictSummary(ctx context.Context, schemasClient pb.SchemasClient, rdbClient pb.ResultDBClient, httpClient *http.Client, invID string, g *VerdictGroup, showMetadata, legacy bool) {
	status := g.DisplayStatus()

	var firstRes *pb.TestResult
	if len(g.Results) > 0 {
		firstRes = g.Results[0]
	}
	aggs := FetchHierarchyAggregations(ctx, rdbClient, invID, firstRes, legacy)
	invLine := invID
	if c := format.FormatVerdictCountsInline(aggs.GetInvocation()); c != "" {
		invLine += " " + c
	}
	fmt.Printf("Invocation:   %s\n", invLine)
	if firstRes != nil {
		format.PrintTestIDWithAggregations(ctx, schemasClient, firstRes, aggs)
		if firstRes.TestMetadata != nil && firstRes.TestMetadata.Name != "" {
			fmt.Printf("Test Name:    %s\n", firstRes.TestMetadata.Name)
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
		if !legacy && grp.parent.ID != "" {
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
	if showMetadata && len(g.Results) > 0 {
		fmt.Println()
		format.PrintAdditionalMetadata(g.Results[0])
	}
}
