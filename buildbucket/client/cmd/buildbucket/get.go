// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"fmt"

	"github.com/maruel/subcommands"

	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/cli"
)

func cmdGet(defaultAuthOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: `get [flags] <build id>`,
		ShortDesc: "get details about a build",
		LongDesc:  "Get details about a build.",
		CommandRun: func() subcommands.CommandRun {
			c := &getRun{}
			c.SetDefaultFlags(defaultAuthOpts)
			return c
		},
	}
}

type getRun struct {
	baseCommandRun
	buildIDArg
}

func (r *getRun) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	ctx := cli.GetContext(a, r, env)

	if err := r.parseArgs(args); err != nil {
		return r.done(ctx, err)
	}

	return r.callAndDone(ctx, "GET", fmt.Sprintf("builds/%d", r.buildID), nil)
}
