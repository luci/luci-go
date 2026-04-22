// Copyright 2026 The LUCI Authors.
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
	"context"
	"fmt"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
)

////////////////////////////////////////////////////////////////////////////////
// 'ls' subcommand.

func cmdListPackages(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "ls [-r] [<prefix string>]",
		ShortDesc: "lists matching packages on the server",
		LongDesc: "Queries the backend for a list of packages in the given path to " +
			"which the user has access, optionally recursively.",
		CommandRun: func() subcommands.CommandRun {
			c := &listPackagesRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.BoolVar(&c.recursive, "r", false, "Whether to list packages in subdirectories.")
			c.Flags.BoolVar(&c.showHidden, "h", false, "Whether also to list hidden packages.")
			return c
		},
	}
}

type listPackagesRun struct {
	cipdSubcommand
	clientOptions

	recursive  bool
	showHidden bool
}

func (c *listPackagesRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 1) {
		return 1
	}
	path, err := "", error(nil)
	if len(args) == 1 {
		path, err = expandTemplate(args[0])
		if err != nil {
			return c.done(nil, err)
		}
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(listPackages(ctx, path, c.recursive, c.showHidden, c.clientOptions))
}

func listPackages(ctx context.Context, path string, recursive, showHidden bool, clientOpts clientOptions) ([]string, error) {
	client, err := clientOpts.makeCIPDClient(ctx, "")
	if err != nil {
		return nil, err
	}
	defer client.Close(ctx)

	packages, err := client.ListPackages(ctx, path, recursive, showHidden)
	if err != nil {
		return nil, err
	}
	if len(packages) == 0 {
		fmt.Println("No matching packages.")
	} else {
		for _, p := range packages {
			fmt.Println(p)
		}
	}
	return packages, nil
}
