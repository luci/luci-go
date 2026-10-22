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
	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/client/cmd/luci/artifact"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
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
		UsageLine: "get -invocationid <invocation_id> -workunitid <work_unit_id>",
		ShortDesc: "Get a work unit",
		LongDesc:  "Get details of a work unit by its root invocation ID and work unit ID.",
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
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			r.Flags.Usage()
			return 0
		}
	}
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "unexpected positional arguments; use flags -invocationid and -workunitid (run 'luci ids <url>' to extract ids)\n")
		return 1
	}
	if r.invocationID == "" || r.workUnitID == "" {
		fmt.Fprintf(os.Stderr, "flags -invocationid and -workunitid are required (run 'luci ids <url>' to extract ids)\n")
		return 1
	}

	ctx := cli.GetContext(a, r, env)
	ctx = format.WithDiscoveryCache(ctx)
	client := r.client
	var httpClient *http.Client
	if client == nil {
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
			fmt.Fprintf(os.Stderr, "GetWorkUnit RPC failed (missing ResultDB realm permission): %s\n", err)
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

	if wu.WorkUnitId != "" {
		fmt.Printf("Work Unit ID: %s\n", wu.WorkUnitId)
	}
	if wu.Parent != "" {
		fmt.Printf("Parent:       %s\n", format.FormatWorkUnitBreadcrumb(wu.Parent))
	}
	if wu.Kind != "" {
		fmt.Printf("Kind:         %s\n", wu.Kind)
	}
	if wu.ModuleId != nil && wu.ModuleId.ModuleName != "" {
		fmt.Printf("Module:       %s\n", wu.ModuleId.ModuleName)
	}
	fmt.Printf("State:        %s\n", wu.State)
	if wu.FinalizationState != pb.WorkUnit_FINALIZATION_STATE_UNSPECIFIED {
		fmt.Printf("Finalization: %s\n", wu.FinalizationState)
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
		fmt.Printf("Full Name:    %s\n", wu.Name)
		if len(wu.Tags) > 0 {
			fmt.Println("Tags:")
			for _, tag := range wu.Tags {
				fmt.Printf("  %s: %s\n", tag.Key, tag.Value)
			}
		}
		if wu.Properties != nil {
			out, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(wu.Properties)
			if err != nil {
				fmt.Fprintf(os.Stderr, "failed to marshal properties: %s\n", err)
			} else {
				fmt.Printf("Properties:\n%s\n", string(out))
			}
		}
	}

	return 0
}
