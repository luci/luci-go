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
	"context"
	"flag"
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
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

const (
	defaultBuildListLimit      = 10
	maxPrecedingPatchsetProbes = 3
)

// ListCmd returns the subcommand for `luci build list`.
func ListCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "list [-project <project>] [-bucket <bucket>] [-builder <builder>] [-tag <key:value>] [-status <status>] [-cl <gerrit_url_or_change_id>] [-patchset <n>] [-limit <n>]",
		ShortDesc: "Search and list builds by builder, tag, status, or Gerrit CL",
		LongDesc: "Search Buildbucket builds by builder, tag, status, or Gerrit CL.\n\n" +
			"By default, prints up to 10 matching builds with their status, builder/number, -buildid and -invocationid flags, timing, and summary.\n" +
			"When -cl is specified without a patchset, resolves the latest patchset and probes up to 3 preceding patchsets if the latest patchset has no builds.\n" +
			"Use -limit / -n to change the maximum number of builds returned.\n\n" +
			"Examples:\n" +
			"  $ luci build list -builder \"chromium/ci/Linux UBSan Tests\" -n 5\n" +
			"  $ luci build list -project chromium -bucket ci -builder linux-rel -status failure\n" +
			"  $ luci build list -project chromium -bucket ci -n 5\n" +
			"  $ luci build list -tag dut-name:chromeos8-row1-rack10-host45\n" +
			"  $ luci build list -cl https://chromium-review.googlesource.com/c/infra/luci/luci-go/+/8565529\n" +
			"  $ luci build list -cl 8565529 -status failure",
		CommandRun: func() subcommands.CommandRun {
			r := &buildListRun{af: af, limit: defaultBuildListLimit}
			if r.af != nil {
				r.af.Register(&r.Flags)
			}
			r.Flags.StringVar(&r.host, "host", chromeinfra.BuildbucketHost, "Buildbucket host")
			r.Flags.StringVar(&r.project, "project", "", "Filter by LUCI project (e.g. chromium)")
			r.Flags.StringVar(&r.bucket, "bucket", "", "Filter by Buildbucket bucket (e.g. ci or try; requires -project)")
			r.Flags.StringVar(&r.builder, "builder", "", "Filter by builder (<builder> when used with -project and -bucket, or <project>[/<bucket>[/<builder>]])")
			r.Flags.Var(&r.tags, "tag", "Filter by key:value tag (repeatable, e.g. -tag dut-name:host1)")
			r.Flags.Var(&r.tags, "t", "Alias for -tag")
			r.Flags.StringVar(&r.status, "status", "", "Filter by status (scheduled, started, ended, success, failure, infra_failure, canceled)")
			r.Flags.StringVar(&r.cl, "cl", "", "Filter by Gerrit CL (URL, e.g. https://chromium-review.googlesource.com/c/..., crrev.com/c/..., or change ID)")
			r.Flags.IntVar(&r.patchset, "patchset", 0, "Filter by specific patchset number (defaults to latest patchset with builds, or patchset in -cl URL)")
			r.Flags.IntVar(&r.patchset, "ps", 0, "Alias for -patchset")
			r.Flags.IntVar(&r.limit, "limit", defaultBuildListLimit, "Maximum number of builds to return (default: 10)")
			r.Flags.IntVar(&r.limit, "n", defaultBuildListLimit, "Alias for -limit")
			return r
		},
	}
}

type buildListRun struct {
	subcommands.CommandRunBase
	af           *base.AuthFlags
	client       grpcpb.BuildsClient
	gerritClient gerritpb.GerritClient
	out          io.Writer

	host     string
	project  string
	bucket   string
	builder  string
	tags     stringlistflag.Flag
	status   string
	limit    int
	cl       string
	patchset int
}

