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

// printDatacenters prints datacenter data to stdout in tab-separated columns.
func printDatacenters(tsv bool, datacenters ...*crimson.Datacenter) {
	if len(datacenters) > 0 {
		p := newStdoutPrinter(tsv)
		defer p.Flush()
		if !tsv {
			p.Row("Name", "Description")
		}
		for _, dc := range datacenters {
			p.Row(dc.Name, dc.Description)
		}
	}
}

// GetDatacentersCmd is the command to get datacenters.
type GetDatacentersCmd struct {
	commandBase
	req crimson.ListDatacentersRequest
}

// Run runs the command to get datacenters.
func (c *GetDatacentersCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	client := getClient(ctx)
	resp, err := client.ListDatacenters(ctx, &c.req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	printDatacenters(c.f.tsv, resp.Datacenters...)
	return 0
}

// getDatacentersCmd returns a command to get datacenters.
func getDatacentersCmd(params *Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-dcs [-name <name>]...",
		ShortDesc: "retrieves datacenters",
		LongDesc:  "Retrieves datacenters matching the given names, or all datacenters if names are omitted.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetDatacentersCmd{}
			cmd.Initialize(params)
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Names), "name", "Name of a datacenter to filter by. Can be specified multiple times.")
			return cmd
		},
	}
}
