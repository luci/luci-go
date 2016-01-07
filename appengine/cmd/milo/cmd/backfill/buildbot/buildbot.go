// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package buildbot

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

// Cmd is the command which runs the buildbot backfiller.
var Cmd = &subcommands.Command{
	UsageLine: "buildbot",
	ShortDesc: "runs the buildbot backfiller",
	LongDesc:  "Runs the buildbot backfiller. This hits chrome build extract, and uploads the data from that into the Cloud datastore, as backed by the remoteURL.",
	CommandRun: func() subcommands.CommandRun {
		c := &cmdRun{}
		c.Flags.StringVar(&c.remoteURL, "remoteURL", "luci-milo.appspot.com", "the URL of the server to connect to via the remote API. Do NOT include the protocol (\"https://\")")
		c.Flags.StringVar(&c.master, "master", "chromium.win", "the master to upload data for")
		return c
	},
}

type cmdRun struct {
	subcommands.CommandRunBase
	remoteURL string
	master    string
}

func (c *cmdRun) Run(a subcommands.Application, args []string) int {
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

	err = buildbot.PopulateMaster(ctx, c.master)
	if err != nil {
		log.Errorf(ctx, "%s", err)
		return 1
	}

	return 0
}
