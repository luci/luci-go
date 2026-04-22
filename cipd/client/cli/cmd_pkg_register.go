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
	"github.com/maruel/subcommands"

	"go.chromium.org/luci/common/cli"
)

////////////////////////////////////////////////////////////////////////////////
// 'pkg-register' subcommand.

func cmdRegister(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "pkg-register <package instance file>",
		ShortDesc: "uploads and registers package instance in the package repository",
		LongDesc:  "Uploads and registers package instance in the package repository.",
		CommandRun: func() subcommands.CommandRun {
			c := &registerRun{}
			c.registerBaseFlags()
			c.Opts.refsOptions.registerFlags(&c.Flags)
			c.Opts.tagsOptions.registerFlags(&c.Flags)
			c.Opts.metadataOptions.registerFlags(&c.Flags)
			c.Opts.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Opts.uploadOptions.registerFlags(&c.Flags)
			c.Opts.hashOptions.registerFlags(&c.Flags)
			return c
		},
	}
}

type registerOpts struct {
	refsOptions
	tagsOptions
	metadataOptions
	clientOptions
	uploadOptions
	hashOptions
}

type registerRun struct {
	cipdSubcommand

	Opts registerOpts
}

func (c *registerRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)
	return c.done(registerInstanceFile(ctx, args[0], nil, &c.Opts))
}
