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

	"github.com/maruel/subcommands"
	"google.golang.org/genproto/protobuf/field_mask"

	pb "go.chromium.org/luci/buildbucket/proto"
	grpcpb "go.chromium.org/luci/buildbucket/proto/grpcpb"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	logdog "go.chromium.org/luci/logdog/api/endpoints/coordinator/logs/v1"
	"go.chromium.org/luci/logdog/common/types"
)

// LogCmd returns the subcommand for `luci build log`.
func LogCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "log <subcommand>",
		ShortDesc: "Manage build step logs",
		LongDesc: "Manage Buildbucket step and build logs stored in LogDog.\n\n" +
			"Available subcommands:\n" +
			"  list      List logs for a build or build step\n" +
			"  get       Download or print full content of a step log\n" +
			"  head      Print first N lines/bytes of a step log\n" +
			"  tail      Print last N lines/bytes of a step log",
		CommandRun: func() subcommands.CommandRun {
			return &logParentRun{af: af}
		},
	}
}

type logParentRun struct {
	subcommands.CommandRunBase
	af *base.AuthFlags
}

func (r *logParentRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return base.RunSubcommandApp(a, "luci build log", "Build step log management", []*subcommands.Command{
		LogListCmd(r.af),
		LogGetCmd(r.af),
		LogHeadCmd(r.af),
		LogTailCmd(r.af),
		subcommands.CmdHelp,
	}, args)
}

func newBuildLogFetchCmd(af *base.AuthFlags, mode string) *subcommands.Command {
	var usage, shortDesc, longDesc string
	switch mode {
	case "head":
		usage = "head -buildid <build_id> -step <step_name> [-log <log_name>] [-n <lines>] [-c <bytes>] [-o <file>]"
		shortDesc = "Print the first N lines of a build step log"
		longDesc = "Print the first N lines (default 10) of a Buildbucket step log via LogDog.\n" +
			"If -log is omitted, defaults to 'stdout' (or 'failure_summary' / the single log if only one exists).\n\n" +
			"Examples:\n" +
			"  $ luci build log head -buildid 8738491827364512345 -step compile -n 50\n" +
			"  $ luci build log head -buildid 8738491827364512345 -step compile -log failure_summary"
	case "tail":
		usage = "tail -buildid <build_id> -step <step_name> [-log <log_name>] [-n <lines>] [-c <bytes>] [-o <file>]"
		shortDesc = "Print the last N lines of a build step log"
		longDesc = "Print the last N lines (default 10) of a Buildbucket step log via LogDog.\n" +
			"If -log is omitted, defaults to 'stdout' (or 'failure_summary' / the single log if only one exists).\n\n" +
			"Examples:\n" +
			"  $ luci build log tail -buildid 8738491827364512345 -step compile -n 50\n" +
			"  $ luci build log tail -buildid 8738491827364512345 -step compile -log failure_summary"
	default:
		usage = "get -buildid <build_id> -step <step_name> [-log <log_name>] [-c <bytes>] [-o <file>]"
		shortDesc = "Download or print full content of a build step log"
		longDesc = "Fetch or save the full content of a Buildbucket step log via LogDog.\n" +
			"If -log is omitted, defaults to 'stdout' (or 'failure_summary' / the single log if only one exists).\n\n" +
			"Examples:\n" +
			"  $ luci build log get -buildid 8738491827364512345 -step compile -log failure_summary\n" +
			"  $ luci build log get -buildid 8738491827364512345 -step \"test|browser_tests\" -log stdout -o /tmp/step.log"
	}

	return &subcommands.Command{
		UsageLine: usage,
		ShortDesc: shortDesc,
		LongDesc:  longDesc,
		CommandRun: func() subcommands.CommandRun {
			r := &buildLogRun{af: af, mode: mode}
			if r.af != nil {
				r.af.Register(&r.Flags)
			}
			r.Flags.StringVar(&r.host, "host", chromeinfra.BuildbucketHost, "Buildbucket host")
			r.Flags.Int64Var(&r.id, "buildid", 0, "Build ID (e.g. 8738491827364512345)")
			r.Flags.Int64Var(&r.id, "id", 0, "Alias for -buildid")
			r.Flags.StringVar(&r.step, "step", "", "Build step name (e.g. 'compile' or 'test|browser_tests')")
			r.Flags.StringVar(&r.logName, "log", "", "Step log name (defaults to 'stdout')")
			if mode == "head" || mode == "tail" {
				r.Flags.IntVar(&r.lines, "n", 10, "Number of lines to fetch (default 10)")
			}
			r.Flags.Int64Var(&r.maxBytes, "c", 0, "Maximum number of bytes to fetch")
			r.Flags.Int64Var(&r.maxBytes, "bytes", 0, "Alias for -c")
			r.Flags.StringVar(&r.outputFile, "o", "", "Optional file path to save the log content to")
			r.Flags.StringVar(&r.outputFile, "output", "", "Alias for -o")
			return r
		},
	}
}

