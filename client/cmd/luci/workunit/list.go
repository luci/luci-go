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
	"bytes"
	"cmp"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/client/cmd/luci/ids"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	defaultListLimit  = 100
	maxBatchWorkUnits = 500
	maxQueryPages     = 10
)

// ListCmd returns the `luci work-unit list` subcommand.
func ListCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "list [-invocationid <invocation_id>] [-parent <work_unit_id>] [<target>]",
		ShortDesc: "List work units in a root invocation",
		LongDesc: "List work units under a root invocation or direct children of a parent work unit,\n" +
			"along with their state, kind, module name, and duration.\n\n" +
			"Examples:\n" +
			"  $ luci work-unit list -invocationid ants-i11700010583257168\n" +
			"  $ luci work-unit list -invocationid ants-i11700010583257168 -parent root\n" +
			"  $ luci work-unit list -invocationid build-123 -sort duration -limit 20",
		CommandRun: func() subcommands.CommandRun {
			r := &workUnitListRun{af: af, limit: defaultListLimit, sortBy: "state"}
			if r.af != nil {
				r.af.Register(&r.Flags)
			}
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.StringVar(&r.invocationID, "invocationid", "", "Root invocation ID (e.g. build-867... or ants-i...)")
			r.Flags.StringVar(&r.invocationID, "invocation-id", "", "Alias for -invocationid")
			r.Flags.StringVar(&r.invocationID, "id", "", "Alias for -invocationid")
			r.Flags.StringVar(&r.parentID, "parent", "", "Parent work unit ID to list direct children of (e.g. root or ants-wu...)")
			r.Flags.StringVar(&r.parentID, "parent-work-unit", "", "Alias for -parent")
			r.Flags.StringVar(&r.stateFilter, "state", "", "Comma-separated work unit states to include (failed,cancelled,running,pending,skipped,succeeded,all)")
			r.Flags.StringVar(&r.kindFilter, "kind", "", "Filter by work unit kind (e.g. TFC_COMMAND_TASK, TF_MODULE)")
			r.Flags.StringVar(&r.moduleFilter, "modulename", "", "Filter by module name")
			r.Flags.StringVar(&r.moduleFilter, "module-name", "", "Alias for -modulename")
			r.Flags.StringVar(&r.moduleFilter, "module", "", "Alias for -modulename")
			r.Flags.StringVar(&r.sortBy, "sort", "state", "Sort order: state (diagnostic priority) or duration (longest first)")
			r.Flags.IntVar(&r.limit, "limit", defaultListLimit, "Maximum number of work units to display (0 for all; setting explicitly bypasses the 9.5 KB output cap)")
			r.Flags.BoolVar(&r.all, "all", false, "Show all matching work units without truncation")
			return r
		},
	}
}

type workUnitListRun struct {
	subcommands.CommandRunBase
	af           *base.AuthFlags
	client       pb.ResultDBClient
	out          io.Writer
	host         string
	invocationID string
	parentID     string
	stateFilter  string
	kindFilter   string
	moduleFilter string
	sortBy       string
	limit        int
	all          bool
	now          time.Time
	customCtx    context.Context
}

// ModifyContext overrides the CLI context when a custom context is injected by tests.
func (r *workUnitListRun) ModifyContext(ctx context.Context) context.Context {
	if r.customCtx != nil {
		return r.customCtx
	}
	return ctx
}

func (r *workUnitListRun) hasExplicitLimitFlag() bool {
	if r.limit > 0 && r.limit != defaultListLimit {
		return true
	}
	explicit := false
	r.Flags.Visit(func(f *flag.Flag) {
		if f.Name == "limit" {
			explicit = true
		}
	})
	return explicit
}

