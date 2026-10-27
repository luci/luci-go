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
	"fmt"
	"io"
	"os"

	"github.com/maruel/subcommands"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/encoding/protojson"

	pb "go.chromium.org/luci/buildbucket/proto"
	grpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/ids"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

func ensureBuildsClient(ctx context.Context, af *base.AuthFlags, host string, existing grpcpb.BuildsClient) (grpcpb.BuildsClient, bool) {
	if existing != nil {
		return existing, true
	}
	if af == nil {
		fmt.Fprintf(os.Stderr, "auth flags not configured\n")
		return nil, false
	}
	if err := af.Parse(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
		return nil, false
	}
	client, _, err := af.NewBuildsClient(ctx, host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create buildbucket client: %s\n", err)
		return nil, false
	}
	return client, true
}

// Cmd returns the top-level command for `luci build`.
func Cmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "build <subcommand>",
		ShortDesc: "Inspect Buildbucket builds",
		LongDesc: "Inspect Buildbucket builds, build status, timing, summary, and step execution.\n\n" +
			"Available subcommands:\n" +
			"  get       Get details and step status of a build\n" +
			"  list      Search and list builds by builder, tag, or status\n" +
			"  log       Manage build step logs (list, get, head, tail)",
		CommandRun: func() subcommands.CommandRun {
			return &buildRun{af: af}
		},
	}
}

type buildRun struct {
	subcommands.CommandRunBase
	af *base.AuthFlags
}

func (r *buildRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if len(args) > 0 && args[0] == "ls" {
		args = append([]string{"list"}, args[1:]...)
	}
	if len(args) > 0 && args[0] == "logs" {
		args = append([]string{"log"}, args[1:]...)
	}
	return base.RunSubcommandApp(a, "luci build", "Build management", []*subcommands.Command{
		GetCmd(r.af),
		ListCmd(r.af),
		LogCmd(r.af),
		subcommands.CmdHelp,
	}, args)
}

// GetCmd returns the subcommand for `luci build get`.
func GetCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get [-buildid <build_id>] [<target>]",
		ShortDesc: "Get details and status of a build",
		LongDesc: "Get build details from Buildbucket by build ID or positional URL/ID <target>.\n\n" +
			"By default, prints high-level build status, duration, code changes, summary, and any failed steps.\n" +
			"Use -steps to print the full hierarchical step tree, or -json for raw JSON output.",
		CommandRun: func() subcommands.CommandRun {
			r := &buildGetRun{af: af}
			if r.af != nil {
				r.af.Register(&r.Flags)
			}
			r.Flags.StringVar(&r.host, "host", chromeinfra.BuildbucketHost, "Buildbucket host")
			r.Flags.Int64Var(&r.id, "buildid", 0, "Build ID (e.g. 8738491827364512345)")
			r.Flags.Int64Var(&r.id, "id", 0, "Alias for -buildid")
			r.Flags.BoolVar(&r.steps, "steps", false, "Print all steps in a hierarchical tree")
			r.Flags.BoolVar(&r.failedSteps, "failed-steps", false, "Print failed steps only")
			r.Flags.BoolVar(&r.properties, "properties", false, "Print input and output properties")
			r.Flags.BoolVar(&r.properties, "p", false, "Alias for -properties")
			r.Flags.BoolVar(&r.json, "json", false, "Output build message as JSON")
			return r
		},
	}
}

type buildGetRun struct {
	subcommands.CommandRunBase
	af          *base.AuthFlags
	client      grpcpb.BuildsClient
	out         io.Writer
	host        string
	id          int64
	steps       bool
	failedSteps bool
	properties  bool
	json        bool
}

func (r *buildGetRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if base.HasHelpFlag(args) {
		r.Flags.Usage()
		return 0
	}

	target, err := base.ParsePositionalTarget(&r.Flags, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	ctx := cli.GetContext(a, r, env)
	extracted, err := ids.ExtractWithAuth(ctx, r.af, nil, r.client, target, ids.TargetBindings{
		BuildIDInt64: &r.id,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to extract IDs from %q: %s\n", target, err)
		return 1
	}

	if r.id <= 0 {
		ids.PrintMissingFlagsError(os.Stderr, extracted, "flag -buildid or a positional target is required")
		return 1
	}

	req := &pb.GetBuildRequest{
		Id: r.id,
	}

	// Configure fields / mask
	if r.json {
		req.Mask = &pb.BuildMask{AllFields: true}
	} else {
		paths := []string{
			"id",
			"builder",
			"number",
			"status",
			"status_details",
			"summary_markdown",
			"create_time",
			"start_time",
			"end_time",
			"update_time",
			"created_by",
			"input.gitiles_commit",
			"input.gerrit_changes",
			"input.experiments",
			"infra.swarming.bot_dimensions",
			"infra.resultdb",
			"steps",
			"tags",
		}
		if r.properties {
			paths = append(paths, "input.properties", "output.properties")
		}
		req.Mask = &pb.BuildMask{
			Fields: &field_mask.FieldMask{Paths: paths},
		}
	}

	client, ok := ensureBuildsClient(ctx, r.af, r.host, r.client)
	if !ok {
		return 1
	}

	b, err := client.GetBuild(ctx, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetBuild RPC failed: %s\n", err)
		return 1
	}

	out := r.out
	if out == nil {
		out = os.Stdout
	}

	if r.json {
		ids.PrintDerivedIDs(os.Stderr, extracted)
		jsonOut, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(b)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal build to JSON: %s\n", err)
			return 1
		}
		fmt.Fprintln(out, string(jsonOut))
		return 0
	}

	ids.PrintDerivedIDs(out, extracted)

	opts := FormatOptions{
		AllSteps:    r.steps,
		FailedSteps: r.failedSteps,
		Properties:  r.properties,
	}
	if err := FormatBuild(out, b, opts); err != nil {
		fmt.Fprintf(os.Stderr, "failed to format build: %s\n", err)
		return 1
	}

	return 0
}
