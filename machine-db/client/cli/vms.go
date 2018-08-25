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
	commandBase
	vm crimson.VM
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
func addVMCmd(params *Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "add-vm -name <name> -host <name> -os <os> -ip <ip address> -state <state> [-desc <description>] [-tick <deployment ticket>]",
		ShortDesc: "adds a VM",
		LongDesc:  "Adds a VM to the database.\n\nExample:\ncrimson add-vm -name vm100-x1 -host esxhost1 -os Windows -ip 99.99.99.99 -state prerelease -desc 'test VM' -tick crbug/111111",
		CommandRun: func() subcommands.CommandRun {
			cmd := &AddVMCmd{}
			cmd.Initialize(params)
			cmd.Flags.StringVar(&cmd.vm.Name, "name", "", "The name of this VM on the network. Required and must be unique within the database.")
			cmd.Flags.StringVar(&cmd.vm.Host, "host", "", "The physical host backing this VM. Required and must be the name of a physical host returned by get-hosts.")
			cmd.Flags.Var(StateFlag(&cmd.vm.State), "state", "The state of this VM. Required and must be the name of a state returned by get-states.")
			cmd.Flags.StringVar(&cmd.vm.Os, "os", "", "The operating system this host is running. Required and must be the name of an operating system returned by get-oses.")
			cmd.Flags.StringVar(&cmd.vm.Ipv4, "ip", "", "The IPv4 address assigned to this host. Required and must be a free IP address returned by get-ips.")
			cmd.Flags.StringVar(&cmd.vm.Description, "desc", "", "A description of this host.")
			cmd.Flags.StringVar(&cmd.vm.DeploymentTicket, "tick", "", "The deployment ticket associated with this host.")
			return cmd
		},
	}
}

// EditVMCmd is the command to edit a VM.
type EditVMCmd struct {
	commandBase
	vm crimson.VM
}

// Run runs the command to edit a physical host.
func (c *EditVMCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	// TODO(smut): Validate required fields client-side.
	req := &crimson.UpdateVMRequest{
		Vm: &c.vm,
		UpdateMask: getUpdateMask(&c.Flags, map[string]string{
			"host":  "host",
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
func editVMCmd(params *Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "edit-vm -name <name> [-host <machine>] [-os <os>] [-state <state>] [-desc <description>] [-tick <deployment ticket>]",
		ShortDesc: "edits a VM",
		LongDesc:  "Edits a VM in the database.\n\nExample to edit the state of a VM to serving:\ncrimson edit -name vm100-x1 -state serving",
		CommandRun: func() subcommands.CommandRun {
			cmd := &EditVMCmd{}
			cmd.Initialize(params)
			cmd.Flags.StringVar(&cmd.vm.Name, "name", "", "The name of this VM on the network. Required and must be the name of a VM returned by get-vms.")
			cmd.Flags.StringVar(&cmd.vm.Host, "host", "", "The physical host backing this VM. Must be the name of a physical host returned by get-hosts.")
			cmd.Flags.StringVar(&cmd.vm.Os, "os", "", "The operating system this VM is running. Must be the name of an operating system returned by get-oses.")
			cmd.Flags.Var(StateFlag(&cmd.vm.State), "state", "The state of this VM. Must be a state returned by get-states.")
			cmd.Flags.StringVar(&cmd.vm.Description, "desc", "", "A description of this VM.")
			cmd.Flags.StringVar(&cmd.vm.DeploymentTicket, "tick", "", "The deployment ticket associated with this VM.")
			return cmd
		},
	}
}

// GetVMsCmd is the command to get VMs.
type GetVMsCmd struct {
	commandBase
	req crimson.ListVMsRequest
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
func getVMsCmd(params *Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-vms [-name <name>]... [-vlan <id>]... [-ip <ip address>]...",
		ShortDesc: "retrieves VMs",
		LongDesc:  "Retrieves VMs matching the given names and VLANs, or all VMs if names and VLANs are omitted.\n\nExample to get all VMs:\ncrimson get-vms\nExample to get VMs on ESX host esxhost1:\ncrimson get-vms -host esxhost1",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetVMsCmd{}
			cmd.Initialize(params)
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Names), "name", "Name of a VM to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.Int64Slice(&cmd.req.Vlans), "vlan", "ID of a VLAN to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Hosts), "host", "Name of a host to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.Int64Slice(&cmd.req.HostVlans), "hvlan", "ID of a host VLAN to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Oses), "os", "Name of an operating system to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Ipv4S), "ip", "IPv4 address to filter by. Can be specified multiple times.")
			cmd.Flags.Var(StateSliceFlag(&cmd.req.States), "state", "State to filter by. Can be specified multiple times.")
			return cmd
		},
	}
}
