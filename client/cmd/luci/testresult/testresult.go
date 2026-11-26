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

package testresult

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"

	"github.com/maruel/subcommands"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/client/cmd/luci/artifact"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/client/cmd/luci/ids"
	"go.chromium.org/luci/client/cmd/luci/verdict"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func Cmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "test-result <subcommand>",
		ShortDesc: "Manage individual test results (single attempt outcomes)",
		LongDesc: "Manage individual test results. A 'test result' represents a single execution attempt of a test in an invocation (identified by invocation_id, test_id, and result_id). For the combined verdict across retries and exonerations, see 'luci verdict'.\n\n" +
			"Available subcommands:\n" +
			"  get       Get details of an individual test result\n" +
			"  artifact  Manage test result artifacts",
		CommandRun: func() subcommands.CommandRun {
			return &testResultRun{af: af}
		},
	}
}

type testResultRun struct {
	subcommands.CommandRunBase
	af *base.AuthFlags
}

func (r *testResultRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return base.RunSubcommandAppWithHint(a, "luci test-result", "Test results management", []*subcommands.Command{
		GetCmd(r.af),
		artifact.TestResultArtifactCmd(r.af),
		subcommands.CmdHelp,
	}, args, testResultUnknownCommandHint)
}

func testResultUnknownCommandHint(a subcommands.Application, unknownCmd string, rest []string) string {
	switch strings.ToLower(strings.TrimSpace(unknownCmd)) {
	case "list", "ls", "query", "search":
		return "Hint: 'luci test-result' inspects a single test result attempt (use 'get' or 'artifact').\n" +
			"To list failing tests/verdicts in an invocation, use:\n" +
			"  luci verdict list -invocationid <INV>   (or pass a URL/ID directly)\n\n" +
			"Available subcommands for 'luci test-result': get, artifact"
	default:
		return ""
	}
}

func GetCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get [-invocationid <invocation_id> -testid <test_id> -resultid <result_id>] [<target>]",
		ShortDesc: "Get a test result",
		LongDesc: "Get details of an individual test result by a positional URL/ID or by its invocation ID, test ID, and result ID.\n\n" +
			"Examples:\n" +
			"  $ luci test-result get rootInvocations/ants-i123/workUnits/wu-1/tests/:ModA!junit:pkg:Cls%23testOne/results/res-1\n" +
			"  $ luci test-result get -invocationid build-123 -testid ninja://chrome/test -resultid 0",
		CommandRun: func() subcommands.CommandRun {
			r := &testResultGetRun{af: af}
			if r.af != nil {
				r.af.Register(&r.Flags)
			}
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.StringVar(&r.invocationID, "invocationid", "", "Invocation ID (e.g. build-867... or ants-i...)")
			r.Flags.StringVar(&r.invocationID, "invocation-id", "", "Alias for -invocationid")
			r.Flags.StringVar(&r.testID, "testid", "", "Test ID (e.g. :module!junit:pkg.Class#Method)")
			r.Flags.StringVar(&r.testID, "test-id", "", "Alias for -testid")
			r.Flags.StringVar(&r.resultID, "resultid", "", "Result ID (e.g. 0, r1, or uuid)")
			r.Flags.StringVar(&r.resultID, "result-id", "", "Alias for -resultid")
			r.Flags.BoolVar(&r.showMetadata, "show-metadata", false, "Show additional result metadata and tags")
			// -legacy is now unused and will be removed in the future.
			r.Flags.BoolVar(&r.legacy, "legacy", false, "Query as legacy invocation instead of root invocation")
			return r
		},
	}
}

type testResultGetRun struct {
	subcommands.CommandRunBase
	af           *base.AuthFlags
	client       pb.ResultDBClient
	out          io.Writer
	host         string
	invocationID string
	testID       string
	resultID     string
	showMetadata bool
	legacy       bool // Unused; will be removed in the future.
}

