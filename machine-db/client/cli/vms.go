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
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
)

// AddVMCmd is the command to add a VM.
type AddVMCmd struct {
	subcommands.CommandRunBase
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
	// TODO(smut): Format this response.
	fmt.Print(proto.MarshalTextString(resp))
	return 0
}

// addVMCmd returns a command to add a VM.
func addVMCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "add-vm -name <name> -vlan <id> -host <name> -hvlan <id> -os os [-desc <description>] [-tick <deployment ticket>]",
		ShortDesc: "adds a VM",
		LongDesc:  "Adds a VM to the database.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &AddVMCmd{}
			cmd.Flags.StringVar(&cmd.vm.Name, "name", "", "The name of this VM on the network. Required and must be unique per VLAN within the database.")
			cmd.Flags.Int64Var(&cmd.vm.Vlan, "vlan", 0, "The VLAN this VM belongs to. Required and must be the ID of a VLAN returned by get-vlans.")
			cmd.Flags.StringVar(&cmd.vm.Host, "host", "", "The physical host backing this host. Required and must be the name of a physical host returned by get-hosts.")
			cmd.Flags.Int64Var(&cmd.vm.HostVlan, "hvlan", 0, "The VLAN the physical host belongs to. Required and must be the ID of a VLAN returned by get-vlans.")
			cmd.Flags.StringVar(&cmd.vm.Os, "os", "", "The operating system this host is running. Required and must be the name of an operating system returned by get-oses.")
			cmd.Flags.StringVar(&cmd.vm.Description, "desc", "", "A description of this host.")
			cmd.Flags.StringVar(&cmd.vm.DeploymentTicket, "tick", "", "The deployment ticket associated with this host.")
			return cmd
		},
	}
}

// GetVMsCmd is the command to get VMs.
type GetVMsCmd struct {
	subcommands.CommandRunBase
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
	// TODO(smut): Format this response.
	fmt.Print(proto.MarshalTextString(resp))
	return 0
}

// getVMsCmd returns a command to get VMs.
func getVMsCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-vms [-name <name>]... [-vlan <id>]...",
		ShortDesc: "retrieves VMs",
		LongDesc:  "Retrieves VMs matching the given names and VLANs, or all VMs if names and VLANs are omitted.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetVMsCmd{}
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Names), "name", "Name of a VM to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.Int64Slice(&cmd.req.Vlans), "vlan", "ID of a VLAN to filter by. Can be specified multiple times.")
			// TODO(smut): Add the other filters.
			return cmd
		},
	}
}
