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
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func Cmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "verdict <subcommand>",
		ShortDesc: "Manage invocation test verdicts (combined variant outcomes)",
		LongDesc:  "Manage invocation test verdicts. An 'invocation test verdict' (or simply test verdict) represents the combined outcome of all test results (retries) for a test variant within an invocation, evaluated alongside any test exonerations. For individual execution attempts, see 'luci test-result'.",
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
	app := &cli.Application{
		Name:  "luci verdict",
		Title: "Test verdicts management",
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
		ShortDesc: "Get a test verdict",
		LongDesc:  "Get details of an invocation test verdict by its resource name or UI URL path. Displays combined retry history and exonerations.",
		CommandRun: func() subcommands.CommandRun {
			r := &verdictGetRun{af: af}
			r.af.Register(&r.Flags)
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.BoolVar(&r.showArtifacts, "show-artifacts", false, "Print content of artifacts embedded in run summary_html")
			r.Flags.BoolVar(&r.showMetadata, "show-metadata", false, "Additionally print test metadata, tags, and properties")
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
}

func ParseVerdictName(name string) (invName, variantHash, testIDRegexp string, matchFunc func(*pb.TestResult) bool, err error) {
	// Clean up domain if full URL was pasted
	if idx := strings.Index(name, "invocations/"); idx != -1 {
		name = name[idx:]
	} else if idx := strings.Index(name, "rootInvocations/"); idx != -1 {
		name = name[idx:]
	}
	// Strip query params or hash if present
	if idx := strings.IndexAny(name, "?#"); idx != -1 {
		name = name[:idx]
	}

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
				return "", "", "", nil, errors.Annotate(err, "failed to unescape case name %q", caseParts[0])
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
			return "", "", "", nil, errors.Annotate(err, "failed to unescape test ID %q", encodedTestID)
		}
		testIDRegexp = regexp.QuoteMeta(testID)
		matchFunc = func(tr *pb.TestResult) bool {
			return tr.VariantHash == variantHash && tr.TestId == testID
		}
		return invName, variantHash, testIDRegexp, matchFunc, nil
	}

	return "", "", "", nil, errors.New("invalid verdict name format: could not parse test ID or case name")
}

func QueryVerdictResultsAndExonerations(ctx context.Context, client pb.ResultDBClient, invName, variantHash, testIDRegexp string, matchFunc func(*pb.TestResult) bool) ([]*pb.TestResult, []*pb.TestExoneration, error) {
	var verdictResults []*pb.TestResult
	pageToken := ""
	for {
		req := &pb.QueryTestResultsRequest{
			Invocations: []string{invName},
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
			return nil, nil, errors.Annotate(err, "QueryTestResults RPC failed")
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
			Invocations: []string{invName},
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
	ctx := cli.GetContext(a, r, env)
	if len(args) != 1 {
		fmt.Fprintf(os.Stderr, "Usage: luci verdict get <name>\n")
		return 1
	}
	name := args[0]

	invName, variantHash, testIDRegexp, matchFunc, err := ParseVerdictName(name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse verdict name %q: %s\n", name, err)
		return 1
	}

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

	verdictResults, exList, err := QueryVerdictResultsAndExonerations(ctx, client, invName, variantHash, testIDRegexp, matchFunc)
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
	status := g.Status()
	isExonerated := len(g.Exonerations) > 0 && (status == "FAILED" || status == "EXECUTION_ERRORED" || status == "PRECLUDED")
	if isExonerated {
		status = fmt.Sprintf("%s (EXONERATED)", status)
	}

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
	fmt.Printf("Runs (%d):\n", len(g.Results))
	for i, tr := range g.Results {
		ctxStr := format.ParseInvocationContext(tr.Name)
		fmt.Printf("  #%d %s in %s (%s)\n", i+1, tr.StatusV2, ctxStr, format.FormatDuration(tr.Duration))
		fmt.Printf("     Result Name: %s (run 'luci test-result get <name>')\n", tr.Name)
		if tr.FailureReason != nil && len(tr.FailureReason.Errors) > 0 {
			fmt.Printf("     Error: %s\n", tr.FailureReason.Errors[0].Message)
		}
		if tr.SummaryHtml != "" {
			fmt.Printf("     Summary:\n%s\n", format.FormatSummaryHTML(ctx, rdbClient, httpClient, tr.Name, tr.SummaryHtml, showArtifacts))
		}
	}
	if showMetadata && len(g.Results) > 0 {
		fmt.Println()
		format.PrintAdditionalMetadata(g.Results[0])
	}
}
