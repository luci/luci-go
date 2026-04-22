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

	"go.chromium.org/luci/cipd/client/cipd"
	"go.chromium.org/luci/cipd/common"
)

////////////////////////////////////////////////////////////////////////////////
// 'set-ref' subcommand.

func cmdSetRef(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "set-ref <package or package prefix> [options]",
		ShortDesc: "moves a ref to point to a given version",
		LongDesc:  "Moves a ref to point to a given version.",
		CommandRun: func() subcommands.CommandRun {
			c := &setRefRun{}
			c.registerBaseFlags()
			c.refsOptions.registerFlags(&c.Flags)
			c.clientOptions.registerFlags(&c.Flags, params, withoutRootDir, withoutMaxThreads)
			c.Flags.StringVar(&c.version, "version", "<version>", "Package version to point the ref to.")
			return c
		},
	}
}

type setRefRun struct {
	cipdSubcommand
	refsOptions
	clientOptions

	version string
}

func (c *setRefRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 1, 1) {
		return 1
	}
	if len(c.refs) == 0 {
		return c.done(nil, makeCLIError("at least one -ref must be provided"))
	}
	pkgPrefix, err := expandTemplate(args[0])
	if err != nil {
		return c.done(nil, err)
	}

	ctx := cli.GetContext(a, c, env)
	return c.doneWithPins(visitPins(ctx, &visitPinsArgs{
		clientOptions: c.clientOptions,
		packagePrefix: pkgPrefix,
		version:       c.version,
		updatePin: func(client cipd.Client, pin common.Pin) error {
			for _, ref := range c.refs {
				if err := client.SetRefWhenReady(ctx, ref, pin); err != nil {
					return err
				}
			}
			return nil
		},
	}))
}