func (r *workUnitListRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
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
	if r.limit < 0 {
		fmt.Fprintf(os.Stderr, "flag -limit must be non-negative\n")
		return 1
	}

	allowedStates, err := parseWorkUnitStateFilter(r.stateFilter)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}
	sortBy := strings.ToLower(strings.TrimSpace(r.sortBy))
	if sortBy == "" {
		sortBy = "state"
	}
	if sortBy != "state" && sortBy != "duration" {
		fmt.Fprintf(os.Stderr, "invalid -sort %q: expected \"state\" or \"duration\"\n", r.sortBy)
		return 1
	}

	ctx := cli.GetContext(a, r, env)
	ctx = format.WithDiscoveryCache(ctx)
	client := r.client
	if client == nil && r.af != nil {
		if err := r.af.Parse(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
			return 1
		}
		var err error
		client, _, _, err = r.af.NewResultDBClient(ctx, r.host)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create resultdb client: %s\n", err)
			return 1
		}
	}
	if r.af != nil && r.af.ParsedOpts != nil {
		ctx = base.WithAuthOptions(ctx, *r.af.ParsedOpts)
	}

	extracted, err := ids.ExtractWithAuth(ctx, r.af, client, nil, target, ids.TargetBindings{
		InvocationID: &r.invocationID,
		WorkUnitID:   &r.parentID,
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
		fmt.Fprintf(os.Stderr, "failed to resolve invocation: %s\n", err)
		return 1
	}
	format.CacheResolvedInvocation(ctx, resolved)
	invID, legacy := resolved.ID, resolved.Legacy
	if legacy {
		fmt.Fprintf(os.Stderr, "work units are only supported for root invocations (invocation %q is a legacy invocation)\n", invID)
		return 1
	}

	out := r.out
	if out == nil && a != nil {
		out = a.GetOut()
	}
	if out == nil {
		out = os.Stdout
	}
	var derivedBuf bytes.Buffer
	ids.PrintDerivedIDs(&derivedBuf, extracted)

	rpcCtx := logging.SetLevel(ctx, logging.Error)
	var workUnits []*pb.WorkUnit
	var truncatedScan bool
	resolvedParentID := ""
	if r.parentID != "" {
		workUnits, resolvedParentID, err = listChildWorkUnits(rpcCtx, ctx, client, invID, r.parentID)
	} else {
		workUnits, truncatedScan, err = queryAllWorkUnits(rpcCtx, client, invID)
	}
	if err != nil {
		if base.IsPermissionDenied(err) {
			fmt.Fprintf(os.Stderr, "failed to list work units (%s): %s\n", base.ResultDBPermissionReason("rootInvocations/"+invID, err), err)
		} else {
			fmt.Fprintf(os.Stderr, "failed to list work units: %s\n", err)
		}
		return 1
	}

	now := r.now
	if now.IsZero() {
		now = time.Now()
	}

	filtered := filterWorkUnits(workUnits, allowedStates, r.kindFilter, r.moduleFilter)
	sortWorkUnits(filtered, sortBy, now)

	limit := r.limit
	if r.all || limit <= 0 {
		limit = len(filtered)
	}
	shown := filtered
	if len(shown) > limit {
		shown = filtered[:limit]
	}

	rendered := renderWorkUnitListOutput(invID, resolvedParentID, len(workUnits), len(filtered), shown, truncatedScan, now)
	if !r.all && r.limit > 0 && !r.hasExplicitLimitFlag() && len(shown) > 1 {
		budget := max(format.DefaultMaxOutputBytes-derivedBuf.Len(), 0)
		if len(rendered) > budget {
			lo, hi := 1, len(shown)
			for lo < hi {
				mid := (lo + hi + 1) / 2
				cand := renderWorkUnitListOutput(invID, resolvedParentID, len(workUnits), len(filtered), shown[:mid], truncatedScan, now)
				if len(cand) <= budget {
					lo = mid
				} else {
					hi = mid - 1
				}
			}
			rendered = renderWorkUnitListOutput(invID, resolvedParentID, len(workUnits), len(filtered), shown[:lo], truncatedScan, now)
		}
	}
	if derivedBuf.Len() > 0 {
		_, _ = out.Write(derivedBuf.Bytes())
	}
	_, _ = io.WriteString(out, rendered)
	return 0
}

