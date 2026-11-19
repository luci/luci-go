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

package module

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/maruel/subcommands"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/types/known/structpb"

	"go.chromium.org/luci/client/cmd/luci/artifact"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/client/cmd/luci/ids"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// Cmd returns the top-level command for `luci module`.
func Cmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "module <subcommand>",
		ShortDesc: "Inspect ResultDB modules and aggregations",
		LongDesc: "Inspect ResultDB modules, their overall execution status across shards,\n" +
			"test verdict counts, and shard-level harness errors.",
		CommandRun: func() subcommands.CommandRun {
			return &moduleRun{af: af}
		},
	}
}

type moduleRun struct {
	subcommands.CommandRunBase
	af *base.AuthFlags
}

func (r *moduleRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return base.RunSubcommandApp(a, "luci module", "Module management", []*subcommands.Command{
		GetCmd(r.af),
		subcommands.CmdHelp,
	}, args)
}

// GetCmd returns the subcommand for `luci module get`.
func GetCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get [-invocationid <invocation_id> -modulename <module_name>] [<target>]",
		ShortDesc: "Get module status, verdict counts, and shard errors",
		LongDesc: "Get details and shard execution status for a specific test module within a root invocation.\n\n" +
			"Examples:\n" +
			"  $ luci module get https://ci.chromium.org/ui/inv/ants-i14600010609614895/modules/CellBroadcastReceiverMTS\n" +
			"  $ luci module get -invocationid ants-i14600010609614895 -modulename CellBroadcastReceiverMTS",
		CommandRun: func() subcommands.CommandRun {
			r := &moduleGetRun{af: af}
			if r.af != nil {
				r.af.Register(&r.Flags)
			}
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.StringVar(&r.invocationID, "invocationid", "", "Root invocation ID (e.g. ants-i... or build-867...)")
			r.Flags.StringVar(&r.invocationID, "invocation-id", "", "Alias for -invocationid")
			r.Flags.StringVar(&r.moduleName, "modulename", "", "Module name (e.g. CellBroadcastReceiverMTS)")
			r.Flags.StringVar(&r.moduleName, "module-name", "", "Alias for -modulename")
			r.Flags.StringVar(&r.moduleName, "module", "", "Alias for -modulename")
			r.Flags.BoolVar(&r.showMetadata, "show-metadata", false, "Show additional module metadata")
			r.Flags.BoolVar(&r.jsonOut, "json", false, "Output module details in JSON format")
			return r
		},
	}
}

type moduleGetRun struct {
	subcommands.CommandRunBase
	af           *base.AuthFlags
	client       pb.ResultDBClient
	host         string
	invocationID string
	moduleName   string
	showMetadata bool
	jsonOut      bool
}

