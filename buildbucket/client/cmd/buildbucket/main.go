// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package main

import (
	"os"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/auth"
	"github.com/luci/luci-go/common/cli"
	"github.com/luci/luci-go/common/data/rand/mathrand"
	"github.com/luci/luci-go/common/logging/gologger"

	"github.com/luci/luci-go/hardcoded/chromeinfra"
)

var logCfg = gologger.LoggerConfig{
	Format: `%{message}`,
	Out:    os.Stderr,
}

func GetApplication(defaultAuthOpts auth.Options) *cli.Application {
	return &cli.Application{
		Name:  "buildbucket",
		Title: "A CLI client for buildbucket.",
		Context: func(ctx context.Context) context.Context {
			return logCfg.Use(ctx)
		},
		Commands: []*subcommands.Command{
			cmdPutBatch(defaultAuthOpts),
			cmdGet(defaultAuthOpts),
			cmdCancel(defaultAuthOpts),
			cmdConvertBuilders,
			cmdInconsistency(defaultAuthOpts),
			subcommands.CmdHelp,
		},
	}
}

func main() {
	mathrand.SeedRandomly()
	app := GetApplication(chromeinfra.DefaultAuthOptions())
	os.Exit(subcommands.Run(app, os.Args[1:]))
}
