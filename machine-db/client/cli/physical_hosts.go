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
func printPhysicalHosts(tsv bool, hosts ...*crimson.PhysicalHost) {
	if len(hosts) > 0 {
		p := newStdoutPrinter(tsv)
		defer p.Flush()
		if !tsv {
			p.Row("Name", "VLAN", "IP Address", "Machine", "NIC", "OS", "Max VM Slots", "Virtual Datacenter", "Description", "Deployment Ticket", "State")
		}
		for _, h := range hosts {
			p.Row(h.Name, h.Vlan, h.Ipv4, h.Machine, h.Nic, h.Os, h.VmSlots, h.VirtualDatacenter, h.Description, h.DeploymentTicket, h.State)
		}
	}
}

// AddPhysicalHostCmd is the command to add a physical host.
type AddPhysicalHostCmd struct {
	commandBase
	host crimson.PhysicalHost
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
	printPhysicalHosts(c.f.tsv, resp)
	return 0
}

// addPhysicalHostCmd returns a command to add a physical host.
func addPhysicalHostCmd(params *Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "add-host -name <name> -machine <machine> -nic <nic> -os <os> -ip <ip address> [-state <state>] [-slots <vm slots>] [-vdc <virtual datacenter>] [-desc <description>] [-tick <deployment ticket>]",
		ShortDesc: "adds a physical host",
		LongDesc:  "Adds a physical host to the database.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &AddPhysicalHostCmd{}
			cmd.Initialize(params)
			cmd.Flags.StringVar(&cmd.host.Name, "name", "", "The name of this host on the network. Required and must be unique within the database.")
			cmd.Flags.StringVar(&cmd.host.Machine, "machine", "", "The machine backing this host. Required and must be the name of a machine returned by get-machines.")
			cmd.Flags.StringVar(&cmd.host.Nic, "nic", "", "The primary NIC for this host. Required and must be the name of a NIC returned by get-nics.")
			cmd.Flags.StringVar(&cmd.host.Os, "os", "", "The operating system this host is running. Required and must be the name of an operating system returned by get-oses.")
			cmd.Flags.StringVar(&cmd.host.Ipv4, "ip", "", "The IPv4 address assigned to this host. Required and must be a free IP address returned by get-ips.")
			cmd.Flags.Var(StateFlag(&cmd.host.State), "state", "The state of this host. Must be the name of a state returned by get-states.")
			cmd.Flags.Var(flag.Int32(&cmd.host.VmSlots), "slots", "The number of VMs which can be deployed on this host.")
			cmd.Flags.StringVar(&cmd.host.VirtualDatacenter, "vdc", "", "The virtual datacenter VMs deployed on this host will belong to.")
			cmd.Flags.StringVar(&cmd.host.Description, "desc", "", "A description of this host.")
			cmd.Flags.StringVar(&cmd.host.DeploymentTicket, "tick", "", "The deployment ticket associated with this host.")
			return cmd
		},
	}
}

// EditPhysicalHostCmd is the command to edit a physical host.
type EditPhysicalHostCmd struct {
	commandBase
	host crimson.PhysicalHost
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
			"vdc":     "virtual_datacenter",
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
	printPhysicalHosts(c.f.tsv, resp)
	return 0
}

// editPhysicalHostCmd returns a command to edit a physical host.
func editPhysicalHostCmd(params *Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "edit-host -name <name> [-machine <machine>] [-os <os>] [-state <state>] [-slots <vm slots>] [-vdc <virtual datacenter>] [-desc <description>] [-tick <deployment ticket>]",
		ShortDesc: "edits a physical host",
		LongDesc:  "Edits a physical host in the database.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &EditPhysicalHostCmd{}
			cmd.Initialize(params)
			cmd.Flags.StringVar(&cmd.host.Name, "name", "", "The name of this host on the network. Required and must be the name of a host returned by get-hosts.")
			cmd.Flags.StringVar(&cmd.host.Machine, "machine", "", "The machine backing this host. Must be the name of a machine returned by get-machines.")
			cmd.Flags.StringVar(&cmd.host.Os, "os", "", "The operating system this host is running. Must be the name of an operating system returned by get-oses.")
			cmd.Flags.Var(StateFlag(&cmd.host.State), "state", "The state of this host. Must be a state returned by get-states.")
			cmd.Flags.Var(flag.Int32(&cmd.host.VmSlots), "slots", "The number of VMs which can be deployed on this host.")
			cmd.Flags.StringVar(&cmd.host.VirtualDatacenter, "vdc", "", "The virtual datacenter VMs deployed on this host will belong to.")
			cmd.Flags.StringVar(&cmd.host.Description, "desc", "", "A description of this host.")
			cmd.Flags.StringVar(&cmd.host.DeploymentTicket, "tick", "", "The deployment ticket associated with this host.")
			return cmd
		},
	}
}

// GetPhysicalHostsCmd is the command to get physical hosts.
type GetPhysicalHostsCmd struct {
	commandBase
	req crimson.ListPhysicalHostsRequest
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
	printPhysicalHosts(c.f.tsv, resp.Hosts...)
	return 0
}

// getPhysicalHostsCmd returns a command to get physical hosts.
func getPhysicalHostsCmd(params *Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-hosts [-name <name>]... [-vlan <id>]... [-machine <machine>]... [-nic <nic>]... [-os <os>]... [-ip <ip address>]... [-state <state>]... [-vdc <virtual datacenter>]...",
		ShortDesc: "retrieves physical hosts",
		LongDesc:  "Retrieves physical hosts matching the given names and VLANs, or all physical hosts if names and VLANs are omitted.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetPhysicalHostsCmd{}
			cmd.Initialize(params)
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Names), "name", "Name of a physical host to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.Int64Slice(&cmd.req.Vlans), "vlan", "ID of a VLAN to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Machines), "machine", "Name of a machine to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Nics), "nic", "Name of a NIC to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Oses), "os", "Name of an operating system to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Ipv4S), "ip", "IPv4 address to filter by. Can be specified multiple times.")
			cmd.Flags.Var(StateSliceFlag(&cmd.req.States), "state", "State to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Platforms), "plat", "Name of a platform to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Racks), "rack", "Name of a rack to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Datacenters), "dc", "Name of a datacenter to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.VirtualDatacenters), "vdc", "Name of a virtual datacenter to filter by. Can be specified multiple times.")
			return cmd
		},
	}
}
