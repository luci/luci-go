// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"os"

	"github.com/maruel/subcommands"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/cli"
	"github.com/luci/luci-go/common/logging/gologger"
)

var logCfg = gologger.LoggerConfig{
	Format: `%{message}`,
	Out:    os.Stderr,
}

var application = &cli.Application{
	Name:  "buildbucket",
	Title: "A cli client for buildbucket.",
	Context: func(ctx context.Context) context.Context {
		return logCfg.Use(ctx)
	},
	Commands: []*subcommands.Command{
		cmdPutBatch,
		cmdGet,
		cmdCancel,
		subcommands.CmdHelp,
	},
}

func main() {
	os.Exit(subcommands.Run(application, os.Args[1:]))
}
