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
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
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
		UsageLine: "get -invocationid <invocation_id> -modulename <module_name>",
		ShortDesc: "Get module status, verdict counts, and shard errors",
		LongDesc: "Get details and shard execution status for a specific test module within a root invocation.\n\n" +
			"Example:\n" +
			"  $ luci module get -invocationid ants-i14600010609614895 -modulename CellBroadcastReceiverMTS",
		CommandRun: func() subcommands.CommandRun {
			r := &moduleGetRun{af: af}
			r.af.Register(&r.Flags)
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.StringVar(&r.invocationID, "invocationid", "", "Root invocation ID (e.g. ants-i... or build-867...)")
			r.Flags.StringVar(&r.moduleName, "modulename", "", "Module name (e.g. CellBroadcastReceiverMTS)")
			r.Flags.StringVar(&r.moduleName, "module", "", "Alias for -modulename")
			r.Flags.BoolVar(&r.showArtifacts, "show-artifacts", false, "Show work unit artifacts for failed shards")
			r.Flags.BoolVar(&r.showMetadata, "show-metadata", false, "Show additional module metadata")
			r.Flags.BoolVar(&r.jsonOut, "json", false, "Output module details in JSON format")
			return r
		},
	}
}

type moduleGetRun struct {
	subcommands.CommandRunBase
	af            *base.AuthFlags
	host          string
	invocationID  string
	moduleName    string
	showArtifacts bool
	showMetadata  bool
	jsonOut       bool
}

func (r *moduleGetRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			r.Flags.Usage()
			return 0
		}
	}
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "unexpected positional arguments; use flags -invocationid and -modulename (run 'luci ids <url>' to extract ids)\n")
		return 1
	}
	if r.invocationID == "" || r.moduleName == "" {
		fmt.Fprintf(os.Stderr, "flags -invocationid and -modulename are required (run 'luci ids <url>' to extract ids)\n")
		return 1
	}

	ctx := cli.GetContext(a, r, env)
	if err := r.af.Parse(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
		return 1
	}

	ctx = format.WithDiscoveryCache(ctx)
	client, _, httpClient, err := r.af.NewResultDBClient(ctx, r.host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create resultdb client: %s\n", err)
		return 1
	}

	modDetails, err := FetchModuleDetails(ctx, client, httpClient, r.invocationID, r.moduleName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to fetch module details: %s\n", err)
		return 1
	}

	if r.jsonOut {
		data, err := json.MarshalIndent(modDetails, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal JSON: %s\n", err)
			return 1
		}
		fmt.Println(string(data))
		return 0
	}

	printModuleDetails(modDetails, r.showArtifacts, r.showMetadata)
	return 0
}

// ModuleDetails contains aggregate information, test counts, and work unit details for a module.
type ModuleDetails struct {
	InvocationID  string             `json:"invocation_id"`
	ModuleName    string             `json:"module_name"`
	ModuleStatus  string             `json:"module_status"`
	TotalVerdicts *VerdictCountsJSON `json:"total_verdicts,omitempty"`
	WorkUnits     []*WorkUnitInfo    `json:"work_units,omitempty"`
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
	WorkUnitID   string                  `json:"work_unit_id"`
	Kind         string                  `json:"kind,omitempty"`
	State        string                  `json:"state"`
	ShardKey     string                  `json:"shard_key,omitempty"`
	Summary      string                  `json:"summary,omitempty"`
	Error        *format.DiscoveredError `json:"error,omitempty"`
	ArtifactList []string                `json:"artifacts,omitempty"`
}

