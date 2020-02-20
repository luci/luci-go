// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"net/http"
	"strconv"
	"strings"

	"golang.org/x/net/context"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/led/job"
	"go.chromium.org/luci/led/ledcmd"
)

func getBuildCmd(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-build <buildbucket_build_id>",
		ShortDesc: "obtain a JobDefinition from a buildbucket build",
		LongDesc: `Obtains the build's definition from buildbucket and produces a JobDefinition.

buildbucket_build_id can be specified with "b" prefix like b8962624445013664976,
which is useful when copying it from ci.chromium.org URL.`,

		CommandRun: func() subcommands.CommandRun {
			ret := &cmdGetBuild{}
			ret.initFlags(authOpts)
			return ret
		},
	}
}

type cmdGetBuild struct {
	cmdBase

	bbHost     string
	pinMachine bool

	buildID int64
}

func (c *cmdGetBuild) initFlags(authOpts auth.Options) {
	c.Flags.StringVar(&c.bbHost, "B", "cr-buildbucket.appspot.com",
		"The buildbucket hostname to grab the definition from.")

	c.Flags.BoolVar(&c.pinMachine, "pin-machine", false,
		"Pin the dimensions of the JobDefinition to run on the same machine.")

	c.cmdBase.initFlags(authOpts)
}

func (c *cmdGetBuild) jobInput() bool                  { return false }
func (c *cmdGetBuild) positionalRange() (min, max int) { return 1, 1 }

func (c *cmdGetBuild) validateFlags(ctx context.Context, positionals []string, env subcommands.Env) (err error) {
	if err := validateHost(c.bbHost); err != nil {
		return errors.Annotate(err, "buildbucket host").Err()
	}

	buildIDStr := positionals[0]
	if strings.HasPrefix(buildIDStr, "b") {
		// Milo URL structure prefixes buildbucket builds id with "b".
		buildIDStr = positionals[0][1:]
	}
	c.buildID, err = strconv.ParseInt(buildIDStr, 10, 64)
	return errors.Annotate(err, "bad <buildbucket_build_id>").Err()
}

func (c *cmdGetBuild) execute(ctx context.Context, authClient *http.Client, inJob *job.Definition) (out interface{}, err error) {
	return ledcmd.GetBuild(ctx, authClient, ledcmd.GetBuildOpts{
		BuildbucketHost: c.bbHost,
		BuildID:         c.buildID,
		PinBot:          c.pinMachine,
	})
}

func (c *cmdGetBuild) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return c.doContextExecute(a, c, args, env)
}
