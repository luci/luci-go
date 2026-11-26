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

package invocation

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/maruel/subcommands"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/client/cmd/luci/ids"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Cmd returns the top-level command for `luci invocation`.
func Cmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "invocation <subcommand>",
		ShortDesc: "Inspect ResultDB root and legacy invocations",
		LongDesc: "Inspect ResultDB root and legacy invocations, including build descriptors\n" +
			"(branch, build target, build ID), extra builds, code sources, test definition\n" +
			"(system, name, run target, cluster), and execution/finalization state.",
		CommandRun: func() subcommands.CommandRun {
			return &invocationRun{af: af}
		},
	}
}

type invocationRun struct {
	subcommands.CommandRunBase
	af *base.AuthFlags
}

func (r *invocationRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return base.RunSubcommandApp(a, "luci invocation", "Invocation management", []*subcommands.Command{
		GetCmd(r.af),
		subcommands.CmdHelp,
	}, args)
}

// GetCmd returns the subcommand for `luci invocation get`.
func GetCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get [-invocationid <invocation_id>] [<target>]",
		ShortDesc: "Get invocation metadata, build, branch, target, sources, and state",
		LongDesc: "Get details of a root or legacy invocation, including primary build, extra builds,\n" +
			"tested code sources, definition properties, state, and verdict counts.\n\n" +
			"Examples:\n" +
			"  $ luci invocation get https://ci.chromium.org/ui/inv/ants-i14600010609614895\n" +
			"  $ luci invocation get -invocationid ants-i14600010609614895",
		CommandRun: func() subcommands.CommandRun {
			r := &invocationGetRun{af: af}
			if r.af != nil {
				r.af.Register(&r.Flags)
			}
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.StringVar(&r.invocationID, "invocationid", "", "Invocation ID (e.g. ants-i... or build-867...)")
			r.Flags.StringVar(&r.invocationID, "invocation-id", "", "Alias for -invocationid")
			r.Flags.StringVar(&r.invocationID, "id", "", "Alias for -invocationid")
			r.Flags.BoolVar(&r.showMetadata, "show-metadata", false, "Show additional invocation tags, properties, and realm metadata")
			r.Flags.BoolVar(&r.jsonOut, "json", false, "Output invocation proto in JSON format")
			return r
		},
	}
}

type invocationGetRun struct {
	subcommands.CommandRunBase
	af           *base.AuthFlags
	client       pb.ResultDBClient
	httpClient   *http.Client
	out          io.Writer
	host         string
	invocationID string
	showMetadata bool
	jsonOut      bool
}

func (r *invocationGetRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if base.HasHelpFlag(args) {
		r.Flags.Usage()
		return 0
	}
	target, err := base.ParsePositionalTarget(&r.Flags, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	if target == "" && r.invocationID == "" {
		fmt.Fprintf(os.Stderr, "flag -invocationid (or a positional URL/ID) is required\n")
		return 1
	}

	ctx := cli.GetContext(a, r, env)
	ctx = format.WithDiscoveryCache(ctx)
	client := r.client
	httpClient := r.httpClient
	if client == nil && r.af != nil {
		if err := r.af.Parse(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
			return 1
		}
		var err error
		client, _, httpClient, err = r.af.NewResultDBClient(ctx, r.host)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create resultdb client: %s\n", err)
			return 1
		}
	}

	extracted, err := ids.ExtractWithAuth(ctx, r.af, client, nil, target, ids.TargetBindings{
		InvocationID: &r.invocationID,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to extract IDs from %q: %s\n", target, err)
		return 1
	}
	if r.invocationID == "" {
		ids.PrintMissingFlagsError(os.Stderr, extracted, "flag -invocationid (or a positional URL/ID containing an invocation ID) is required")
		return 1
	}

	resolved, err := base.ResolveInvocation(ctx, client, r.invocationID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get invocation: %s\n", err)
		return 1
	}
	format.CacheResolvedInvocation(ctx, resolved)
	if extracted != nil {
		extracted.Legacy = resolved.Legacy
	}

	out := r.out
	if out == nil {
		out = os.Stdout
	}

	if r.jsonOut {
		ids.PrintDerivedIDs(os.Stderr, extracted)
		var msg proto.Message
		if resolved.Legacy {
			msg = resolved.Invocation
		} else {
			msg = resolved.RootInvocation
		}
		b, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(msg)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal JSON: %s\n", err)
			return 1
		}
		fmt.Fprintln(out, string(b))
		return 0
	}

	ids.PrintDerivedIDs(out, extracted)
	printInvocationDetails(ctx, out, client, httpClient, resolved, r.showMetadata)
	return 0
}

