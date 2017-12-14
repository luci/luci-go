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
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
)

// GetDatacentersClient is the command to get datacenters.
type GetDatacentersClient struct {
	subcommands.CommandRunBase
	names MultiValue
}

// Run runs the command to get datacenters.
func (c *GetDatacentersClient) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	req := &crimson.DatacentersRequest{
		Names: c.names.values,
	}
	client := getClient(ctx)
	resp, err := client.GetDatacenters(ctx, req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	// TODO(smut): Format this response.
	logging.Infof(ctx, "%s", resp)
	return 0
}

// getDatacentersCmd returns a command to get datacenters.
func getDatacentersCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-dcs [-name <name>]...",
		ShortDesc: "retrieves datacenters",
		LongDesc:  "Retrieves datacenters matching the given names, or all datacenters if names are omitted.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetDatacentersClient{}
			cmd.Flags.Var(&cmd.names, "name", "Datacenter to retrieve.")
			return cmd
		},
	}
}
