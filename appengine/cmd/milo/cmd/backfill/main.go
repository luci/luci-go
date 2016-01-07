// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"os"

	"github.com/luci/luci-go/appengine/cmd/milo/cmd/backfill/buildbot"
	"github.com/maruel/subcommands"
)

var application = &subcommands.DefaultApplication{
	Name:  "backfill",
	Title: "Backfill Build and Revision data into the milo backend from various data sources.",
	Commands: []*subcommands.Command{
		buildbot.Cmd,
		subcommands.CmdHelp,
	},
}

func main() {
	os.Exit(subcommands.Run(application, nil))
}
