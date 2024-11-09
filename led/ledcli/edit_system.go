// Copyright 2020 The LUCI Authors.
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

package ledcli

import (
	"context"
	"net/http"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/common/flag/stringmapflag"

	"go.chromium.org/luci/led/job"
)

func editSystemCmd(opts cmdBaseOptions) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "edit-system [options]",
		ShortDesc: "edits the systemland of a JobDescription",
		LongDesc:  `Allows manipulations of the 'system' data in a JobDescription.`,

		CommandRun: func() subcommands.CommandRun {
			ret := &cmdEditSystem{}
			ret.initFlags(opts)
			return ret
		},
	}
}

type cmdEditSystem struct {
	cmdBase

	environment   stringmapflag.Value
	cipdPackages  stringmapflag.Value
	prefixPathEnv stringlistflag.Flag
	tags          stringlistflag.Flag
	priority      int64
}

func (c *cmdEditSystem) initFlags(opts cmdBaseOptions) {
	c.Flags.Var(&c.environment, "e",
		"(repeatable) override an environment variable. This takes a parameter of `env_var=value`. "+
			"Providing an empty value will remove that envvar.")

	c.Flags.Var(&c.cipdPackages, "cp",
		"(repeatable) override a cipd package. This takes a parameter of `[subdir:]pkgname=version`. "+
			"Using an empty version will remove the package. The subdir is optional and defaults to '.'.")

	c.Flags.Var(&c.prefixPathEnv, "ppe",
		"(repeatable) override a PATH prefix entry. Using a value like '!value' will remove a path entry.")

	c.Flags.Var(&c.tags, "tag",
		"(repeatable) append a new tag.")

	c.Flags.Int64Var(&c.priority, "p", -1, "set the swarming task priority (0-255), lower is faster to schedule.")

	c.cmdBase.initFlags(opts)
}

func (c *cmdEditSystem) positionalRange() (min, max int) { return 0, 0 }
func (c *cmdEditSystem) jobInput() bool                  { return true }

func (c *cmdEditSystem) validateFlags(ctx context.Context, _ []string, _ subcommands.Env) (err error) {
	return
}

func (c *cmdEditSystem) execute(ctx context.Context, _ *http.Client, _ auth.Options, inJob *job.Definition) (out any, err error) {
	return inJob, inJob.Edit(func(je job.Editor) {
		je.Env(c.environment)
		je.CIPDPkgs(job.CIPDPkgs(c.cipdPackages))
		je.PrefixPathEnv(c.prefixPathEnv)
		if c.priority >= 0 {
			je.Priority(int32(c.priority))
		}
		je.Tags(c.tags)
	})
}

func (c *cmdEditSystem) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return c.doContextExecute(a, c, args, env)
}
