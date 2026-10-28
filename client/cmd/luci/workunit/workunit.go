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

package workunit

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/maruel/subcommands"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/client/cmd/luci/artifact"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/client/cmd/luci/ids"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Cmd returns the `luci work-unit` top-level subcommand.
func Cmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "work-unit <subcommand>",
		ShortDesc: "Manage ResultDB work units",
		LongDesc: "Manage ResultDB work units. Work units represent modular execution units and test suites within an invocation.\n\n" +
			"Available subcommands:\n" +
			"  get       Get details of a work unit\n" +
			"  artifact  Manage work unit artifacts",
		CommandRun: func() subcommands.CommandRun {
			return &workUnitRun{af: af}
		},
	}
}

type workUnitRun struct {
	subcommands.CommandRunBase
	af *base.AuthFlags
}

func (r *workUnitRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return base.RunSubcommandApp(a, "luci work-unit", "Work units management", []*subcommands.Command{
		GetCmd(r.af),
		artifact.WorkUnitArtifactCmd(r.af),
		subcommands.CmdHelp,
	}, args)
}

// GetCmd returns the `luci work-unit get` subcommand.
func GetCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get [-invocationid <invocation_id> -workunitid <work_unit_id>] [<target>]",
		ShortDesc: "Get a work unit",
		LongDesc: "Get details of a work unit by a positional URL/ID or by its root invocation ID and work unit ID.\n\n" +
			"Examples:\n" +
			"  $ luci work-unit get rootInvocations/ants-i77100010600769898/workUnits/ants-wu123\n" +
			"  $ luci work-unit get -invocationid build-123 -workunitid run-tests",
		CommandRun: func() subcommands.CommandRun {
			r := &workUnitGetRun{af: af}
			if r.af != nil {
				r.af.Register(&r.Flags)
			}
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.StringVar(&r.invocationID, "invocationid", "", "Root invocation ID (e.g. build-867... or ants-i...)")
			r.Flags.StringVar(&r.workUnitID, "workunitid", "", "Work unit ID (e.g. run-tests or ants-wu...)")
			r.Flags.BoolVar(&r.showMetadata, "show-metadata", false, "Show additional work unit metadata and tags")
			return r
		},
	}
}

type workUnitGetRun struct {
	subcommands.CommandRunBase
	af           *base.AuthFlags
	client       pb.ResultDBClient
	host         string
	invocationID string
	workUnitID   string
	showMetadata bool
	customCtx    context.Context
}

// ModifyContext overrides the CLI context when a custom context is injected by tests.
func (r *workUnitGetRun) ModifyContext(ctx context.Context) context.Context {
	if r.customCtx != nil {
		return r.customCtx
	}
	return ctx
}

