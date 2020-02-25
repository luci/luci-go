// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"net/http"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/common/flag/stringmapflag"
	job "go.chromium.org/luci/led/job"
)

func editSystemCmd(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "edit-system [options]",
		ShortDesc: "edits the systemland of a JobDescription",
		LongDesc:  `Allows manipulations of the 'system' data in a JobDescription.`,

		CommandRun: func() subcommands.CommandRun {
			ret := &cmdEditSystem{}
			ret.initFlags(authOpts)
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

func (c *cmdEditSystem) initFlags(authOpts auth.Options) {
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

	c.Flags.Int64Var(&c.priority, "p", -1, "set the swarming task priority (0-255), lower is faster to scheduler.")

	c.cmdBase.initFlags(authOpts)
}

func (c *cmdEditSystem) positionalRange() (min, max int) { return 0, 0 }
func (c *cmdEditSystem) jobInput() bool                  { return true }

func (c *cmdEditSystem) validateFlags(ctx context.Context, _ []string, _ subcommands.Env) (err error) {
	return
}

func (c *cmdEditSystem) execute(ctx context.Context, _ *http.Client, inJob *job.Definition) (out interface{}, err error) {
	return inJob, inJob.Edit(func(je job.Editor) {
		je.Env(c.environment)
		je.CIPDPkgs(job.CIPDPkgs(c.cipdPackages))
		je.PrefixPathEnv(c.prefixPathEnv)
		je.Priority(int32(c.priority))
		je.Tags(c.tags)
	})
}

func (c *cmdEditSystem) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return c.doContextExecute(a, c, args, env)
}
