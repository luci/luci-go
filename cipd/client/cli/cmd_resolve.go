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

	"go.chromium.org/luci/cipd/common"
)

////////////////////////////////////////////////////////////////////////////////
// 'resolve' subcommand.

func cmdResolve(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "resolve <package or package prefix> [options]",
		ShortDesc: "returns concrete package instance ID given a version",
		LongDesc:  "Returns concrete package instance ID given a version.",
		CommandRun: func() subcommands.CommandRun {
			c := &resolveRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.StringVar(&c.version, "version", "<version>", "Package version to resolve.")
			return c
		},
	}
}

type resolveRun struct {
	cipdSubcommand
	clientOptions

	version string
}

func (c *resolveRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.doneWithPins(resolveVersion(ctx, args[0], c.version, c.clientOptions))
}

func resolveVersion(ctx context.Context, packagePrefix, version string, clientOpts clientOptions) ([]pinInfo, error) {
	packagePrefix, err := expandTemplate(packagePrefix)
	if err != nil {
		return nil, err
	}

	client, err := clientOpts.makeCIPDClient(ctx, "", nil)
	if err != nil {
		return nil, err
	}
	defer client.Close(ctx)

	return performBatchOperation(ctx, batchOperation{
		client:        client,
		packagePrefix: packagePrefix,
		callback: func(pkg string) (common.Pin, error) {
			return client.ResolveVersion(ctx, pkg, version)
		},
	})
}
