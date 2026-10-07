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
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/hardcoded/chromeinfra"
)

// LogListCmd returns the subcommand for `luci build log list`.
func LogListCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "list -buildid <build_id> [-step <step_name>]",
		ShortDesc: "List logs for a build or build step",
		LongDesc: "List LogDog logs for all steps in a build, or for a specific step when -step is specified.\n\n" +
			"Examples:\n" +
			"  $ luci build log list -buildid 8738491827364512345\n" +
			"  $ luci build log list -buildid 8738491827364512345 -step compile",
		CommandRun: func() subcommands.CommandRun {
			r := &buildLogListRun{af: af}
			if r.af != nil {
				r.af.Register(&r.Flags)
			}
			r.Flags.StringVar(&r.host, "host", chromeinfra.BuildbucketHost, "Buildbucket host")
			r.Flags.Int64Var(&r.id, "buildid", 0, "Build ID (e.g. 8738491827364512345)")
			r.Flags.Int64Var(&r.id, "id", 0, "Alias for -buildid")
			r.Flags.StringVar(&r.step, "step", "", "Optional build step name to filter logs")
			return r
		},
	}
}

type buildLogListRun struct {
	subcommands.CommandRunBase
	af           *base.AuthFlags
	buildsClient grpcpb.BuildsClient
	out          io.Writer

	host string
	id   int64
	step string
}

func (r *buildLogListRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			r.Flags.Usage()
			return 0
		}
	}
	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "unexpected positional arguments; use flags -buildid and -step (run 'luci ids <url>' to extract IDs)\n")
		return 1
	}
	if r.id <= 0 {
		fmt.Fprintf(os.Stderr, "flag -buildid is required (run 'luci ids <url>' to extract IDs)\n")
		return 1
	}

	ctx := cli.GetContext(a, r, env)
	buildsClient := r.buildsClient
	if buildsClient == nil {
		if err := r.af.Parse(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
			return 1
		}
		var errClient error
		buildsClient, _, errClient = r.af.NewBuildsClient(ctx, r.host)
		if errClient != nil {
			fmt.Fprintf(os.Stderr, "failed to create buildbucket client: %s\n", errClient)
			return 1
		}
	}

	b, err := buildsClient.GetBuild(ctx, &pb.GetBuildRequest{
		Id: r.id,
		Mask: &pb.BuildMask{
			Fields: &field_mask.FieldMask{
				Paths: []string{"id", "steps", "output.logs"},
			},
		},
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetBuild RPC failed: %s\n", err)
		return 1
	}

	out := r.out
	if out == nil {
		out = os.Stdout
	}

	if r.step != "" {
		matchedStep, err := resolveTargetStep(b, r.step)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s\n", err)
			return 1
		}
		fmt.Fprintf(out, "Step: %s (%s)\n", matchedStep.Name, matchedStep.Status)
		if len(matchedStep.Logs) == 0 {
			fmt.Fprintf(out, "  (no logs)\n")
			return 0
		}
		for _, l := range matchedStep.Logs {
			if l.ViewUrl != "" {
				fmt.Fprintf(out, "  %-32s %s\n", l.Name, l.ViewUrl)
			} else {
				fmt.Fprintf(out, "  %s\n", l.Name)
			}
		}
		return 0
	}

	printed := false
	if buildLogs := b.GetOutput().GetLogs(); len(buildLogs) > 0 {
		printed = true
		fmt.Fprintf(out, "Build Logs:\n")
		for _, l := range buildLogs {
			if l.ViewUrl != "" {
				fmt.Fprintf(out, "  %-32s %s\n", l.Name, l.ViewUrl)
			} else {
				fmt.Fprintf(out, "  %s\n", l.Name)
			}
		}
	}
	for _, s := range b.Steps {
		if len(s.Logs) == 0 {
			continue
		}
		printed = true
		var names []string
		for _, l := range s.Logs {
			names = append(names, l.Name)
		}
		fmt.Fprintf(out, "%s (%s): %s\n", s.Name, s.Status, strings.Join(names, ", "))
	}
	if !printed {
		fmt.Fprintf(out, "No logs found in build %d.\n", b.Id)
	}
	return 0
}
