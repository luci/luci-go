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
	"context"
	"fmt"
	"os"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/luci/artifact"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
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
	return base.RunSubcommandApp(a, "luci test-result", "Test results management", []*subcommands.Command{
		GetCmd(r.af),
		artifact.TestResultArtifactCmd(r.af),
		subcommands.CmdHelp,
	}, args)
}

func GetCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get -invocationid <invocation_id> -testid <test_id> -resultid <result_id>",
		ShortDesc: "Get a test result",
		LongDesc:  "Get details of an individual test result by its invocation ID, test ID, and result ID.",
		CommandRun: func() subcommands.CommandRun {
			r := &testResultGetRun{af: af}
			r.af.Register(&r.Flags)
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.StringVar(&r.invocationID, "invocationid", "", "Invocation ID (e.g. build-867... or ants-i...)")
			r.Flags.StringVar(&r.testID, "testid", "", "Test ID (e.g. :module!junit:pkg.Class#Method)")
			r.Flags.StringVar(&r.resultID, "resultid", "", "Result ID (e.g. 0, r1, or uuid)")
			r.Flags.BoolVar(&r.showArtifacts, "show-artifacts", false, "Render HTML artifact links inline")
			r.Flags.BoolVar(&r.showArtifacts, "artifacts", false, "Alias for -show-artifacts")
			r.Flags.BoolVar(&r.showMetadata, "metadata", false, "Show additional result metadata and tags")
			r.Flags.BoolVar(&r.showMetadata, "show-metadata", false, "Alias for -metadata")
			r.Flags.BoolVar(&r.legacy, "legacy", false, "Query as legacy invocation instead of root invocation")
			return r
		},
	}
}

type testResultGetRun struct {
	subcommands.CommandRunBase
	af            *base.AuthFlags
	host          string
	invocationID  string
	testID        string
	resultID      string
	showArtifacts bool
	showMetadata  bool
	legacy        bool
}

// FetchTestResult fetches a test result by its decomposed IDs.
func FetchTestResult(ctx context.Context, client pb.ResultDBClient, invID, testID, resultID string, legacy bool) (*pb.TestResult, error) {
	name := base.FormatTestResultResourceName(invID, testID, resultID)
	req := &pb.GetTestResultRequest{Name: name}
	res, err := client.GetTestResult(ctx, req)
	if err == nil && res != nil {
		return res, nil
	}

	// Try resolving via verdict query on root invocation (passing maxFetch: 0 to paginate all results for this test)
	results, _, _, vErr := verdict.QueryVerdictResultsAndExonerations(ctx, client, invID, testID, "", legacy, 0)
	if vErr != nil {
		if err != nil {
			return nil, errors.Fmt("direct lookup failed (%w); fallback verdict query failed (%w)", err, vErr)
		}
		return nil, vErr
	}
	for _, tr := range results {
		if tr.ResultId == resultID {
			return tr, nil
		}
	}

	return nil, errors.Fmt("test result %q not found for test %q in invocation %q", resultID, testID, invID)
}

func (r *testResultGetRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			r.Flags.Usage()
			return 0
		}
	}
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "unexpected positional arguments; use flags -invocationid, -testid, -resultid (run 'luci ids <url>' to extract ids)\n")
		return 1
	}
	if r.invocationID == "" || r.testID == "" || r.resultID == "" {
		fmt.Fprintf(os.Stderr, "flags -invocationid, -testid, and -resultid are required (run 'luci ids <url>' to extract ids)\n")
		return 1
	}
	ctx := cli.GetContext(a, r, env)

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

	res, err := FetchTestResult(ctx, client, r.invocationID, r.testID, r.resultID, r.legacy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get test result: %s\n", err)
		return 1
	}

	fmt.Printf("Result ID: %s\n", res.ResultId)
	format.PrintTestID(ctx, schemasClient, res)
	if res.TestMetadata != nil && res.TestMetadata.Name != "" {
		fmt.Printf("Test Name: %s\n", res.TestMetadata.Name)
	}
	pg := format.GetParentGroup(res.Name)
	if pg.Label != "" {
		fmt.Printf("Run in:    %s\n", pg.Label)
		if pg.ID != "" {
			modErr, _ := format.DiscoverWorkUnitError(ctx, client, httpClient, pg.ID)
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
	}
	printVerdictContext(ctx, client, r.invocationID, res, r.legacy)
	if res.Variant != nil && len(res.Variant.GetDef()) > 0 {
		fmt.Printf("Variant:   %s\n", format.FormatVariant(res.Variant))
	}
	fmt.Printf("Status:    %s\n", res.StatusV2)
	fmt.Printf("Duration:  %s\n", format.FormatDuration(res.Duration))
	if res.FailureReason != nil {
		format.PrintFailureReason(res.FailureReason, "")
	}
	if res.SummaryHtml != "" {
		fmt.Printf("Summary:\n%s\n", format.FormatSummaryHTML(ctx, client, httpClient, res.Name, res.SummaryHtml, r.showArtifacts))
	}
	if r.showMetadata {
		fmt.Println()
		fmt.Printf("Full Name: %s\n", res.Name)
		format.PrintAdditionalMetadata(res)
	}

	return 0
}

func printVerdictContext(ctx context.Context, rdbClient pb.ResultDBClient, invID string, res *pb.TestResult, legacy bool) {
	verdictResults, exList, _, err := verdict.QueryVerdictResultsAndExonerations(ctx, rdbClient, invID, res.TestId, res.VariantHash, legacy, 1000)
	if err != nil || (len(verdictResults) <= 1 && len(exList) == 0) {
		return
	}

	vg := &verdict.VerdictGroup{Results: verdictResults, Exonerations: exList}
	verdictStatus := vg.DisplayStatus()

	fmt.Printf("Verdict:   One of %d results in this verdict (overall status: %s)\n", len(verdictResults), verdictStatus)
}
