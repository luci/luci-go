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

// GetRacksCmd is the command to get racks.
type GetRacksCmd struct {
	subcommands.CommandRunBase
	names       stringlistflag.Flag
	datacenters stringlistflag.Flag
}

// Run runs the command to get racks.
func (c *GetRacksCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	req := &crimson.RacksRequest{
		Names:       c.names,
		Datacenters: c.datacenters,
	}
	client := getClient(ctx)
	resp, err := client.GetRacks(ctx, req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	// TODO(smut): Format this response.
	fmt.Print(proto.MarshalTextString(resp))
	return 0
}

// getRacksCmd returns a command to get racks.
func getRacksCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-racks [-name <name>]... [-dc <datacenter>]...",
		ShortDesc: "retrieves racks",
		LongDesc:  "Retrieves racks matching the given names and dcs, or all racks if names and dcs are omitted.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetRacksCmd{}
			cmd.Flags.Var(&cmd.names, "name", "Name of a rack to retrieve. Can be specified multiple times.")
			cmd.Flags.Var(&cmd.datacenters, "dc", "Name of a datacenter to retrieve. Can be specified multiple times.")
			return cmd
		},
	}
}
