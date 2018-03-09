// Copyright 2018 The LUCI Authors.
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

package cli

import (
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
)

// printDRACs prints DRAC data to stdout in tab-separated columns.
func printDRACs(tsv bool, dracs ...*crimson.DRAC) {
	if len(dracs) > 0 {
		p := newStdoutPrinter(tsv)
		defer p.Flush()
		if !tsv {
			p.Row("Name", "Machine", "IP Address", "VLAN")
		}
		for _, d := range dracs {
			p.Row(d.Name, d.Machine, d.Ipv4, d.Vlan)
		}
	}
}

// AddDRACCmd is the command to add a DRAC.
type AddDRACCmd struct {
	subcommands.CommandRunBase
	drac crimson.DRAC
	f    FormattingFlags
}

// Run runs the command to add a DRAC.
func (c *AddDRACCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	// TODO(smut): Validate required fields client-side.
	req := &crimson.CreateDRACRequest{
		Drac: &c.drac,
	}
	client := getClient(ctx)
	resp, err := client.CreateDRAC(ctx, req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	printDRACs(c.f.tsv, resp)
	return 0
}

// addDRACCmd returns a command to add a DRAC.
func addDRACCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "add-drac -name <name> -machine <machine> -ipv4 <ip address>",
		ShortDesc: "adds a DRAC",
		LongDesc:  "Adds a DRAC to the database.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &AddDRACCmd{}
			cmd.Flags.StringVar(&cmd.drac.Name, "name", "", "The name of the DRAC. Required and must be unique per machine within the database.")
			cmd.Flags.StringVar(&cmd.drac.Machine, "machine", "", "The machine this DRAC belongs to. Required and must be the name of a machine returned by get-machines.")
			cmd.Flags.StringVar(&cmd.drac.Ipv4, "ipv4", "", "The IPv4 address assigned to this DRAC. Required and must be a free IP address returned by get-ips.")
			cmd.f.Register(cmd)
			return cmd
		},
	}
}

// GetDRACsCmd is the command to get DRACs.
type GetDRACsCmd struct {
	subcommands.CommandRunBase
	req crimson.ListDRACsRequest
	f   FormattingFlags
}

// Run runs the command to get DRACs.
func (c *GetDRACsCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	client := getClient(ctx)
	resp, err := client.ListDRACs(ctx, &c.req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	printDRACs(c.f.tsv, resp.Dracs...)
	return 0
}

// getDRACCmd returns a command to get DRACs.
func getDRACsCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-dracs [-name <name>]... [-machine <machine>]... [-ipv4 <ip address>]... [-vlan <id>]...",
		ShortDesc: "retrieves DRACs",
		LongDesc:  "Retrieves DRACs matching the given names and machines, or all DRACs if names and machines are omitted.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetDRACsCmd{}
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Names), "name", "Name of a DRAC to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Machines), "machine", "Name of a machine to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Ipv4S), "ipv4", "IPv4 address to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.Int64Slice(&cmd.req.Vlans), "vlan", "ID of a VLAN to filter by. Can be specified multiple times.")
			cmd.f.Register(cmd)
			return cmd
		},
	}
}
