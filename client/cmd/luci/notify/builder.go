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
	"os"
	"slices"
	"strings"
	"time"

	"github.com/maruel/subcommands"
	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/ids"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	lucinotifypb "go.chromium.org/luci/luci_notify/api/service/v1"
)

const (
	defaultBuilderListLimit = 20
	maxNotifyPageSize       = 1000
)

var errMissingBuilderFilter = errors.New("at least one filter flag (-rotation, -builder, -status, or -filter) or a positional rotation target is required")

// BuilderCmd returns the subcommand group for `luci notify builder`.
func BuilderCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "builder <subcommand>",
		ShortDesc: "Inspect LUCI Notify builder statuses",
		LongDesc: "Query LUCI Notify builder statuses across on-call rotations, projects, and buckets.\n\n" +
			"Available subcommands:\n" +
			"  list  List builders and their latest build status by rotation, builder, status, or AIP-160 filter",
		CommandRun: func() subcommands.CommandRun {
			return &builderRun{af: af}
		},
	}
}

type builderRun struct {
	subcommands.CommandRunBase
	af *base.AuthFlags
}

func (r *builderRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return base.RunSubcommandApp(a, "luci notify builder", "LUCI Notify builder inspection", []*subcommands.Command{
		BuilderListCmd(r.af),
		subcommands.CmdHelp,
	}, args)
}

// BuilderListCmd returns the subcommand for `luci notify builder list`.
func BuilderListCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "list [-rotation <rotation>] [-builder <project>[/<bucket>[/<builder>]]] [-status <status>] [-filter <aip160_filter>] [-limit <n>] [-json] [<target>]",
		ShortDesc: "List builders and their latest status from LUCI Notify",
		LongDesc: "List builders and their latest status tracked by LUCI Notify (notify.api.luci.app).\n\n" +
			"Filter by on-call rotation (-rotation), builder path (-builder <project>[/<bucket>[/<builder>]]),\n" +
			"latest build status (-status), or a raw AIP-160 filter expression (-filter).\n" +
			"Use -limit 0 to fetch all matching pages, or -json to output raw ListBuildersResponse JSON.\n\n" +
			"Examples:\n" +
			"  luci notify builder list -rotation chrome_browser_release -status FAILURE,INFRA_FAILURE\n" +
			"  luci notify builder list -builder \"chrome/m146/mac64\"\n" +
			"  luci notify builder list -builder chromium-m156/ci -status FAILURE\n" +
			"  luci notify builder list https://ci.chromium.org/ui/labs/monitoring/chrome-browser-release -status FAILURE",
		CommandRun: func() subcommands.CommandRun {
			r := &builderListRun{af: af, limit: defaultBuilderListLimit}
			if r.af != nil {
				r.af.Register(&r.Flags)
			}
			r.Flags.StringVar(&r.host, "host", chromeinfra.NotifyHost, "LUCI Notify host")
			r.Flags.StringVar(&r.rotation, "rotation", "", "Filter by on-call rotation (e.g. chrome_browser_release or chrome-browser-release)")
			r.Flags.StringVar(&r.builder, "builder", "", "Filter by builder: <project>, <project>/<bucket>, or <project>/<bucket>/<builder>")
			r.Flags.Var(&r.statuses, "status", "Filter by latest completed build status: SUCCESS, FAILURE, INFRA_FAILURE, CANCELED (repeatable or comma-separated)")
			r.Flags.StringVar(&r.filter, "filter", "", "Additional AIP-160 filter expression (fields: project, bucket, builder, status, on_call_rotations)")
			r.Flags.IntVar(&r.limit, "limit", defaultBuilderListLimit, "Maximum number of builders to return (default: 20; 0 for all)")
			r.Flags.IntVar(&r.limit, "n", defaultBuilderListLimit, "Alias for -limit")
			r.Flags.BoolVar(&r.json, "json", false, "Output ListBuildersResponse as JSON")
			return r
		},
	}
}

type builderListRun struct {
	subcommands.CommandRunBase
	af             *base.AuthFlags
	buildersClient lucinotifypb.BuildersClient
	out            io.Writer
	errOut         io.Writer

	host     string
	rotation string
	builder  string
	statuses stringlistflag.Flag
	filter   string
	limit    int
	json     bool
}

