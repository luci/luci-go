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

package build

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/maruel/subcommands"
	"google.golang.org/genproto/protobuf/field_mask"

	pb "go.chromium.org/luci/buildbucket/proto"
	grpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

const defaultBuildListLimit = 10

// ListCmd returns the subcommand for `luci build list`.
func ListCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "list [-builder <project/bucket/builder>] [-tag <key:value>] [-status <status>] [-limit <n>]",
		ShortDesc: "Search and list builds by builder, tag, or status",
		LongDesc: "Search Buildbucket builds by builder, tag, or status.\n\n" +
			"By default, prints up to 10 matching builds with their status, builder/number, -buildid and -invocationid flags, timing, and summary.\n" +
			"Use -limit / -n to change the maximum number of builds returned.\n\n" +
			"Examples:\n" +
			"  $ luci build list -builder \"chromium/ci/Linux UBSan Tests\" -n 5\n" +
			"  $ luci build list -builder chromium/ci/linux-rel -status failure\n" +
			"  $ luci build list -tag dut-name:chromeos8-row1-rack10-host45",
		CommandRun: func() subcommands.CommandRun {
			r := &buildListRun{af: af, limit: defaultBuildListLimit}
			if r.af != nil {
				r.af.Register(&r.Flags)
			}
			r.Flags.StringVar(&r.host, "host", chromeinfra.BuildbucketHost, "Buildbucket host")
			r.Flags.StringVar(&r.builder, "builder", "", "Filter by builder (<project>[/<bucket>[/<builder>]])")
			r.Flags.Var(&r.tags, "tag", "Filter by key:value tag (repeatable, e.g. -tag dut-name:host1)")
			r.Flags.Var(&r.tags, "t", "Alias for -tag")
			r.Flags.StringVar(&r.status, "status", "", "Filter by status (scheduled, started, ended, success, failure, infra_failure, canceled)")
			r.Flags.IntVar(&r.limit, "limit", defaultBuildListLimit, "Maximum number of builds to return (default: 10)")
			r.Flags.IntVar(&r.limit, "n", defaultBuildListLimit, "Alias for -limit")
			return r
		},
	}
}

type buildListRun struct {
	subcommands.CommandRunBase
	af     *base.AuthFlags
	client grpcpb.BuildsClient

	host    string
	builder string
	tags    stringlistflag.Flag
	status  string
	limit   int
}

func (r *buildListRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			r.Flags.Usage()
			return 0
		}
	}
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "unexpected positional arguments; use flags -builder, -tag, or -status\n")
		return 1
	}
	if r.limit <= 0 {
		fmt.Fprintf(os.Stderr, "flag -limit / -n must be a positive integer\n")
		return 1
	}

	predicate, err := buildSearchPredicate(r.builder, r.tags, r.status)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	ctx := cli.GetContext(a, r, env)
	client := r.client
	if client == nil {
		if err := r.af.Parse(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
			return 1
		}
		var errClient error
		client, _, errClient = r.af.NewBuildsClient(ctx, r.host)
		if errClient != nil {
			fmt.Fprintf(os.Stderr, "failed to create buildbucket client: %s\n", errClient)
			return 1
		}
	}

	mask := &pb.BuildMask{
		Fields: &field_mask.FieldMask{
			Paths: []string{
				"id",
				"builder",
				"number",
				"status",
				"status_details",
				"summary_markdown",
				"create_time",
				"start_time",
				"end_time",
				"infra.resultdb",
			},
		},
	}

	resp, err := client.SearchBuilds(ctx, &pb.SearchBuildsRequest{
		Predicate: predicate,
		Mask:      mask,
		PageSize:  int32(r.limit),
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "SearchBuilds RPC failed: %s\n", err)
		return 1
	}

	if err := FormatBuildList(os.Stdout, resp.GetBuilds()); err != nil {
		fmt.Fprintf(os.Stderr, "failed to format builds: %s\n", err)
		return 1
	}
	return 0
}

// buildSearchPredicate parses the CLI filter flags (-builder, -tag, -status)
// into a Buildbucket BuildPredicate, which specifies the filter criteria that
// builds must match in a SearchBuilds RPC request.
func buildSearchPredicate(builderRaw string, tagsRaw []string, statusRaw string) (*pb.BuildPredicate, error) {
	builderID, err := parseBuilderFilter(builderRaw)
	if err != nil {
		return nil, err
	}

	tags, err := parseTagFilters(tagsRaw)
	if err != nil {
		return nil, err
	}

	status, err := parseBuildStatus(statusRaw)
	if err != nil {
		return nil, err
	}

	if builderID == nil && len(tags) == 0 && status == pb.Status_STATUS_UNSPECIFIED {
		return nil, errors.New("at least one filter flag (-builder, -tag, or -status) is required")
	}

	return &pb.BuildPredicate{
		Builder: builderID,
		Tags:    tags,
		Status:  status,
	}, nil
}

func parseBuilderFilter(raw string) (*pb.BuilderID, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}

	clean := strings.Trim(strings.TrimSpace(raw), "/")
	parts := strings.Split(clean, "/")
	if len(parts) > 3 {
		return nil, errors.Fmt("invalid -builder %q: expected <project>, <project>/<bucket>, or <project>/<bucket>/<builder>", raw)
	}
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			return nil, errors.Fmt("invalid -builder %q: expected <project>, <project>/<bucket>, or <project>/<bucket>/<builder>", raw)
		}
	}

	bid := &pb.BuilderID{
		Project: strings.TrimSpace(parts[0]),
	}
	if len(parts) >= 2 {
		bid.Bucket = strings.TrimSpace(parts[1])
	}
	if len(parts) == 3 {
		bid.Builder = strings.TrimSpace(parts[2])
	}
	return bid, nil
}

