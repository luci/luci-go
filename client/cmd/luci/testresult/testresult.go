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
	"io"
	"net/http"
	"os"

	"github.com/maruel/subcommands"

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
	return base.RunSubcommandApp(a, "luci test-result", "Test results management", []*subcommands.Command{
		GetCmd(r.af),
		artifact.TestResultArtifactCmd(r.af),
		subcommands.CmdHelp,
	}, args)
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
			r.Flags.StringVar(&r.testID, "testid", "", "Test ID (e.g. :module!junit:pkg.Class#Method)")
			r.Flags.StringVar(&r.resultID, "resultid", "", "Result ID (e.g. 0, r1, or uuid)")
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
	results, _, _, err := verdict.QueryVerdictResultsAndExonerations(ctx, client, invID, testID, "", legacy, 0)
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

	res, err := FetchTestResult(ctx, client, invID, r.testID, r.resultID, legacy)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get test result: %s\n", err)
		return 1
	}

	out := r.out
	if out == nil {
		out = os.Stdout
	}
	extracted.EnrichFromTestResult(res)
	ids.PrintDerivedIDs(out, extracted)

	r.printTestResult(ctx, out, client, schemasClient, httpClient, invID, res, legacy)
	return 0
}

func (r *testResultGetRun) printTestResult(ctx context.Context, out io.Writer, client pb.ResultDBClient, schemasClient pb.SchemasClient, httpClient *http.Client, invID string, res *pb.TestResult, legacy bool) {
	if out == nil {
		out = os.Stdout
	}
	aggs := verdict.FetchHierarchyAggregations(ctx, client, invID, res, legacy)
	if c := format.FormatVerdictCountsInline(aggs.GetInvocation()); c != "" {
		fmt.Fprintf(out, "Invocation: %s %s\n", invID, c)
	}
	fmt.Fprintf(out, "Result ID: %s\n", res.ResultId)
	format.FprintTestIDWithAggregations(out, ctx, schemasClient, res, aggs)
	if res.TestMetadata != nil && res.TestMetadata.Name != "" {
		fmt.Fprintf(out, "Test Name: %s\n", res.TestMetadata.Name)
	}
	pg := format.GetParentGroup(res.Name)
	if pg.Label != "" {
		fmt.Fprintf(out, "Run in:    %s\n", pg.Label)
		if !legacy && pg.ID != "" {
			modErr, _ := format.DiscoverWorkUnitError(ctx, client, httpClient, pg.ID)
			if modErr != nil {
				firstLine, truncated := format.FormatDiscoveredErrorFirstLine(modErr, 120)
				if firstLine != "" {
					label := "Module Error:"
					if truncated {
						label = "Module Error (truncated):"
					}
					fmt.Fprintf(out, "  %s %s\n", label, firstLine)
				}
			}
		}
	}
	printVerdictContext(ctx, out, client, invID, res, legacy)
	if res.Variant != nil && len(res.Variant.GetDef()) > 0 {
		fmt.Fprintf(out, "Variant:   %s\n", format.FormatVariant(res.Variant))
	}
	fmt.Fprintf(out, "Status:    %s\n", res.StatusV2)
	fmt.Fprintf(out, "Duration:  %s\n", format.FormatDuration(res.Duration))
	if res.FailureReason != nil {
		format.FprintFailureReason(out, res.FailureReason, "")
	}
	if res.SummaryHtml != "" {
		fmt.Fprintf(out, "Summary:\n%s\n", format.FormatSummaryHTML(ctx, client, httpClient, res.Name, res.SummaryHtml))
	}
	if r.showMetadata {
		fmt.Fprintln(out)
		fmt.Fprintf(out, "Full Name: %s\n", res.Name)
		format.FprintAdditionalMetadata(out, res)
		if !legacy {
			if rootInv := format.FetchRootInvocation(ctx, client, invID); rootInv != nil {
				fmt.Fprintln(out)
				format.FprintRootInvocationMetadata(out, rootInv)
				format.FprintRootInvocationAdditionalMetadata(out, rootInv)
			}
		}
	}
	if arts, err := artifact.QueryAllArtifacts(ctx, client, res.Name); err == nil {
		if len(arts) > 0 {
			fmt.Fprintf(out, "Artifact Count: %d (run 'luci test-result artifact list -invocationid %s -testid %q -resultid %s' to view)\n", len(arts), invID, res.TestId, res.ResultId)
		} else {
			fmt.Fprintf(out, "Artifact Count: 0\n")
		}
	}
	if legacy {
		artifact.PrintImmediateParentInvocationNotice(ctx, client, res.Name, out)
	} else {
		artifact.PrintAncestorWorkUnitNotices(ctx, client, res.Name, artifact.ParentTypeTestResult, out)
	}
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
