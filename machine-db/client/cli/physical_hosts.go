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

// printPhysicalHosts prints physical host data to stdout in tab-separated columns.
func printPhysicalHosts(showHeaders bool, hosts ...*crimson.PhysicalHost) {
	if len(hosts) > 0 {
		p := newStdoutPrinter()
		defer p.Flush()
		if showHeaders {
			p.Row("Name", "VLAN", "IP Address", "Machine", "OS", "VM Slots", "Description", "Deployment Ticket", "State")
		}
		for _, h := range hosts {
			p.Row(h.Name, h.Vlan, h.Ipv4, h.Machine, h.Os, h.VmSlots, h.Description, h.DeploymentTicket, h.State)
		}
	}
}

// AddPhysicalHostCmd is the command to add a physical host.
type AddPhysicalHostCmd struct {
	subcommands.CommandRunBase
	host crimson.PhysicalHost
	f    FormattingFlags
}

// Run runs the command to add a physical host.
func (c *AddPhysicalHostCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	// TODO(smut): Validate required fields client-side.
	req := &crimson.CreatePhysicalHostRequest{
		Host: &c.host,
	}
	client := getClient(ctx)
	resp, err := client.CreatePhysicalHost(ctx, req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	printPhysicalHosts(c.f.showHeaders, resp)
	return 0
}

// addPhysicalHostCmd returns a command to add a physical host.
func addPhysicalHostCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "add-host -name <name> -machine <machine> -os <os> -ipv4 <ip address> [-state <state>] [-slots <vm slots>] [-desc <description>] [-tick <deployment ticket>]",
		ShortDesc: "adds a physical host",
		LongDesc:  "Adds a physical host to the database.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &AddPhysicalHostCmd{}
			cmd.Flags.StringVar(&cmd.host.Name, "name", "", "The name of this host on the network. Required and must be unique per VLAN within the database.")
			cmd.Flags.StringVar(&cmd.host.Machine, "machine", "", "The machine backing this host. Required and must be the name of a machine returned by get-machines.")
			cmd.Flags.StringVar(&cmd.host.Os, "os", "", "The operating system this host is running. Required and must be the name of an operating system returned by get-oses.")
			cmd.Flags.StringVar(&cmd.host.Ipv4, "ipv4", "", "The IPv4 address assigned to this host. Required and must be a free IP address returned by get-ips.")
			cmd.Flags.Var(StateFlag(&cmd.host.State), "state", "The state of this host. Must be the name of a state returned by get-states.")
			cmd.Flags.Var(flag.Int32(&cmd.host.VmSlots), "slots", "The number of VMs which can be deployed on this host.")
			cmd.Flags.StringVar(&cmd.host.Description, "desc", "", "A description of this host.")
			cmd.Flags.StringVar(&cmd.host.DeploymentTicket, "tick", "", "The deployment ticket associated with this host.")
			cmd.f.Register(cmd)
			return cmd
		},
	}
}

// EditPhysicalHostCmd is the command to edit a physical host.
type EditPhysicalHostCmd struct {
	subcommands.CommandRunBase
	host crimson.PhysicalHost
	f    FormattingFlags
}

// Run runs the command to edit a physical host.
func (c *EditPhysicalHostCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	// TODO(smut): Validate required fields client-side.
	req := &crimson.UpdatePhysicalHostRequest{
		Host: &c.host,
		UpdateMask: getUpdateMask(&c.Flags, map[string]string{
			"machine": "machine",
			"os":      "os",
			"state":   "state",
			"slots":   "vm_slots",
			"desc":    "description",
			"tick":    "deployment_ticket",
		}),
	}
	client := getClient(ctx)
	resp, err := client.UpdatePhysicalHost(ctx, req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	printPhysicalHosts(c.f.showHeaders, resp)
	return 0
}

// editPhysicalHostCmd returns a command to edit a physical host.
func editPhysicalHostCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "edit-host -name <name> -vlan <id> [-machine <machine>] [-os <os>] [-state <state>] [-slots <vm slots>] [-desc <description>] [-tick <deployment ticket>]",
		ShortDesc: "edits a physical host",
		LongDesc:  "Edits a physical host in the database.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &EditPhysicalHostCmd{}
			cmd.Flags.StringVar(&cmd.host.Name, "name", "", "The name of this host on the network. Required and must be the name of a host returned by get-hosts.")
			cmd.Flags.Int64Var(&cmd.host.Vlan, "vlan", 0, "The VLAN this host belongs to. Required and must be the ID of a VLAN returned by get-vlans.")
			cmd.Flags.StringVar(&cmd.host.Machine, "machine", "", "The machine backing this host. Must be the name of a machine returned by get-machines.")
			cmd.Flags.StringVar(&cmd.host.Os, "os", "", "The operating system this host is running. Must be the name of an operating system returned by get-oses.")
			cmd.Flags.Var(StateFlag(&cmd.host.State), "state", "The state of this host. Must be a state returned by get-states.")
			cmd.Flags.Var(flag.Int32(&cmd.host.VmSlots), "slots", "The number of VMs which can be deployed on this host.")
			cmd.Flags.StringVar(&cmd.host.Description, "desc", "", "A description of this host.")
			cmd.Flags.StringVar(&cmd.host.DeploymentTicket, "tick", "", "The deployment ticket associated with this host.")
			cmd.f.Register(cmd)
			return cmd
		},
	}
}

// GetPhysicalHostsCmd is the command to get physical hosts.
type GetPhysicalHostsCmd struct {
	subcommands.CommandRunBase
	req crimson.ListPhysicalHostsRequest
	f   FormattingFlags
}

// Run runs the command to get physical hosts.
func (c *GetPhysicalHostsCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	client := getClient(ctx)
	resp, err := client.ListPhysicalHosts(ctx, &c.req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	printPhysicalHosts(c.f.showHeaders, resp.Hosts...)
	return 0
}

// getPhysicalHostsCmd returns a command to get physical hosts.
func getPhysicalHostsCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-hosts [-name <name>]... [-vlan <id>]... [-ipv4 <ip address>]...",
		ShortDesc: "retrieves physical hosts",
		LongDesc:  "Retrieves physical hosts matching the given names and VLANs, or all physical hosts if names and VLANs are omitted.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetPhysicalHostsCmd{}
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Names), "name", "Name of a physical host to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.Int64Slice(&cmd.req.Vlans), "vlan", "ID of a VLAN to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Ipv4S), "ipv4", "IPv4 address to filter by. Can be specified multiple times.")
			// TODO(smut): Add the other filters.
			cmd.f.Register(cmd)
			return cmd
		},
	}
}
