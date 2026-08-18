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
	"fmt"
	"os"

	"github.com/maruel/subcommands"
	"google.golang.org/protobuf/encoding/protojson"

	"go.chromium.org/luci/client/cmd/luci/artifact"
	"go.chromium.org/luci/client/cmd/luci/base"
	"go.chromium.org/luci/client/cmd/luci/format"
	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/hardcoded/chromeinfra"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func Cmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "work-unit <subcommand>",
		ShortDesc: "Manage ResultDB work units",
		LongDesc: "Manage ResultDB work units (V2 execution hierarchy nodes).\n\n" +
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
	return base.RunSubcommandApp(a, "luci work-unit", "Work unit management", []*subcommands.Command{
		GetCmd(r.af),
		artifact.WorkUnitArtifactCmd(r.af),
		subcommands.CmdHelp,
	}, args)
}

func GetCmd(af *base.AuthFlags) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get <name> | get - [<work_unit_id>] | get <root_inv> <work_unit_id>",
		ShortDesc: "Get a work unit",
		LongDesc:  "Get details of a work unit by its resource name or decomposed IDs.\nUse '-' to reuse the cached invocation with trailing work_unit_id override.",
		CommandRun: func() subcommands.CommandRun {
			r := &workUnitGetRun{af: af}
			r.af.Register(&r.Flags)
			r.Flags.StringVar(&r.host, "host", chromeinfra.ResultDBHost, "ResultDB host")
			r.Flags.BoolVar(&r.showMetadata, "show-metadata", false, "Additionally print metadata, tags, and properties")
			return r
		},
	}
}

type workUnitGetRun struct {
	subcommands.CommandRunBase
	af           *base.AuthFlags
	host         string
	showMetadata bool
}

func (r *workUnitGetRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" || arg == "-help" {
			r.Flags.Usage()
			return 0
		}
	}
	ctx := cli.GetContext(a, r, env)
	name, err := base.ParseWorkUnitTargetArgs(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		return 1
	}

	clean := base.TrimResourceURL(name)
	base.RecordWorkUnit(clean, "")

	if err := r.af.Parse(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to parse auth flags: %s\n", err)
		return 1
	}

	client, _, _, err := r.af.NewResultDBClient(ctx, r.host)
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create resultdb client: %s\n", err)
		return 1
	}

	wu, err := client.GetWorkUnit(ctx, &pb.GetWorkUnitRequest{Name: clean})
	if err != nil {
		fmt.Fprintf(os.Stderr, "GetWorkUnit RPC failed: %s\n", err)
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
	fmt.Printf("State:        %s\n", wu.State)
	if wu.FinalizationState != pb.WorkUnit_FINALIZATION_STATE_UNSPECIFIED {
		fmt.Printf("Finalization: %s\n", wu.FinalizationState)
	}
	if wu.SummaryMarkdown != "" {
		fmt.Printf("Summary:\n%s\n", wu.SummaryMarkdown)
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
