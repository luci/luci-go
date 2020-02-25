// Copyright 2020 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"net/http"
	"strings"

	"golang.org/x/net/context"

	"github.com/maruel/subcommands"

	"go.chromium.org/luci/auth"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/flag/stringlistflag"
	"go.chromium.org/luci/led/job"
	"go.chromium.org/luci/led/ledcmd"
)

func getBuilderCmd(authOpts auth.Options) *subcommands.Command {
	return &subcommands.Command{
		UsageLine: "get-builder bucket_name:builder_name",
		ShortDesc: "obtain a JobDefinition from a buildbucket builder",
		LongDesc:  `Obtains the builder definition from buildbucket and produces a JobDefinition.`,

		CommandRun: func() subcommands.CommandRun {
			ret := &cmdGetBuilder{}
			ret.initFlags(authOpts)
			return ret
		},
	}
}

type cmdGetBuilder struct {
	cmdBase

	tags   stringlistflag.Flag
	bbHost string
	canary bool

	bucket  string
	builder string
}

func (c *cmdGetBuilder) initFlags(authOpts auth.Options) {
	c.Flags.Var(&c.tags, "t",
		"(repeatable) set tags for this build. Buildbucket expects these to be `key:value`.")
	c.Flags.StringVar(&c.bbHost, "B", "cr-buildbucket.appspot.com",
		"The buildbucket hostname to grab the definition from.")
	c.Flags.BoolVar(&c.canary, "canary", false,
		"Get a 'canary' build, rather than a 'prod' build.")

	c.cmdBase.initFlags(authOpts)
}

func (c *cmdGetBuilder) jobInput() bool                  { return false }
func (c *cmdGetBuilder) positionalRange() (min, max int) { return 1, 1 }

func (c *cmdGetBuilder) validateFlags(ctx context.Context, positionals []string, env subcommands.Env) (err error) {
	if err := validateHost(c.bbHost); err != nil {
		return errors.Annotate(err, "buildbucket host").Err()
	}

	toks := strings.SplitN(positionals[0], ":", 2)
	if len(toks) != 2 {
		err = errors.Reason("cannot parse bucket:builder: %q", positionals[0]).Err()
		return
	}

	c.bucket, c.builder = toks[0], toks[1]
	if c.bucket == "" {
		return errors.New("empty bucket")
	}
	if c.builder == "" {
		return errors.New("empty builder")
	}
	return nil
}

func (c *cmdGetBuilder) execute(ctx context.Context, authClient *http.Client, inJob *job.Definition) (out interface{}, err error) {
	return ledcmd.GetBuilder(ctx, authClient, ledcmd.GetBuildersOpts{
		BuildbucketHost: c.bbHost,
		Bucket:          c.bucket,
		Builder:         c.builder,
		Canary:          c.canary,
		ExtraTags:       c.tags,
	})
}

func (c *cmdGetBuilder) Run(a subcommands.Application, args []string, env subcommands.Env) int {
	return c.doContextExecute(a, c, args, env)
}