func (r *builderListRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
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
	if target != "" && extracted.AlertGroupID != "" {
		ids.PrintMissingFlagsError(errOut, extracted, "positional target is an alert group, not a rotation")
		return 1
	}

	filterExpr, err := buildBuilderListFilter(r.rotation, r.builder, r.statuses, r.filter)
	if err != nil {
		if errors.Is(err, errMissingBuilderFilter) {
			ids.PrintMissingFlagsError(errOut, extracted, err.Error())
		} else {
			fmt.Fprintf(errOut, "%s\n", err)
		}
		return 1
	}

	client, _, ok := ensureNotifyClients(ctx, r.af, r.host, r.buildersClient, nil)
	if !ok {
		return 1
	}

	builders, hasMore, err := fetchBuilders(ctx, client, filterExpr, r.limit)
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
			fmt.Fprintf(errOut, "(showing %d builders; use -limit 0 to fetch all)\n", len(builders))
		}
		resp := &lucinotifypb.ListBuildersResponse{Builders: builders}
		jsonOut, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(resp)
		if err != nil {
			fmt.Fprintf(errOut, "failed to marshal builders to JSON: %s\n", err)
			return 1
		}
		fmt.Fprintln(out, string(jsonOut))
		return 0
	}

	ids.PrintDerivedIDs(out, extracted)
	hasStatusFilter := len(r.statuses) > 0 || strings.Contains(strings.ToLower(r.filter), "status")
	if err := formatBuilderList(out, builders, hasMore, hasStatusFilter); err != nil {
		fmt.Fprintf(errOut, "failed to format builders: %s\n", err)
		return 1
	}
	return 0
}

// buildBuilderListFilter constructs an AIP-160 filter expression for
// luci.notify.v1.Builders.List from CLI flags.
func buildBuilderListFilter(rotation, builderRaw string, statuses []string, rawFilter string) (string, error) {
	rotation = strings.TrimSpace(rotation)
	rawFilter = strings.TrimSpace(rawFilter)

	var clauses []string

	if rotation != "" {
		rot := trimRotationPrefix(rotation)
		if rot == "" {
			return "", errors.New("flag -rotation cannot be empty")
		}
		replaceSep := func(sep string) string {
			return strings.NewReplacer("-", sep, "_", sep, ".", sep).Replace(rot)
		}
		var variants []string
		var parts []string
		for _, v := range []string{rot, replaceSep("_"), replaceSep("-"), replaceSep(".")} {
			if !slices.Contains(variants, v) {
				variants = append(variants, v)
				parts = append(parts, fmt.Sprintf("on_call_rotations = %q", v))
			}
		}
		if len(parts) == 1 {
			clauses = append(clauses, parts[0])
		} else {
			clauses = append(clauses, "("+strings.Join(parts, " OR ")+")")
		}
	}

	project, bucket, builder, err := parseBuilderFilter(builderRaw)
	if err != nil {
		return "", err
	}
	if project != "" {
		clauses = append(clauses, fmt.Sprintf("project = %q", project))
	}
	if bucket != "" {
		clauses = append(clauses, fmt.Sprintf("bucket = %q", bucket))
	}
	if builder != "" {
		clauses = append(clauses, fmt.Sprintf("builder = %q", builder))
	}

	parsedStatuses, err := parseBuilderStatuses(statuses)
	if err != nil {
		return "", err
	}
	if len(parsedStatuses) == 1 {
		clauses = append(clauses, fmt.Sprintf("status = %q", parsedStatuses[0]))
	} else if len(parsedStatuses) > 1 {
		parts := make([]string, 0, len(parsedStatuses))
		for _, s := range parsedStatuses {
			parts = append(parts, fmt.Sprintf("status = %q", s))
		}
		clauses = append(clauses, "("+strings.Join(parts, " OR ")+")")
	}

	if rawFilter != "" {
		if len(clauses) > 0 {
			clauses = append(clauses, "("+rawFilter+")")
		} else {
			clauses = append(clauses, rawFilter)
		}
	}

	if len(clauses) == 0 {
		return "", errMissingBuilderFilter
	}
	return strings.Join(clauses, " AND "), nil
}

func parseBuilderFilter(raw string) (project, bucket, builder string, err error) {
	if strings.TrimSpace(raw) == "" {
		return "", "", "", nil
	}
	clean := strings.Trim(strings.TrimSpace(raw), "/")
	parts := strings.Split(clean, "/")
	if clean == "" || len(parts) > 3 {
		return "", "", "", errors.Fmt("invalid -builder %q: expected <project>, <project>/<bucket>, or <project>/<bucket>/<builder>", raw)
	}
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
		if parts[i] == "" {
			return "", "", "", errors.Fmt("invalid -builder %q: expected <project>, <project>/<bucket>, or <project>/<bucket>/<builder>", raw)
		}
	}
	project = parts[0]
	if len(parts) >= 2 {
		bucket = parts[1]
	}
	if len(parts) == 3 {
		builder = parts[2]
	}
	return project, bucket, builder, nil
}