func (r *buildListRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			r.Flags.Usage()
			return 0
		}
	}
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "unexpected positional arguments; use flags -project, -bucket, -builder, -tag, -status, or -cl\n")
		return 1
	}
	if r.limit <= 0 {
		fmt.Fprintf(os.Stderr, "flag -limit / -n must be a positive integer\n")
		return 1
	}
	patchsetExplicit := r.patchset != 0
	r.Flags.Visit(func(f *flag.Flag) {
		if f.Name == "patchset" || f.Name == "ps" {
			patchsetExplicit = true
		}
	})
	if patchsetExplicit && r.patchset <= 0 {
		fmt.Fprintf(os.Stderr, "flag -patchset / -ps must be a positive integer\n")
		return 1
	}
	if patchsetExplicit && strings.TrimSpace(r.cl) == "" {
		fmt.Fprintf(os.Stderr, "flag -patchset / -ps requires -cl\n")
		return 1
	}

	var err error
	r.project, r.bucket, r.builder, err = base.ParseBuilderFlags(r.project, r.bucket, r.builder)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	var gerritChange *pb.GerritChange
	if r.cl != "" {
		parsed, err := base.ParseGerritCL(r.cl)
		if err != nil {
			fmt.Fprintf(os.Stderr, "invalid -cl: %s\n", err)
			return 1
		}
		if r.patchset > 0 {
			parsed.Patchset = int64(r.patchset)
		}
		gerritChange = parsed
	}

	predicate, err := buildSearchPredicate(r.project, r.bucket, r.builder, r.tags, r.status, gerritChange)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	ctx := cli.GetContext(a, r, env)
	autoPatchset := gerritChange != nil && gerritChange.Patchset == 0
	if autoPatchset {
		if r.gerritClient == nil && r.af != nil {
			if err := r.af.Parse(); err == nil {
				r.gerritClient, _ = r.af.NewGerritClient(ctx, gerritChange.Host)
			}
		}
		resolvedChange, errPatchset := base.ResolveGerritPatchset(ctx, r.gerritClient, gerritChange)
		if errPatchset != nil {
			fmt.Fprintf(os.Stderr, "failed to resolve latest patchset for CL %d: %s\n", gerritChange.Change, errPatchset)
			return 1
		}
		gerritChange = resolvedChange
		predicate.GerritChanges = []*pb.GerritChange{gerritChange}
	}

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

	builds, notice, err := searchBuildsWithFallback(ctx, client, predicate, mask, int32(r.limit), gerritChange, autoPatchset)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SearchBuilds RPC failed: %s\n", err)
		return 1
	}

	out := r.out
	if out == nil {
		out = os.Stdout
	}
	if notice != "" {
		fmt.Fprintf(out, "%s\n\n", notice)
	}
	if err := FormatBuildList(out, builds); err != nil {
		fmt.Fprintf(os.Stderr, "failed to format builds: %s\n", err)
		return 1
	}
	return 0
}