func renderWorkUnitListOutput(invID, resolvedParentID string, totalCount, filteredCount int, shown []*pb.WorkUnit, truncatedScan bool, now time.Time) string {
	var buf bytes.Buffer
	var countStr string
	switch {
	case truncatedScan && filteredCount != totalCount:
		countStr = fmt.Sprintf("(%d matching, %d+ scanned)", filteredCount, totalCount)
	case truncatedScan:
		countStr = fmt.Sprintf("(%d+ scanned)", totalCount)
	case filteredCount != totalCount:
		countStr = fmt.Sprintf("(%d matching, %d total)", filteredCount, totalCount)
	default:
		countStr = fmt.Sprintf("(%d)", totalCount)
	}
	suffix := ""
	if len(shown) > 0 {
		suffix = ":"
	}
	if resolvedParentID != "" {
		fmt.Fprintf(&buf, "Child Work Units of %s in rootInvocations/%s %s%s\n",
			resolvedParentID, invID, countStr, suffix)
	} else {
		fmt.Fprintf(&buf, "Work Units in rootInvocations/%s %s%s\n",
			invID, countStr, suffix)
	}

	if len(shown) > 0 {
		tw := tabwriter.NewWriter(&buf, 0, 2, 2, ' ', 0)
		fmt.Fprintln(tw, "WORK_UNIT_ID\tSTATE\tKIND\tMODULE\tDURATION")
		for _, wu := range shown {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				wu.GetWorkUnitId(),
				wu.GetState(),
				cmp.Or(wu.GetKind(), "-"),
				formatWorkUnitModule(wu),
				formatWorkUnitDurationCell(wu, now),
			)
		}
		_ = tw.Flush()
	}

	if filteredCount > len(shown) {
		fmt.Fprintf(&buf, "... and %d more (use -limit=0, -all, or -state/-kind/-modulename)\n",
			filteredCount-len(shown))
	}
	if truncatedScan {
		fmt.Fprintf(&buf, "Note: stopped after scanning %d pages of work units; pass -parent=<WORK_UNIT_ID> to list children of a specific work unit.\n", maxQueryPages)
	}
	if len(shown) > 0 {
		fmt.Fprintf(&buf, "Next step: luci work-unit get -invocationid=%s -workunitid=%s\n",
			base.ShellQuoteIfNeeded(invID), base.ShellQuoteIfNeeded(shown[0].GetWorkUnitId()))
	}
	return buf.String()
}

func listChildWorkUnits(rpcCtx, fullCtx context.Context, client pb.ResultDBClient, invID, parentID string) ([]*pb.WorkUnit, string, error) {
	parentWU, normParent, _, _, err := fetchWorkUnitWithAntsFallback(rpcCtx, fullCtx, client, invID, base.NormalizeWorkUnit(parentID), pb.WorkUnitView_WORK_UNIT_VIEW_BASIC)
	if err != nil {
		return nil, "", err
	}

	childNames := parentWU.GetChildWorkUnits()
	if len(childNames) == 0 {
		return nil, normParent, nil
	}

	var children []*pb.WorkUnit
	for start := 0; start < len(childNames); start += maxBatchWorkUnits {
		end := min(start+maxBatchWorkUnits, len(childNames))
		batch := childNames[start:end]
		resp, batchErr := client.BatchGetWorkUnits(rpcCtx, &pb.BatchGetWorkUnitsRequest{
			Parent: "rootInvocations/" + invID,
			Names:  batch,
			View:   pb.WorkUnitView_WORK_UNIT_VIEW_BASIC,
		})
		if batchErr != nil {
			return nil, normParent, batchErr
		}
		children = append(children, resp.GetWorkUnits()...)
	}
	return children, normParent, nil
}

func queryAllWorkUnits(ctx context.Context, client pb.ResultDBClient, invID string) ([]*pb.WorkUnit, bool, error) {
	var all []*pb.WorkUnit
	pageToken := ""
	for range maxQueryPages {
		resp, err := client.QueryWorkUnits(ctx, &pb.QueryWorkUnitsRequest{
			Parent:    "rootInvocations/" + invID,
			View:      pb.WorkUnitView_WORK_UNIT_VIEW_BASIC,
			PageSize:  1000,
			PageToken: pageToken,
		})
		if err != nil {
			return nil, false, err
		}
		all = append(all, resp.GetWorkUnits()...)
		pageToken = resp.GetNextPageToken()
		if pageToken == "" {
			return all, false, nil
		}
	}
	return all, true, nil
}