func parseBuilderStatuses(rawStatuses []string) ([]string, error) {
	if len(rawStatuses) == 0 {
		return nil, nil
	}
	var out []string
	for _, entry := range rawStatuses {
		for _, token := range strings.Split(entry, ",") {
			normalized := strings.ReplaceAll(strings.ToLower(strings.TrimSpace(token)), "-", "_")
			if normalized == "" {
				continue
			}
			var canonical string
			switch normalized {
			case "success", "succeeded", "passed", "pass":
				canonical = "SUCCESS"
			case "failure", "failed", "fail":
				canonical = "FAILURE"
			case "infra_failure", "infra":
				canonical = "INFRA_FAILURE"
			case "canceled", "cancelled", "cancel":
				canonical = "CANCELED"
			default:
				return nil, errors.Fmt("invalid -status %q; valid values are: SUCCESS, FAILURE, INFRA_FAILURE, CANCELED", token)
			}
			if !slices.Contains(out, canonical) {
				out = append(out, canonical)
			}
		}
	}
	if len(out) == 0 {
		return nil, errors.New("flag -status cannot be empty; valid values are: SUCCESS, FAILURE, INFRA_FAILURE, CANCELED")
	}
	return out, nil
}

func fetchBuilders(ctx context.Context, client lucinotifypb.BuildersClient, filter string, limit int) ([]*lucinotifypb.BuilderStatus, bool, error) {
	var builders []*lucinotifypb.BuilderStatus
	pageToken := ""
	for {
		pageSize := int32(maxNotifyPageSize)
		if limit > 0 {
			pageSize = int32(min(maxNotifyPageSize, limit-len(builders)))
		}
		resp, err := client.List(ctx, &lucinotifypb.ListBuildersRequest{
			PageSize:  pageSize,
			PageToken: pageToken,
			Filter:    filter,
		})
		if err != nil {
			return nil, false, errors.Fmt("Builders.List RPC failed: %w", err)
		}
		builders = append(builders, resp.GetBuilders()...)
		nextToken := resp.GetNextPageToken()
		if limit > 0 && len(builders) >= limit {
			return builders[:limit], len(builders) > limit || nextToken != "", nil
		}
		if len(resp.GetBuilders()) == 0 || nextToken == "" || nextToken == pageToken {
			return builders, false, nil
		}
		pageToken = nextToken
	}
}

// FormatBuilderList formats a slice of LUCI Notify BuilderStatus messages into
// human-readable terminal output.
func FormatBuilderList(w io.Writer, builders []*lucinotifypb.BuilderStatus) error {
	return formatBuilderList(w, builders, false, false)
}

func formatBuilderList(w io.Writer, builders []*lucinotifypb.BuilderStatus, hasMore, hasStatusFilter bool) error {
	if len(builders) == 0 {
		fmt.Fprintf(w, "No matching builders found.\n")
		return nil
	}

	if hasMore {
		if hasStatusFilter {
			fmt.Fprintf(w, "Builders (showing %d; use -limit 0 to see all):\n", len(builders))
		} else {
			fmt.Fprintf(w, "Builders (showing %d; use -status FAILURE,INFRA_FAILURE to filter failures or -limit 0 to see all):\n", len(builders))
		}
	} else {
		fmt.Fprintf(w, "Builders (%d):\n", len(builders))
	}
	var tipTarget *lucinotifypb.BuilderStatus
	for i, b := range builders {
		if i > 0 {
			fmt.Fprintln(w)
		}
		builderPath := b.GetName()
		if builderPath == "" && b.GetProject() != "" && b.GetBucket() != "" && b.GetBuilder() != "" {
			builderPath = fmt.Sprintf("%s/%s/%s", b.GetProject(), b.GetBucket(), b.GetBuilder())
		}
		if builderPath != "" {
			fmt.Fprintf(w, "Builder:       %s\n", builderPath)
		}
		statusStr := b.GetStatus()
		if statusStr == "" {
			statusStr = "UNKNOWN"
		}
		fmt.Fprintf(w, "Status:        %s\n", statusStr)
		if b.GetBuildId() != 0 {
			fmt.Fprintf(w, "Flags:         -buildid %d -invocationid build-%d\n", b.GetBuildId(), b.GetBuildId())
			if tipTarget == nil || ((tipTarget.GetStatus() != "FAILURE" && tipTarget.GetStatus() != "INFRA_FAILURE") && (statusStr == "FAILURE" || statusStr == "INFRA_FAILURE")) {
				tipTarget = b
			}
		}
		if ts := b.GetUpdateTime(); ts != nil && ts.IsValid() {
			fmt.Fprintf(w, "Updated:       %s\n", ts.AsTime().UTC().Format(time.RFC3339))
		}
		if rots := b.GetOnCallRotations(); len(rots) > 0 {
			fmt.Fprintf(w, "Rotations:     %s\n", strings.Join(rots, ", "))
		}
	}

	if tipTarget != nil {
		fmt.Fprintf(w, "\n(Tip: To inspect build details and failed steps, run: luci build get -buildid %d)\n", tipTarget.GetBuildId())
		if tipTarget.GetStatus() == "FAILURE" || tipTarget.GetStatus() == "INFRA_FAILURE" {
			fmt.Fprintf(w, "(Tip: To list failed test verdicts, run: luci verdict list -invocationid build-%d)\n", tipTarget.GetBuildId())
		}
	}

	return nil
}