func (r *moduleGetRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if base.HasHelpFlag(args) {
		r.Flags.Usage()
		return 0
	}
	target, err := base.ParsePositionalTarget(&r.Flags, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	if target == "" && (r.invocationID == "" || r.moduleName == "") {
		fmt.Fprintf(os.Stderr, "flags -invocationid and -modulename (or a positional URL/ID) are required\n")
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

	extracted, err := ids.ExtractWithAuth(ctx, r.af, client, nil, target, ids.TargetBindings{
		InvocationID: &r.invocationID,
		ModuleName:   &r.moduleName,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to extract IDs from %q: %s\n", target, err)
		return 1
	}
	if r.invocationID == "" || r.moduleName == "" {
		ids.PrintMissingFlagsError(os.Stderr, extracted, "flags -invocationid and -modulename (or a positional URL/ID containing them) are required")
		return 1
	}

	resolved, err := base.ResolveInvocation(ctx, client, r.invocationID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to fetch module details: %s\n", err)
		return 1
	}
	format.CacheResolvedInvocation(ctx, resolved)
	if resolved.Legacy {
		fmt.Fprintf(os.Stderr, "modules are only supported for root invocations (invocation %q is a legacy invocation)\n", resolved.ID)
		return 1
	}

	modDetails, err := FetchModuleDetails(ctx, client, httpClient, resolved.ID, r.moduleName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to fetch module details: %s\n", err)
		return 1
	}

	if r.jsonOut {
		ids.PrintDerivedIDs(os.Stderr, extracted)
		data, err := json.MarshalIndent(modDetails, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal JSON: %s\n", err)
			return 1
		}
		fmt.Println(string(data))
		return 0
	}

	var derivedBuf bytes.Buffer
	ids.PrintDerivedIDs(&derivedBuf, extracted)

	printModuleDetailsWithDerived(derivedBuf.String(), modDetails, r.showMetadata)
	return 0
}

const (
	maxSucceededModuleWorkUnitsForArtifacts = 5
	maxDisplayedModuleWorkUnits             = 20
	maxModuleDetailsHeaderBytes             = 6000
	maxModuleSummaryBytes                   = 4096
)

// ModuleDetails contains aggregate information, test counts, and work unit details for a module.
type ModuleDetails struct {
	InvocationID    string             `json:"invocation_id"`
	ModuleName      string             `json:"module_name"`
	ModuleStatus    string             `json:"module_status"`
	TotalVerdicts   *VerdictCountsJSON `json:"total_verdicts,omitempty"`
	WorkUnits       []*WorkUnitInfo    `json:"work_units,omitempty"`
	RootInvocation  *pb.RootInvocation `json:"-"`
	ArtifactNotices string             `json:"-"`
	formatNotices   func(maxBytes int) string
}

// VerdictCountsJSON contains test verdict counts for JSON output.
type VerdictCountsJSON struct {
	Passed           int32 `json:"passed"`
	Failed           int32 `json:"failed"`
	Flaky            int32 `json:"flaky"`
	Skipped          int32 `json:"skipped"`
	ExecutionErrored int32 `json:"execution_errored"`
	Precluded        int32 `json:"precluded"`
	Exonerated       int32 `json:"exonerated"`
	Total            int32 `json:"total"`
}

// WorkUnitInfo contains details about a work unit executing part of a module.
type WorkUnitInfo struct {
	WorkUnitID    string                    `json:"work_unit_id"`
	FullName      string                    `json:"full_name,omitempty"`
	Parent        string                    `json:"parent,omitempty"`
	Kind          string                    `json:"kind,omitempty"`
	State         string                    `json:"state"`
	ShardKey      string                    `json:"shard_key,omitempty"`
	ModuleScheme  string                    `json:"module_scheme,omitempty"`
	Variant       *pb.Variant               `json:"variant,omitempty"`
	VariantHash   string                    `json:"variant_hash,omitempty"`
	Tags          []*pb.StringPair          `json:"tags,omitempty"`
	Properties    *structpb.Struct          `json:"properties,omitempty"`
	Summary       string                    `json:"summary,omitempty"`
	Error         *format.DiscoveredError   `json:"error,omitempty"`
	ChildFailures []*format.WorkUnitFailure `json:"-"`
	ArtifactList  []string                  `json:"artifacts,omitempty"`
}

// FetchModuleDetails queries ResultDB for module aggregations and associated work units.
func FetchModuleDetails(ctx context.Context, client pb.ResultDBClient, httpClient *http.Client, invID, moduleName string) (*ModuleDetails, error) {
	ctx = format.WithDiscoveryCache(ctx)
	normInv := base.NormalizeInvocation(invID)
	rootInvName := "rootInvocations/" + normInv

	details := &ModuleDetails{
		InvocationID:   normInv,
		ModuleName:     moduleName,
		ModuleStatus:   "STATUS_UNSPECIFIED",
		RootInvocation: format.FetchRootInvocation(ctx, client, normInv),
	}

	// 1. Query Test Aggregations for the module
	reqAgg := &pb.QueryTestAggregationsRequest{
		Parent: rootInvName,
		Predicate: &pb.TestAggregationPredicate{
			AggregationLevel: pb.AggregationLevel_MODULE,
			ContentsFilter:   fmt.Sprintf("test_id_structured.module_name = %q", moduleName),
		},
		PageSize: 1000,
	}

	resAgg, errAgg := client.QueryTestAggregations(ctx, reqAgg)
	if errAgg != nil {
		if base.IsPermissionDenied(errAgg) {
			return nil, errors.Fmt("query test aggregations for %s (%s): %w", rootInvName, base.ResultDBPermissionReason(rootInvName, errAgg), errAgg)
		}
		return nil, errors.Fmt("query test aggregations for %s: %w", rootInvName, errAgg)
	}
	if resAgg != nil {
		for _, agg := range resAgg.Aggregations {
			if agg.Id != nil && agg.Id.Id != nil && agg.Id.Id.ModuleName == moduleName {
				details.ModuleStatus = agg.ModuleStatus.String()
				if agg.TotalVerdictCounts != nil {
					c := agg.TotalVerdictCounts
					total := c.Passed + c.Failed + c.Flaky + c.Skipped + c.ExecutionErrored + c.Precluded
					details.TotalVerdicts = &VerdictCountsJSON{
						Passed:           c.Passed,
						Failed:           c.Failed,
						Flaky:            c.Flaky,
						Skipped:          c.Skipped,
						ExecutionErrored: c.ExecutionErrored,
						Precluded:        c.Precluded,
						Exonerated:       c.Exonerated,
						Total:            total,
					}
				}
				break
			}
		}
	}

	hasVerdictFailures := details.TotalVerdicts != nil &&
		(details.TotalVerdicts.Failed > 0 || details.TotalVerdicts.ExecutionErrored > 0 || details.TotalVerdicts.Precluded > 0)
	noTestsRan := details.TotalVerdicts != nil && details.TotalVerdicts.Total == 0

	// 2. Discover Work Units for this module
	workUnits, err := discoverModuleWorkUnits(ctx, client, httpClient, rootInvName, moduleName, hasVerdictFailures, noTestsRan)
	if err != nil {
		return nil, errors.Fmt("discover module work units: %w", err)
	}
	details.WorkUnits = workUnits

	allSucceeded := len(workUnits) > 0
	anyFailed := false
	shardHasFailed := make(map[string]bool)
	shardHasClean := make(map[string]bool)
	for _, wu := range workUnits {
		if wu.State == pb.WorkUnit_FAILED.String() || wu.Error != nil {
			anyFailed = true
			allSucceeded = false
			shardHasFailed[wu.ShardKey] = true
		} else if wu.State == pb.WorkUnit_SUCCEEDED.String() || wu.State == pb.WorkUnit_SKIPPED.String() {
			shardHasClean[wu.ShardKey] = true
		} else {
			allSucceeded = false
		}
	}
	anyUnrecoveredShard := false
	for shardKey := range shardHasFailed {
		if !shardHasClean[shardKey] {
			anyUnrecoveredShard = true
			break
		}
	}

	// Report FAILED when verdicts failed/errored/precluded or at least one shard
	// failed without a clean retry; if failed shard attempts were all recovered by
	// clean retries on the same shard key, preserve or upgrade to FLAKY.
	if anyUnrecoveredShard || hasVerdictFailures {
		details.ModuleStatus = pb.TestAggregation_FAILED.String()
	} else if anyFailed && (details.ModuleStatus == pb.TestAggregation_SUCCEEDED.String() ||
		details.ModuleStatus == "STATUS_UNSPECIFIED" || details.ModuleStatus == "MODULE_STATUS_UNSPECIFIED") {
		details.ModuleStatus = pb.TestAggregation_FLAKY.String()
	}

	// If ModuleStatus was unspecified from aggregations but we found work units, infer status
	if details.ModuleStatus == "STATUS_UNSPECIFIED" || details.ModuleStatus == "MODULE_STATUS_UNSPECIFIED" {
		if allSucceeded {
			details.ModuleStatus = pb.TestAggregation_SUCCEEDED.String()
		}
	}

	if details.TotalVerdicts == nil && len(details.WorkUnits) == 0 && (details.ModuleStatus == "STATUS_UNSPECIFIED" || details.ModuleStatus == "MODULE_STATUS_UNSPECIFIED") {
		return nil, errors.Fmt("no module found matching %q in %s", moduleName, rootInvName)
	}

	orderedWUs := slices.Clone(workUnits)
	slices.SortStableFunc(orderedWUs, func(a, b *WorkUnitInfo) int {
		return cmp.Compare(workUnitSortRank(a), workUnitSortRank(b))
	})

	var targetWUNames []string
	for _, wu := range orderedWUs {
		if anyFailed && wu.State != pb.WorkUnit_FAILED.String() && wu.Error == nil {
			continue
		}
		if wu.FullName != "" {
			targetWUNames = append(targetWUNames, wu.FullName)
			if !anyFailed && len(targetWUNames) >= maxSucceededModuleWorkUnitsForArtifacts {
				break
			}
		}
	}
	formatter, directArtIDs := artifact.FetchWorkUnitAndAncestorNoticeFormatter(ctx, client, targetWUNames)
	details.formatNotices = formatter
	if formatter != nil {
		details.ArtifactNotices = formatter(format.DefaultInlineGetArtifactBudget)
	}
	for _, wu := range workUnits {
		if artIDs := directArtIDs[wu.FullName]; len(artIDs) > 0 {
			wu.ArtifactList = artIDs
		}
	}

	return details, nil
}

// discoverModuleWorkUnits traverses the invocation's work unit tree starting from "root"
// to discover top-level work units for the specified module.
//
// Invariant: ResultDB enforces that if a work unit has module_id set, all child work
// units must have the same module_id set. Therefore:
//  1. If a work unit has module_id matching moduleName, it is a top-level module work unit
//     (or shard). We collect it and only fetch its direct children when it is a TF_MODULE
//     (to inspect child TF_TEST_RUN work units without exploring individual test cases).
//  2. If a work unit has module_id set to a different module, we prune the entire subtree
//     and do NOT explore its children.
//  3. Only intermediate container work units (module_id == nil) have their children explored.
//
// This bounds the traversal to O(modules + container_depth), requiring only a handful of
// batch RPCs even on invocations with hundreds of thousands of test case work units.
func discoverModuleWorkUnits(ctx context.Context, client pb.ResultDBClient, httpClient *http.Client, rootInvName, moduleName string, hasVerdictFailures, noTestsRan bool) ([]*WorkUnitInfo, error) {
	rootWUName := rootInvName + "/workUnits/root"
	rootWU, err := client.GetWorkUnit(ctx, &pb.GetWorkUnitRequest{
		Name: rootWUName,
		View: pb.WorkUnitView_WORK_UNIT_VIEW_BASIC,
	})
	if err != nil {
		if base.IsPermissionDenied(err) {
			return nil, errors.Fmt("get root work unit %s (%s): %w", rootWUName, base.ResultDBPermissionReason(rootInvName, err), err)
		}
		return nil, errors.Fmt("get root work unit %s: %w", rootWUName, err)
	}
	if rootWU == nil {
		return nil, errors.Fmt("root work unit %s not found", rootWUName)
	}
	format.CacheWorkUnit(ctx, rootWU)

	var matchedWUs []*pb.WorkUnit

	if rootWU.ModuleId != nil {
		if rootWU.ModuleId.ModuleName == moduleName {
			matchedWUs = append(matchedWUs, rootWU)
		}
		// Invariant: all children inherit the same module_id, so we stop here.
	} else {
		visited := make(map[string]bool)
		queue := append([]string{}, rootWU.ChildWorkUnits...)

		for depth := 0; depth < 10 && len(queue) > 0; depth++ {
			currentBatch := queue
			queue = nil

			var toFetch []string
			for _, name := range currentBatch {
				if !visited[name] {
					visited[name] = true
					toFetch = append(toFetch, name)
				}
			}
			if len(toFetch) == 0 {
				break
			}

			for i := 0; i < len(toFetch); i += 500 {
				end := i + 500
				if end > len(toFetch) {
					end = len(toFetch)
				}
				batchRes, err := client.BatchGetWorkUnits(ctx, &pb.BatchGetWorkUnitsRequest{
					Parent: rootInvName,
					Names:  toFetch[i:end],
					View:   pb.WorkUnitView_WORK_UNIT_VIEW_BASIC,
				})
				if err != nil {
					if base.IsPermissionDenied(err) {
						return nil, errors.Fmt("batch get work units (%s): %w", base.ResultDBPermissionReason(rootInvName, err), err)
					}
					return nil, errors.Fmt("batch get work units: %w", err)
				}
				if batchRes == nil {
					continue
				}

				for _, wu := range batchRes.WorkUnits {
					if wu == nil {
						continue
					}
					format.CacheWorkUnit(ctx, wu)
					if wu.ModuleId != nil {
						if wu.ModuleId.ModuleName == moduleName {
							matchedWUs = append(matchedWUs, wu)
						}
						// Invariant: ResultDB enforces that if a work unit sets module_id,
						// all of its descendant work units inherit the same module_id.
						// We do NOT explore children of any work unit with module_id set.
						continue
					}

					// Only intermediate container work units (module_id == nil)
					// can have children belonging to different modules.
					for _, child := range wu.ChildWorkUnits {
						if !visited[child] {
							queue = append(queue, child)
						}
					}
				}
			}
		}
	}

	childrenByName := make(map[string]*pb.WorkUnit)
	var childToFetch []string
	seenChildren := make(map[string]bool)
	for _, wu := range matchedWUs {
		if wu.Kind != "TF_MODULE" {
			continue
		}
		for _, childName := range wu.ChildWorkUnits {
			if !seenChildren[childName] {
				seenChildren[childName] = true
				childToFetch = append(childToFetch, childName)
			}
		}
	}
	for i := 0; i < len(childToFetch); i += 500 {
		end := i + 500
		if end > len(childToFetch) {
			end = len(childToFetch)
		}
		batchRes, err := client.BatchGetWorkUnits(ctx, &pb.BatchGetWorkUnitsRequest{
			Parent: rootInvName,
			Names:  childToFetch[i:end],
			View:   pb.WorkUnitView_WORK_UNIT_VIEW_BASIC,
		})
		if err != nil {
			if base.IsPermissionDenied(err) {
				return nil, errors.Fmt("batch get work units (%s): %w", base.ResultDBPermissionReason(rootInvName, err), err)
			}
			return nil, errors.Fmt("batch get work units: %w", err)
		}
		for _, childWU := range batchRes.GetWorkUnits() {
			if childWU == nil {
				continue
			}
			format.CacheWorkUnit(ctx, childWU)
			childrenByName[childWU.Name] = childWU
		}
	}

	type shardPrep struct {
		info                     *WorkUnitInfo
		wu                       *pb.WorkUnit
		wuIsFallback             bool
		failedChildren           []*pb.WorkUnit
		childSummaries           []string
		hasNonGenericDirectLayer bool
		fetchChildArtifacts      bool
		hasAncestorLayer1        bool
		hasFailedOrGenAncestor   bool
		runWithHTTP              bool
	}
	preps := make([]*shardPrep, len(matchedWUs))
	anyDirectFailed := false
	childArtifactShards := 0

	for i, wu := range matchedWUs {
		info := &WorkUnitInfo{
			WorkUnitID:   wu.WorkUnitId,
			FullName:     wu.Name,
			Parent:       wu.Parent,
			Kind:         wu.Kind,
			State:        wu.State.String(),
			ShardKey:     wu.ModuleShardKey,
			ModuleScheme: wu.GetModuleId().GetModuleScheme(),
			Variant:      wu.GetModuleId().GetModuleVariant(),
			VariantHash:  wu.GetModuleId().GetModuleVariantHash(),
			Tags:         wu.GetTags(),
			Properties:   wu.GetProperties(),
			Summary:      wu.SummaryMarkdown,
		}
		wuIsFallback := format.IsFallbackWorkUnitSummary(ctx, client, wu.Name, wu.SummaryMarkdown)
		hasDirect := format.HasNonFallbackErrorSummary(ctx, client, wu)
		var failedChildren []*pb.WorkUnit
		var childSummaries []string
		if wu.Kind == "TF_MODULE" {
			for _, childName := range wu.ChildWorkUnits {
				childWU := childrenByName[childName]
				if childWU == nil || (childWU.Kind != "" && childWU.Kind != "TF_TEST_RUN") {
					continue
				}
				if childWU.State == pb.WorkUnit_FAILED {
					info.State = pb.WorkUnit_FAILED.String()
					failedChildren = append(failedChildren, childWU)
					cs := strings.TrimSpace(childWU.SummaryMarkdown)
					if cs != "" && !format.IsGenericHarnessSummary(cs) {
						hasDirect = true
						if !slices.Contains(childSummaries, cs) {
							childSummaries = append(childSummaries, cs)
						}
					}
				}
			}
		}
		info.Summary = mergeModuleAndChildSummaries(wu.SummaryMarkdown, wuIsFallback, childSummaries)
		if info.State == pb.WorkUnit_FAILED.String() {
			anyDirectFailed = true
		}
		fetchChild := false
		if len(failedChildren) > 0 && childArtifactShards < maxDisplayedModuleWorkUnits {
			fetchChild = true
			childArtifactShards++
		}
		preps[i] = &shardPrep{
			info:                     info,
			wu:                       wu,
			wuIsFallback:             wuIsFallback,
			failedChildren:           failedChildren,
			childSummaries:           childSummaries,
			hasNonGenericDirectLayer: hasDirect,
			fetchChildArtifacts:      fetchChild,
		}
	}

	order := make([]int, 0, len(preps))
	for i, p := range preps {
		if p.info.State == pb.WorkUnit_FAILED.String() {
			order = append(order, i)
		}
	}
	for i, p := range preps {
		if p.info.State != pb.WorkUnit_FAILED.String() && p.info.State != pb.WorkUnit_SKIPPED.String() && (hasVerdictFailures || noTestsRan) {
			order = append(order, i)
		}
	}

	var prepGroup errgroup.Group
	prepGroup.SetLimit(8)
	for _, idx := range order {
		p := preps[idx]
		if !p.fetchChildArtifacts && (httpClient == nil || p.hasNonGenericDirectLayer) {
			continue
		}
		prepGroup.Go(func() error {
			if p.fetchChildArtifacts {
				var enriched []string
				for _, childWU := range p.failedChildren {
					cf := format.DiscoverSingleWorkUnitFailure(ctx, client, httpClient, childWU)
					if cf == nil {
						continue
					}
					s := cf.FullSummaryText
					if s == "" {
						s = cf.SummaryMarkdown
					}
					if s != "" && !format.IsGenericHarnessSummary(s) {
						p.hasNonGenericDirectLayer = true
						if !slices.Contains(enriched, s) {
							enriched = append(enriched, s)
						}
					}
					if len(cf.Artifacts) > 0 {
						p.info.ChildFailures = append(p.info.ChildFailures, cf)
					}
				}
				if len(enriched) > 0 {
					p.childSummaries = enriched
					p.info.Summary = mergeModuleAndChildSummaries(p.wu.SummaryMarkdown, p.wuIsFallback, enriched)
				}
			}
			if httpClient != nil && !p.hasNonGenericDirectLayer {
				for _, anc := range format.QueryAncestorWorkUnits(ctx, client, p.wu.Name) {
					if anc.GetState() == pb.WorkUnit_SKIPPED {
						continue
					}
					if !strings.HasSuffix(anc.GetName(), "/workUnits/root") &&
						(anc.GetState() == pb.WorkUnit_FAILED || format.IsGenericHarnessSummary(anc.GetSummaryMarkdown())) {
						p.hasFailedOrGenAncestor = true
					}
					if format.HasNonFallbackErrorSummary(ctx, client, anc) {
						p.hasAncestorLayer1 = true
						break
					}
				}
			}
			return nil
		})
	}
	_ = prepGroup.Wait()

	failedLayer2 := 0
	succeededLayer2 := 0
	for _, idx := range order {
		p := preps[idx]
		if httpClient == nil {
			continue
		}
		if p.hasNonGenericDirectLayer || p.hasAncestorLayer1 {
			p.runWithHTTP = true
			continue
		}
		if p.info.State == pb.WorkUnit_FAILED.String() || noTestsRan || p.hasFailedOrGenAncestor {
			if failedLayer2 < maxDisplayedModuleWorkUnits {
				failedLayer2++
				p.runWithHTTP = true
			}
		} else if !anyDirectFailed && succeededLayer2 < maxSucceededModuleWorkUnitsForArtifacts {
			succeededLayer2++
			p.runWithHTTP = true
		}
	}

	runErrorPass := func(withHTTP bool) {
		var eg errgroup.Group
		eg.SetLimit(8)
		for _, idx := range order {
			p := preps[idx]
			if p.runWithHTTP != withHTTP {
				continue
			}
			eg.Go(func() error {
				var hc *http.Client
				if withHTTP {
					hc = httpClient
				}
				discErr, _ := format.DiscoverWorkUnitError(ctx, client, hc, p.wu.Name)
				if discErr != nil && p.info.State != pb.WorkUnit_FAILED.String() &&
					discErr.ArtifactID == "" && p.wu.Name != rootWUName &&
					(discErr.SourceWorkUnit == rootWUName || discErr.SourceWorkUnit == rootInvName) {
					discErr = nil
				}
				if discErr != nil {
					cloned := *discErr
					if len(p.childSummaries) > 0 && cloned.Message == "" &&
						(cloned.SourceWorkUnit == p.wu.Name || childrenByName[cloned.SourceWorkUnit] != nil) {
						cloned.RawSummary = p.info.Summary
					}
					p.info.Error = &cloned
				}
				if len(p.childSummaries) > 0 && len(p.failedChildren) > 0 &&
					(p.info.Error == nil || (p.info.Error.SourceWorkUnit != p.wu.Name && childrenByName[p.info.Error.SourceWorkUnit] == nil)) {
					child := p.failedChildren[0]
					p.info.Error = &format.DiscoveredError{
						Source:          fmt.Sprintf("Child Work Unit %s", child.Name),
						SourceWorkUnit:  child.Name,
						IsChildWorkUnit: true,
						RawSummary:      p.info.Summary,
					}
				}
				return nil
			})
		}
		_ = eg.Wait()
	}
	if httpClient != nil {
		runErrorPass(true)
	}
	runErrorPass(false)

	results := make([]*WorkUnitInfo, len(preps))
	for i, p := range preps {
		results[i] = p.info
	}
	return results, nil
}

func mergeModuleAndChildSummaries(wuSummary string, wuIsFallback bool, childSummaries []string) string {
	if len(childSummaries) == 0 {
		return wuSummary
	}
	combined := strings.Join(childSummaries, "\n\n")
	if strings.TrimSpace(wuSummary) == "" || wuIsFallback {
		return combined
	}
	if strings.Contains(wuSummary, combined) {
		return wuSummary
	}
	return strings.TrimSpace(wuSummary) + "\n\n" + combined
}

func (wu *WorkUnitInfo) toWorkUnitProto() *pb.WorkUnit {
	if wu == nil {
		return nil
	}
	var modID *pb.ModuleIdentifier
	if wu.ModuleScheme != "" || wu.Variant != nil || wu.VariantHash != "" {
		modID = &pb.ModuleIdentifier{
			ModuleScheme:      wu.ModuleScheme,
			ModuleVariant:     wu.Variant,
			ModuleVariantHash: wu.VariantHash,
		}
	}
	return &pb.WorkUnit{
		Name:       wu.FullName,
		Parent:     wu.Parent,
		ModuleId:   modID,
		Tags:       wu.Tags,
		Properties: wu.Properties,
	}
}

func workUnitSortRank(w *WorkUnitInfo) int {
	if w.State == pb.WorkUnit_FAILED.String() {
		return 0
	}
	if w.Error != nil {
		return 1
	}
	return 2
}

func truncateModuleSummary(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	end := maxBytes
	for end > 0 && !utf8.RuneStart(s[end]) {
		end--
	}
	return s[:end] + "\n... (truncated)"
}

func printModuleDetailsWithDerived(derivedPrefix string, details *ModuleDetails, showMetadata bool) {
	fprintModuleDetailsWithDerived(os.Stdout, derivedPrefix, details, showMetadata)
}

func fprintModuleDetails(out io.Writer, details *ModuleDetails, showMetadata bool) {
	fprintModuleDetailsWithDerived(out, "", details, showMetadata)
}

func fprintModuleDetailsWithDerived(out io.Writer, derivedPrefix string, details *ModuleDetails, showMetadata bool) {
	var buf bytes.Buffer
	if derivedPrefix != "" {
		buf.WriteString(derivedPrefix)
	}
	statusClean := strings.TrimPrefix(details.ModuleStatus, "MODULE_STATUS_")
	fmt.Fprintf(&buf, "Module:        %s\n", details.ModuleName)
	fmt.Fprintf(&buf, "Invocation:    rootInvocations/%s\n", details.InvocationID)
	fmt.Fprintf(&buf, "Status:        %s\n", statusClean)

	if details.TotalVerdicts != nil {
		v := details.TotalVerdicts
		if v.Total == 0 {
			fmt.Fprintf(&buf, "Test Verdicts: 0 tests ran\n")
		} else {
			var parts []string
			if v.Failed > 0 {
				parts = append(parts, fmt.Sprintf("%d failed", v.Failed))
			}
			if v.Flaky > 0 {
				parts = append(parts, fmt.Sprintf("%d flaky", v.Flaky))
			}
			if v.ExecutionErrored > 0 {
				parts = append(parts, fmt.Sprintf("%d execution errored", v.ExecutionErrored))
			}
			if v.Precluded > 0 {
				parts = append(parts, fmt.Sprintf("%d precluded", v.Precluded))
			}
			if v.Passed > 0 {
				parts = append(parts, fmt.Sprintf("%d passed", v.Passed))
			}
			if v.Skipped > 0 {
				parts = append(parts, fmt.Sprintf("%d skipped", v.Skipped))
			}
			summary := strings.Join(parts, ", ")
			if summary == "" {
				summary = fmt.Sprintf("%d tests", v.Total)
			}
			fmt.Fprintf(&buf, "Test Verdicts: %s (%d tests ran)\n", summary, v.Total)
		}
	}

	if len(details.WorkUnits) > 0 {
		fmt.Fprintln(&buf, "\nShards & Work Units:")
		displayWUs := slices.Clone(details.WorkUnits)
		slices.SortStableFunc(displayWUs, func(a, b *WorkUnitInfo) int {
			return cmp.Compare(workUnitSortRank(a), workUnitSortRank(b))
		})
		shownWUs := 0
		seenAncestorErrors := make(map[string]bool)
		for i, wu := range displayWUs {
			if !showMetadata && (shownWUs >= maxDisplayedModuleWorkUnits || (shownWUs > 0 && buf.Len() >= maxModuleDetailsHeaderBytes)) {
				fmt.Fprintf(&buf, "  ... and %d more work units\n", len(displayWUs)-i)
				break
			}
			var wuBuf bytes.Buffer
			stateClean := strings.TrimPrefix(wu.State, "STATE_")
			var meta []string
			if wu.ShardKey != "" {
				meta = append(meta, fmt.Sprintf("Shard: %s", wu.ShardKey))
			}
			if wu.Kind != "" {
				meta = append(meta, fmt.Sprintf("Kind: %s", wu.Kind))
			}
			metaDesc := ""
			if len(meta) > 0 {
				metaDesc = fmt.Sprintf(" (%s)", strings.Join(meta, ", "))
			}
			fmt.Fprintf(&wuBuf, "  [%s] %s%s\n", stateClean, wu.WorkUnitID, metaDesc)

			if wu.Error != nil {
				isAncestorErr := wu.Error.SourceWorkUnit != "" && wu.FullName != "" &&
					wu.Error.SourceWorkUnit != wu.FullName && !wu.Error.IsChildWorkUnit &&
					strings.TrimSpace(wu.Summary) == ""
				errKey := wu.Error.SourceWorkUnit + "\x00" + wu.Error.RawSummary + "\x00" + wu.Error.Message
				if !isAncestorErr || !seenAncestorErrors[errKey] {
					if isAncestorErr {
						seenAncestorErrors[errKey] = true
					}
					if wu.Error.Message != "" {
						fmt.Fprintf(&wuBuf, "    Error: %s\n", wu.Error.Message)
					} else if wu.Error.RawSummary != "" {
						rawSummary := wu.Error.RawSummary
						if !showMetadata {
							rawSummary = truncateModuleSummary(rawSummary, maxModuleSummaryBytes)
						}
						fmt.Fprintf(&wuBuf, "    Summary:\n      %s\n", strings.ReplaceAll(rawSummary, "\n", "\n      "))
					}
					if wu.Error.StackTrace != "" {
						fmt.Fprintf(&wuBuf, "    Stack Trace:\n      %s\n", strings.ReplaceAll(wu.Error.StackTrace, "\n", "\n      "))
					}
				}
			} else if strings.TrimSpace(wu.Summary) != "" {
				summary := strings.TrimSpace(wu.Summary)
				if !showMetadata {
					summary = truncateModuleSummary(summary, maxModuleSummaryBytes)
				}
				fmt.Fprintf(&wuBuf, "    Summary:\n      %s\n", strings.ReplaceAll(summary, "\n", "\n      "))
			}
			for _, cf := range wu.ChildFailures {
				format.FprintWorkUnitFailureArtifacts(&wuBuf, details.InvocationID, cf, "    ")
			}

			if showMetadata {
				format.FprintWorkUnitMetadata(&wuBuf, wu.toWorkUnitProto(), "    ", true)
			}
			if !showMetadata && shownWUs > 0 && buf.Len()+wuBuf.Len() > maxModuleDetailsHeaderBytes {
				fmt.Fprintf(&buf, "  ... and %d more work units\n", len(displayWUs)-i)
				break
			}
			buf.Write(wuBuf.Bytes())
			shownWUs++
		}
	}

	if showMetadata && details.RootInvocation != nil {
		fmt.Fprintln(&buf)
		format.FprintRootInvocationMetadata(&buf, details.RootInvocation)
		format.FprintRootInvocationAdditionalMetadata(&buf, details.RootInvocation)
	}

	formatInline := details.formatNotices
	if formatInline == nil && details.ArtifactNotices != "" {
		notices := details.ArtifactNotices
		formatInline = func(maxBytes int) string {
			if maxBytes > 0 && len(notices) > maxBytes {
				return ""
			}
			return notices
		}
	}
	fmt.Fprint(out, format.FitPrimaryAndInlineArtifacts(buf.String(), formatInline, !showMetadata))
}