func printInvocationDetails(ctx context.Context, out io.Writer, client pb.ResultDBClient, httpClient *http.Client, resolved *base.ResolvedInvocation, showMetadata bool) {
	if resolved == nil {
		return
	}
	if resolved.Legacy {
		inv := resolved.Invocation
		summary := fetchLegacyInvocationSummary(ctx, client, resolved.ID)
		inlineCounts := formatLegacyVerdictCountsInline(summary)
		var countsSuffix string
		if inlineCounts != "" {
			countsSuffix = " " + inlineCounts
		}
		fmt.Fprintf(out, "Invocation:    invocations/%s (legacy)%s\n", resolved.ID, countsSuffix)
		if inv == nil {
			return
		}
		if inv.GetState() != pb.Invocation_STATE_UNSPECIFIED {
			fmt.Fprintf(out, "Inv State:     %s\n", inv.GetState())
		}
		if s := format.FormatSources(inv.GetSourceSpec().GetSources()); s != "" {
			fmt.Fprintf(out, "Sources:       %s\n", s)
		}
		if inv.GetProducerResource() != "" {
			fmt.Fprintf(out, "Producer:      %s\n", inv.GetProducerResource())
		}
		if inv.GetCreateTime() != nil {
			fmt.Fprintf(out, "Created:       %s\n", inv.GetCreateTime().AsTime().UTC().Format(time.RFC3339))
		}
		if inv.GetFinalizeTime() != nil {
			fmt.Fprintf(out, "Finalized:     %s\n", inv.GetFinalizeTime().AsTime().UTC().Format(time.RFC3339))
		}
		if verdictSummary := formatLegacyVerdictCountsSummary(summary, inlineCounts); verdictSummary != "" {
			fmt.Fprintf(out, "Test Verdicts:   %s\n", verdictSummary)
		}
		if showMetadata {
			fmt.Fprintln(out)
			format.FprintLegacyInvocationAdditionalMetadata(out, inv)
		}
		if summary != nil && summary.isZeroTestsRan() {
			fprintLegacyZeroVerdictDiagnostics(out, resolved.ID, inv)
		}
		return
	}

	rootInv := resolved.RootInvocation
	rootInvName := "rootInvocations/" + resolved.ID
	agg := format.FetchRootInvocationAggregation(ctx, client, resolved.ID)
	var countsSuffix string
	if c := format.FormatVerdictCountsInline(agg.GetTotalVerdictCounts()); c != "" {
		countsSuffix = " " + c
	}
	fmt.Fprintf(out, "Invocation:    %s%s\n", rootInvName, countsSuffix)
	format.FprintRootInvocationMetadata(out, rootInv)
	if rootInv != nil {
		if rootInv.GetCreateTime() != nil {
			fmt.Fprintf(out, "Created:       %s\n", rootInv.GetCreateTime().AsTime().UTC().Format(time.RFC3339))
		}
		if rootInv.GetFinalizeTime() != nil {
			fmt.Fprintf(out, "Finalized:     %s\n", rootInv.GetFinalizeTime().AsTime().UTC().Format(time.RFC3339))
		}
	}
	format.FprintRootInvocationAggregationSummary(out, agg)
	if rootInv != nil {
		var rootFailure *format.WorkUnitFailure
		var failures []*format.WorkUnitFailure
		if client != nil && rootInv.GetState() != pb.RootInvocation_SUCCEEDED {
			rootWU := format.FetchWorkUnit(ctx, client, base.FormatWorkUnitResourceName(resolved.ID, "root"))
			rootFailure = format.DiscoverSingleWorkUnitFailure(ctx, client, httpClient, rootWU)
			failures = format.DiscoverFirstLevelWorkUnitFailures(ctx, client, httpClient, resolved.ID, rootWU)
		}
		summary := rootInv.GetSummaryMarkdown()
		if rootFailure != nil && strings.TrimSpace(rootFailure.FullSummaryText) != "" {
			summary = rootFailure.FullSummaryText
		}
		format.FprintRootInvocationSummary(out, summary, rootInv)
		if rootFailure != nil {
			format.FprintWorkUnitFailureArtifacts(out, resolved.ID, rootFailure, "")
		}
		format.FprintFirstLevelWorkUnitFailures(out, resolved.ID, failures, "")
		if showMetadata {
			fmt.Fprintln(out)
			format.FprintRootInvocationAdditionalMetadata(out, rootInv)
		}
		format.FprintZeroVerdictDiagnostics(out, resolved.ID, rootInv, agg, format.ZeroVerdictDiagnosticOptions{
			IncludeWorkUnitGet: true,
		})
	}
}

type legacyInvocationSummary struct {
	hasTotalResults  bool
	totalTestResults int64
	hasVariants      bool
	anyVariants      bool
	counts           pb.TestAggregation_VerdictCounts
	moreUnexpected   bool
}

func (s *legacyInvocationSummary) isZeroTestsRan() bool {
	if s == nil {
		return false
	}
	return s.hasTotalResults && s.totalTestResults == 0 && !s.anyVariants && !s.moreUnexpected
}

