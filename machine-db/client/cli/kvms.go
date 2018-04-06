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

// printKVMs prints KVM data to stdout in tab-separated columns.
func printKVMs(tsv bool, kvms ...*crimson.KVM) {
	if len(kvms) > 0 {
		p := newStdoutPrinter(tsv)
		defer p.Flush()
		if !tsv {
			p.Row("Name", "VLAN", "Platform", "Datacenter", "Rack", "Description", "MAC Address", "IP Address", "State")
		}
		for _, k := range kvms {
			p.Row(k.Name, k.Vlan, k.Platform, k.Datacenter, k.Rack, k.Description, k.MacAddress, k.Ipv4, k.State)
		}
	}
}

// GetKVMsCmd is the command to get KVMs.
type GetKVMsCmd struct {
	commandBase
	req crimson.ListKVMsRequest
}

// Run runs the command to get KVMs.
func (c *GetKVMsCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	client := getClient(ctx)
	resp, err := client.ListKVMs(ctx, &c.req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	printKVMs(c.f.tsv, resp.Kvms...)
	return 0
}

// getKVMsCmd returns a command to get KVMs.
func getKVMsCmd(params *Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-kvms [-name <name>]... [-vlan <id>]... [-plat <platform>]... [-rack <rack>]... [-dc <datacenter>]... [-mac <mac address>]... [-ip <ip address>]... [-state <state>]...",
		ShortDesc: "retrieves KVMs",
		LongDesc:  "Retrieves KVMs matching the given filters, or all KVMs if filters are omitted.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetKVMsCmd{}
			cmd.Initialize(params)
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Names), "name", "Name of a KVM to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.Int64Slice(&cmd.req.Vlans), "vlan", "ID of a VLAN to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Platforms), "plat", "Name of a platform to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Racks), "rack", "Name of a rack to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Datacenters), "dc", "Name of a datacenter to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.MacAddresses), "mac", "MAC address to filter by. Can be specified multiple times.")
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Ipv4S), "ip", "IPv4 address to filter by. Can be specified multiple times.")
			cmd.Flags.Var(StateSliceFlag(&cmd.req.States), "state", "State to filter by. Can be specified multiple times.")
			return cmd
		},
	}
}
