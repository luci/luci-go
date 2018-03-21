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

	"go.chromium.org/luci/machine-db/api/crimson/v1"
)

// DeleteHostCmd is the command to delete a host.
type DeleteHostCmd struct {
	commandBase
	req crimson.DeleteHostRequest
}

// Run runs the command to delete a host.
func (c *DeleteHostCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	// TODO(smut): Validate required fields client-side.
	client := getClient(ctx)
	_, err := client.DeleteHost(ctx, &c.req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	return 0
}

// deleteHostCmd returns a command to delete a host.
func deleteHostCmd(params *Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "del-host -name name -vlan vlan",
		ShortDesc: "deletes a host",
		LongDesc:  "Deletes a physical or virtual host from the database.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &DeleteHostCmd{}
			cmd.Initialize(params)
			cmd.Flags.StringVar(&cmd.req.Name, "name", "", "The name of the host to delete.")
			cmd.Flags.Int64Var(&cmd.req.Vlan, "vlan", 0, "The VLAN the host belongs to.")
			return cmd
		},
	}
}