func fetchLegacyInvocationSummary(ctx context.Context, client pb.ResultDBClient, invID string) *legacyInvocationSummary {
	if client == nil || invID == "" {
		return nil
	}
	invName := "invocations/" + base.NormalizeInvocation(invID)
	probeCtx := logging.SetLevel(ctx, logging.Error)

	var (
		statsResp *pb.QueryTestResultStatisticsResponse
		statsErr  error
		tvResp    *pb.QueryTestVariantsResponse
		tvErr     error
		wg        sync.WaitGroup
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		statsResp, statsErr = client.QueryTestResultStatistics(probeCtx, &pb.QueryTestResultStatisticsRequest{
			Invocations: []string{invName},
		})
	}()
	go func() {
		defer wg.Done()
		tvResp, tvErr = client.QueryTestVariants(probeCtx, &pb.QueryTestVariantsRequest{
			Invocations: []string{invName},
			Predicate: &pb.TestVariantPredicate{
				Status: pb.TestVariantStatus_UNEXPECTED_MASK,
			},
			PageSize:    1000,
			ResultLimit: 1,
			ReadMask:    &fieldmaskpb.FieldMask{Paths: []string{"status", "status_v2", "status_override"}},
		})
	}()
	wg.Wait()

	if (statsErr != nil || statsResp == nil) && (tvErr != nil || tvResp == nil) {
		return nil
	}

	summary := &legacyInvocationSummary{}
	if statsErr == nil && statsResp != nil {
		summary.hasTotalResults = true
		summary.totalTestResults = statsResp.GetTotalTestResults()
	}
	if tvErr == nil && tvResp != nil {
		summary.hasVariants = true
		summary.anyVariants = len(tvResp.GetTestVariants()) > 0
		summary.moreUnexpected = tvResp.GetNextPageToken() != ""
		for _, tv := range tvResp.GetTestVariants() {
			if format.IsLegacyTestVariantExonerated(tv) {
				summary.counts.Exonerated++
				continue
			}
			switch format.LegacyTestVariantStatusV2(tv) {
			case pb.TestVerdict_FAILED:
				summary.counts.Failed++
			case pb.TestVerdict_EXECUTION_ERRORED:
				summary.counts.ExecutionErrored++
			case pb.TestVerdict_PRECLUDED:
				summary.counts.Precluded++
			case pb.TestVerdict_FLAKY:
				summary.counts.Flaky++
			}
		}
	}
	return summary
}

func formatLegacyVerdictCountsInline(s *legacyInvocationSummary) string {
	if s == nil {
		return ""
	}
	if inline := format.FormatUnexpectedVerdictCountsInline(&s.counts, s.moreUnexpected); inline != "" {
		return inline
	}
	if s.hasVariants && !s.moreUnexpected && s.hasTotalResults && s.totalTestResults > 0 {
		return "[0 unexpected]"
	}
	return ""
}

func formatLegacyVerdictCountsSummary(s *legacyInvocationSummary, inline string) string {
	if s == nil {
		return ""
	}
	if s.isZeroTestsRan() {
		return "0 tests ran"
	}
	if s.hasTotalResults && s.totalTestResults > 0 {
		if inline != "" {
			return fmt.Sprintf("%s (%d total test results)", inline, s.totalTestResults)
		}
		return fmt.Sprintf("%d total test results", s.totalTestResults)
	}
	return inline
}

func fprintLegacyZeroVerdictDiagnostics(out io.Writer, invID string, inv *pb.Invocation) {
	quotedInv := base.ShellQuoteIfNeeded(invID)
	switch inv.GetState() {
	case pb.Invocation_FINALIZED:
		fmt.Fprintf(out, "Note: Inv State: FINALIZED with 0 ResultDB test results only means the invocation closed (it does not mean tests passed).\n")
	case pb.Invocation_ACTIVE:
		fmt.Fprintf(out, "Note: Inv State: ACTIVE with 0 ResultDB test results so far (test execution may still be in progress).\n")
	case pb.Invocation_FINALIZING:
		fmt.Fprintf(out, "Note: Inv State: FINALIZING with 0 ResultDB test results so far (results may still be uploading until finalized).\n")
	default:
		fmt.Fprintf(out, "Note: No test results were uploaded to ResultDB for invocation %q (0 tests ran).\n", invID)
	}
	fmt.Fprintln(out, "Next steps:")
	if buildID := base.BuildIDFromInvocation(invID); buildID != "" {
		fmt.Fprintf(out, "  - Inspect build steps and compile/setup errors: luci build get -buildid=%s\n", base.ShellQuoteIfNeeded(buildID))
	}
	fmt.Fprintf(out, "  - List invocation-level logs and artifacts: luci artifact list -invocationid=%s\n", quotedInv)
}
