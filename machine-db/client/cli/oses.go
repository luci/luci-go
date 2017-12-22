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

// GetOSesCmd is the command to get operating systems.
type GetOSesCmd struct {
	subcommands.CommandRunBase
	names stringlistflag.Flag
}

// Run runs the command to get operating systems.
func (c *GetOSesCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	req := &crimson.OSesRequest{
		Names: c.names,
	}
	client := getClient(ctx)
	resp, err := client.GetOSes(ctx, req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	// TODO(smut): Format this response.
	fmt.Print(proto.MarshalTextString(resp))
	return 0
}

// getOSesCmd returns a command to get operating systems.
func getOSesCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-oses [-name <name>]...",
		ShortDesc: "retrieves operating systems",
		LongDesc:  "Retrieves operating systems matching the given names, or all operating systems if names are omitted.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetOSesCmd{}
			cmd.Flags.Var(&cmd.names, "name", "Name of an operating system to filter by. Can be specified multiple times.")
			return cmd
		},
	}
}