type buildLogRun struct {
	subcommands.CommandRunBase
	af           *base.AuthFlags
	buildsClient grpcpb.BuildsClient
	logsClient   logdog.LogsClient
	out          io.Writer
	mode         string

	host       string
	id         int64
	step       string
	logName    string
	lines      int
	maxBytes   int64
	outputFile string
}

func (r *buildLogRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			r.Flags.Usage()
			return 0
		}
	}

	if len(args) > 0 {
		fmt.Fprintf(os.Stderr, "unexpected positional arguments; use flags -buildid, -step, and -log (run 'luci ids <url>' to extract IDs)\n")
		return 1
	}

	if r.id <= 0 {
		fmt.Fprintf(os.Stderr, "flag -buildid is required (run 'luci ids <url>' to extract IDs)\n")
		return 1
	}

	if r.step == "" && r.logName == "" {
		fmt.Fprintf(os.Stderr, "flag -step is required (run 'luci build get -buildid %d' to see steps and logs)\n", r.id)
		return 1
	}

	if r.lines < 0 || r.maxBytes < 0 {
		fmt.Fprintf(os.Stderr, "-n and -c/-bytes must be non-negative\n")
		return 1
	}

	if (r.mode == "head" || r.mode == "tail") && r.lines <= 0 && r.maxBytes <= 0 {
		fmt.Fprintf(os.Stderr, "-n or -c/-bytes must be positive\n")
		return 1
	}

	if r.maxBytes > 0 {
		r.lines = 0
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

	req := &pb.GetBuildRequest{
		Id: r.id,
		Mask: &pb.BuildMask{
			Fields: &field_mask.FieldMask{
				Paths: []string{"id", "steps", "output.logs"},
			},
		},
	}

	b, err := buildsClient.GetBuild(ctx, req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetBuild RPC failed: %s\n", err)
		return 1
	}

	targetLog, err := resolveTargetLog(b, r.step, r.logName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	if targetLog.Url == "" {
		fmt.Fprintf(os.Stderr, "log %q has an empty LogDog URL\n", targetLog.Name)
		return 1
	}

	addr, err := types.ParseURL(targetLog.Url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "log %q has invalid LogDog URL %q: %s\n", targetLog.Name, targetLog.Url, err)
		return 1
	}

	logsClient := r.logsClient
	if logsClient == nil {
		var errClient error
		logsClient, errClient = r.af.NewLogDogClient(ctx, addr.Host)
		if errClient != nil {
			fmt.Fprintf(os.Stderr, "failed to create LogDog client for %s: %s\n", addr.Host, errClient)
			return 1
		}
	}

	out := r.out
	if r.outputFile != "" {
		f, err := os.Create(r.outputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "failed to create output file %q: %s\n", r.outputFile, err)
			return 1
		}
		defer f.Close()
		out = f
	} else if out == nil {
		out = os.Stdout
	}

	var fetchErr error
	switch r.mode {
	case "tail":
		fetchErr = FetchLogDogTail(ctx, logsClient, addr.Project, addr.Path, r.lines, r.maxBytes, out)
	case "head":
		fetchErr = FetchLogDogHead(ctx, logsClient, addr.Project, addr.Path, r.lines, r.maxBytes, out)
	default:
		fetchErr = FetchLogDogFull(ctx, logsClient, addr.Project, addr.Path, r.maxBytes, out)
	}

	if fetchErr != nil {
		fmt.Fprintf(os.Stderr, "failed to fetch log %q: %s\n", targetLog.Name, fetchErr)
		return 1
	}

	if r.outputFile != "" {
		fmt.Fprintf(os.Stderr, "Saved log %q to %s\n", targetLog.Name, r.outputFile)
	}
	return 0
}
