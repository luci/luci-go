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

// printVMs prints VM data to stdout in tab-separated columns.
func printVMs(tsv bool, vms ...*crimson.VM) {
	if len(vms) > 0 {
		p := newStdoutPrinter(tsv)
		defer p.Flush()
		if !tsv {
			p.Row("Name", "VLAN", "IP Address", "Host", "Host VLAN", "Operating System", "Description", "Deployment Ticket", "State")
		}
		for _, vm := range vms {
			p.Row(vm.Name, vm.Vlan, vm.Ipv4, vm.Host, vm.HostVlan, vm.Os, vm.Description, vm.DeploymentTicket, vm.State)
		}
	}
}

// AddVMCmd is the command to add a VM.
type AddVMCmd struct {
	subcommands.CommandRunBase
	vm crimson.VM
	f  FormattingFlags
}

// Run runs the command to add a VM.
func (c *AddVMCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	// TODO(smut): Validate required fields client-side.
	req := &crimson.CreateVMRequest{
		Vm: &c.vm,
	}
	client := getClient(ctx)
	resp, err := client.CreateVM(ctx, req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	printVMs(c.f.tsv, resp)
	return 0
}

// addVMCmd returns a command to add a VM.
func addVMCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "add-vm -name <name> -host <name> -hvlan <id> -os <os> -ipv4 <ip address> -state <state> [-desc <description>] [-tick <deployment ticket>]",
		ShortDesc: "adds a VM",
		LongDesc:  "Adds a VM to the database.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &AddVMCmd{}
			cmd.Flags.StringVar(&cmd.vm.Name, "name", "", "The name of this VM on the network. Required and must be unique per VLAN within the database.")
			cmd.Flags.StringVar(&cmd.vm.Host, "host", "", "The physical host backing this VM. Required and must be the name of a physical host returned by get-hosts.")
			cmd.Flags.Int64Var(&cmd.vm.HostVlan, "hvlan", 0, "The VLAN the physical host belongs to. Required and must be the ID of a VLAN returned by get-vlans.")
			cmd.Flags.Var(StateFlag(&cmd.vm.State), "state", "The state of this VM. Required and must be the name of a state returned by get-states.")
			cmd.Flags.StringVar(&cmd.vm.Os, "os", "", "The operating system this host is running. Required and must be the name of an operating system returned by get-oses.")
			cmd.Flags.StringVar(&cmd.vm.Ipv4, "ipv4", "", "The IPv4 address assigned to this host. Required and must be a free IP address returned by get-ips.")
			cmd.Flags.StringVar(&cmd.vm.Description, "desc", "", "A description of this host.")
			cmd.Flags.StringVar(&cmd.vm.DeploymentTicket, "tick", "", "The deployment ticket associated with this host.")
			cmd.f.Register(cmd)
			return cmd
		},
	}
}

// EditVMCmd is the command to edit a VM.
type EditVMCmd struct {
	subcommands.CommandRunBase
	vm crimson.VM
	f  FormattingFlags
}

// Run runs the command to edit a physical host.
func (c *EditVMCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	// TODO(smut): Validate required fields client-side.
	req := &crimson.UpdateVMRequest{
		Vm: &c.vm,
		UpdateMask: getUpdateMask(&c.Flags, map[string]string{
			"host":  "host",
			"hvlan": "host_vlan",
			"os":    "os",
			"state": "state",
			"desc":  "description",
			"tick":  "deployment_ticket",
		}),
	}
	client := getClient(ctx)
	resp, err := client.UpdateVM(ctx, req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	printVMs(c.f.tsv, resp)
	return 0
}

// editVMCmd returns a command to edit a VM.
func editVMCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "edit-vm -name <name> -vlan <id> [-host <machine> -hvlan <id>] [-os <os>] [-state <state>] [-desc <description>] [-tick <deployment ticket>]",
		ShortDesc: "edits a VM",
		LongDesc:  "Edits a VM in the database.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &EditVMCmd{}
			cmd.Flags.StringVar(&cmd.vm.Name, "name", "", "The name of this VM on the network. Required and must be the name of a VM returned by get-vms.")
			cmd.Flags.Int64Var(&cmd.vm.Vlan, "vlan", 0, "The VLAN this VM belongs to. Required and must be the ID of a VLAN returned by get-vlans.")
			cmd.Flags.StringVar(&cmd.vm.Host, "host", "", "The physical host backing this VM. Must be the name of a physical host returned by get-hosts.")
			cmd.Flags.Int64Var(&cmd.vm.HostVlan, "hvlan", 0, "The VLAN the physical host belongs to. Must be the ID of a VLAN returned by get-vlans.")
			cmd.Flags.StringVar(&cmd.vm.Os, "os", "", "The operating system this VM is running. Must be the name of an operating system returned by get-oses.")
			cmd.Flags.Var(StateFlag(&cmd.vm.State), "state", "The state of this VM. Must be a state returned by get-states.")
			cmd.Flags.StringVar(&cmd.vm.Description, "desc", "", "A description of this VM.")
			cmd.Flags.StringVar(&cmd.vm.DeploymentTicket, "tick", "", "The deployment ticket associated with this VM.")
			cmd.f.Register(cmd)
			return cmd
		},
	}
}

// GetVMsCmd is the command to get VMs.
type GetVMsCmd struct {
	subcommands.CommandRunBase
	req crimson.ListVMsRequest
	f   FormattingFlags
}

// Run runs the command to get VMs.
func (c *GetVMsCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	client := getClient(ctx)
	resp, err := client.ListVMs(ctx, &c.req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	printVMs(c.f.tsv, resp.Vms...)
	return 0
}

// getVMsCmd returns a command to get VMs.
func getVMsCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-vms [-name <name>]... [-vlan <id>]... [-ipv4 <ip address>]...",
		ShortDesc: "retrieves VMs",
		LongDesc:  "Retrieves VMs matching the given names and VLANs, or all VMs if names and VLANs are omitted.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetVMsCmd{}
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Names), "name", "Name of a VM to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.Int64Slice(&cmd.req.Vlans), "vlan", "ID of a VLAN to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Hosts), "host", "Name of a host to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.Int64Slice(&cmd.req.HostVlans), "hvlan", "ID of a host VLAN to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Oses), "os", "Name of an operating system to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Ipv4S), "ipv4", "IPv4 address to filter by. Can be specified multiple times.")
			cmd.Flags.Var(StateSliceFlag(&cmd.req.States), "state", "State to filter by. Can be specified multiple times.")
			cmd.f.Register(cmd)
			return cmd
		},
	}
}
