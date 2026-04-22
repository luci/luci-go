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
	"go.chromium.org/luci/common/retry/transient"
)

////////////////////////////////////////////////////////////////////////////////
// 'puppet-check-updates' subcommand.

func cmdPuppetCheckUpdates(params Parameters) *subcommands.Command {
	return &subcommands.Command{
		Advanced:  true,
		UsageLine: "puppet-check-updates [options]",
		ShortDesc: "returns 0 exit code iff 'ensure' will do some actions",
		LongDesc: "Returns 0 exit code iff 'ensure' will do some actions.\n\n" +
			"Exists to be used from Puppet's Exec 'onlyif' option to trigger " +
			"'ensure' only if something is out of date. If puppet-check-updates " +
			"fails with a transient error, it returns non-zero exit code (as usual), " +
			"so that Puppet doesn't trigger notification chain (that can result in " +
			"service restarts). On fatal errors it returns 0 to let Puppet run " +
			"'ensure' for real and catch an error.",
		CommandRun: func() subcommands.CommandRun {
			c := &checkUpdatesRun{}
			c.registerBaseFlags()
			c.clientOptions.registerFlags(&c.Flags, params, withRootDir, withMaxThreads)
			c.ensureFileOptions.registerFlags(&c.Flags, withoutEnsureOutFlag, withLegacyListFlag)
			return c
		},
	}
}

type checkUpdatesRun struct {
	cipdSubcommand
	clientOptions
	ensureFileOptions
}

func (c *checkUpdatesRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	if !c.checkArgs(args, 0, 0) {
		return 1
	}
	ctx := cli.GetContext(a, c, env)

	ef, err := c.loadEnsureFile(ctx, &c.clientOptions, ignoreVerifyPlatforms, parseVersionsFile)
	if err != nil {
		return 0 // on fatal errors ask puppet to run 'ensure' for real
	}

	_, actions, err := ensurePackages(ctx, ef, "", true, c.clientOptions)
	if err != nil {
		ret := c.done(actions, err)
		if transient.Tag.In(err) {
			return ret // fail as usual
		}
		return 0 // on fatal errors ask puppet to run 'ensure' for real
	}
	c.done(actions, nil)
	if len(actions) == 0 {
		return 5 // some arbitrary non-zero number, unlikely to show up on errors
	}
	return 0
}
