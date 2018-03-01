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

// printIPs prints IP address data to stdout in tab-separated columns.
func printIPs(noHeaders bool, ips ...*crimson.IP) {
	if len(ips) > 0 {
		p := newStdoutPrinter()
		defer p.Flush()
		if !noHeaders {
			p.Row("IPv4", "VLAN", "Hostname")
		}
		for _, ip := range ips {
			p.Row(ip.Ipv4, ip.Vlan, ip.Hostname)
		}
	}
}

// GetIPsCmd is the command to get free IP addresses.
type GetIPsCmd struct {
	subcommands.CommandRunBase
	req crimson.ListFreeIPsRequest
	f   FormattingFlags
}

// Run runs the command to get free IP addresses.
func (c *GetIPsCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	client := getClient(ctx)
	resp, err := client.ListFreeIPs(ctx, &c.req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	printIPs(c.f.noHeaders, resp.Ips...)
	return 0
}

// getIPsCmd returns a command to get free IP addresses.
func getIPsCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-ips -vlan <id> [-n <limit>]",
		ShortDesc: "retrieves free IPs",
		LongDesc:  "Retrieves free IP addresses on the given VLAN.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetIPsCmd{}
			cmd.Flags.Int64Var(&cmd.req.Vlan, "vlan", 0, "VLAN to get free IP addresses on.")
			cmd.Flags.Var(flag.Int32(&cmd.req.PageSize), "n", "The number of free IP addresses to get.")
			cmd.f.Register(cmd)
			return cmd
		},
	}
}
