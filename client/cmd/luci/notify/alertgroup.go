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

package notify

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/maruel/subcommands"
	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/client/cmd/luci/ids"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	lucinotifypb "go.chromium.org/luci/luci_notify/api/service/v1"
)

const (
	defaultAlertGroupListLimit = 20
	maxPreviewAlertKeys        = 5
)

var alertGroupRotationReplacer = strings.NewReplacer("_", "-", ".", "-")

// normalizeAlertGroupRotation strips an optional "rotations/" prefix and
// converts underscores and dots to hyphens to match LUCI Notify's AlertGroups
// resource format (rotations/[a-z0-9-]+).
func normalizeAlertGroupRotation(raw string) string {
	return alertGroupRotationReplacer.Replace(trimRotationPrefix(raw))
}

// AlertGroupCmd returns the subcommand group for `luci notify alert-group`.
func AlertGroupCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "alert-group <subcommand>",
		ShortDesc: "Inspect LUCI Notify rotation alert groups",
		LongDesc: "Query LUCI Notify alert groups for an on-call gardener rotation.\n\n" +
			"Available subcommands:\n" +
			"  list  List active alert groups for a rotation",
		CommandRun: func() subcommands.CommandRun {
			return &alertGroupRun{af: af}
		},
	}
}

type alertGroupRun struct {
	subcommands.CommandRunBase
	af *base.AuthFlags
}

func (r *alertGroupRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return base.RunSubcommandApp(a, "luci notify alert-group", "LUCI Notify alert group inspection", []*subcommands.Command{
		AlertGroupListCmd(r.af),
		subcommands.CmdHelp,
	}, args)
}

// AlertGroupListCmd returns the subcommand for `luci notify alert-group list`.
func AlertGroupListCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "list [-rotation <rotation>] [-limit <n>] [-json] [<target>]",
		ShortDesc: "List alert groups for a rotation",
		LongDesc: "List alert groups in LUCI Notify for an on-call rotation.\n\n" +
			"Accepts -rotation (with underscores or hyphens, e.g. chrome_browser_release or chrome-browser-release)\n" +
			"or a positional rotation resource name / Milo monitoring URL.\n" +
			"Use -json to output raw ListAlertGroupsResponse JSON.\n\n" +
			"Examples:\n" +
			"  luci notify alert-group list -rotation chrome-browser-release\n" +
			"  luci notify alert-group list rotations/chrome-browser-release\n" +
			"  luci notify alert-group list https://ci.chromium.org/ui/labs/monitoring/chrome-browser-release",
		CommandRun: func() subcommands.CommandRun {
			r := &alertGroupListRun{af: af, limit: defaultAlertGroupListLimit}
			if r.af != nil {
				r.af.Register(&r.Flags)
			}
			r.Flags.StringVar(&r.host, "host", chromeinfra.NotifyHost, "LUCI Notify host")
			r.Flags.StringVar(&r.rotation, "rotation", "", "Rotation name (e.g. chrome-browser-release or chrome_browser_release)")
			r.Flags.IntVar(&r.limit, "limit", defaultAlertGroupListLimit, "Maximum number of alert groups to return (default: 20; 0 for all)")
			r.Flags.IntVar(&r.limit, "n", defaultAlertGroupListLimit, "Alias for -limit")
			r.Flags.BoolVar(&r.json, "json", false, "Output ListAlertGroupsResponse as JSON")
			return r
		},
	}
}

type alertGroupListRun struct {
	subcommands.CommandRunBase
	af                *base.AuthFlags
	alertGroupsClient lucinotifypb.AlertGroupsClient
	out               io.Writer
	errOut            io.Writer

	host     string
	rotation string
	limit    int
	json     bool
}

func (r *alertGroupListRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if base.HasHelpFlag(args) {
		r.Flags.Usage()
		return 0
	}

	errOut := r.errOut
	if errOut == nil {
		errOut = os.Stderr
	}

	target, err := base.ParsePositionalTarget(&r.Flags, args)
	if err != nil {
		fmt.Fprintf(errOut, "%s\n", err)
		return 1
	}
	if r.limit < 0 {
		fmt.Fprintf(errOut, "flag -limit / -n must be >= 0\n")
		return 1
	}

	r.rotation = strings.TrimSpace(r.rotation)

	ctx := cli.GetContext(a, r, env)
	extracted, err := ids.ExtractWithAuth(ctx, r.af, nil, nil, target, ids.TargetBindings{
		Rotation: &r.rotation,
	})
	if err != nil {
		fmt.Fprintf(errOut, "failed to extract IDs from %q: %s\n", target, err)
		return 1
	}
	if target != "" && (extracted == nil || extracted.Rotation == "") {
		ids.PrintMissingFlagsError(errOut, extracted, "positional target must be a LUCI Notify rotation")
		return 1
	}

	rot := normalizeAlertGroupRotation(r.rotation)
	if rot == "" {
		ids.PrintMissingFlagsError(errOut, extracted, "flag -rotation or a positional rotation target is required")
		return 1
	}
	if extracted != nil && extracted.Rotation != "" {
		extracted.Rotation = rot
	}
	if target != "" && extracted.AlertGroupID != "" {
		ids.PrintMissingFlagsError(errOut, extracted, "positional target is an alert group, not a rotation")
		return 1
	}

	_, client, ok := ensureNotifyClients(ctx, r.af, r.host, nil, r.alertGroupsClient)
	if !ok {
		return 1
	}

	groups, hasMore, err := fetchAlertGroups(ctx, client, rot, r.limit)
	if err != nil {
		fmt.Fprintf(errOut, "%s\n", err)
		return 1
	}

	out := r.out
	if out == nil {
		out = os.Stdout
	}

	if r.json {
		ids.PrintDerivedIDs(errOut, extracted)
		if hasMore {
			fmt.Fprintf(errOut, "(showing %d alert groups; use -limit 0 to fetch all)\n", len(groups))
		}
		resp := &lucinotifypb.ListAlertGroupsResponse{AlertGroups: groups}
		jsonOut, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(resp)
		if err != nil {
			fmt.Fprintf(errOut, "failed to marshal alert groups to JSON: %s\n", err)
			return 1
		}
		fmt.Fprintln(out, string(jsonOut))
		return 0
	}

	ids.PrintDerivedIDs(out, extracted)
	if err := formatAlertGroupList(out, rot, groups, hasMore); err != nil {
		fmt.Fprintf(errOut, "failed to format alert groups: %s\n", err)
		return 1
	}
	return 0
}

