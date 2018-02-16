// Copyright 2017 The LUCI Authors.
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

// GetSwitchesCmd is the command to get switches.
type GetSwitchesCmd struct {
	subcommands.CommandRunBase
	req     crimson.ListSwitchesRequest
	headers bool
}

// Run runs the command to get switches.
func (c *GetSwitchesCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	client := getClient(ctx)
	resp, err := client.ListSwitches(ctx, &c.req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	if len(resp.Switches) > 0 {
		p := newStdoutPrinter()
		defer p.Flush()
		if c.headers {
			p.Row("Name", "Rack", "Datacenter", "Description")
		}
		for _, s := range resp.Switches {
			p.Row(s.Name, s.Rack, s.Datacenter, s.Description)
		}
	}
	return 0
}

// getSwitchesCmd returns a command to get switches.
func getSwitchesCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-switches [-name <name>]... [-rack <rack>]... [-dc <datacenter>]...",
		ShortDesc: "retrieves switches",
		LongDesc:  "Retrieves switches matching the given names, racks and dcs, or all switches if names, racks, and dcs are omitted.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetSwitchesCmd{}
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Names), "name", "Name of a switch to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Racks), "rack", "Name of a rack to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Datacenters), "dc", "Name of a datacenter to filter by. Can be specified multiple times.")
			cmd.Flags.BoolVar(&cmd.headers, "headers", false, "Show column headers.")
			return cmd
		},
	}
}