func parseTagFilters(raw []string) ([]*pb.StringPair, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	tags := make([]*pb.StringPair, 0, len(raw))
	for _, entry := range raw {
		entry = strings.TrimSpace(entry)
		sep := strings.IndexByte(entry, ':')
		if sep <= 0 {
			return nil, errors.Fmt("invalid -tag %q: expected key:value", entry)
		}
		key := strings.TrimSpace(entry[:sep])
		val := strings.TrimSpace(entry[sep+1:])
		if key == "" || val == "" {
			return nil, errors.Fmt("invalid -tag %q: expected key:value", entry)
		}
		tags = append(tags, &pb.StringPair{
			Key:   key,
			Value: val,
		})
	}
	return tags, nil
}

func parseBuildStatus(raw string) (pb.Status, error) {
	normalized := strings.ToLower(strings.TrimSpace(raw))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	switch normalized {
	case "":
		return pb.Status_STATUS_UNSPECIFIED, nil
	case "scheduled":
		return pb.Status_SCHEDULED, nil
	case "started", "running":
		return pb.Status_STARTED, nil
	case "ended":
		return pb.Status_ENDED_MASK, nil
	case "success", "succeeded", "passed", "pass":
		return pb.Status_SUCCESS, nil
	case "failure", "failed", "fail":
		return pb.Status_FAILURE, nil
	case "infra_failure", "infra":
		return pb.Status_INFRA_FAILURE, nil
	case "canceled", "cancelled", "cancel":
		return pb.Status_CANCELED, nil
	default:
		return pb.Status_STATUS_UNSPECIFIED, errors.Fmt(
			"invalid -status %q; valid values are: scheduled, started, ended, success, failure, infra_failure, canceled", raw)
	}
}

// FormatBuildList formats a slice of Build messages into human-readable terminal output.
func FormatBuildList(w io.Writer, builds []*pb.Build) error {
	if len(builds) == 0 {
		fmt.Fprintf(w, "No matching builds found.\n")
		return nil
	}

	fmt.Fprintf(w, "Builds (%d):\n", len(builds))
	for i, b := range builds {
		if i > 0 {
			fmt.Fprintln(w)
		}

		// 1. Builder and Number
		builderStr := ""
		if b.Builder != nil {
			builderStr = fmt.Sprintf("%s/%s/%s", b.Builder.Project, b.Builder.Bucket, b.Builder.Builder)
			if b.Number != 0 {
				builderStr = fmt.Sprintf("%s/%d", builderStr, b.Number)
			}
		}
		if builderStr != "" {
			if b.Number != 0 {
				fmt.Fprintf(w, "Build:         %s\n", builderStr)
			} else {
				fmt.Fprintf(w, "Builder:       %s\n", builderStr)
			}
		}

		// 2. Status
		statusStr := b.Status.String()
		if b.StatusDetails != nil {
			if b.StatusDetails.GetTimeout() != nil {
				statusStr += " (Timeout)"
			} else if b.StatusDetails.GetResourceExhaustion() != nil {
				statusStr += " (Resource Exhaustion)"
			}
		}
		fmt.Fprintf(w, "Status:        %s\n", statusStr)

		// 3. Build ID and ResultDB Invocation ID flags
		if b.Id != 0 {
			fmt.Fprintf(w, "Flags:         -buildid %d -invocationid %s\n", b.Id, buildInvocationID(b))
		}

		// 4. Timing
		if timingStr := formatTiming(b); timingStr != "" {
			fmt.Fprintf(w, "Timing:        %s\n", timingStr)
		}

		// 5. Summary Markdown
		if summary, truncated := format.TruncateFirstLine(b.SummaryMarkdown, 160); summary != "" {
			if truncated {
				fmt.Fprintf(w, "Summary(truncated): %s\n", summary)
			} else {
				fmt.Fprintf(w, "Summary:       %s\n", summary)
			}
		}
	}

	// 6. Helpful guidance tips
	target := builds[0]
	for _, b := range builds {
		if b.Status == pb.Status_FAILURE || b.Status == pb.Status_INFRA_FAILURE {
			target = b
			break
		}
	}
	if target.Id != 0 {
		fmt.Fprintf(w, "\n(Tip: To inspect build details, steps and full summaries, run: luci build get -buildid %d)\n", target.Id)
		if target.Status == pb.Status_FAILURE || target.Status == pb.Status_INFRA_FAILURE {
			fmt.Fprintf(w, "(Tip: To list failed test verdicts, run: luci verdict list -invocationid %s)\n", buildInvocationID(target))
		}
	}

	return nil
}

func buildInvocationID(b *pb.Build) string {
	if inv := b.GetInfra().GetResultdb().GetInvocation(); inv != "" {
		inv = strings.TrimPrefix(inv, "rootInvocations/")
		inv = strings.TrimPrefix(inv, "invocations/")
		inv = strings.Replace(inv, "build:", "build-", 1)
		return inv
	}
	if b.GetId() != 0 {
		return fmt.Sprintf("build-%d", b.GetId())
	}
	return ""
}