func fetchAlertGroups(ctx context.Context, client lucinotifypb.AlertGroupsClient, rotation string, limit int) ([]*lucinotifypb.AlertGroup, bool, error) {
	parent := "rotations/" + rotation
	var groups []*lucinotifypb.AlertGroup
	pageToken := ""
	for {
		pageSize := int32(maxNotifyPageSize)
		if limit > 0 {
			pageSize = int32(min(maxNotifyPageSize, limit-len(groups)))
		}
		resp, err := client.ListAlertGroups(ctx, &lucinotifypb.ListAlertGroupsRequest{
			Parent:    parent,
			PageSize:  pageSize,
			PageToken: pageToken,
		})
		if err != nil {
			return nil, false, errors.Fmt("ListAlertGroups(%s) RPC failed: %w", parent, err)
		}
		groups = append(groups, resp.GetAlertGroups()...)
		nextToken := resp.GetNextPageToken()
		if limit > 0 && len(groups) >= limit {
			return groups[:limit], len(groups) > limit || nextToken != "", nil
		}
		if len(resp.GetAlertGroups()) == 0 || nextToken == "" || nextToken == pageToken {
			return groups, false, nil
		}
		pageToken = nextToken
	}
}

func formatAlertKey(raw string) string {
	trimmed := strings.TrimPrefix(strings.TrimSpace(raw), "alerts/")
	if unescaped, err := url.PathUnescape(trimmed); err == nil && unescaped != "" {
		return unescaped
	}
	return trimmed
}

func formatBugList(bugs []int64) string {
	var parts []string
	for _, b := range bugs {
		if b > 0 {
			parts = append(parts, fmt.Sprintf("b/%d", b))
		}
	}
	return strings.Join(parts, ", ")
}

func printAlertGroupSummary(w io.Writer, g *lucinotifypb.AlertGroup) {
	if g.GetName() != "" {
		fmt.Fprintf(w, "Name:          %s\n", g.GetName())
	}
	if g.GetDisplayName() != "" {
		fmt.Fprintf(w, "Display Name:  %s\n", g.GetDisplayName())
	}
	if bugStr := formatBugList(g.GetBugs()); bugStr != "" {
		fmt.Fprintf(w, "Bugs:          %s\n", bugStr)
	}
	if statusMsg, truncated := format.TruncateFirstLine(g.GetStatusMessage(), 160); statusMsg != "" {
		if truncated {
			fmt.Fprintf(w, "Status Msg (truncated): %s\n", statusMsg)
		} else {
			fmt.Fprintf(w, "Status Msg:    %s\n", statusMsg)
		}
	}
	if ts := g.GetUpdateTime(); ts != nil && ts.IsValid() {
		tsStr := ts.AsTime().UTC().Format(time.RFC3339)
		if by := strings.TrimSpace(g.GetUpdatedBy()); by != "" {
			fmt.Fprintf(w, "Updated:       %s (by %s)\n", tsStr, by)
		} else {
			fmt.Fprintf(w, "Updated:       %s\n", tsStr)
		}
	}
}

// FormatAlertGroupList formats a slice of LUCI Notify AlertGroup messages into
// human-readable terminal output.
func FormatAlertGroupList(w io.Writer, rotation string, groups []*lucinotifypb.AlertGroup) error {
	return formatAlertGroupList(w, rotation, groups, false)
}

func formatAlertGroupList(w io.Writer, rotation string, groups []*lucinotifypb.AlertGroup, hasMore bool) error {
	if len(groups) == 0 {
		fmt.Fprintf(w, "No alert groups found for rotation %q.\n", rotation)
		return nil
	}

	if hasMore {
		fmt.Fprintf(w, "Alert Groups (showing %d; use -limit 0 to see all):\n", len(groups))
	} else {
		fmt.Fprintf(w, "Alert Groups (%d):\n", len(groups))
	}
	for i, g := range groups {
		if i > 0 {
			fmt.Fprintln(w)
		}
		printAlertGroupSummary(w, g)
		keys := g.GetAlertKeys()
		fmt.Fprintf(w, "Alerts (%d):\n", len(keys))
		for _, k := range keys[:min(len(keys), maxPreviewAlertKeys)] {
			fmt.Fprintf(w, "  - %s\n", formatAlertKey(k))
		}
		if len(keys) > maxPreviewAlertKeys {
			fmt.Fprintf(w, "  ... and %d more\n", len(keys)-maxPreviewAlertKeys)
		}
	}
	return nil
}
