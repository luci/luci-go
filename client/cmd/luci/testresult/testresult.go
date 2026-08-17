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
	"regexp"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/client/cmd/luci/verdict"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func Cmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "test-result <subcommand>",
		ShortDesc: "Manage individual test results (single attempt outcomes)",
		LongDesc: "Manage individual test results. A 'test result' represents a single execution attempt of a test in an invocation (identified by invocation_id, test_id, and result_id). For the combined verdict across retries and exonerations, see 'luci verdict'.\n\n" +
			"Available subcommands:\n" +
			"  get       Get details of an individual test result",
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
		subcommands.CmdHelp,
	}, args)
}

func GetCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get <name> | get - [<result_id>] | get - <test_id> <result_id> | get <inv> <test_id> <result_id>",
		ShortDesc: "Get a test result",
		LongDesc:  "Get details of an individual test result by its resource name, URL, or decomposed IDs.\nUse '-' to reuse cached parent resource components with trailing overrides.",
		CommandRun: func() subcommands.CommandRun {
			r := &testResultGetRun{af: af}
			r.af.Register(&r.Flags)
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.BoolVar(&r.showArtifacts, "show-artifacts", false, "Print content of artifacts embedded in run summary_html")
			r.Flags.BoolVar(&r.showMetadata, "show-metadata", false, "Additionally print test metadata, tags, and properties")
			r.Flags.BoolVar(&r.legacy, "legacy", false, "Query as legacy invocation instead of root invocation")
			return r
		},
	}
}

type testResultGetRun struct {
	subcommands.CommandRunBase
	af            *base.AuthFlags
	host          string
	showArtifacts bool
	showMetadata  bool
	legacy        bool
}

// FetchTestResult fetches a test result by its resource name or verdict target with result ID.
func FetchTestResult(ctx context.Context, client pb.ResultDBClient, target string, legacy bool) (*pb.TestResult, error) {
	clean := base.TrimResourceURL(target)
	if strings.Contains(clean, "/tests/") && strings.Contains(clean, "/results/") && !strings.Contains(clean, "/modules/") && !strings.Contains(clean, "/variants/") {
		req := &pb.GetTestResultRequest{Name: clean}
		res, err := client.GetTestResult(ctx, req)
		if err == nil {
			return res, nil
		}
	}

	// Try resolving via verdict query
	_, matched, err := verdict.ResolveVerdictResults(ctx, client, target, legacy)
	if err == nil {
		return matched, nil
	}

	return client.GetTestResult(ctx, &pb.GetTestResultRequest{Name: clean})
}

func (r *testResultGetRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			r.Flags.Usage()
			return 0
		}
	}
	ctx := cli.GetContext(a, r, env)
	name, err := base.ParseTestResultTargetArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	base.RecordTestResult(name, "", "")

	if err := r.af.Parse(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
		return 1
	}

	client, schemasClient, httpClient, err := r.af.NewResultDBClient(ctx, r.host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create resultdb client: %s\n", err)
		return 1
	}

	res, err := FetchTestResult(ctx, client, name, r.legacy)
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
	}
	printVerdictContext(ctx, client, res, r.legacy)
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

func printVerdictContext(ctx context.Context, rdbClient pb.ResultDBClient, res *pb.TestResult, legacy bool) {
	idx := strings.Index(res.Name, "/tests/")
	if idx == -1 {
		return
	}
	invName := res.Name[:idx]
	if wuIdx := strings.Index(invName, "/workUnits/"); wuIdx != -1 {
		invName = invName[:wuIdx]
	}
	testIDRegexp := regexp.QuoteMeta(res.TestId)
	matchFunc := func(tr *pb.TestResult) bool {
		return tr.VariantHash == res.VariantHash && tr.TestId == res.TestId
	}

	verdictResults, exList, err := verdict.QueryVerdictResultsAndExonerations(ctx, rdbClient, invName, res.VariantHash, testIDRegexp, matchFunc, legacy)
	if err != nil || (len(verdictResults) <= 1 && len(exList) == 0) {
		return
	}

	vg := &verdict.VerdictGroup{Results: verdictResults, Exonerations: exList}
	verdictStatus := vg.DisplayStatus()

	fmt.Printf("Verdict:   One of %d results in this verdict (overall status: %s)\n", len(verdictResults), verdictStatus)
	fmt.Printf("           Run 'luci verdict get -' to view the full verdict\n")
}
