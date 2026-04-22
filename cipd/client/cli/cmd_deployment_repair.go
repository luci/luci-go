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

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"

	"go.chromium.org/luci/cipd/client/cipd"
)

////////////////////////////////////////////////////////////////////////////////
// 'deployment-repair' subcommand.

func cmdRepairDeployment(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "deployment-repair [options]",
		ShortDesc: "attempts to repair a deployment if it is broken",
		LongDesc:  "This is equivalent of running 'ensure' in paranoia.",
		CommandRun: func() subcommands.CommandRun {
			c := &repairDeploymentRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withRootDir, withMaxThreads)
			return c
		},
	}
}

type repairDeploymentRun struct {
	cipdSubcommand
	clientOptions
}

func (c *repairDeploymentRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(repairDeployment(ctx, c.clientOptions))
}

func repairDeployment(ctx context.Context, clientOpts clientOptions) (cipd.ActionMap, error) {
	client, err := clientOpts.makeCIPDClient(ctx, "")
	if err != nil {
		return nil, err
	}
	defer client.Close(ctx)

	currentDeployment, err := client.FindDeployed(ctx)
	if err != nil {
		return nil, err
	}

	return client.EnsurePackages(ctx, currentDeployment, &cipd.EnsureOptions{
		Paranoia: cipd.CheckIntegrity,
	})
}