var workUnitStateNames = map[string]pb.WorkUnit_State{
	"FAILED":    pb.WorkUnit_FAILED,
	"CANCELLED": pb.WorkUnit_CANCELLED,
	"CANCELED":  pb.WorkUnit_CANCELLED,
	"RUNNING":   pb.WorkUnit_RUNNING,
	"PENDING":   pb.WorkUnit_PENDING,
	"SKIPPED":   pb.WorkUnit_SKIPPED,
	"SUCCEEDED": pb.WorkUnit_SUCCEEDED,
}

func parseWorkUnitStateFilter(raw string) (map[pb.WorkUnit_State]bool, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, nil
	}
	allowed := make(map[pb.WorkUnit_State]bool)
	for _, part := range strings.Split(trimmed, ",") {
		tok := strings.ToUpper(strings.TrimSpace(part))
		switch tok {
		case "":
			continue
		case "ALL":
			return nil, nil
		}
		st, ok := workUnitStateNames[tok]
		if !ok {
			return nil, fmt.Errorf("invalid -state %q: expected comma-separated values from failed,cancelled,running,pending,skipped,succeeded,all", part)
		}
		allowed[st] = true
	}
	if len(allowed) == 0 {
		return nil, nil
	}
	return allowed, nil
}

func filterWorkUnits(wus []*pb.WorkUnit, allowedStates map[pb.WorkUnit_State]bool, kindFilter, moduleFilter string) []*pb.WorkUnit {
	kindNeedle := strings.ToLower(strings.TrimSpace(kindFilter))
	modNeedle := strings.ToLower(strings.TrimSpace(moduleFilter))
	var out []*pb.WorkUnit
	for _, wu := range wus {
		if wu == nil {
			continue
		}
		if len(allowedStates) > 0 && !allowedStates[wu.GetState()] {
			continue
		}
		if kindNeedle != "" && strings.ToLower(wu.GetKind()) != kindNeedle {
			continue
		}
		if modNeedle != "" && !strings.Contains(strings.ToLower(wu.GetModuleId().GetModuleName()), modNeedle) {
			continue
		}
		out = append(out, wu)
	}
	return out
}

func workUnitStatePriority(s pb.WorkUnit_State) int {
	switch s {
	case pb.WorkUnit_FAILED:
		return 0
	case pb.WorkUnit_CANCELLED:
		return 1
	case pb.WorkUnit_RUNNING:
		return 2
	case pb.WorkUnit_PENDING:
		return 3
	case pb.WorkUnit_SKIPPED:
		return 4
	case pb.WorkUnit_SUCCEEDED:
		return 5
	default:
		return 6
	}
}

func sortWorkUnits(wus []*pb.WorkUnit, sortBy string, now time.Time) {
	slices.SortStableFunc(wus, func(a, b *pb.WorkUnit) int {
		if sortBy == "state" {
			if c := cmp.Compare(workUnitStatePriority(a.GetState()), workUnitStatePriority(b.GetState())); c != 0 {
				return c
			}
		}
		durA, _, okA := format.WorkUnitDuration(a, now)
		durB, _, okB := format.WorkUnitDuration(b, now)
		if okA != okB {
			if okA {
				return -1
			}
			return 1
		}
		if c := cmp.Compare(durB, durA); c != 0 {
			return c
		}
		return cmp.Compare(a.GetWorkUnitId(), b.GetWorkUnitId())
	})
}

func formatWorkUnitModule(wu *pb.WorkUnit) string {
	mod := wu.GetModuleId().GetModuleName()
	if mod == "" {
		return "-"
	}
	if wu.GetModuleShardKey() != "" {
		return fmt.Sprintf("%s [shard:%s]", mod, wu.GetModuleShardKey())
	}
	return mod
}

func formatWorkUnitDurationCell(wu *pb.WorkUnit, now time.Time) string {
	dur, _, ok := format.WorkUnitDuration(wu, now)
	if !ok {
		return "-"
	}
	return format.FormatTimeDuration(dur)
}
