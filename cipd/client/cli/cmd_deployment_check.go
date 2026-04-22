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
	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/common/cipderr"
)

////////////////////////////////////////////////////////////////////////////////
// 'deployment-check' subcommand.

func cmdCheckDeployment(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "deployment-check [options]",
		ShortDesc: "verifies all files that are supposed to be installed are present",
		LongDesc: "Compares CIPD package manifests stored in .cipd/* with what's on disk.\n\n" +
			"Useful when debugging issues with broken installations.",
		CommandRun: func() subcommands.CommandRun {
			c := &checkDeploymentRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withRootDir, withoutMaxThreads)
			return c
		},
	}
}

type checkDeploymentRun struct {
	cipdSubcommand
	clientOptions
}

func (c *checkDeploymentRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(checkDeployment(ctx, c.clientOptions))
}

func checkDeployment(ctx context.Context, clientOpts clientOptions) (cipd.ActionMap, error) {
	client, err := clientOpts.makeCIPDClient(ctx, "")
	if err != nil {
		return nil, err
	}
	defer client.Close(ctx)

	currentDeployment, err := client.FindDeployed(ctx)
	if err != nil {
		return nil, err
	}

	actions, err := client.EnsurePackages(ctx, currentDeployment, &cipd.EnsureOptions{
		Paranoia: cipd.CheckIntegrity,
		DryRun:   true,
		Silent:   true,
	})
	if err != nil {
		return nil, err
	}
	actions.Log(ctx, true)
	if len(actions) != 0 {
		err = cipderr.Stale.Apply(errors.New("the deployment needs a repair"))
	}
	return actions, err
}
