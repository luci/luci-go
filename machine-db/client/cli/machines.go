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

// printMachines prints machine data to stdout in tab-separated columns.
func printMachines(showHeaders bool, machines ...*crimson.Machine) {
	if len(machines) > 0 {
		p := newStdoutPrinter()
		defer p.Flush()
		if showHeaders {
			p.Row("Name", "Platform", "Rack", "Datacenter", "Description", "Asset Tag", "Service Tag", "Deployment Ticket", "State")
		}
		for _, m := range machines {
			p.Row(m.Name, m.Platform, m.Rack, m.Datacenter, m.Description, m.AssetTag, m.ServiceTag, m.DeploymentTicket, m.State)
		}
	}
}

// AddMachineCmd is the command to add a machine.
type AddMachineCmd struct {
	subcommands.CommandRunBase
	machine crimson.Machine
	f       FormattingFlags
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
	printMachines(c.f.showHeaders, resp)
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
			cmd.Flags.Var(StateFlag(&cmd.machine.State), "state", "The state of this machine. Required and must be a state returned by get-states.")
			cmd.Flags.StringVar(&cmd.machine.Description, "desc", "", "A description of this machine.")
			cmd.Flags.StringVar(&cmd.machine.AssetTag, "atag", "", "The asset tag associated with this machine.")
			cmd.Flags.StringVar(&cmd.machine.ServiceTag, "stag", "", "The service tag associated with this machine.")
			cmd.Flags.StringVar(&cmd.machine.DeploymentTicket, "tick", "", "The deployment ticket associated with this machine.")
			cmd.f.Register(cmd)
			return cmd
		},
	}
}

// DeleteMachineCmd is the command to delete a machine.
type DeleteMachineCmd struct {
	subcommands.CommandRunBase
	req crimson.DeleteMachineRequest
	f   FormattingFlags
}

// Run runs the command to delete a machine.
func (c *DeleteMachineCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	// TODO(smut): Validate required fields client-side.
	client := getClient(ctx)
	_, err := client.DeleteMachine(ctx, &c.req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
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

// EditMachineCmd is the command to edit a machine.
type EditMachineCmd struct {
	subcommands.CommandRunBase
	machine crimson.Machine
	f       FormattingFlags
}

// Run runs the command to edit a machine.
func (c *EditMachineCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	// TODO(smut): Validate required fields client-side.
	req := &crimson.UpdateMachineRequest{
		Machine: &c.machine,
		UpdateMask: getUpdateMask(&c.Flags, map[string]string{
			"plat":  "platform",
			"rack":  "rack",
			"state": "state",
			"desc":  "description",
			"atag":  "asset_tag",
			"stag":  "service_tag",
			"tick":  "deployment_ticket",
		}),
	}
	client := getClient(ctx)
	resp, err := client.UpdateMachine(ctx, req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	printMachines(c.f.showHeaders, resp)
	return 0
}

// editMachineCmd returns a command to edit a machine.
func editMachineCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "edit-machine -name <name> [-plat <platform>] [-rack <rack>] [-state <state>] [-desc <description>] [-atag <asset tag>] [-stag <service tag>] [-tick <deployment ticket>]",
		ShortDesc: "edits a machine",
		LongDesc:  "Edits a machine in the database.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &EditMachineCmd{}
			cmd.Flags.StringVar(&cmd.machine.Name, "name", "", "The name of the machine. Required and must be the name of a machine returned by get-machines.")
			cmd.Flags.StringVar(&cmd.machine.Platform, "plat", "", "The platform type this machine is. Must be the name of a platform returned by get-platforms.")
			cmd.Flags.StringVar(&cmd.machine.Rack, "rack", "", "The rack this machine belongs to. Must be the name of a rack returned by get-racks.")
			cmd.Flags.Var(StateFlag(&cmd.machine.State), "state", "The state of this machine. Must be a state returned by get-states.")
			cmd.Flags.StringVar(&cmd.machine.Description, "desc", "", "A description of this machine.")
			cmd.Flags.StringVar(&cmd.machine.AssetTag, "atag", "", "The asset tag associated with this machine.")
			cmd.Flags.StringVar(&cmd.machine.ServiceTag, "stag", "", "The service tag associated with this machine.")
			cmd.Flags.StringVar(&cmd.machine.DeploymentTicket, "tick", "", "The deployment ticket associated with this machine.")
			cmd.f.Register(cmd)
			return cmd
		},
	}
}

// GetMachinesCmd is the command to get machines.
type GetMachinesCmd struct {
	subcommands.CommandRunBase
	req crimson.ListMachinesRequest
	f   FormattingFlags
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
	printMachines(c.f.showHeaders, resp.Machines...)
	return 0
}

// getMachinesCmd returns a command to get machines.
func getMachinesCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-machines [-name <name>]... [-plat <plat>]... [-rack <rack>]... [-dc <dc>]... [-state <state>]...",
		ShortDesc: "retrieves machines",
		LongDesc:  "Retrieves machines matching the given names, platforms, racks, and states, or all machines if names are omitted.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetMachinesCmd{}
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Names), "name", "Name of a machine to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Platforms), "plat", "Name of a platform to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Racks), "rack", "Name of a rack to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Datacenters), "dc", "Name of a datacenter to filter by. Can be specified multiple times.")
			cmd.Flags.Var(StateSliceFlag(&cmd.req.States), "state", "State to filter by. Can be specified multiple times.")
			cmd.f.Register(cmd)
			return cmd
		},
	}
}
