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

	"go.chromium.org/luci/cipd/common"
)

////////////////////////////////////////////////////////////////////////////////
// 'search' subcommand.

func cmdSearch(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "search <package> -tag key:value [options]",
		ShortDesc: "searches for package instances by tag",
		LongDesc:  "Searches for instances of some package with all given tags.",
		CommandRun: func() subcommands.CommandRun {
			c := &searchRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.tagsOptions.registerFlags(&c.Flags)
			return c
		},
	}
}

type searchRun struct {
	cipdSubcommand
	clientOptions
	tagsOptions
}

func (c *searchRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	if len(c.tags) == 0 {
		return c.done(nil, makeCLIError("at least one -tag must be provided"))
	}
	packageName, err := expandTemplate(args[0])
	if err != nil {
		return c.done(nil, err)
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(searchInstances(ctx, packageName, c.tags, c.clientOptions))
}

func searchInstances(ctx context.Context, packageName string, tags []string, clientOpts clientOptions) ([]common.Pin, error) {
	client, err := clientOpts.makeCIPDClient(ctx)
	if err != nil {
		return nil, err
	}
	defer client.Close(ctx)

	pins, err := client.SearchInstances(ctx, packageName, tags)
	if err != nil {
		return nil, err
	}
	if len(pins) == 0 {
		fmt.Println("No matching instances.")
	} else {
		fmt.Println("Instances:")
		for _, pin := range pins {
			fmt.Printf("  %s\n", pin)
		}
	}
	return pins, nil
}
