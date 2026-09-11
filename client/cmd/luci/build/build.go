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
	"os"

	"github.com/maruel/subcommands"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/protobuf/encoding/protojson"

	pb "go.chromium.org/luci/buildbucket/proto"
	grpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

// Cmd returns the top-level command for `luci build`.
func Cmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "build <subcommand>",
		ShortDesc: "Inspect Buildbucket builds",
		LongDesc: "Inspect Buildbucket builds, build status, timing, summary, and step execution.\n\n" +
			"Available subcommands:\n" +
			"  get       Get details and step status of a build",
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
	return base.RunSubcommandApp(a, "luci build", "Build management", []*subcommands.Command{
		GetCmd(r.af),
		subcommands.CmdHelp,
	}, args)
}

// GetCmd returns the subcommand for `luci build get`.
func GetCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get -buildid <build_id>",
		ShortDesc: "Get details and status of a build",
		LongDesc: "Get build details from Buildbucket by build ID (run 'luci ids <url>' to extract ids).\n\n" +
			"By default, prints high-level build status, duration, code changes, summary, and any failed steps.\n" +
			"Use -steps to print the full hierarchical step tree, or -json for raw JSON output.",
		CommandRun: func() subcommands.CommandRun {
			r := &buildGetRun{af: af}
			r.af.Register(&r.Flags)
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
	host        string
	id          int64
	steps       bool
	failedSteps bool
	properties  bool
	json        bool
}

func (r *buildGetRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "unexpected positional arguments; use flag -buildid (run 'luci ids <url>' to extract ids)\n")
		return 1
	}

	if r.id <= 0 {
		fmt.Fprintf(os.Stderr, "flag -buildid is required (run 'luci ids <url>' to extract ids)\n")
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

	b, err := client.GetBuild(ctx, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetBuild RPC failed: %s\n", err)
		return 1
	}

	if r.json {
		out, err := protojson.MarshalOptions{Multiline: true, Indent: "  "}.Marshal(b)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to marshal build to JSON: %s\n", err)
			return 1
		}
		fmt.Println(string(out))
		return 0
	}

	opts := FormatOptions{
		AllSteps:    r.steps,
		FailedSteps: r.failedSteps,
		Properties:  r.properties,
	}
	if err := FormatBuild(os.Stdout, b, opts); err != nil {
		fmt.Fprintf(os.Stderr, "failed to format build: %s\n", err)
		return 1
	}

	return 0
}
