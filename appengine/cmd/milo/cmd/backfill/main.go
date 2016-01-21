// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"os"

	"github.com/luci/gae/impl/prod"
	"github.com/luci/luci-go/appengine/cmd/milo/collectors/buildbot"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/gologger"
	"github.com/maruel/subcommands"
	gol "github.com/op/go-logging"
	"golang.org/x/net/context"
	// Imported so you can upload this as a "module" to app engine, and access
	// the remote API.
	_ "google.golang.org/appengine/remote_api"
)

var application = &subcommands.DefaultApplication{
	Name:  "backfill",
	Title: "Backfill Build and Revision data into the milo backend from various data sources.",
	Commands: []*subcommands.Command{
		buildBotCmd,
		subcommands.CmdHelp,
	},
}

func main() {
	os.Exit(subcommands.Run(application, nil))
}

// commandRunBase is the base of all backfill subcommands. It defines the common
// flags dryRun and remoteURL
type commandRunBase struct {
	subcommands.CommandRunBase
	dryRun    bool
	remoteURL string
}

func (c *commandRunBase) Init() {
	c.Flags.StringVar(&c.remoteURL, "remote-url", "luci-milo.appspot.com", "the URL of the server to connect to via the remote API. Do NOT include the protocol (\"https://\")")
	c.Flags.BoolVar(&c.dryRun, "dryrun", true, "if this run is a dryrun, and should make no modifications to the datastore.")
}

////////////////////////////////////////////////////////////////////////////////
// Buildbot

var buildBotCmd = &subcommands.Command{
	UsageLine: "buildbot",
	ShortDesc: "runs the buildbot backfiller",
	LongDesc:  "Runs the buildbot backfiller. This hits chrome build extract, and uploads the data from that into the Cloud datastore, as backed by the remoteURL.",
	CommandRun: func() subcommands.CommandRun {
		c := &cmdBuildBotRun{}
		c.Init()
		c.Flags.StringVar(&c.master, "master", "chromium.win", "the master to upload data for")
		c.Flags.BoolVar(&c.buildbotFallback, "buildbot-fallback", false, "if getting the data from CBE fails, get the json data directly from the buildbot master")
		return c
	},
}

type cmdBuildBotRun struct {
	commandRunBase
	master           string
	buildbotFallback bool
}

func (c *cmdBuildBotRun) Run(a subcommands.Application, args []string) int {
	cfg := gologger.LoggerConfig{
		Format: "%{message}",
		Level:  gol.INFO,
		Out:    os.Stdout,
	}
	ctx := cfg.Use(context.Background())

	err := prod.UseRemote(&ctx, c.remoteURL, nil)
	if err != nil {
		log.Errorf(ctx, "%s", err)
		return 1
	}

	err = buildbot.PopulateMaster(ctx, c.master, c.dryRun, c.buildbotFallback)
	if err != nil {
		log.Errorf(ctx, "%s", err)
		return 1
	}

	return 0
}
