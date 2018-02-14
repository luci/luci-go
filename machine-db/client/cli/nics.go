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

// AddNICCmd is the command to add a network interface.
type AddNICCmd struct {
	subcommands.CommandRunBase
	nic crimson.NIC
}

// Run runs the command to add a network interface.
func (c *AddNICCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	// TODO(smut): Validate required fields client-side.
	req := &crimson.CreateNICRequest{
		Nic: &c.nic,
	}
	client := getClient(ctx)
	resp, err := client.CreateNIC(ctx, req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	// TODO(smut): Format this response.
	fmt.Print(proto.MarshalTextString(resp))
	return 0
}

// addNICCmd returns a command to add a network interface.
func addNICCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "add-nic -name <name> -machine <machine> -mac <mac address> -switch <switch> [-port <switch port>]",
		ShortDesc: "adds a NIC",
		LongDesc:  "Adds a network interface to the database.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &AddNICCmd{}
			cmd.Flags.StringVar(&cmd.nic.Name, "name", "", "The name of the NIC. Required and must be unique per machine within the database.")
			cmd.Flags.StringVar(&cmd.nic.Machine, "machine", "", "The machine this NIC belongs to. Required and must be the name of a machine returned by get-machines.")
			cmd.Flags.StringVar(&cmd.nic.MacAddress, "mac", "", "The MAC address of this NIC. Required and must be a valid MAC-48 address.")
			cmd.Flags.StringVar(&cmd.nic.Switch, "switch", "", "The switch this NIC is connected to. Required and must be the name of a switch returned by get-switches.")
			cmd.Flags.Var(flag.Int32(&cmd.nic.Switchport), "port", "The switchport this NIC is connected to.")
			return cmd
		},
	}
}

// DeleteNICCmd is the command to delete a network interface.
type DeleteNICCmd struct {
	subcommands.CommandRunBase
	req crimson.DeleteNICRequest
}

// Run runs the command to delete a network interface.
func (c *DeleteNICCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	// TODO(smut): Validate required fields client-side.
	client := getClient(ctx)
	resp, err := client.DeleteNIC(ctx, &c.req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	// TODO(smut): Format this response.
	fmt.Print(proto.MarshalTextString(resp))
	return 0
}

// deleteNICCmd returns a command to delete a network interface.
func deleteNICCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "del-nic -name <name> -machine <machine>",
		ShortDesc: "deletes a NIC",
		LongDesc:  "Deletes a network interface from the database.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &DeleteNICCmd{}
			cmd.Flags.StringVar(&cmd.req.Name, "name", "", "The name of the NIC to delete.")
			cmd.Flags.StringVar(&cmd.req.Machine, "machine", "", "The machine the NIC belongs to.")
			return cmd
		},
	}
}

// GetNICsCmd is the command to get network interfaces.
type GetNICsCmd struct {
	subcommands.CommandRunBase
	req crimson.ListNICsRequest
}

// Run runs the command to get network interfaces.
func (c *GetNICsCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	client := getClient(ctx)
	resp, err := client.ListNICs(ctx, &c.req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	// TODO(smut): Format this response.
	fmt.Print(proto.MarshalTextString(resp))
	return 0
}

// getNICCmd returns a command to get network interfaces.
func getNICsCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-nics [-name <name>]... [-machine <machine>]...",
		ShortDesc: "retrieves NICs",
		LongDesc:  "Retrieves network interfaces matching the given names and machines, or all network interfaces if names and machines are omitted.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetNICsCmd{}
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Names), "name", "Name of a NIC to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Machines), "machine", "Name of a machine to filter by. Can be specified multiple times.")
			// TODO(smut): Add the other filters.
			return cmd
		},
	}
}

// UpdateNICCmd is the command to update a network interface.
type UpdateNICCmd struct {
	subcommands.CommandRunBase
	nic crimson.NIC
}

// Run runs the command to update a network interface.
func (c *UpdateNICCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	// TODO(smut): Validate required fields client-side.
	req := &crimson.UpdateNICRequest{
		Nic: &c.nic,
		UpdateMask: getUpdateMask(&c.Flags, map[string]string{
			"mac":    "mac_address",
			"switch": "switch",
			"port":   "switchport",
		}),
	}
	client := getClient(ctx)
	resp, err := client.UpdateNIC(ctx, req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	// TODO(smut): Format this response.
	fmt.Print(proto.MarshalTextString(resp))
	return 0
}

// updateNICCmd returns a command to update a network interface.
func updateNICCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "update-nic -name <name> -machine <machine> [-mac <mac address>] [-switch <switch>] [-port <switch port>]",
		ShortDesc: "updates a NIC",
		LongDesc:  "Updates a network interface in the database.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &UpdateNICCmd{}
			cmd.Flags.StringVar(&cmd.nic.Name, "name", "", "The name of the NIC. Required and must be the name of a NIC returned by get-nics.")
			cmd.Flags.StringVar(&cmd.nic.Machine, "machine", "", "The machine this NIC belongs to. Required and must be the name of a machine returned by get-machines.")
			cmd.Flags.StringVar(&cmd.nic.MacAddress, "mac", "", "The MAC address of this NIC. Must be a valid MAC-48 address.")
			cmd.Flags.StringVar(&cmd.nic.Switch, "switch", "", "The switch this NIC is connected to. Must be the name of a switch returned by get-switches.")
			cmd.Flags.Var(flag.Int32(&cmd.nic.Switchport), "port", "The switchport this NIC is connected to.")
			return cmd
		},
	}
}
