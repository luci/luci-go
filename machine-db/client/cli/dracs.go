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
			p.Row("Name", "Machine", "MAC Address", "Switch", "Port", "IP Address", "VLAN")
		}
		for _, d := range dracs {
			p.Row(d.Name, d.Machine, d.MacAddress, d.Switch, d.Switchport, d.Ipv4, d.Vlan)
		}
	}
}

// AddDRACCmd is the command to add a DRAC.
type AddDRACCmd struct {
	commandBase
	drac crimson.DRAC
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
func addDRACCmd(params *Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "add-drac -name <name> -machine <machine> -mac <mac address> -switch <switch> -port <port> -ip <ip address>",
		ShortDesc: "adds a DRAC",
		LongDesc:  "Adds a DRAC to the database.\n\nExample:\ncrimson add-drac -name server01-yy-drac -machine xx1-07-720 -mac 00:00:b3:c0:00:11 -switch switch.lab0 -port 99 -ip 999.999.999.999",
		CommandRun: func() subcommands.CommandRun {
			cmd := &AddDRACCmd{}
			cmd.Initialize(params)
			cmd.Flags.StringVar(&cmd.drac.Name, "name", "", "The name of the DRAC. Required and must be unique per machine within the database.")
			cmd.Flags.StringVar(&cmd.drac.Machine, "machine", "", "The machine this DRAC belongs to. Required and must be the name of a machine returned by get-machines.")
			cmd.Flags.StringVar(&cmd.drac.MacAddress, "mac", "", "The MAC address of this DRAC. Required and must be a valid MAC-48 address.")
			cmd.Flags.StringVar(&cmd.drac.Switch, "switch", "", "The switch this DRAC is connected to. Required and must be the name of a switch returned by get-switches.")
			cmd.Flags.Var(flag.Int32(&cmd.drac.Switchport), "port", "The switchport this DRAC is connected to.")
			cmd.Flags.StringVar(&cmd.drac.Ipv4, "ip", "", "The IPv4 address assigned to this DRAC. Required and must be a free IP address returned by get-ips.")
			return cmd
		},
	}
}

// EditDRACCmd is the command to edit a DRAC.
type EditDRACCmd struct {
	commandBase
	drac crimson.DRAC
}

// Run runs the command to edit a DRAC.
func (c *EditDRACCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	// TODO(smut): Validate required fields client-side.
	req := &crimson.UpdateDRACRequest{
		Drac: &c.drac,
		UpdateMask: getUpdateMask(&c.Flags, map[string]string{
			"machine": "machine",
			"mac":     "mac_address",
			"switch":  "switch",
			"port":    "switchport",
		}),
	}
	client := getClient(ctx)
	resp, err := client.UpdateDRAC(ctx, req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	printDRACs(c.f.tsv, resp)
	return 0
}

// editDRACCmd returns a command to edit a DRAC.
func editDRACCmd(params *Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "edit-drac -name <name> [-machine <machine>] [-mac <mac address>] [-switch <switch>] [-port <switch port>]",
		ShortDesc: "edit a DRAC",
		LongDesc:  "Edits a DRAC in the database.\n\nExample to edit MAC address of a DRAC:\ncrimson edit-drac -name server01-y1-drac -mac 00:00:b3:c0:00:11",
		CommandRun: func() subcommands.CommandRun {
			cmd := &EditDRACCmd{}
			cmd.Initialize(params)
			cmd.Flags.StringVar(&cmd.drac.Name, "name", "", "The name of the DRAC. Required and must be the name of a DRAC returned by get-dracs.")
			cmd.Flags.StringVar(&cmd.drac.Machine, "machine", "", "The machine this DRAC belongs to. Must be the name of a machine returned by get-machines.")
			cmd.Flags.StringVar(&cmd.drac.MacAddress, "mac", "", "The MAC address of this DRAC. Must be a valid MAC-48 address.")
			cmd.Flags.StringVar(&cmd.drac.Switch, "switch", "", "The switch this DRAC is connected to. Must be the name of a switch returned by get-switches.")
			cmd.Flags.Var(flag.Int32(&cmd.drac.Switchport), "port", "The switchport this DRAC is connected to.")
			return cmd
		},
	}
}

// GetDRACsCmd is the command to get DRACs.
type GetDRACsCmd struct {
	commandBase
	req crimson.ListDRACsRequest
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
func getDRACsCmd(params *Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-dracs [-name <name>]... [-machine <machine>]... [-mac <mac address>]... [-switch <switch>]... [-ip <ip address>]... [-vlan <id>]...",
		ShortDesc: "retrieves DRACs",
		LongDesc:  "Retrieves DRACs matching the given names and machines, or all DRACs if names and machines are omitted.\n\nExample to get all DRACs:\ncrimson get-dracs\nExample to get all DRACs in VLAN 1:\ncrimson get-dracs -vlan 1",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetDRACsCmd{}
			cmd.Initialize(params)
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Names), "name", "Name of a DRAC to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Machines), "machine", "Name of a machine to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.MacAddresses), "mac", "MAC address to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Switches), "switch", "Name of a switch to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Ipv4S), "ip", "IPv4 address to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.Int64Slice(&cmd.req.Vlans), "vlan", "ID of a VLAN to filter by. Can be specified multiple times.")
			return cmd
		},
	}
}
