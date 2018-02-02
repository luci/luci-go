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

// AddMachineCmd is the command to add a machine.
type AddMachineCmd struct {
	subcommands.CommandRunBase
	machine crimson.Machine
}

// Run runs the command to add a machine.
func (c *AddMachineCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	// TODO(smut): Validate required fields client-side.
	req := &crimson.CreateMachineRequest{
		Machine: &c.machine,
	}
	client := getClient(ctx)
	resp, err := client.CreateMachine(ctx, req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	// TODO(smut): Format this response.
	fmt.Print(proto.MarshalTextString(resp))
	return 0
}

// addMachineCmd returns a command to add a machine.
func addMachineCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "add-machine -name <name> -plat <platform> -rack <rack> -state <state> [-desc <description>] [-atag <asset tag>] [-stag <service tag>] [-tick <deployment ticket>]",
		ShortDesc: "adds a machine",
		LongDesc:  "Adds a machine to the database.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &AddMachineCmd{}
			cmd.Flags.StringVar(&cmd.machine.Name, "name", "", "The name of the machine. Required and must be unique within the database.")
			cmd.Flags.StringVar(&cmd.machine.Platform, "plat", "", "The platform type this machine is. Required and must be the name of a platform returned by get-platforms.")
			cmd.Flags.StringVar(&cmd.machine.Rack, "rack", "", "The rack this machine belongs to. Required and must be the name of a rack returned by get-racks.")
			cmd.Flags.Var(StateFlag(&cmd.machine.State), "state", "The state of this machine. Required and must be the name of a state returned by get-states.")
			cmd.Flags.StringVar(&cmd.machine.Description, "desc", "", "A description of this machine.")
			cmd.Flags.StringVar(&cmd.machine.AssetTag, "atag", "", "The asset tag associated with this machine.")
			cmd.Flags.StringVar(&cmd.machine.ServiceTag, "stag", "", "The service tag associated with this machine.")
			cmd.Flags.StringVar(&cmd.machine.DeploymentTicket, "tick", "", "The deployment ticket associated with this machine.")
			return cmd
		},
	}
}

// DeleteMachineCmd is the command to delete a machine.
type DeleteMachineCmd struct {
	subcommands.CommandRunBase
	req crimson.DeleteMachineRequest
}

// Run runs the command to delete a machine.
func (c *DeleteMachineCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	// TODO(smut): Validate required fields client-side.
	client := getClient(ctx)
	resp, err := client.DeleteMachine(ctx, &c.req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	// TODO(smut): Format this response.
	fmt.Print(proto.MarshalTextString(resp))
	return 0
}

// deleteMachineCmd returns a command to delete a machine.
func deleteMachineCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "del-machine -name <name>",
		ShortDesc: "deletes a machine",
		LongDesc:  "Deletes a machine from the database.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &DeleteMachineCmd{}
			cmd.Flags.StringVar(&cmd.req.Name, "name", "", "The name of the machine to delete.")
			return cmd
		},
	}
}

// GetMachinesCmd is the command to get machines.
type GetMachinesCmd struct {
	subcommands.CommandRunBase
	req crimson.ListMachinesRequest
}

// Run runs the command to get machines.
func (c *GetMachinesCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	client := getClient(ctx)
	resp, err := client.ListMachines(ctx, &c.req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	// TODO(smut): Format this response.
	fmt.Print(proto.MarshalTextString(resp))
	return 0
}

// getMachinesCmd returns a command to get machines.
func getMachinesCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-machines [-name <name>]...",
		ShortDesc: "retrieves machines",
		LongDesc:  "Retrieves machines matching the given names, or all machines if names are omitted.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetMachinesCmd{}
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Names), "name", "Name of a machine to filter by. Can be specified multiple times.")
			// TODO(smut): Add the other filters.
			return cmd
		},
	}
}
