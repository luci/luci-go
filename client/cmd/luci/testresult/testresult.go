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
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/client/cmd/luci/verdict"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func Cmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "test-result <subcommand>",
		ShortDesc: "Manage individual test results (single attempt outcomes)",
		LongDesc:  "Manage individual test results. A 'test result' represents a single execution attempt of a test in an invocation (identified by invocation_id, test_id, and result_id). For the combined verdict across retries and exonerations, see 'luci verdict'.",
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
	app := &cli.Application{
		Name:  "luci test-result",
		Title: "Test results management",
		Commands: []*subcommands.Command{
			GetCmd(r.af),
			subcommands.CmdHelp,
		},
		Context: func(ctx context.Context) context.Context {
			if m, ok := a.(cli.ContextModificator); ok {
				return m.ModifyContext(ctx)
			}
			return ctx
		},
	}
	return subcommands.Run(app, args)
}

func GetCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get <name>",
		ShortDesc: "Get a test result",
		LongDesc:  "Get details of an individual test result by its resource name.",
		CommandRun: func() subcommands.CommandRun {
			r := &testResultGetRun{af: af}
			r.af.Register(&r.Flags)
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.BoolVar(&r.showArtifacts, "show-artifacts", false, "Print content of artifacts embedded in run summary_html")
			r.Flags.BoolVar(&r.showMetadata, "show-metadata", false, "Additionally print test metadata, tags, and properties")
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
}

func (r *testResultGetRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: luci test-result get <name>\n")
		return 1
	}
	name := args[0]

	if err := r.af.Parse(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
		return 1
	}

	httpClient, err := r.af.NewHTTPClient(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create http client: %s\n", err)
		return 1
	}

	prpcClient := &prpc.Client{
		C:       httpClient,
		Host:    r.host,
		Options: prpc.DefaultOptions(),
	}
	client := pb.NewResultDBPRPCClient(prpcClient)
	schemasClient := pb.NewSchemasPRPCClient(prpcClient)

	req := &pb.GetTestResultRequest{Name: name}
	res, err := client.GetTestResult(ctx, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetTestResult RPC failed: %s\n", err)
		return 1
	}

	fmt.Printf("Name:      %s\n", res.Name)
	format.PrintTestID(ctx, schemasClient, res)
	if res.TestMetadata != nil && res.TestMetadata.Name != "" {
		fmt.Printf("Test Name: %s\n", res.TestMetadata.Name)
	}
	ctxStr := format.ParseInvocationContext(res.Name)
	if ctxStr != "" {
		fmt.Printf("Run in:    %s\n", ctxStr)
	}
	printVerdictContext(ctx, client, res)
	if res.Variant != nil && len(res.Variant.GetDef()) > 0 {
		fmt.Printf("Variant:   %s\n", format.FormatVariant(res.Variant))
	}
	fmt.Printf("Status:    %s\n", res.StatusV2)
	fmt.Printf("Duration:  %s\n", format.FormatDuration(res.Duration))
	if res.FailureReason != nil && len(res.FailureReason.Errors) > 0 {
		fmt.Printf("Error:     %s\n", res.FailureReason.Errors[0].Message)
	}
	if res.SummaryHtml != "" {
		fmt.Printf("Summary:\n%s\n", format.FormatSummaryHTML(ctx, client, httpClient, res.Name, res.SummaryHtml, r.showArtifacts))
	}
	if r.showMetadata {
		fmt.Println()
		format.PrintAdditionalMetadata(res)
	}

	return 0
}

func printVerdictContext(ctx context.Context, rdbClient pb.ResultDBClient, res *pb.TestResult) {
	idx := strings.Index(res.Name, "/tests/")
	if idx == -1 {
		return
	}
	invName := res.Name[:idx]
	testIDRegexp := regexp.QuoteMeta(res.TestId)
	matchFunc := func(tr *pb.TestResult) bool {
		return tr.VariantHash == res.VariantHash && tr.TestId == res.TestId
	}

	verdictResults, exList, err := verdict.QueryVerdictResultsAndExonerations(ctx, rdbClient, invName, res.VariantHash, testIDRegexp, matchFunc)
	if err != nil || (len(verdictResults) <= 1 && len(exList) == 0) {
		return
	}

	vg := &verdict.VerdictGroup{Results: verdictResults, Exonerations: exList}
	verdictStatus := vg.Status()
	if len(exList) > 0 && (verdictStatus == "FAILED" || verdictStatus == "EXECUTION_ERRORED" || verdictStatus == "PRECLUDED") {
		verdictStatus = fmt.Sprintf("%s (EXONERATED)", verdictStatus)
	}

	var resultIndex int
	for i, tr := range verdictResults {
		if tr.Name == res.Name || tr.ResultId == res.ResultId {
			resultIndex = i + 1
			break
		}
	}
	verdictName := fmt.Sprintf("%s/tests/%s/variants/%s", invName, url.PathEscape(res.TestId), res.VariantHash)
	if resultIndex > 0 {
		fmt.Printf("Verdict:   Result %d of %d in this verdict (overall status: %s)\n", resultIndex, len(verdictResults), verdictStatus)
	} else {
		fmt.Printf("Verdict:   One of %d results in this verdict (overall status: %s)\n", len(verdictResults), verdictStatus)
	}
	fmt.Printf("           Verdict Name: %s (run 'luci verdict get <name>')\n", verdictName)
}