func (r *workUnitGetRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if base.HasHelpFlag(args) {
		r.Flags.Usage()
		return 0
	}
	target, err := base.ParsePositionalTarget(&r.Flags, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	if target == "" && (r.invocationID == "" || r.workUnitID == "") {
		fmt.Fprintf(os.Stderr, "flags -invocationid and -workunitid (or a positional URL/ID) are required\n")
		return 1
	}

	ctx := cli.GetContext(a, r, env)
	ctx = format.WithDiscoveryCache(ctx)
	client := r.client
	var httpClient *http.Client
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
	if r.af != nil && r.af.ParsedOpts != nil {
		ctx = base.WithAuthOptions(ctx, *r.af.ParsedOpts)
	}

	extracted, err := ids.ExtractWithAuth(ctx, r.af, client, nil, target, ids.TargetBindings{
		InvocationID:           &r.invocationID,
		WorkUnitID:             &r.workUnitID,
		DefaultRootForWorkUnit: true,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to extract IDs from %q: %s\n", target, err)
		return 1
	}
	if r.invocationID == "" || r.workUnitID == "" {
		ids.PrintMissingFlagsError(os.Stderr, extracted, "flags -invocationid and -workunitid (or a positional URL/ID containing them) are required")
		return 1
	}

	resolved, err := base.ResolveInvocation(ctx, client, r.invocationID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to get work unit: %s\n", err)
		return 1
	}
	format.CacheResolvedInvocation(ctx, resolved)
	invID, legacy := resolved.ID, resolved.Legacy
	if legacy {
		fmt.Fprintf(os.Stderr, "work units are only supported for root invocations (invocation %q is a legacy invocation)\n", invID)
		return 1
	}

	normWUID := base.NormalizeWorkUnit(r.workUnitID)
	clean := base.FormatWorkUnitResourceName(invID, normWUID)
	rpcCtx := logging.SetLevel(ctx, logging.Error)

	wu, err := client.GetWorkUnit(rpcCtx, &pb.GetWorkUnitRequest{Name: clean})
	if err != nil {
		// In AnTS/TradeFed, leaf module and test-run work units are stored in ResultDB
		// under composite IDs (<task_work_unit_id>:u-<uuid>) rather than their own
		// AnTS serial WU<digits> ID. If direct GetWorkUnit returns NotFound for a bare
		// AnTS work unit ID, query the AnTS v4 API and ResultDB hierarchy to resolve
		// the composite work unit ID and retry GetWorkUnit.
		if status.Code(err) == codes.NotFound && base.IsAntsWorkUnitID(normWUID) && !strings.Contains(normWUID, ":u-") {
			if info, antsErr := base.ResolveAntsWorkUnit(ctx, normWUID); antsErr == nil && info != nil {
				resolvedWUID := base.ResolveResultDBAntsWorkUnit(ctx, client, invID, normWUID, info)
				if resolvedWUID != "" && resolvedWUID != normWUID {
					resolvedClean := base.FormatWorkUnitResourceName(invID, resolvedWUID)
					if resolvedWU, retryErr := client.GetWorkUnit(rpcCtx, &pb.GetWorkUnitRequest{Name: resolvedClean}); retryErr == nil && resolvedWU != nil {
						fmt.Fprintf(os.Stderr, "Note: resolved AnTS work unit %q to ResultDB work unit %q\n", r.workUnitID, resolvedWUID)
						wu = resolvedWU
						clean = resolvedClean
						err = nil
					}
				}
			}
		}
	}
	if err != nil {
		if base.IsPermissionDenied(err) {
			fmt.Fprintf(os.Stderr, "GetWorkUnit RPC failed (%s): %s\n", base.ResultDBPermissionReason(clean, err), err)
		} else if status.Code(err) == codes.NotFound && base.IsAntsWorkUnitID(normWUID) && !strings.Contains(normWUID, ":u-") {
			fmt.Fprintf(os.Stderr, "Work unit %q not found in invocation %q.\n\n", normWUID, invID)
			fmt.Fprintf(os.Stderr, "Note: In Android CI (AnTS / TradeFed), module and test-run work units are stored in ResultDB\n"+
				"under composite IDs (<task_work_unit_id>:u-<uuid>) rather than AnTS serial WU... IDs.\n"+
				"  1. If you have an AnTS test result ID (TR...) or ATI URL, run:\n"+
				"       luci ids <TR_ID_or_ATI_URL>\n"+
				"  2. To list failing test verdicts and their composite work units, run:\n"+
				"       luci verdict list -invocationid=%s\n"+
				"  3. To list artifacts across the invocation without knowing the work unit ID, run:\n"+
				"       luci artifact list -invocationid=%s\n", invID, invID)
		} else {
			fmt.Fprintf(os.Stderr, "GetWorkUnit RPC failed: %s\n", err)
		}
		return 1
	}

	extracted.EnrichFromWorkUnit(wu)
	ids.PrintDerivedIDs(os.Stdout, extracted)

	isRoot := wu.WorkUnitId == "root"
	var rootAgg *pb.TestAggregation
	if isRoot {
		rootAgg = fetchRootInvocationAggregation(rpcCtx, client, invID)
	}

	if wu.WorkUnitId != "" && (extracted == nil || extracted.WorkUnitID == "") {
		fmt.Printf("Work Unit ID: %s\n", wu.WorkUnitId)
	}
	if wu.Parent != "" {
		fmt.Printf("Parent:       %s\n", format.FormatWorkUnitBreadcrumb(wu.Parent))
	}
	if isRoot {
		printRootInvocationMetadata(resolved.RootInvocation)
	}
	if wu.Kind != "" {
		fmt.Printf("Kind:         %s\n", wu.Kind)
	}
	if modID := wu.GetModuleId(); modID != nil && modID.GetModuleName() != "" {
		fmt.Printf("Module:       %s\n", modID.GetModuleName())
	}
	if wu.ModuleShardKey != "" {
		fmt.Printf("Shard Key:    %s\n", wu.ModuleShardKey)
	}
	fmt.Printf("State:        %s\n", wu.State)
	if wu.FinalizationState != pb.WorkUnit_FINALIZATION_STATE_UNSPECIFIED {
		fmt.Printf("Finalization: %s\n", wu.FinalizationState)
	}
	if rootAgg != nil {
		if modSummary := format.FormatModuleStatusCountsSummary(rootAgg.GetModuleStatusCounts()); modSummary != "" {
			fmt.Printf("Module Statuses: %s\n", modSummary)
		}
		if verdictSummary := format.FormatVerdictCountsSummary(rootAgg.GetTotalVerdictCounts()); verdictSummary != "" {
			fmt.Printf("Test Verdicts:   %s\n", verdictSummary)
		}
	}
	if len(wu.ChildWorkUnits) > 0 {
		fmt.Printf("Child Work Units (%d):\n", len(wu.ChildWorkUnits))
		const maxChildrenShown = 10
		for i, child := range wu.ChildWorkUnits {
			if i >= maxChildrenShown {
				fmt.Printf("  ... and %d more child work units\n", len(wu.ChildWorkUnits)-maxChildrenShown)
				break
			}
			_, childWUID := base.ExtractWorkUnitComponents(child)
			if childWUID == "" {
				childWUID = child
			}
			fmt.Printf("  - %s\n", childWUID)
		}
	}
	if wu.SummaryMarkdown != "" {
		fmt.Printf("Summary:\n%s\n", wu.SummaryMarkdown)
	} else {
		discErr, _ := format.DiscoverWorkUnitError(ctx, client, httpClient, clean)
		if discErr != nil {
			format.PrintModuleErrorForTarget(discErr, clean, "")
		}
	}
	if r.showMetadata {
		fmt.Println()
		format.FprintWorkUnitMetadata(os.Stdout, wu, "", false)
		if resolved.RootInvocation != nil {
			fmt.Println()
			format.FprintRootInvocationMetadata(os.Stdout, resolved.RootInvocation)
			format.FprintRootInvocationAdditionalMetadata(os.Stdout, resolved.RootInvocation)
		}
	}

	return 0
}

func fetchRootInvocationAggregation(ctx context.Context, client pb.ResultDBClient, normInv string) *pb.TestAggregation {
	if client == nil || normInv == "" {
		return nil
	}
	resp, err := client.QueryTestAggregations(ctx, &pb.QueryTestAggregationsRequest{
		Parent: "rootInvocations/" + normInv,
		Predicate: &pb.TestAggregationPredicate{
			AggregationLevel: pb.AggregationLevel_INVOCATION,
		},
		PageSize: 1,
	})
	if err != nil || resp == nil || len(resp.Aggregations) == 0 {
		return nil
	}
	return resp.Aggregations[0]
}

func printRootInvocationMetadata(rootInv *pb.RootInvocation) {
	if rootInv == nil {
		return
	}
	if realm := rootInv.GetRealm(); realm != "" {
		fmt.Printf("Realm:        %s\n", realm)
	}
	if ab := rootInv.GetPrimaryBuild().GetAndroidBuild(); ab != nil {
		if ab.GetBranch() != "" {
			fmt.Printf("Branch:       %s\n", ab.GetBranch())
		}
		if ab.GetBuildTarget() != "" {
			fmt.Printf("Target:       %s\n", ab.GetBuildTarget())
		}
		if ab.GetBuildId() != "" {
			fmt.Printf("Build ID:     %s\n", ab.GetBuildId())
		}
	}
	if sys := rootInv.GetProducerResource().GetSystem(); sys != "" {
		fmt.Printf("Scheduler:    %s\n", sys)
	}
	tags := make(map[string]string, len(rootInv.GetTags()))
	for _, t := range rootInv.GetTags() {
		if k, v := t.GetKey(), t.GetValue(); k != "" && v != "" && tags[k] == "" {
			tags[k] = v
		}
	}
	for _, item := range []struct {
		key   string
		label string
	}{
		{key: "runner", label: "Runner:       "},
		{key: "result_link", label: "Result Link:  "},
		{key: "sponge_invocation_id", label: "Sponge Invocation ID: "},
		{key: "test_uri", label: "Test URI:     "},
		{key: "runner_url", label: "Runner URL:   "},
	} {
		if v := tags[item.key]; v != "" {
			fmt.Printf("%s%s\n", item.label, v)
		}
	}
}