// FetchModuleDetails queries ResultDB for module aggregations and associated work units.
func FetchModuleDetails(ctx context.Context, client pb.ResultDBClient, httpClient *http.Client, invID, moduleName string) (*ModuleDetails, error) {
	normInv := base.NormalizeInvocation(invID)
	rootInvName := "rootInvocations/" + normInv

	details := &ModuleDetails{
		InvocationID: normInv,
		ModuleName:   moduleName,
		ModuleStatus: "STATUS_UNSPECIFIED",
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
		return nil, errors.Fmt("failed to query test aggregations for %s: %w", rootInvName, errAgg)
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

	// 2. Discover Work Units for this module
	workUnits, err := discoverModuleWorkUnits(ctx, client, httpClient, rootInvName, moduleName)
	if err != nil {
		return nil, errors.Fmt("failed to discover module work units: %w", err)
	}
	details.WorkUnits = workUnits

	allSucceeded := len(workUnits) > 0
	anyFailed := false
	for _, wu := range workUnits {
		if wu.State == pb.WorkUnit_FAILED.String() || wu.Error != nil {
			anyFailed = true
			allSucceeded = false
		} else if wu.State != pb.WorkUnit_SUCCEEDED.String() && wu.State != pb.WorkUnit_SKIPPED.String() {
			allSucceeded = false
		}
	}

	// If no tests ran and there were failed work units or discovered errors, the module execution failed
	if details.TotalVerdicts == nil || details.TotalVerdicts.Total == 0 {
		if anyFailed {
			details.ModuleStatus = pb.TestAggregation_FAILED.String()
		}
	}

	// If ModuleStatus was unspecified from aggregations but we found work units, infer status
	if details.ModuleStatus == "STATUS_UNSPECIFIED" || details.ModuleStatus == "MODULE_STATUS_UNSPECIFIED" {
		if anyFailed {
			details.ModuleStatus = pb.TestAggregation_FAILED.String()
		} else if allSucceeded {
			details.ModuleStatus = pb.TestAggregation_SUCCEEDED.String()
		}
	}

	if details.TotalVerdicts == nil && len(details.WorkUnits) == 0 && (details.ModuleStatus == "STATUS_UNSPECIFIED" || details.ModuleStatus == "MODULE_STATUS_UNSPECIFIED") {
		return nil, errors.Fmt("no module found matching %q in %s", moduleName, rootInvName)
	}

	return details, nil
}

// discoverModuleWorkUnits traverses the invocation's work unit tree starting from "root"
// to discover top-level work units for the specified module.
//
// Invariant: ResultDB enforces that if a work unit has module_id set, all child work
// units must have the same module_id set. Therefore:
//  1. If a work unit has module_id matching moduleName, it is a top-level module work unit
//     (or shard). We collect it and do NOT explore its children.
//  2. If a work unit has module_id set to a different module, we prune the entire subtree
//     and do NOT explore its children.
//  3. Only intermediate container work units (module_id == nil) have their children explored.
//
// This bounds the traversal to O(modules + container_depth), requiring only a handful of
// batch RPCs even on invocations with hundreds of thousands of test case work units.
func discoverModuleWorkUnits(ctx context.Context, client pb.ResultDBClient, httpClient *http.Client, rootInvName, moduleName string) ([]*WorkUnitInfo, error) {
	rootWUName := rootInvName + "/workUnits/root"
	rootWU, err := client.GetWorkUnit(ctx, &pb.GetWorkUnitRequest{
		Name: rootWUName,
		View: pb.WorkUnitView_WORK_UNIT_VIEW_BASIC,
	})
	if err != nil {
		return nil, errors.Fmt("failed to get root work unit %s: %w", rootWUName, err)
	}
	if rootWU == nil {
		return nil, errors.Fmt("root work unit %s not found", rootWUName)
	}

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
					return nil, errors.Fmt("failed to batch get work units: %w", err)
				}
				if batchRes == nil {
					continue
				}

				for _, wu := range batchRes.WorkUnits {
					if wu == nil {
						continue
					}
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

	var results []*WorkUnitInfo
	for _, wu := range matchedWUs {
		info := &WorkUnitInfo{
			WorkUnitID: wu.WorkUnitId,
			Kind:       wu.Kind,
			State:      wu.State.String(),
			ShardKey:   wu.ModuleShardKey,
			Summary:    wu.SummaryMarkdown,
		}
		if wu.State == pb.WorkUnit_FAILED {
			discErr, _ := format.DiscoverWorkUnitError(ctx, client, httpClient, wu.Name)
			if discErr != nil {
				info.Error = discErr
			}
		}
		results = append(results, info)
	}

	return results, nil
}

func printModuleDetails(details *ModuleDetails, showArtifacts, showMetadata bool) {
	statusClean := strings.TrimPrefix(details.ModuleStatus, "MODULE_STATUS_")
	fmt.Printf("Module:        %s\n", details.ModuleName)
	fmt.Printf("Invocation:    rootInvocations/%s\n", details.InvocationID)
	fmt.Printf("Status:        %s\n", statusClean)

	if details.TotalVerdicts != nil {
		v := details.TotalVerdicts
		if v.Total == 0 {
			fmt.Printf("Test Verdicts: 0 tests ran\n")
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
			fmt.Printf("Test Verdicts: %s (%d tests ran)\n", summary, v.Total)
		}
	}

	if len(details.WorkUnits) > 0 {
		fmt.Println("\nShards & Work Units:")
		for _, wu := range details.WorkUnits {
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
			fmt.Printf("  [%s] %s%s\n", stateClean, wu.WorkUnitID, metaDesc)

			if wu.Error != nil {
				if wu.Error.Message != "" {
					fmt.Printf("    Error: %s\n", wu.Error.Message)
				} else if wu.Error.RawSummary != "" {
					fmt.Printf("    Summary:\n      %s\n", strings.ReplaceAll(wu.Error.RawSummary, "\n", "\n      "))
				}
				if wu.Error.StackTrace != "" {
					fmt.Printf("    Stack Trace:\n      %s\n", strings.ReplaceAll(wu.Error.StackTrace, "\n", "\n      "))
				}
			} else if strings.TrimSpace(wu.Summary) != "" {
				fmt.Printf("    Summary:\n      %s\n", strings.ReplaceAll(strings.TrimSpace(wu.Summary), "\n", "\n      "))
			}
		}
	}
}
