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
	"fmt"

	"github.com/golang/protobuf/proto"
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/stringlistflag"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
)

// GetSwitchesCmd is the command to get switches.
type GetSwitchesCmd struct {
	subcommands.CommandRunBase
	names       stringlistflag.Flag
	racks       stringlistflag.Flag
	datacenters stringlistflag.Flag
}

// Run runs the command to get switches.
func (c *GetSwitchesCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	req := &crimson.ListSwitchesRequest{
		Names:       c.names,
		Racks:       c.racks,
		Datacenters: c.datacenters,
	}
	client := getClient(ctx)
	resp, err := client.ListSwitches(ctx, req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	// TODO(smut): Format this response.
	fmt.Print(proto.MarshalTextString(resp))
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
			cmd.Flags.Var(&cmd.names, "name", "Name of a switch to filter by. Can be specified multiple times.")
			cmd.Flags.Var(&cmd.racks, "rack", "Name of a rack to filter by. Can be specified multiple times.")
			cmd.Flags.Var(&cmd.datacenters, "dc", "Name of a datacenter to filter by. Can be specified multiple times.")
			return cmd
		},
	}
}
