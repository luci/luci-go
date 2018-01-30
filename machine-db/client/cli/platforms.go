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
	"go.chromium.org/luci/common/flag"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
)

// GetPlatformsCmd is the command to get platforms.
type GetPlatformsCmd struct {
	subcommands.CommandRunBase
	req crimson.ListPlatformsRequest
}

// Run runs the command to get platforms.
func (c *GetPlatformsCmd) Run(app subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(app, c, env)
	client := getClient(ctx)
	resp, err := client.ListPlatforms(ctx, &c.req)
	if err != nil {
		errors.Log(ctx, err)
		return 1
	}
	// TODO(smut): Format this response.
	fmt.Print(proto.MarshalTextString(resp))
	return 0
}

// getPlatformsCmd returns a command to get platforms.
func getPlatformsCmd() *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-platforms [-name <name>]...",
		ShortDesc: "retrieves platforms",
		LongDesc:  "Retrieves platforms matching the given names, or all platforms if names are omitted.",
		CommandRun: func() subcommands.CommandRun {
			cmd := &GetPlatformsCmd{}
			cmd.Flags.Var(flag.StringSlice(&cmd.req.Names), "name", "Name of a platform to filter by. Can be specified multiple times.")
			return cmd
		},
	}
}