// FetchTestResult fetches a test result by its decomposed IDs.
func FetchTestResult(ctx context.Context, client pb.ResultDBClient, invID, testID, resultID string, legacy bool) (*pb.TestResult, error) {
	return fetchTestResultWithMask(ctx, client, invID, testID, resultID, legacy, base.LegacyFullReadMask())
}

func fetchTestResultWithMask(ctx context.Context, client pb.ResultDBClient, invID, testID, resultID string, legacy bool, legacyReadMask *fieldmaskpb.FieldMask) (*pb.TestResult, error) {
	results, _, _, err := base.QueryVerdictResultsAndExonerationsWithMask(ctx, client, invID, testID, "", legacy, 0, legacyReadMask)
	if err != nil {
		return nil, err
	}
	for _, tr := range results {
		if tr.ResultId == resultID {
			return tr, nil
		}
	}
	return nil, errors.Fmt("test result %q not found for test %q in invocation %q", resultID, testID, invID)
}

func (r *testResultGetRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if base.HasHelpFlag(args) {
		r.Flags.Usage()
		return 0
	}
	target, err := base.ParsePositionalTarget(&r.Flags, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	if target == "" && (r.invocationID == "" || r.testID == "" || r.resultID == "") {
		fmt.Fprintf(os.Stderr, "flags -invocationid, -testid, and -resultid (or a positional URL/ID) are required\n")
		return 1
	}
	ctx := cli.GetContext(a, r, env)
	ctx = format.WithDiscoveryCache(ctx)
	client := r.client
	var schemasClient pb.SchemasClient
	var httpClient *http.Client
	if client == nil && r.af != nil {
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

	extracted, err := ids.ExtractWithAuth(ctx, r.af, client, nil, target, ids.TargetBindings{
		InvocationID: &r.invocationID,
		TestID:       &r.testID,
		ResultID:     &r.resultID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to extract IDs from %q: %s\n", target, err)
		return 1
	}
	if r.invocationID == "" || r.testID == "" || r.resultID == "" {
		ids.PrintMissingFlagsError(os.Stderr, extracted, "flags -invocationid, -testid, and -resultid (or a positional URL/ID containing them) are required")
		return 1
	}

	resolved, err := base.ResolveInvocation(ctx, client, r.invocationID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get test result: %s\n", err)
		return 1
	}
	format.CacheResolvedInvocation(ctx, resolved)
	invID, legacy := resolved.ID, resolved.Legacy
	if extracted != nil {
		extracted.Legacy = legacy
	}

	readMask := base.LegacySummaryReadMask()
	if r.showMetadata {
		readMask = base.LegacyFullReadMask()
	}
	res, err := fetchTestResultWithMask(ctx, client, invID, r.testID, r.resultID, legacy, readMask)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get test result: %s\n", err)
		return 1
	}

	out := r.out
	if out == nil {
		out = os.Stdout
	}
	var derivedBuf bytes.Buffer
	extracted.EnrichFromTestResult(res)
	ids.PrintDerivedIDs(&derivedBuf, extracted)

	r.printTestResultWithDerived(ctx, out, derivedBuf.String(), client, schemasClient, httpClient, invID, res, legacy)
	return 0
}

func (r *testResultGetRun) printTestResult(ctx context.Context, out io.Writer, client pb.ResultDBClient, schemasClient pb.SchemasClient, httpClient *http.Client, invID string, res *pb.TestResult, legacy bool) {
	r.printTestResultWithDerived(ctx, out, "", client, schemasClient, httpClient, invID, res, legacy)
}

func (r *testResultGetRun) printTestResultWithDerived(ctx context.Context, out io.Writer, derivedPrefix string, client pb.ResultDBClient, schemasClient pb.SchemasClient, httpClient *http.Client, invID string, res *pb.TestResult, legacy bool) {
	if out == nil {
		out = os.Stdout
	}
	ctx = format.WithDiscoveryCache(ctx)

	var (
		artFormatter format.TestResultArtifactFormatter
		wg           sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		artFormatter = format.FetchTestResultInlineArtifactFormatter(ctx, client, invID, res, legacy)
	}()

	var primaryBuf bytes.Buffer
	if derivedPrefix != "" {
		primaryBuf.WriteString(derivedPrefix)
	}

	aggs := verdict.FetchHierarchyAggregations(ctx, client, invID, res, legacy)
	if c := format.FormatVerdictCountsInline(aggs.GetInvocation()); c != "" {
		fmt.Fprintf(&primaryBuf, "Invocation: %s %s\n", invID, c)
	}
	fmt.Fprintf(&primaryBuf, "Result ID: %s\n", res.ResultId)
	format.FprintTestIDWithAggregations(&primaryBuf, ctx, schemasClient, res, aggs)
	if res.TestMetadata != nil && res.TestMetadata.Name != "" {
		fmt.Fprintf(&primaryBuf, "Test Name: %s\n", res.TestMetadata.Name)
	}
	pg := format.GetParentGroup(res.Name)
	if pg.Label != "" {
		fmt.Fprintf(&primaryBuf, "Run in:    %s\n", pg.Label)
		if !legacy && pg.ID != "" {
			modErr, _ := format.DiscoverWorkUnitError(ctx, client, httpClient, pg.ID)
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
	}
	printVerdictContext(ctx, &primaryBuf, client, invID, res, legacy)
	if res.Variant != nil && len(res.Variant.GetDef()) > 0 {
		fmt.Fprintf(&primaryBuf, "Variant:   %s\n", format.FormatVariant(res.Variant))
	}
	fmt.Fprintf(&primaryBuf, "Status:    %s\n", res.StatusV2)
	fmt.Fprintf(&primaryBuf, "Duration:  %s\n", format.FormatDuration(res.Duration))
	if res.FailureReason != nil {
		format.FprintFailureReason(&primaryBuf, res.FailureReason, "")
	}
	var expandedArtIDs map[string]bool
	if res.SummaryHtml != "" {
		var summaryText string
		summaryText, expandedArtIDs = format.FormatSummaryHTMLWithExpanded(ctx, client, httpClient, res.Name, res.SummaryHtml)
		fmt.Fprintf(&primaryBuf, "Summary:\n%s\n", summaryText)
	}
	if r.showMetadata {
		fmt.Fprintln(&primaryBuf)
		fmt.Fprintf(&primaryBuf, "Full Name: %s\n", res.Name)
		format.FprintAdditionalMetadata(&primaryBuf, res)
		if !legacy {
			if rootInv := format.FetchRootInvocation(ctx, client, invID); rootInv != nil {
				fmt.Fprintln(&primaryBuf)
				format.FprintRootInvocationMetadata(&primaryBuf, rootInv)
				format.FprintRootInvocationAdditionalMetadata(&primaryBuf, rootInv)
			}
		}
	}

	wg.Wait()
	var formatInline func(int) string
	if artFormatter != nil {
		formatInline = func(maxBytes int) string {
			return artFormatter(expandedArtIDs, maxBytes)
		}
	}
	finalOut := format.FitPrimaryAndInlineArtifacts(primaryBuf.String(), formatInline, !r.showMetadata)
	fmt.Fprint(out, finalOut)
}

func printVerdictContext(ctx context.Context, out io.Writer, rdbClient pb.ResultDBClient, invID string, res *pb.TestResult, legacy bool) {
	verdictResults, exList, _, err := verdict.QueryVerdictResultsAndExonerations(ctx, rdbClient, invID, res.TestId, res.VariantHash, legacy, 1000)
	if err != nil || (len(verdictResults) <= 1 && len(exList) == 0) {
		return
	}

	vg := &verdict.VerdictGroup{Results: verdictResults, Exonerations: exList}
	verdictStatus := vg.DisplayStatus()

	fmt.Fprintf(out, "Verdict:   One of %d results in this verdict (overall status: %s)\n", len(verdictResults), verdictStatus)
}