// searchBuildsWithFallback runs SearchBuilds for predicate, and when autoPatchset
// is true and the latest patchset N (N > 1) has 0 builds in Buildbucket, probes up
// to maxPrecedingPatchsetProbes preceding patchsets (N-1..max(1, N-3)).
func searchBuildsWithFallback(ctx context.Context, client grpcpb.BuildsClient, predicate *pb.BuildPredicate, mask *pb.BuildMask, limit int32, gerritChange *pb.GerritChange, autoPatchset bool) ([]*pb.Build, string, error) {
	resp, err := client.SearchBuilds(ctx, &pb.SearchBuildsRequest{
		Predicate: predicate,
		Mask:      mask,
		PageSize:  limit,
	})
	if err != nil {
		return nil, "", err
	}
	builds := resp.GetBuilds()

	if !autoPatchset || gerritChange.Patchset <= 1 || len(builds) > 0 {
		return builds, "", nil
	}

	hasExtraFilters := predicate.Builder != nil || len(predicate.Tags) > 0 || predicate.Status != pb.Status_STATUS_UNSPECIFIED
	if hasExtraFilters {
		anyBuilds, err := patchsetHasAnyBuilds(ctx, client, gerritChange)
		if err != nil {
			return nil, "", err
		}
		if anyBuilds {
			return builds, "", nil
		}
	}

	latestPatchset := gerritChange.Patchset
	minPatchset := max(int64(1), latestPatchset-maxPrecedingPatchsetProbes)
	for ps := latestPatchset - 1; ps >= minPatchset; ps-- {
		candidate := &pb.GerritChange{
			Host:     gerritChange.Host,
			Project:  gerritChange.Project,
			Change:   gerritChange.Change,
			Patchset: ps,
		}
		probePredicate := &pb.BuildPredicate{
			Builder:       predicate.Builder,
			Tags:          predicate.Tags,
			Status:        predicate.Status,
			GerritChanges: []*pb.GerritChange{candidate},
		}
		probeResp, err := client.SearchBuilds(ctx, &pb.SearchBuildsRequest{
			Predicate: probePredicate,
			Mask:      mask,
			PageSize:  limit,
		})
		if err != nil {
			return nil, "", err
		}
		if len(probeResp.GetBuilds()) > 0 {
			notice := fmt.Sprintf("Notice: No builds on latest patchset %d; showing builds from patchset %d", latestPatchset, ps)
			return probeResp.GetBuilds(), notice, nil
		}
		if hasExtraFilters {
			anyBuilds, err := patchsetHasAnyBuilds(ctx, client, candidate)
			if err != nil {
				return nil, "", err
			}
			if anyBuilds {
				notice := fmt.Sprintf("Notice: No builds on latest patchset %d; patchset %d has builds, but none match the filters", latestPatchset, ps)
				return nil, notice, nil
			}
		}
	}

	if minPatchset == latestPatchset-1 {
		return nil, fmt.Sprintf("Notice: No builds on latest patchset %d (also checked patchset %d)", latestPatchset, minPatchset), nil
	}
	return nil, fmt.Sprintf("Notice: No builds on latest patchset %d (also checked patchsets %d..%d)", latestPatchset, latestPatchset-1, minPatchset), nil
}

// patchsetHasAnyBuilds reports whether Buildbucket has at least one build of any
// status/builder/tag associated with gc.
func patchsetHasAnyBuilds(ctx context.Context, client grpcpb.BuildsClient, gc *pb.GerritChange) (bool, error) {
	resp, err := client.SearchBuilds(ctx, &pb.SearchBuildsRequest{
		Predicate: &pb.BuildPredicate{
			GerritChanges: []*pb.GerritChange{gc},
		},
		Mask: &pb.BuildMask{
			Fields: &field_mask.FieldMask{Paths: []string{"id"}},
		},
		PageSize: 1,
	})
	if err != nil {
		return false, err
	}
	return len(resp.GetBuilds()) > 0, nil
}

// buildSearchPredicate constructs a Buildbucket BuildPredicate from the
// normalized project, bucket, and builder components and the remaining CLI
// filter flags (-tag, -status, -cl).
func buildSearchPredicate(project, bucket, builder string, tagsRaw []string, statusRaw string, gerritChange *pb.GerritChange) (*pb.BuildPredicate, error) {
	var builderID *pb.BuilderID
	if project != "" || bucket != "" || builder != "" {
		builderID = &pb.BuilderID{
			Project: project,
			Bucket:  bucket,
			Builder: builder,
		}
	}

	tags, err := parseTagFilters(tagsRaw)
	if err != nil {
		return nil, err
	}

	status, err := parseBuildStatus(statusRaw)
	if err != nil {
		return nil, err
	}

	if builderID == nil && len(tags) == 0 && status == pb.Status_STATUS_UNSPECIFIED && gerritChange == nil {
		return nil, errors.New("at least one filter flag (-project, -bucket, -builder, -tag, -status, or -cl) is required")
	}

	var gerritChanges []*pb.GerritChange
	if gerritChange != nil {
		gerritChanges = []*pb.GerritChange{gerritChange}
	}

	return &pb.BuildPredicate{
		Builder:       builderID,
		Tags:          tags,
		Status:        status,
		GerritChanges: gerritChanges,
	}, nil
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
