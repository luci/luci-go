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

// printVMSlots prints available VM slot data to stdout in tab-separated columns.
func printVMSlots(tsv bool, hosts ...*crimson.PhysicalHost) {
	if len(hosts) > 0 {
		p := newStdoutPrinter(tsv)
		defer p.Flush()
		if !tsv {
			p.Row("Name", "VLAN", "VM Slots")
		}
		for _, h := range hosts {
			p.Row(h.Name, h.Vlan, h.VmSlots)
		}
	}
}

// GetVMSlotsCmd is the command to get available VM slots.
type GetVMSlotsCmd struct {
	commandBase
	req crimson.FindVMSlotsRequest
}

// Run runs the command to get available VM slots.
func (c *GetVMSlotsCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	client := getClient(ctx)
	resp, err := client.FindVMSlots(ctx, &c.req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	printVMSlots(c.f.tsv, resp.Hosts...)
	return 0
}

// getVMSlotsCmd returns a command to get available VM slots.
func getVMSlotsCmd(params *Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-slots -n <slots> [-man <manufacturer>]... [-vdc <virtual datacenter>]",
		ShortDesc: "retrieves available VM slots",
		LongDesc:  "Retrieves available VM slots.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetVMSlotsCmd{}
			cmd.Initialize(params)
			cmd.Flags.Var(flag.Int32(&cmd.req.Slots), "n", "The number of available VM slots to get.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Manufacturers), "man", "Manufacturer to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.VirtualDatacenters), "vdc", "Virtual datacenter to filter by. Can be specified multiple times.")
			return cmd
		},
	}
}
